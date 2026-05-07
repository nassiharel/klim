package web

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/nassiharel/klim/internal/custompacks"
	"github.com/nassiharel/klim/internal/manifest"
	"github.com/nassiharel/klim/internal/paths"
	"github.com/nassiharel/klim/internal/registry"
	"github.com/nassiharel/klim/internal/share"
)

// validBackupName matches the safe filename characters we accept for
// user-supplied backup names. Restricts to alphanumerics, underscore,
// dash, and dot, with a leading-letter requirement so paths can't
// start with "." (hidden files) or special characters. Length cap
// keeps directory listings sane.
var validBackupName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

// validPackName follows the same rules as marketplace tool names —
// lowercase alphanumerics + dash. We're stricter than marketplace
// validate (which uses Unicode letters) because user input goes
// straight to a YAML file path and we want it grep-friendly.
var validPackName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)

// pageBackupSave persists the current toolchain manifest to a named
// file under ~/.klim/backups/<name>.yaml. The flash banner on
// the next render confirms.
// maxFormBytes caps the request body size for form/multipart parsing
// in this package. 8 MiB comfortably fits the largest realistic
// manifest/import payload (toolchain YAML for hundreds of tools is
// well under 1 MiB) while preventing memory exhaustion from a
// malicious oversize submission.
const maxFormBytes = 8 << 20

func (s *Server) pageBackupSave(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxFormBytes)
	name := strings.TrimSpace(r.FormValue("name"))
	if !validBackupName.MatchString(name) {
		s.redirectBackupFlash(w, r, "err", fmt.Sprintf("invalid backup name %q (use letters, digits, underscores, dashes — first character must be alphanumeric)", name))
		return
	}
	tools, _, err := s.loader.LoadInstalled(r.Context())
	if err != nil {
		s.serveError(w, r, err, http.StatusInternalServerError)
		return
	}
	manifestTools := buildManifestTools(tools)
	if len(manifestTools) == 0 {
		s.redirectBackupFlash(w, r, "err", "no installed tools to back up")
		return
	}
	body, err := yaml.Marshal(map[string]any{"tools": manifestTools})
	if err != nil {
		s.serveError(w, r, err, http.StatusInternalServerError)
		return
	}
	dir, err := paths.BackupsDir()
	if err != nil {
		s.serveError(w, r, err, http.StatusInternalServerError)
		return
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		s.serveError(w, r, err, http.StatusInternalServerError)
		return
	}
	full := filepath.Join(dir, name+".yaml")
	if err := os.WriteFile(full, body, 0o644); err != nil { //nolint:gosec
		s.serveError(w, r, err, http.StatusInternalServerError)
		return
	}
	s.redirectBackupFlash(w, r, "ok", fmt.Sprintf("Saved %s.yaml (%d tools)", name, len(manifestTools)))
}

// pageBackupSavedDelete removes one *.yaml file under
// ~/.klim/backups/. Path-escapes are validated against
// validBackupName to prevent directory traversal.
func (s *Server) pageBackupSavedDelete(w http.ResponseWriter, r *http.Request) {
	rawName := r.PathValue("name")
	name, err := url.PathUnescape(rawName)
	if err != nil {
		s.serveError(w, r, fmt.Errorf("invalid name: %w", err), http.StatusBadRequest)
		return
	}
	stem := strings.TrimSuffix(name, ".yaml")
	if !validBackupName.MatchString(stem) {
		s.serveError(w, r, errors.New("invalid backup name"), http.StatusBadRequest)
		return
	}
	dir, err := paths.BackupsDir()
	if err != nil {
		s.serveError(w, r, err, http.StatusInternalServerError)
		return
	}
	if err := os.Remove(filepath.Join(dir, stem+".yaml")); err != nil { //nolint:gosec // G703: stem is validated by validBackupName above.
		if errors.Is(err, os.ErrNotExist) {
			s.redirectBackupFlash(w, r, "err", fmt.Sprintf("backup %q not found", stem))
			return
		}
		s.serveError(w, r, err, http.StatusInternalServerError)
		return
	}
	s.redirectBackupFlash(w, r, "ok", fmt.Sprintf("Deleted %s.yaml", stem))
}

// pageBackupPreview parses input from one of three sources — a YAML
// manifest pasted into a textarea, a share token, or the name of a
// saved backup file — and renders a preview page listing the tools
// the user is about to install. The preview page reuses the existing
// per-tool /tools/{name}/install endpoint, so the actual install runs
// through the same job pipeline as everything else.
func (s *Server) pageBackupPreview(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxFormBytes)
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		// Fallback for non-multipart submissions (the form's enctype
		// only matters when the user attaches a file).
		if err := r.ParseForm(); err != nil {
			s.serveError(w, r, err, http.StatusBadRequest)
			return
		}
	}

	yamlInput := r.FormValue("manifest")
	tokenInput := strings.TrimSpace(r.FormValue("token"))
	restoreName := strings.TrimSpace(r.FormValue("restore"))

	// File upload takes precedence over the textarea so users can
	// drag-drop a backup without copy/pasting it.
	if file, _, err := r.FormFile("manifest_file"); err == nil {
		body, ferr := io.ReadAll(io.LimitReader(file, 1<<20))
		_ = file.Close()
		if ferr != nil {
			s.serveError(w, r, ferr, http.StatusBadRequest)
			return
		}
		if len(body) > 0 {
			yamlInput = string(body)
		}
	}

	var (
		toolNames []string
		source    string
		err       error
	)
	switch {
	case restoreName != "":
		stem := strings.TrimSuffix(restoreName, ".yaml")
		if !validBackupName.MatchString(stem) {
			s.serveError(w, r, errors.New("invalid backup name"), http.StatusBadRequest)
			return
		}
		dir, derr := paths.BackupsDir()
		if derr != nil {
			s.serveError(w, r, derr, http.StatusInternalServerError)
			return
		}
		body, rerr := os.ReadFile(filepath.Join(dir, stem+".yaml")) //nolint:gosec // G703: stem is validated by validBackupName above.
		if rerr != nil {
			s.redirectBackupFlash(w, r, "err", fmt.Sprintf("couldn't read backup %s.yaml: %v", stem, rerr))
			return
		}
		toolNames, err = manifestToolNames(body)
		source = "Saved backup: " + stem + ".yaml"
	case strings.HasPrefix(strings.TrimSpace(tokenInput), "klim:"):
		toolNames, err = share.Decode(tokenInput)
		source = "Share token"
	case strings.TrimSpace(yamlInput) != "":
		toolNames, err = manifestToolNames([]byte(yamlInput))
		source = "Manifest YAML"
	default:
		s.redirectBackupFlash(w, r, "err", "paste a manifest, a share token, or click Restore on a saved backup")
		return
	}
	if err != nil {
		s.redirectBackupFlash(w, r, "err", fmt.Sprintf("couldn't parse input: %v", err))
		return
	}
	if len(toolNames) == 0 {
		s.redirectBackupFlash(w, r, "err", "no tools found in the input")
		return
	}

	tools, _, lerr := s.loader.LoadInstalled(r.Context())
	if lerr != nil {
		s.serveError(w, r, lerr, http.StatusInternalServerError)
		return
	}
	rows := buildPreviewRows(toolNames, tools)
	view := previewView{
		Source: source,
		Rows:   rows,
		Total:  len(rows),
	}
	for _, row := range rows {
		switch row.Status {
		case "missing-catalog":
			view.NotInCatalog++
		case "installed":
			view.AlreadyInstalled++
		case "ready":
			view.ReadyToInstall++
		}
	}
	s.renderPage(w, r, "backup_preview.html", pageData{
		Title:     "Preview install",
		ActiveTab: "backup",
		Data:      view,
	})
}

// pageBackupPackCreate creates a custom pack from the form values.
// Validates the slug, normalises tool names (one per line OR comma-
// separated), and persists via custompacks.Add. Redirects to the
// pack's detail page on success so the user can review what they
// just created.
func (s *Server) pageBackupPackCreate(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxFormBytes)
	if err := r.ParseForm(); err != nil {
		s.serveError(w, r, err, http.StatusBadRequest)
		return
	}
	name := strings.ToLower(strings.TrimSpace(r.FormValue("name")))
	if !validPackName.MatchString(name) {
		s.redirectBackupFlash(w, r, "err", fmt.Sprintf("invalid pack name %q (use lowercase letters, digits, dashes; must start with a letter or digit)", name))
		return
	}
	display := strings.TrimSpace(r.FormValue("display_name"))
	desc := strings.TrimSpace(r.FormValue("description"))
	tools := normalisePackTools(r.FormValue("tools"))
	if len(tools) == 0 {
		s.redirectBackupFlash(w, r, "err", "pack must include at least one tool")
		return
	}
	if err := custompacks.Add(registry.Pack{
		Name:        name,
		DisplayName: display,
		Description: desc,
		ToolNames:   tools,
	}); err != nil {
		s.serveError(w, r, err, http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/packs/"+url.PathEscape(name), http.StatusSeeOther)
}

// pageBackupPackDelete removes a custom pack. Marketplace packs are
// untouched (custompacks.Delete only operates on the user's
// custom-packs.yaml).
func (s *Server) pageBackupPackDelete(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !validPackName.MatchString(name) {
		s.serveError(w, r, errors.New("invalid pack name"), http.StatusBadRequest)
		return
	}
	if err := custompacks.Delete(name); err != nil {
		s.serveError(w, r, err, http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/packs", http.StatusSeeOther)
}

// --- preview view ---

type previewView struct {
	Source           string
	Rows             []previewRow
	Total            int
	NotInCatalog     int
	AlreadyInstalled int
	ReadyToInstall   int
}

type previewRow struct {
	Name      string
	Tool      *registry.Tool // nil when "missing-catalog"
	Status    string         // "installed" | "ready" | "missing-catalog"
	Installed bool
}

func buildPreviewRows(names []string, tools []registry.Tool) []previewRow {
	byName := registry.ToolMap(tools)
	out := make([]previewRow, 0, len(names))
	seen := make(map[string]struct{}, len(names))
	for _, n := range names {
		if _, dup := seen[n]; dup {
			continue
		}
		seen[n] = struct{}{}
		row := previewRow{Name: n}
		if t, ok := byName[n]; ok {
			row.Tool = t
			row.Installed = t.IsInstalled()
			if row.Installed {
				row.Status = "installed"
			} else {
				row.Status = "ready"
			}
		} else {
			row.Status = "missing-catalog"
		}
		out = append(out, row)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// --- helpers ---

// manifestToolNames parses a YAML manifest body and returns the list
// of tool names. Accepts both the export format (`tools: [...]`) and
// the older flat list format for compatibility.
func manifestToolNames(body []byte) ([]string, error) {
	var wrapped struct {
		Tools []manifest.Tool `yaml:"tools"`
	}
	if err := yaml.Unmarshal(body, &wrapped); err == nil && len(wrapped.Tools) > 0 {
		out := make([]string, 0, len(wrapped.Tools))
		for _, t := range wrapped.Tools {
			if t.Name != "" {
				out = append(out, t.Name)
			}
		}
		return out, nil
	}
	// Fallback: a top-level list of strings. The TUI's older exports
	// used this shape, so we keep accepting it for restore flows.
	var flat []string
	if err := yaml.Unmarshal(body, &flat); err == nil && len(flat) > 0 {
		return flat, nil
	}
	return nil, errors.New("manifest contains no tools (expected `tools: [...]` or a YAML list)")
}

// normalisePackTools accepts the textarea content from the create-
// pack form and produces a clean, deduped slice. Splits on commas
// and newlines so users can paste either format.
func normalisePackTools(raw string) []string {
	rawSplit := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == '\n' || r == '\r'
	})
	seen := make(map[string]struct{}, len(rawSplit))
	out := make([]string, 0, len(rawSplit))
	for _, s := range rawSplit {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if _, dup := seen[s]; dup {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

// redirectBackupFlash bounces the user back to /backup with a flash
// message in the URL. Cookies would be cleaner but they'd require
// CSRF wrapping and the flash messages are non-sensitive UI strings,
// so query params are fine.
func (s *Server) redirectBackupFlash(w http.ResponseWriter, r *http.Request, level, msg string) {
	q := url.Values{}
	q.Set("flash", level)
	q.Set("msg", msg)
	http.Redirect(w, r, "/backup?"+q.Encode(), http.StatusSeeOther)
}
