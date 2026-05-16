package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/nassiharel/klim/internal/badge"
	"github.com/nassiharel/klim/internal/registry"
	"github.com/nassiharel/klim/internal/score"
)

var (
	badgeAll         bool
	badgeScore       bool
	badgeTools       bool
	badgeAudit       bool
	badgeFresh       bool
	badgeRefreshFlag bool
	badgeFormatFn    func() (OutputFormat, error)
)

var badgeCmd = &cobra.Command{
	Use:   "badge",
	Short: "Generate README badges for your klim environment",
	Long: `Print Shields.io-compatible badge markdown for your current klim
state. Drop the output into a README, a personal profile, or a team
dashboard to show off (or alert on) your toolchain health.

By default every badge is printed. Use the per-badge flags to pick a
subset.

Badges:
  klim score   overall env score X/Y graded A+..F
  klim tools   number of installed tools
  klim audit   audit findings count (clean / N issues from internal/audit)
  klim fresh   percent of installed tools up to date

Examples:
  klim badge
  klim badge --score --audit
  klim badge --output json
  klim badge --output yaml > badges.yaml`,
	Args: cobra.NoArgs,
	RunE: runBadge,
}

func init() {
	badgeCmd.Flags().BoolVar(&badgeAll, "all", false, "print every badge (default when no per-badge flag is set)")
	badgeCmd.Flags().BoolVar(&badgeScore, "score", false, "include the score badge")
	badgeCmd.Flags().BoolVar(&badgeTools, "tools", false, "include the tools badge")
	badgeCmd.Flags().BoolVar(&badgeAudit, "audit", false, "include the audit badge")
	badgeCmd.Flags().BoolVar(&badgeFresh, "fresh", false, "include the fresh-percent badge")
	badgeCmd.Flags().BoolVar(&badgeRefreshFlag, "refresh", false, "ignore the scan cache and rescan")
	badgeFormatFn = addOutputFlag(badgeCmd, OutputText, OutputJSON, OutputYAML)
	// Registered in root.go.
}

// badgeReport is the structured shape for --output json|yaml.
type badgeReport struct {
	Score    string         `json:"score" yaml:"score"`
	Total    int            `json:"total" yaml:"total"`
	MaxTotal int            `json:"max_total" yaml:"max_total"`
	Grade    string         `json:"grade" yaml:"grade"`
	Badges   []badgeReportB `json:"badges" yaml:"badges"`
}

type badgeReportB struct {
	ID       string `json:"id" yaml:"id"`
	Label    string `json:"label" yaml:"label"`
	Value    string `json:"value" yaml:"value"`
	Color    string `json:"color" yaml:"color"`
	URL      string `json:"url" yaml:"url"`
	Markdown string `json:"markdown" yaml:"markdown"`
}

func runBadge(cmd *cobra.Command, _ []string) error {
	out, err := badgeFormatFn()
	if err != nil {
		return err
	}

	svc := svcFrom(cmd)
	sp := spinnerFor(out, "Scanning…")
	tools, _, _, err := svc.LoadAndResolveCached(cmd.Context(), badgeRefreshFlag)
	if err != nil {
		sp.Fail(err.Error())
		return fmt.Errorf("klim badge: %w", err)
	}
	sp.Stop()

	// Resolve which badges the user actually asked for FIRST, so we
	// can skip the doctor/compliance/audit scan for runs that don't
	// need score or audit inputs (e.g. `klim badge --tools --fresh`).
	ids := selectedBadgeIDs()
	wantScore := needsBadge(ids, "score")
	wantAudit := needsBadge(ids, "audit")
	wantStructured := out == OutputJSON || out == OutputYAML

	var (
		result            score.Result
		auditWarns        int
		auditInfos        int
		havePolicyOrAudit bool
	)
	// Structured output always carries the full score block, so
	// fall through to the heavy path for --output json/yaml even
	// when the user only asked for one badge.
	if wantScore || wantAudit || wantStructured {
		result, auditWarns, auditInfos = buildScoreInputs(cmd, tools)
		havePolicyOrAudit = true
	}
	_ = havePolicyOrAudit

	pct := 0
	if result.MaxTotal > 0 {
		pct = result.Total * 100 / result.MaxTotal
	}
	in := badge.Inputs{
		ScorePoints: result.Total,
		ScoreMax:    result.MaxTotal,
		ScoreGrade:  result.Grade,
		// Use score.BadgeColor so `klim badge --score` and
		// `klim score --badge` always agree on colour for the
		// same input. Passing nothing here would fall back to the
		// package's local table, which can drift.
		ScoreColor:   score.BadgeColor(pct),
		ToolCount:    countInstalled(tools),
		AuditIssues:  auditWarns + auditInfos,
		FreshPercent: freshPercent(tools),
	}

	badges := badge.ByID(in, ids...)

	if wantStructured {
		return printStructured(out, buildBadgeReport(result, badges))
	}

	for _, b := range badges {
		fmt.Println(b.Markdown())
	}
	return nil
}

// needsBadge reports whether the given badge id is requested. nil
// ids means "all badges" (the no-flag default).
func needsBadge(ids []string, id string) bool {
	if ids == nil {
		return true
	}
	for _, x := range ids {
		if x == id {
			return true
		}
	}
	return false
}

func selectedBadgeIDs() []string {
	// `--all` and "no flag at all" both mean "every badge".
	if badgeAll || (!badgeScore && !badgeTools && !badgeAudit && !badgeFresh) {
		return nil // empty filter => Build's default order
	}
	var ids []string
	if badgeScore {
		ids = append(ids, "score")
	}
	if badgeTools {
		ids = append(ids, "tools")
	}
	if badgeAudit {
		ids = append(ids, "audit")
	}
	if badgeFresh {
		ids = append(ids, "fresh")
	}
	return ids
}

func buildBadgeReport(result score.Result, badges []badge.Badge) badgeReport {
	rep := badgeReport{
		Score:    fmt.Sprintf("%d/%d", result.Total, result.MaxTotal),
		Total:    result.Total,
		MaxTotal: result.MaxTotal,
		Grade:    result.Grade,
	}
	for _, b := range badges {
		rep.Badges = append(rep.Badges, badgeReportB{
			ID:       b.ID,
			Label:    b.Label,
			Value:    b.Value,
			Color:    b.Color,
			URL:      b.URL(),
			Markdown: b.Markdown(),
		})
	}
	return rep
}

func countInstalled(tools []registry.Tool) int {
	n := 0
	for _, t := range tools {
		if t.IsInstalled() {
			n++
		}
	}
	return n
}

func freshPercent(tools []registry.Tool) int {
	installed, fresh := 0, 0
	for _, t := range tools {
		if !t.IsInstalled() {
			continue
		}
		installed++
		if !t.HasUpdate() {
			fresh++
		}
	}
	if installed == 0 {
		return 100
	}
	return fresh * 100 / installed
}
