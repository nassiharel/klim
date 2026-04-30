package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/nassiharel/clim/internal/audit"
	"github.com/nassiharel/clim/internal/compliance"
	"github.com/nassiharel/clim/internal/doctor"
	"github.com/nassiharel/clim/internal/progress"
	"github.com/nassiharel/clim/internal/score"
)

var scoreJSONFlag bool
var scoreBadgeFlag bool
var scoreRefreshFlag bool

var scoreCmd = &cobra.Command{
	Use:   "score",
	Short: "Calculate your environment health score (0-100)",
	Long: `Compute a single health score for your dev environment by combining
tool freshness, doctor diagnostics, audit findings, compliance status,
and source management into a 0-100 metric.

Grade scale: A+ (95+), A (90+), B (80+), C (70+), D (60+), F (<60)`,
	RunE: runScore,
}

func init() {
	scoreCmd.Flags().BoolVar(&scoreJSONFlag, "json", false, "Output as JSON")
	scoreCmd.Flags().BoolVar(&scoreBadgeFlag, "badge", false, "Output shields.io badge URL")
	scoreCmd.Flags().BoolVar(&scoreRefreshFlag, "refresh", false, "Force fresh scan")
	// Registered in root.go with command group.
}

func runScore(cmd *cobra.Command, args []string) error {
	sp := progress.New("Scanning...")
	tools, _, _, err := svc.LoadAndResolveCached(cmd.Context(), scoreRefreshFlag)
	if err != nil {
		sp.Fail(err.Error())
		return err
	}
	sp.Done("Done")

	// Run doctor.
	doctorIssues := doctor.Diagnose(tools, doctor.ScanMeta{})

	// Run compliance if configured.
	var compResult *compliance.Result
	policyPath := cfg.Compliance.Policy
	if policyPath == "" {
		for _, candidate := range []string{".clim-policy.yaml", ".clim-policy.yml"} {
			if _, err := os.Stat(candidate); err == nil {
				policyPath = candidate
				break
			}
		}
	}
	var compErrStr string
	if policyPath != "" {
		policy, loadErr := compliance.LoadPolicy(policyPath)
		if loadErr != nil {
			fmt.Fprintf(os.Stderr, "  ⚠ Compliance policy error: %v\n", loadErr)
			compErrStr = loadErr.Error()
		} else {
			r := compliance.Check(policy, tools)
			compResult = &r
		}
	}

	findings, _ := audit.Analyze(tools)
	auditWarns, auditInfos := audit.CountBySeverity(findings)

	result := score.Compute(score.ScoreInput{
		Tools:         tools,
		DoctorIssues:  doctorIssues,
		AuditWarnings: auditWarns,
		AuditInfos:    auditInfos,
		CompResult:    compResult,
		ComplianceErr: compErrStr,
	})

	if scoreBadgeFlag {
		fmt.Println(score.BadgeURL(result))
		return nil
	}

	if scoreJSONFlag {
		data, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		return nil
	}

	// Human output.
	fmt.Fprintln(os.Stderr)

	// Score box.
	barWidth := 20
	filled := 0
	if result.MaxTotal > 0 {
		filled = result.Total * barWidth / result.MaxTotal
	}
	if filled > barWidth {
		filled = barWidth
	}
	bar := strings.Repeat("█", filled) + strings.Repeat("░", barWidth-filled)

	fmt.Fprintf(os.Stderr, "  ╔══════════════════════════════════╗\n")
	fmt.Fprintf(os.Stderr, "  ║  Environment Score: %3d / %-3d   ║\n", result.Total, result.MaxTotal)
	fmt.Fprintf(os.Stderr, "  ║  %s  Grade: %-2s ║\n", bar, result.Grade)
	fmt.Fprintf(os.Stderr, "  ╚══════════════════════════════════╝\n\n")

	// Category breakdown.
	for _, c := range result.Categories {
		icon := "✓"
		if c.Status == "warning" {
			icon = "⚠"
		} else if c.Status == "error" {
			icon = "✗"
		}
		fmt.Fprintf(os.Stderr, "  %s %-22s %2d/%d", icon, c.Name, c.Points, c.MaxPts)
		if c.Details != "" {
			fmt.Fprintf(os.Stderr, "  %s", c.Details)
		}
		fmt.Fprintln(os.Stderr)
	}

	// Tip.
	fmt.Fprintln(os.Stderr)
	if result.Total < result.MaxTotal {
		for _, c := range result.Categories {
			if c.Points < c.MaxPts {
				gap := c.MaxPts - c.Points
				fmt.Fprintf(os.Stderr, "  Tip: Improve %q to gain up to %d points\n", c.Name, gap)
				break
			}
		}
	} else {
		fmt.Fprintln(os.Stderr, "  ★ Perfect score! Your environment is in great shape.")
	}

	return nil
}
