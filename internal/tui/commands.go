package tui

import (
	"github.com/nassiharel/clim/internal/detector"
	"github.com/nassiharel/clim/internal/finder"
	"github.com/nassiharel/clim/internal/pkgmgr"
	"github.com/nassiharel/clim/internal/registry"
)

// scanResultMsg is sent when tool finding completes (Phase 1).
type scanResultMsg struct {
	tools []registry.Tool
}

// toolVersionMsg is sent when a single tool's version is resolved.
type toolVersionMsg struct {
	index int
	tool  registry.Tool
}

// findToolsCmd finds all curated tools on PATH.
func findToolsCmd() func() scanResultMsg {
	return func() scanResultMsg {
		tools := registry.DefaultTools()
		_ = finder.FindAll(tools) // Best-effort; empty PATH returns no instances.
		return scanResultMsg{tools: tools}
	}
}

// resolveToolVersionCmd resolves version for a single tool (package manager + fallback).
func resolveToolVersionCmd(index int, tool registry.Tool) func() toolVersionMsg {
	return func() toolVersionMsg {
		if tool.IsInstalled() && !tool.Disabled {
			pkgmgr.ResolveOne(&tool)
			detector.EnrichOne(&tool)
		}
		return toolVersionMsg{index: index, tool: tool}
	}
}
