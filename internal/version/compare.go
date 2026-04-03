package version

import (
	"fmt"
	"strings"

	"github.com/blang/semver/v4"
)

// Status represents the upgrade status of a tool.
type Status int

const (
	StatusLoading      Status = iota
	StatusUpToDate            // Installed version >= latest
	StatusUpgradable          // Latest version > installed
	StatusNotInstalled        // Tool not found
	StatusError               // Could not determine status
)

// StatusString returns a human-readable status label.
func StatusString(s Status) string {
	switch s {
	case StatusUpToDate:
		return "✓ up to date"
	case StatusUpgradable:
		return "⬆ upgrade available"
	case StatusNotInstalled:
		return "✗ not found"
	case StatusLoading:
		return "⏳ loading"
	case StatusError:
		return "? error"
	default:
		return "?"
	}
}

// StatusIcon returns a short status indicator.
func StatusIcon(s Status) string {
	switch s {
	case StatusUpToDate:
		return "✓"
	case StatusUpgradable:
		return "⬆"
	case StatusNotInstalled:
		return "✗"
	case StatusLoading:
		return "…"
	case StatusError:
		return "?"
	default:
		return "?"
	}
}

// CompareVersions determines the status by comparing installed vs latest.
func CompareVersions(installed, latest string) (Status, error) {
	if installed == "" {
		return StatusNotInstalled, nil
	}
	if latest == "" {
		return StatusError, fmt.Errorf("no latest version available")
	}

	// Normalize: strip leading 'v', handle missing patch version.
	installed = strings.TrimPrefix(installed, "v")
	latest = strings.TrimPrefix(latest, "v")

	iv, err := semver.ParseTolerant(installed)
	if err != nil {
		return StatusError, fmt.Errorf("parse installed %q: %w", installed, err)
	}

	lv, err := semver.ParseTolerant(latest)
	if err != nil {
		return StatusError, fmt.Errorf("parse latest %q: %w", latest, err)
	}

	if iv.GTE(lv) {
		return StatusUpToDate, nil
	}
	return StatusUpgradable, nil
}
