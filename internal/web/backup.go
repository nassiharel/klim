package web

import (
	"errors"
	"fmt"
	"net/http"

	"gopkg.in/yaml.v3"

	"github.com/nassiharel/clim/internal/manifest"
	"github.com/nassiharel/clim/internal/registry"
	"github.com/nassiharel/clim/internal/share"
)

// backupView powers the read-only Backup tab. It bundles together
// every piece of state the TUI's Backup tab exposes today: a manifest
// preview of the current toolchain, a share token, and the file path
// the manifest would be written to on disk.
type backupView struct {
	Tools      []manifest.Tool
	Count      int
	YAML       string
	ShareToken string
	ShareErr   string
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
	s.renderPage(w, r, "backup.html", pageData{
		Title:     "Backup",
		ActiveTab: "backup",
		Data:      view,
	})
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
