package web

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/nassiharel/clim/internal/manifest"
	"github.com/nassiharel/clim/internal/paths"
	"github.com/nassiharel/clim/internal/registry"
	"github.com/nassiharel/clim/internal/share"
)

// backupView powers the read-only Backup tab. It bundles together
// every piece of state the TUI's Backup tab exposes today: a manifest
// preview of the current toolchain, a share token, the file path the
// manifest would be written to on disk, and the list of saved
// backups under ~/.config/clim/backups/.
type backupView struct {
	Tools      []manifest.Tool
	Count      int
	YAML       string
	ShareToken string
	ShareErr   string
	Backups    []savedBackup
	BackupsErr string
}

// savedBackup describes one *.yaml file under ~/.config/clim/backups/.
// The web UI lets the user download any of them as a fresh manifest.
type savedBackup struct {
	Name        string // file name without path
	Size        int64
	ModifiedAt  time.Time
	DownloadURL string
}

// pageBackup renders the Backup tab. We compute the manifest + share
// token on every request because the user can install / upgrade /
// remove between page loads, and showing stale data would be
// confusing. The cost is one PATH scan; cached responses would defeat
// the point of looking at backups.
func (s *Server) pageBackup(w http.ResponseWriter, r *http.Request) {
	tools, _, err := s.loader.LoadInstalled(r.Context())
	if err != nil {
		s.serveError(w, r, err, http.StatusInternalServerError)
		return
	}
	view := buildBackupView(tools)
	if backups, berr := loadSavedBackups(); berr != nil {
		view.BackupsErr = berr.Error()
	} else {
		view.Backups = backups
	}
	s.renderPage(w, r, "backup.html", pageData{
		Title:     "Backup",
		ActiveTab: "backup",
		Data:      view,
	})
}

// loadSavedBackups lists *.yaml files under ~/.config/clim/backups/.
// Missing dir is fine — that's the empty case. We sort newest-first so
// the most recent backup is at the top.
func loadSavedBackups() ([]savedBackup, error) {
	dir, err := paths.BackupsDir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	out := make([]savedBackup, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if !strings.HasSuffix(strings.ToLower(e.Name()), ".yaml") {
			continue
		}
		info, ierr := e.Info()
		if ierr != nil {
			continue
		}
		out = append(out, savedBackup{
			Name:        e.Name(),
			Size:        info.Size(),
			ModifiedAt:  info.ModTime(),
			DownloadURL: "/backup/saved/" + url.PathEscape(e.Name()),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ModifiedAt.After(out[j].ModifiedAt) })
	return out, nil
}

// downloadSavedBackup serves the raw file under ~/.config/clim/backups/
// as a YAML attachment. Path-escapes are validated to prevent
// directory traversal — only filenames matching the listing logic are
// allowed through.
func (s *Server) downloadSavedBackup(w http.ResponseWriter, r *http.Request) {
	rawName := r.PathValue("name")
	name, err := url.PathUnescape(rawName)
	if err != nil {
		s.serveError(w, r, fmt.Errorf("invalid name: %w", err), http.StatusBadRequest)
		return
	}
	// Reject anything that looks like a path component or a hidden
	// file. Even though paths.BackupsDir() is a fixed location, a
	// careless join could escape it via "..".
	if name == "" || strings.ContainsAny(name, "/\\") || strings.HasPrefix(name, ".") {
		s.serveError(w, r, fmt.Errorf("invalid backup name"), http.StatusBadRequest)
		return
	}
	if !strings.HasSuffix(strings.ToLower(name), ".yaml") {
		s.serveError(w, r, fmt.Errorf("only .yaml backups are downloadable"), http.StatusBadRequest)
		return
	}
	dir, err := paths.BackupsDir()
	if err != nil {
		s.serveError(w, r, err, http.StatusInternalServerError)
		return
	}
	full := filepath.Join(dir, name)
	body, err := os.ReadFile(full)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			s.serveError(w, r, fmt.Errorf("backup %q not found", name), http.StatusNotFound)
			return
		}
		s.serveError(w, r, err, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename=%q`, name))
	_, _ = w.Write(body)
}

// downloadExport returns the manifest as a YAML attachment so the user
// can `clim import` it elsewhere. It mirrors `clim export` output —
// the same struct shape, the same field set, and a trailing newline.
func (s *Server) downloadExport(w http.ResponseWriter, r *http.Request) {
	tools, _, err := s.loader.LoadInstalled(r.Context())
	if err != nil {
		s.serveError(w, r, err, http.StatusInternalServerError)
		return
	}
	manifestTools := buildManifestTools(tools)
	body, err := yaml.Marshal(map[string]any{"tools": manifestTools})
	if err != nil {
		s.serveError(w, r, err, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="clim-export.yaml"`)
	_, _ = w.Write(body)
}

// pageConfig renders the read-only Config tab — the running clim
// config as YAML. The TUI has a full editor today; we'll add an
// editor in a follow-up. For now even read-only access is useful
// because users can copy the YAML into ~/.config/clim/config.yaml as
// a starting point.
func (s *Server) pageConfig(w http.ResponseWriter, r *http.Request) {
	view := buildConfigView(s)
	s.renderPage(w, r, "config.html", pageData{
		Title:     "Config",
		ActiveTab: "config",
		Data:      view,
	})
}

type configView struct {
	YAML  string
	Error string
}

func buildConfigView(s *Server) configView {
	if s.opts.Config == nil {
		return configView{Error: "no config loaded"}
	}
	body, err := yaml.Marshal(s.opts.Config)
	if err != nil {
		return configView{Error: err.Error()}
	}
	return configView{YAML: string(body)}
}

// buildManifestTools maps a registry slice through manifest's
// FromRegistryTool, dropping non-installed entries. Same rule
// `clim export` uses — backups are about the user's actual current
// toolchain, not the catalog.
func buildManifestTools(tools []registry.Tool) []manifest.Tool {
	out := make([]manifest.Tool, 0, len(tools))
	for _, t := range tools {
		if !t.IsInstalled() {
			continue
		}
		out = append(out, manifest.FromRegistryTool(t))
	}
	return out
}

// buildBackupView computes everything pageBackup renders so the logic
// is testable without firing up an HTTP server.
func buildBackupView(tools []registry.Tool) backupView {
	manifestTools := buildManifestTools(tools)
	view := backupView{
		Tools: manifestTools,
		Count: len(manifestTools),
	}
	if len(manifestTools) > 0 {
		body, err := yaml.Marshal(map[string]any{"tools": manifestTools})
		if err == nil {
			view.YAML = string(body)
		}
	}
	names := make([]string, 0, len(manifestTools))
	for _, t := range manifestTools {
		names = append(names, t.Name)
	}
	if token, err := share.Encode(names); err == nil {
		view.ShareToken = token
	} else if !errors.Is(err, share.ErrEmptyToolList) {
		view.ShareErr = fmt.Sprintf("couldn't generate share token: %v", err)
	}
	return view
}
