package web

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/nassiharel/clim/internal/config"
)

// pageConfig renders the Config tab. Each Setting from
// internal/config drives one form input — checkbox, text, number,
// duration, or select — so the TUI's config editor and the web
// editor stay in lockstep automatically.
func (s *Server) pageConfig(w http.ResponseWriter, r *http.Request) {
	view := buildConfigView(s, "")
	view.Saved = r.URL.Query().Get("saved") == "1"
	s.renderPage(w, r, "config.html", pageData{
		Title:     "Config",
		ActiveTab: "config",
		Data:      view,
	})
}

// pageConfigSave applies form values to the running config and
// writes it to disk. Errors keep the user on the page with the
// rejected values preserved so they can correct without re-typing
// the whole form.
//
// We stage changes onto a copy of the config first; if any field
// fails to parse, nothing is persisted and the running config is
// unchanged. On success, the running pointer is updated in place so
// every subsequent request sees the new values without a restart
// (most fields still need a restart to take effect — the running
// service caches several values — and we surface that in the UI).
//
// Reads and writes go through s.cfgMu so concurrent GET /config
// can't observe a half-updated struct.
func (s *Server) pageConfigSave(w http.ResponseWriter, r *http.Request) {
	if s.opts.Config == nil {
		s.serveError(w, r, fmt.Errorf("no config loaded"), http.StatusInternalServerError)
		return
	}
	if err := r.ParseForm(); err != nil {
		s.serveError(w, r, err, http.StatusBadRequest)
		return
	}
	staged := s.snapshotConfig()
	var fieldErrors []string
	for _, st := range config.AllSettings() {
		raw := r.FormValue(st.Key)
		if err := st.SetFromString(&staged, raw); err != nil {
			fieldErrors = append(fieldErrors, err.Error())
		}
	}
	if len(fieldErrors) > 0 {
		view := buildConfigView(s, strings.Join(fieldErrors, "\n"))
		view.Form = r.PostForm
		s.renderPage(w, r, "config.html", pageData{
			Title:     "Config",
			ActiveTab: "config",
			Data:      view,
		})
		return
	}
	// Save to disk FIRST, so a write failure doesn't leave the
	// running config diverged from what's on disk. Only after a
	// successful save do we commit the staged copy into the running
	// pointer; concurrent renders therefore always see a value that
	// matches the persisted state.
	stagedCopy := staged
	if err := config.Save(&stagedCopy); err != nil {
		view := buildConfigView(s, "save failed: "+err.Error())
		view.Form = r.PostForm
		s.renderPage(w, r, "config.html", pageData{
			Title:     "Config",
			ActiveTab: "config",
			Data:      view,
		})
		return
	}
	s.writeConfig(staged)
	// Post / Redirect / Get: refreshing won't re-submit.
	http.Redirect(w, r, "/config?saved=1", http.StatusSeeOther)
}

type configView struct {
	YAML     string
	Settings []configRow
	Error    string
	Saved    bool
	// Form holds the submitted-but-invalid values when re-rendering
	// after a validation error, so the user sees what to fix.
	Form url.Values
}

// configRow flattens one Setting + its current rendered/raw values
// so the template can stay simple.
type configRow struct {
	Setting config.Setting
	Display string
	Raw     string
	Form    string // form-submitted value (if re-rendering after error)
}

func buildConfigView(s *Server, errMsg string) configView {
	if s.opts.Config == nil {
		return configView{Error: "no config loaded"}
	}
	// Snapshot under the read lock so we don't race with a concurrent
	// pageConfigSave; render against the local copy.
	cfg := s.snapshotConfig()
	body, err := yaml.Marshal(&cfg)
	view := configView{Error: errMsg}
	if err != nil && errMsg == "" {
		view.Error = err.Error()
	} else {
		view.YAML = string(body)
	}
	for _, st := range config.AllSettings() {
		view.Settings = append(view.Settings, configRow{
			Setting: st,
			Display: st.Display(&cfg),
			Raw:     st.Raw(&cfg),
		})
	}
	return view
}
