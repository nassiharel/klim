package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/nassiharel/klim/internal/audit"
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
  klim share badge
  klim share badge --score --audit
  klim share badge --output json
  klim share badge --output yaml > badges.yaml`,
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

	// Decide what work is needed BEFORE doing it.
	ids := selectedBadgeIDs()
	wantScore := needsBadge(ids, "score")
	wantTools := needsBadge(ids, "tools")
	wantAudit := needsBadge(ids, "audit")
	wantFresh := needsBadge(ids, "fresh")
	wantStructured := out == OutputJSON || out == OutputYAML
	// Structured output always carries the full score block, so a
	// `klim share badge --tools --output yaml` still needs the score path.
	needScorePipeline := wantScore || wantStructured
	// audit.Analyze surfaces "No Version" / "Outdated" findings,
	// which depend on a tool's resolved Latest field. Without
	// version resolution those findings are silently dropped — the
	// audit count under-reports. So treat --audit as needing
	// versions too (matches what `klim audit` does internally).
	needVersions := wantFresh || wantAudit || needScorePipeline
	_ = wantTools // every badge needs tools data; tracked here only for symmetry

	svc := svcFrom(cmd)
	sp := spinnerFor(out, "Scanning…")

	var tools []registry.Tool
	if needVersions {
		// Need installed/not + latest versions: full resolve.
		// In practice this covers everything except the
		// `--tools`-only case (no audit, no fresh, no score, no
		// structured output), which is the one path that can use
		// the cheap ScanOnly fast lane below.
		tools, _, _, err = svc.LoadAndResolveCached(cmd.Context(), badgeRefreshFlag)
	} else {
		// `klim share badge --tools` (text only) only needs the
		// installed-or-not state — ScanOnly skips per-tool
		// version resolution which is the expensive bit on a
		// cold cache.
		tools, _, err = svc.ScanOnly(cmd.Context())
	}
	if err != nil {
		sp.Fail(err.Error())
		return fmt.Errorf("klim share badge: %w", err)
	}
	sp.Stop()

	var (
		result     score.Result
		auditWarns int
		auditInfos int
	)
	switch {
	case needScorePipeline:
		// Full pipeline: doctor + compliance + audit + score.Compute.
		// Shared with `klim security score` via computeScoreReport.
		result, auditWarns, auditInfos = computeScoreReport(cmd, tools)
	case wantAudit:
		// Audit badge only: skip doctor and compliance entirely,
		// which means no spurious compliance-policy warning when
		// the user asked only for `--audit`.
		findings, _ := audit.Analyze(tools)
		auditWarns, auditInfos = audit.CountBySeverity(findings)
	}

	pct := 0
	if result.MaxTotal > 0 {
		pct = result.Total * 100 / result.MaxTotal
	}
	in := badge.Inputs{
		ScorePoints: result.Total,
		ScoreMax:    result.MaxTotal,
		ScoreGrade:  result.Grade,
		// Use score.BadgeColor so `klim share badge --score` and
		// `klim security score --badge` always agree on colour for the
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
