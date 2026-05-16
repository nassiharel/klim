// Package badge renders Shields.io-compatible README badges from
// klim's environment state. Pure-go, no network — Shields.io URLs
// are deterministic string formatting.
//
// klim badge prints four standard badges as ready-to-copy markdown
// snippets:
//
//   - score: overall klim env score, X/Y (graded)
//   - tools: number of installed tools
//   - audit: vulnerability status (clean / N issues)
//   - fresh: percentage of installed tools currently up to date
//
// Each Badge knows its label, value, color, and link target so the
// command layer can render markdown / html / plain URL forms without
// reaching into Shields.io specifics.
package badge

import (
	"fmt"
	"net/url"
	"strings"
)

// Badge is one renderable badge. ID is a stable identifier (score /
// tools / audit / fresh) the CLI uses for `--score`-style flag
// selectors; Label and Value go on the badge itself; Color is a
// Shields.io named color; Link is the optional click-through target
// embedded in the markdown wrapper.
type Badge struct {
	ID    string
	Label string
	Value string
	Color string
	Link  string
}

// URL returns the bare Shields.io URL for the badge image.
func (b Badge) URL() string {
	label := strings.ReplaceAll(url.PathEscape(b.Label), "-", "--")
	value := strings.ReplaceAll(url.PathEscape(b.Value), "-", "--")
	return fmt.Sprintf("https://img.shields.io/badge/%s-%s-%s", label, value, b.Color)
}

// Markdown returns a markdown image snippet. When Link is set the
// image is wrapped in a link.
func (b Badge) Markdown() string {
	alt := fmt.Sprintf("%s: %s", b.Label, b.Value)
	img := fmt.Sprintf("![%s](%s)", alt, b.URL())
	if b.Link != "" {
		return fmt.Sprintf("[%s](%s)", img, b.Link)
	}
	return img
}

// Inputs feeds Build with the raw numbers we need.
type Inputs struct {
	ScorePoints int    // current score
	ScoreMax    int    // max score (avoid div-by-zero issues)
	ScoreGrade  string // A+/A/B/… per score.Result

	ToolCount int // count of installed tools

	AuditIssues int // total vuln findings (errors+warnings)

	FreshPercent int // 0-100 percentage of installed tools up to date
}

// Build returns the four standard badges for the given inputs. Each
// badge's color follows the simple thresholds documented in
// internal/badge/badge.go's doc comment.
func Build(in Inputs) []Badge {
	return []Badge{
		scoreBadge(in),
		toolsBadge(in),
		auditBadge(in),
		freshBadge(in),
	}
}

// ByID returns the subset of Build's badges matching one of the
// requested IDs. Empty ids returns all badges.
func ByID(in Inputs, ids ...string) []Badge {
	all := Build(in)
	if len(ids) == 0 {
		return all
	}
	want := make(map[string]bool, len(ids))
	for _, id := range ids {
		want[strings.ToLower(strings.TrimSpace(id))] = true
	}
	out := make([]Badge, 0, len(all))
	for _, b := range all {
		if want[b.ID] {
			out = append(out, b)
		}
	}
	return out
}

func scoreBadge(in Inputs) Badge {
	pct := pctOf(in.ScorePoints, in.ScoreMax)
	value := fmt.Sprintf("%d/%d %s", in.ScorePoints, in.ScoreMax, strings.TrimSpace(in.ScoreGrade))
	return Badge{
		ID:    "score",
		Label: "klim score",
		Value: value,
		Color: colorByPercent(pct),
	}
}

func toolsBadge(in Inputs) Badge {
	color := "blue"
	if in.ToolCount == 0 {
		color = "lightgrey"
	}
	return Badge{
		ID:    "tools",
		Label: "klim tools",
		Value: fmt.Sprintf("%d installed", in.ToolCount),
		Color: color,
	}
}

func auditBadge(in Inputs) Badge {
	value := "clean"
	color := "brightgreen"
	switch {
	case in.AuditIssues == 0:
		// keep defaults
	case in.AuditIssues <= 3:
		value = fmt.Sprintf("%d issues", in.AuditIssues)
		color = "yellow"
	default:
		value = fmt.Sprintf("%d issues", in.AuditIssues)
		color = "red"
	}
	return Badge{
		ID:    "audit",
		Label: "klim audit",
		Value: value,
		Color: color,
	}
}

func freshBadge(in Inputs) Badge {
	return Badge{
		ID:    "fresh",
		Label: "klim fresh",
		Value: fmt.Sprintf("%d%% up to date", in.FreshPercent),
		Color: colorByFreshness(in.FreshPercent),
	}
}

func colorByPercent(p int) string {
	switch {
	case p >= 90:
		return "brightgreen"
	case p >= 75:
		return "green"
	case p >= 60:
		return "yellow"
	case p >= 40:
		return "orange"
	default:
		return "red"
	}
}

func colorByFreshness(p int) string {
	switch {
	case p >= 100:
		return "brightgreen"
	case p >= 90:
		return "green"
	case p >= 75:
		return "yellow"
	default:
		return "red"
	}
}

func pctOf(num, denom int) int {
	if denom <= 0 {
		return 0
	}
	return num * 100 / denom
}
