package tui

import (
	"runtime"

	"github.com/nassiharel/clim/internal/detector"
	"github.com/nassiharel/clim/internal/finder"
	"github.com/nassiharel/clim/internal/pkgmgr"
	"github.com/nassiharel/clim/internal/registry"
)

// scanResultMsg is sent when tool finding completes (Phase 1).
type scanResultMsg struct {
	tools []registry.Tool
}

// versionResultMsg is sent when version resolution completes (Phase 2).
type versionResultMsg struct{}

// findToolsCmd finds all curated tools on PATH.
func findToolsCmd() func() scanResultMsg {
	return func() scanResultMsg {
		tools := registry.DefaultTools()
		finder.FindAll(tools)
		return scanResultMsg{tools: tools}
	}
}

// resolveVersionsCmd resolves versions from package managers + fallback.
func resolveVersionsCmd(tools []registry.Tool) func() versionResultMsg {
	return func() versionResultMsg {
		pkgmgr.ResolveVersions(tools, runtime.NumCPU())
		detector.EnrichFallback(tools)
		return versionResultMsg{}
	}
}
