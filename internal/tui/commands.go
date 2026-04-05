package tui

import (
	"github.com/nassiharel/clim/internal/config"
	"github.com/nassiharel/clim/internal/registry"
	"github.com/nassiharel/clim/internal/scanner"
)

// scanResultMsg is sent when PATH scanning completes.
type scanResultMsg struct {
	tools []registry.Tool
	err   error
}

// scanPATHCmd returns a Bubbletea command that scans PATH for executables.
func scanPATHCmd(cfg config.Config) func() scanResultMsg {
	return func() scanResultMsg {
		tools, err := scanner.ScanPATH(cfg)
		return scanResultMsg{tools: tools, err: err}
	}
}
