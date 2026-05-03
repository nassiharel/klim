package web

import (
	"embed"
	"fmt"
	"html/template"
	"io/fs"
)

// staticFiles holds CSS / JS / favicon served under /static/.
//
//go:embed static
var staticFiles embed.FS

// templateFiles holds the HTML page templates rendered server-side.
//
//go:embed templates
var templateFiles embed.FS

// pageTemplates is the set of standalone page names. Each entry is
// parsed into its own template tree so per-page {{define "content"}}
// blocks don't collide in a single namespace (Go's html/template puts
// every parsed file into one map, so the last-parsed wins otherwise).
var pageTemplates = []string{
	"installed.html",
	"tool.html",
	"updates.html",
	"discover.html",
	"favorites.html",
	"dashboard.html",
	"trail.html",
	"snapshot.html",
	"job.html",
	"backup.html",
	"config.html",
	"stub.html",
}

// loadTemplates returns one fully-parsed template per page. Each tree
// has the layout plus exactly one content-providing page so executing
// "layout" picks up the correct {{define "content"}}.
func loadTemplates() (map[string]*template.Template, error) {
	sub, err := fs.Sub(templateFiles, "templates")
	if err != nil {
		return nil, err
	}
	base, err := template.New("clim-browser").Funcs(templateFuncs()).ParseFS(sub, "layout.html")
	if err != nil {
		return nil, fmt.Errorf("parsing layout.html: %w", err)
	}
	out := make(map[string]*template.Template, len(pageTemplates))
	for _, name := range pageTemplates {
		clone, err := base.Clone()
		if err != nil {
			return nil, fmt.Errorf("cloning base for %s: %w", name, err)
		}
		if _, err := clone.ParseFS(sub, name); err != nil {
			return nil, fmt.Errorf("parsing %s: %w", name, err)
		}
		out[name] = clone
	}
	return out, nil
}
