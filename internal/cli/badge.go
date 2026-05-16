package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/nassiharel/klim/internal/audit"
	"github.com/nassiharel/klim/internal/badge"
	"github.com/nassiharel/klim/internal/compliance"
	"github.com/nassiharel/klim/internal/doctor"
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
  klim audit   vulnerability status (clean / N issues)
  klim fresh   percent of installed tools up to date

Examples:
  klim badge
  klim badge --score --audit
  klim badge --output json
  klim badge --output yaml > badges.yaml`,
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

func runBadge(cmd *cobra.Command, args []string) error {
	out, err := badgeFormatFn()
	if err != nil {
		return err
	}
	_ = args

	svc := svcFrom(cmd)
	sp := spinnerFor(out, "Scanning…")
	tools, _, _, err := svc.LoadAndResolveCached(cmd.Context(), badgeRefreshFlag)
	if err != nil {
		sp.Fail(err.Error())
		return fmt.Errorf("klim badge: %w", err)
	}
	sp.Stop()

	// Reuse the existing inputs the score CLI builds — keep them in
	// sync so `klim score` and `klim badge` never disagree.
	doctorIssues := doctor.Diagnose(tools, doctor.ScanMeta{})
	var compResult *compliance.Result
	var compErrStr string
	if policyPath := findPolicyPath(cfgFrom(cmd)); policyPath != "" {
		policy, loadErr := compliance.LoadPolicy(policyPath)
		if loadErr != nil {
			compErrStr = loadErr.Error()
		} else {
			r := compliance.Check(policy, tools, loadVulnSeveritiesForCompliance())
			compResult = &r
		}
	}
	findings, _ := audit.Analyze(tools)
	auditWarns, auditInfos := audit.CountBySeverity(findings)

	result := score.Compute(score.Input{
		Tools:         tools,
		DoctorIssues:  doctorIssues,
		AuditWarnings: auditWarns,
		AuditInfos:    auditInfos,
		CompResult:    compResult,
		ComplianceErr: compErrStr,
	})

	in := badge.Inputs{
		ScorePoints:  result.Total,
		ScoreMax:     result.MaxTotal,
		ScoreGrade:   result.Grade,
		ToolCount:    countInstalled(tools),
		AuditIssues:  auditWarns + auditInfos,
		FreshPercent: freshPercent(tools),
	}

	ids := selectedBadgeIDs()
	badges := badge.ByID(in, ids...)

	switch out {
	case OutputJSON:
		return printJSON(buildBadgeReport(result, badges))
	case OutputYAML:
		return printYAML(buildBadgeReport(result, badges))
	}

	for _, b := range badges {
		fmt.Println(b.Markdown())
	}
	return nil
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

// Stop the linter complaining about os.Stderr in shared file; we
// intentionally keep stdout-only output for the markdown body.
var _ = os.Stderr
var _ = strings.TrimSpace
