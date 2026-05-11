package plan

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/nassiharel/klim/internal/registry"
)

// RenderText pretty-prints the plan in the Terraform-plan-style
// layout the user wants to see. Sections, in order:
//
//	Planned changes (grouped by PM)
//	Confidence (per change + factors)
//	Risk analysis
//	Disk impact
//	Estimated time
//
// Empty sections are skipped — a clean plan prints "No changes
// pending." and nothing else.
func RenderText(p Plan) string {
	var b strings.Builder

	if len(p.Changes) == 0 {
		b.WriteString("No changes pending. Your toolchain is up to date.\n")
		return b.String()
	}

	b.WriteString("Planned changes:\n\n")
	grouped := groupBySource(p.Changes)
	for _, src := range groupedKeys(grouped) {
		b.WriteString("  " + string(src) + ":\n")
		for _, c := range grouped[src] {
			b.WriteString("    " + formatChangeLine(c) + "\n")
		}
		b.WriteString("\n")
	}

	if hasConfidence(p.Changes) {
		b.WriteString("Upgrade confidence:\n")
		for _, c := range p.Changes {
			if c.Kind != ChangeUpgrade {
				continue
			}
			fmt.Fprintf(&b, "  %s upgrade confidence: %d%%\n", c.DisplayName, c.Confidence)
			for _, f := range c.ConfidenceFactors {
				if f.Delta == 0 {
					continue
				}
				sign := "-"
				delta := f.Delta
				if delta > 0 {
					sign = "+"
				} else {
					delta = -delta
				}
				fmt.Fprintf(&b, "    %s%d  %s\n", sign, delta, f.Reason)
			}
		}
		b.WriteString("\n")
	}

	if len(p.Risks) > 0 {
		b.WriteString("Risk analysis:\n")
		for _, r := range p.Risks {
			icon := "ℹ"
			switch r.Severity {
			case SeverityWarning:
				icon = "⚠"
			case SeverityError:
				icon = "✗"
			}
			label := r.Message
			if r.Tool != "" {
				label = r.Tool + ": " + r.Message
			}
			b.WriteString("  " + icon + " " + label + "\n")
		}
		b.WriteString("\n")
	}

	if p.Totals.DiskAddedMB > 0 || p.Totals.DiskReclaimableMB > 0 {
		b.WriteString("Disk impact:\n")
		if p.Totals.DiskAddedMB > 0 {
			fmt.Fprintf(&b, "  +%s cache\n", formatMB(p.Totals.DiskAddedMB))
		}
		if p.Totals.DiskReclaimableMB > 0 {
			fmt.Fprintf(&b, "  -%s old runtimes removable\n", formatMB(p.Totals.DiskReclaimableMB))
		}
		b.WriteString("\n")
	}

	if p.Totals.EstimatedTime > 0 {
		b.WriteString("Estimated time:\n")
		b.WriteString("  " + formatDuration(p.Totals.EstimatedTime) + "\n")
	}

	return b.String()
}

// formatChangeLine returns the single-line summary used inside the
// "Planned changes" group, e.g.
//
//	terraform 1.8.0 -> 1.9.0    (confidence: 92%)
//	new-tool install (-> 0.5.0)
//	stale remove (1.0.0 ->)
func formatChangeLine(c Change) string {
	display := c.DisplayName
	if display == "" {
		display = c.Tool
	}
	switch c.Kind {
	case ChangeUpgrade:
		s := fmt.Sprintf("%s %s -> %s", display, c.FromVersion, c.ToVersion)
		if c.Confidence > 0 {
			s += fmt.Sprintf("    (confidence: %d%%)", c.Confidence)
		}
		return s
	case ChangeInstall:
		return fmt.Sprintf("%s install -> %s", display, c.ToVersion)
	case ChangeRemove:
		return fmt.Sprintf("%s remove (was %s)", display, c.FromVersion)
	}
	return display
}

func groupBySource(changes []Change) map[registry.InstallSource][]Change {
	grouped := make(map[registry.InstallSource][]Change)
	for _, c := range changes {
		grouped[c.Source] = append(grouped[c.Source], c)
	}
	return grouped
}

func groupedKeys(g map[registry.InstallSource][]Change) []registry.InstallSource {
	keys := make([]registry.InstallSource, 0, len(g))
	for k := range g {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return string(keys[i]) < string(keys[j]) })
	return keys
}

func hasConfidence(changes []Change) bool {
	for _, c := range changes {
		if c.Kind == ChangeUpgrade && c.Confidence > 0 {
			return true
		}
	}
	return false
}

// formatMB renders an MB integer with sensible units. 1024+ becomes
// GB with one decimal.
func formatMB(mb int) string {
	if mb >= 1024 {
		return fmt.Sprintf("%.1fGB", float64(mb)/1024.0)
	}
	return fmt.Sprintf("%dMB", mb)
}

// formatDuration returns "4m 20s"-style output the user asked for.
func formatDuration(d time.Duration) string {
	if d < time.Second {
		return d.Round(time.Millisecond).String()
	}
	d = d.Round(time.Second)
	mins := int(d / time.Minute)
	secs := int(d % time.Minute / time.Second)
	switch {
	case mins == 0:
		return fmt.Sprintf("%ds", secs)
	case secs == 0:
		return fmt.Sprintf("%dm", mins)
	default:
		return fmt.Sprintf("%dm %ds", mins, secs)
	}
}
