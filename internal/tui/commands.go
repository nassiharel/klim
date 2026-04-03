package tui

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"github.com/nassiharel/clim/internal/detector"
	"github.com/nassiharel/clim/internal/registry"
	"github.com/nassiharel/clim/internal/version"
)

// DetectionCompleteMsg is sent when a tool's local detection finishes.
type DetectionCompleteMsg struct {
	Index  int
	Result detector.DetectionResult
}

// LatestVersionMsg is sent when a tool's latest version check finishes.
type LatestVersionMsg struct {
	Index  int
	Result version.LatestVersion
}

// detectToolCmd returns a Cmd that runs detection for one tool.
func detectToolCmd(ctx context.Context, index int, tool registry.Tool) tea.Cmd {
	return func() tea.Msg {
		result := detector.DetectOne(ctx, tool)
		return DetectionCompleteMsg{Index: index, Result: result}
	}
}

// checkLatestCmd returns a Cmd that checks the latest version for one tool.
func checkLatestCmd(ctx context.Context, index int, tool registry.Tool, checker version.Checker, cache *version.Cache) tea.Cmd {
	return func() tea.Msg {
		// Check cache first.
		key := cacheKeyForTool(tool)
		if v, ok := cache.Get(key); ok {
			return LatestVersionMsg{Index: index, Result: version.LatestVersion{Version: v}}
		}

		result := checker.Latest(ctx, tool.LatestSource)
		if result.Error == nil && result.Version != "" {
			cache.Set(key, result.Version)
		}
		return LatestVersionMsg{Index: index, Result: result}
	}
}

// cacheKeyForTool generates a cache key for a tool.
func cacheKeyForTool(tool registry.Tool) string {
	src := tool.LatestSource
	switch src.Type {
	case registry.SourceGitHub:
		return "github:" + src.Repo
	case registry.SourcePyPI:
		return "pypi:" + src.Package
	case registry.SourceNPM:
		return "npm:" + src.Package
	case registry.SourceCustom:
		return "custom:" + src.URLPattern
	default:
		return "unknown:" + tool.Name
	}
}
