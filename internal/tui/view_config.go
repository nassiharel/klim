package tui

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/nassiharel/clim/internal/build"
	"github.com/nassiharel/clim/internal/config"
	"github.com/nassiharel/clim/internal/logging"
	"github.com/nassiharel/clim/internal/registry"
	"github.com/nassiharel/clim/internal/scancache"
)

func (m Model) renderConfigView() string {
	var b strings.Builder
	label := detailLabelStyle.Render
	dim := dimVersion.Render

	// Version info.
	b.WriteString("\n")
	fmt.Fprintf(&b, "  %s  %s\n", label(fixedWidth("Version", 18)), build.Info())
	fmt.Fprintf(&b, "  %s  %s / %s\n", label(fixedWidth("OS / Arch", 18)), runtime.GOOS, runtime.GOARCH)
	fmt.Fprintf(&b, "  %s  %s\n", label(fixedWidth("Go", 18)), runtime.Version())

	// File paths.
	b.WriteString("\n")
	configPath := dim("(unknown)")
	if p, err := config.Path(); err == nil {
		configPath = p
	}
	fmt.Fprintf(&b, "  %s  %s\n", label(fixedWidth("Config", 18)), configPath)

	logPath := logging.Path()
	if logPath == "" {
		logPath = dim("(unavailable)")
	}
	fmt.Fprintf(&b, "  %s  %s\n", label(fixedWidth("Log", 18)), logPath)

	// Last scan time.
	scanTime := dim("(never)")
	if p, err := scancache.Path(); err == nil {
		if info, err := os.Stat(p); err == nil {
			ago := time.Since(info.ModTime())
			scanTime = info.ModTime().Format("2006-01-02 15:04:05")
			switch {
			case ago < time.Minute:
				scanTime += dim("  (just now)")
			case ago < time.Hour:
				scanTime += dim(fmt.Sprintf("  (%d min ago)", int(ago.Minutes())))
			case ago < 24*time.Hour:
				scanTime += dim(fmt.Sprintf("  (%d hours ago)", int(ago.Hours())))
			default:
				scanTime += dim(fmt.Sprintf("  (%d days ago)", int(ago.Hours()/24)))
			}
		}
	}
	fmt.Fprintf(&b, "  %s  %s\n", label(fixedWidth("Last Scan", 18)), scanTime)

	// Package managers.
	b.WriteString("\n")
	b.WriteString("  " + label("Package Managers") + "\n")
	for _, pm := range registry.AllPMStatusForOS() {
		icon := upgradableStyle.Render("✗")
		status := dim("not found")
		if pm.Available {
			icon = upToDateStyle.Render("✓")
			status = upToDateStyle.Render("installed")
		}
		fmt.Fprintf(&b, "    %s  %-10s %s\n", icon, string(pm.Source), status)
	}

	// Config warnings.
	if len(m.configWarnings) > 0 {
		b.WriteString("\n  " + upgradableStyle.Render("⚠ Config Warnings") + "\n\n")
		for _, w := range m.configWarnings {
			b.WriteString("  " + upgradableStyle.Render("•") + "  " + dimVersion.Render(w) + "\n")
		}
	}

	// Editable settings.
	b.WriteString(m.renderConfigEditor())

	return b.String()
}

// --- Help ---

// renderHelp moved to view_help.go.

// --- Two-column layout builders ---

// buildSidebarLines renders the filter sidebar as a slice of fixed-width strings.
