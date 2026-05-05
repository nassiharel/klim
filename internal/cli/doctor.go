package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/nassiharel/clim/internal/doctor"
	"github.com/nassiharel/clim/internal/progress"
	"github.com/nassiharel/clim/internal/service"
)

var doctorRefreshFlag bool
var doctorOutput func() (OutputFormat, error)

var doctorCmd = &cobra.Command{
	Use:   "health",
	Short: "Check environment health and diagnose common issues",
	Long: `Run environment diagnostics to detect PATH problems, conflicting
tool installations, missing package managers, stale caches, and
available updates.

Exit codes:
  0  No errors found (warnings and info may still be reported)
  1  One or more errors detected`,
	RunE: runDoctor,
}

func init() {
	doctorOutput = addOutputFlag(doctorCmd, OutputText, OutputJSON)
	doctorCmd.Flags().BoolVar(&doctorRefreshFlag, "refresh", false, "Force fresh scan (ignore cache)")
	// Registered in root.go with command group.
}

// jsonDoctorOutput is the JSON output schema for doctor results.
type jsonDoctorOutput struct {
	Issues  []doctor.Issue `json:"issues"`
	Summary struct {
		Errors   int `json:"errors"`
		Warnings int `json:"warnings"`
		Infos    int `json:"infos"`
	} `json:"summary"`
	Healthy bool `json:"healthy"`
}

func runDoctor(cmd *cobra.Command, args []string) error {
	out, err := doctorOutput()
	if err != nil {
		return err
	}

	sp := progress.New("Running diagnostics...")
	tools, _, scanInfo, err := svcFrom(cmd).LoadAndResolveCached(cmd.Context(), doctorRefreshFlag)
	if err != nil {
		sp.Fail(err.Error())
		return err
	}
	sp.Done("Scan complete")

	meta := doctor.ScanMeta{}
	if scanInfo != nil && scanInfo.Source == service.ScanSourceCache {
		meta.FromCache = true
	}

	issues := doctor.Diagnose(tools, meta)
	errors, warnings, infos := doctor.CountBySeverity(issues)

	if out == OutputJSON {
		return printDoctorJSON(issues, errors, warnings, infos)
	}

	if len(issues) == 0 {
		fmt.Fprintln(os.Stderr, "\n✓ No issues found — your environment looks healthy!")
		return nil
	}

	// Group by category for display.
	grouped := make(map[string][]doctor.Issue)
	var categoryOrder []string
	for _, issue := range issues {
		if _, ok := grouped[issue.Category]; !ok {
			categoryOrder = append(categoryOrder, issue.Category)
		}
		grouped[issue.Category] = append(grouped[issue.Category], issue)
	}

	fmt.Fprintln(os.Stderr)
	for _, cat := range categoryOrder {
		fmt.Fprintf(os.Stderr, "  %s\n", cat)
		for _, issue := range grouped[cat] {
			icon := severityIcon(issue.Severity)
			fmt.Fprintf(os.Stderr, "    %s %s\n", icon, issue.Title)
			if issue.Detail != "" {
				for _, line := range splitLines(issue.Detail) {
					fmt.Fprintf(os.Stderr, "      %s\n", line)
				}
			}
			if issue.Fix != "" {
				fmt.Fprintf(os.Stderr, "      → %s\n", issue.Fix)
			}
		}
		fmt.Fprintln(os.Stderr)
	}

	fmt.Fprintf(os.Stderr, "Result: %d error(s), %d warning(s), %d info(s)\n", errors, warnings, infos)

	if doctor.HasErrors(issues) {
		return fmt.Errorf("%d error(s) found", errors)
	}
	return nil
}

func printDoctorJSON(issues []doctor.Issue, errors, warnings, infos int) error {
	out := jsonDoctorOutput{
		Issues:  issues,
		Healthy: !doctor.HasErrors(issues),
	}
	out.Summary.Errors = errors
	out.Summary.Warnings = warnings
	out.Summary.Infos = infos

	if err := printJSON(out); err != nil {
		return err
	}

	if doctor.HasErrors(issues) {
		return fmt.Errorf("%d error(s) found", errors)
	}
	return nil
}

func severityIcon(s doctor.Severity) string {
	switch s {
	case doctor.SeverityError:
		return "✗"
	case doctor.SeverityWarning:
		return "⚠"
	case doctor.SeverityInfo:
		return "ℹ"
	}
	return "?"
}

func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	var lines []string
	for _, line := range strings.Split(s, "\n") {
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}
