package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/nassiharel/klim/internal/pathconflict"
	"github.com/nassiharel/klim/internal/registry"
)

var healthPathRefreshFlag bool
var healthPathOutput func() (OutputFormat, error)

// healthPathCmd renders the PATH-conflict visualization in text or JSON
// form. The TUI shows the same model through a two-pane interactive view
// under Health → PATH; this command is the script-friendly counterpart.
var healthPathCmd = &cobra.Command{
	Use:   "path",
	Short: "Visualize PATH conflicts: which binary wins, what's shadowed",
	Long: `Inspect every tool that has more than one installation on PATH
and report which copy resolves first, which are shadowed, and where
versions diverge across copies.

Output sections:
  By tool   — each tool with multiple PATH instances (active vs shadowed)
  By dir    — every PATH entry with the tools it provides and whether
              this dir wins or loses the PATH lookup for each one

Use --output=json for the machine-readable model (same schema the TUI
renders).

Exit codes:
  0  No conflicts (every tool has at most one copy, or all versions match)
  1  One or more tools have differing versions across PATH copies`,
	RunE: runHealthPath,
}

func init() {
	healthPathOutput = addOutputFlag(healthPathCmd, OutputText, OutputJSON, OutputYAML)
	healthPathCmd.Flags().BoolVar(&healthPathRefreshFlag, "refresh", false, "Force fresh scan (ignore cache)")
	doctorCmd.AddCommand(healthPathCmd)
}

func runHealthPath(cmd *cobra.Command, _ []string) error {
	out, err := healthPathOutput()
	if err != nil {
		return err
	}

	sp := spinnerFor(out, "Scanning PATH...")
	tools, _, _, err := svcFrom(cmd).LoadAndResolveCached(cmd.Context(), healthPathRefreshFlag)
	if err != nil {
		sp.Fail(err.Error())
		return err
	}
	sp.Done("Scan complete")

	report := pathconflict.Analyze(tools)

	if out == OutputJSON || out == OutputYAML {
		if err := printStructured(out, report); err != nil {
			return err
		}
		if n := countVersionConflicts(report); n > 0 {
			return fmt.Errorf("%d tool(s) have differing versions across PATH copies", n)
		}
		return nil
	}

	printHealthPathText(report)

	if n := countVersionConflicts(report); n > 0 {
		return fmt.Errorf("%d tool(s) have differing versions across PATH copies", n)
	}
	return nil
}

func countVersionConflicts(r pathconflict.Report) int {
	n := 0
	for _, v := range r.ByTool {
		if v.VersionConflict {
			n++
		}
	}
	return n
}

func printHealthPathText(r pathconflict.Report) {
	if len(r.ByTool) == 0 {
		fmt.Fprintln(os.Stderr, "\n✓ No PATH conflicts — every tool resolves to a single copy.")
		return
	}

	fmt.Fprintln(os.Stderr)
	fmt.Fprintf(os.Stderr, "  By tool — %d tool(s) with multiple PATH copies, %d shadowed total\n\n",
		len(r.ByTool), r.CountShadowed())
	for _, v := range r.ByTool {
		flag := ""
		switch {
		case v.VersionConflict:
			flag = "  ⚠ version conflict"
		case v.PrivilegeRisk:
			flag = "  ⚠ user-writable shadows system"
		}
		fmt.Fprintf(os.Stderr, "  %s%s\n", v.DisplayName, flag)
		fmt.Fprintf(os.Stderr, "    ✓ active   %s  %s\n", formatVerSource(v.Active), v.Active.Path)
		for _, s := range v.Shadowed {
			fmt.Fprintf(os.Stderr, "    ⊘ shadowed %s  %s\n", formatVerSource(s), s.Path)
			if s.UninstallCmd != "" {
				fmt.Fprintf(os.Stderr, "        → %s\n", s.UninstallCmd)
			}
		}
		fmt.Fprintln(os.Stderr)
	}

	if len(r.ByDir) == 0 {
		return
	}
	fmt.Fprintln(os.Stderr, "  By PATH directory")
	fmt.Fprintln(os.Stderr)
	for _, d := range r.ByDir {
		annotations := []string{}
		if !d.Exists {
			annotations = append(annotations, "missing")
		} else if !d.IsDir {
			annotations = append(annotations, "not a directory")
		}
		if d.Duplicate {
			annotations = append(annotations, "duplicate")
		}
		if d.UserWrite {
			annotations = append(annotations, "user-writable")
		}
		if d.SystemDir {
			annotations = append(annotations, "system")
		}
		ann := ""
		if len(annotations) > 0 {
			ann = "  [" + strings.Join(annotations, ", ") + "]"
		}
		fmt.Fprintf(os.Stderr, "  %2d. %s%s\n", d.Order, d.Dir, ann)
		for _, te := range d.Tools {
			marker := "    ⊘"
			if te.Active {
				marker = "    ✓"
			}
			fmt.Fprintf(os.Stderr, "%s %s (%s, %s)\n", marker, te.DisplayName, formatVerOrDash(te.Version), sourceOrDash(te.Source))
		}
	}
	fmt.Fprintln(os.Stderr)
}

func formatVerSource(iv pathconflict.InstanceView) string {
	return fmt.Sprintf("(%s, %s)", formatVerOrDash(iv.Version), sourceOrDash(iv.Source))
}

func formatVerOrDash(v string) string {
	if v == "" {
		return "?"
	}
	return v
}

func sourceOrDash(s registry.InstallSource) string {
	if s == "" {
		return "manual"
	}
	return string(s)
}
