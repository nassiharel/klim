package tui

import (
	"context"
	"time"

	"github.com/nassiharel/clim/internal/config"
	"github.com/nassiharel/clim/internal/detector"
	"github.com/nassiharel/clim/internal/registry"
	"github.com/nassiharel/clim/internal/scanner"
)

// scanResultMsg is sent when PATH scanning and version detection complete.
type scanResultMsg struct {
	tools []registry.Tool
	err   error
}

// scanPATH wraps scanner.ScanPATH for use within the tui package.
func scanPATH(cfg config.Config) ([]registry.Tool, error) {
	return scanner.ScanPATH(cfg)
}

// detectAll wraps detector.DetectAll for use within the tui package.
func detectAll(ctx context.Context, tools []registry.Tool, timeout time.Duration, concurrency int) {
	detector.DetectAll(ctx, tools, timeout, concurrency)
}
