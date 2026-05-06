package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/nassiharel/klim/internal/progress"
	"github.com/nassiharel/klim/internal/teamfile"
)

var checkFileFlag string
var checkRefreshFlag bool
var checkOutput func() (OutputFormat, error)

var checkCmd = &cobra.Command{
	Use:   "check",
	Short: "Check installed tools against .klim.yaml requirements",
	Long: `Validate that all tools required by the project's .klim.yaml are
installed and meet version constraints.

Searches for .klim.yaml in the current directory and parent directories.
Use --file to specify an explicit path.

Exit codes:
  0  All requirements satisfied
  1  One or more tools missing or outdated

Usage:
  klim check                      # auto-find .klim.yaml
  klim check --file path/to/.klim.yaml
  klim check --output json        # machine-readable output for CI`,
	RunE: runCheck,
}

func init() {
	checkCmd.Flags().StringVarP(&checkFileFlag, "file", "f", "", "Path to .klim.yaml (default: auto-detect)")
	checkOutput = addOutputFlag(checkCmd, OutputText, OutputJSON)
	checkCmd.Flags().BoolVar(&checkRefreshFlag, "refresh", false, "Force fresh scan (ignore cache)")
	// Registered in root.go with command group.
}

// jsonCheckResult is the JSON output schema.
type jsonCheckResult struct {
	Name      string `json:"name"`
	Required  string `json:"required,omitempty"`
	Installed string `json:"installed,omitempty"`
	Status    string `json:"status"` // "ok", "missing", "outdated", "unknown"
	Message   string `json:"message"`
}

type jsonCheckOutput struct {
	Project string            `json:"project,omitempty"`
	File    string            `json:"file"`
	Results []jsonCheckResult `json:"results"`
	Summary struct {
		OK       int `json:"ok"`
		Missing  int `json:"missing"`
		Outdated int `json:"outdated"`
		Unknown  int `json:"unknown"`
	} `json:"summary"`
	AllSatisfied bool `json:"all_satisfied"`
}

func runCheck(cmd *cobra.Command, args []string) error {
	out, err := checkOutput()
	if err != nil {
		return err
	}

	// Find .klim.yaml.
	path := checkFileFlag
	if path == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("getting working directory: %w", err)
		}
		path = teamfile.Find(cwd)
		if path == "" {
			return fmt.Errorf("no .klim.yaml found (searched from %s to root)", cwd)
		}
	}

	// Parse.
	tf, err := teamfile.Parse(path)
	if err != nil {
		return err
	}

	// Scan installed tools and resolve versions (cached by default for speed).
	sp := progress.New("Scanning installed tools...")
	tools, _, _, err := svcFrom(cmd).LoadAndResolveCached(cmd.Context(), checkRefreshFlag)
	if err != nil {
		sp.Fail(err.Error())
		return err
	}
	sp.Done("Tools scanned")

	// Check.
	results := teamfile.Check(tf, tools)
	ok, missing, outdated, unknown := teamfile.Summary(results)

	// Auto-register project.
	name := tf.Name
	if name == "" {
		name = filepath.Base(filepath.Dir(path))
	}
	_ = teamfile.AddProject(filepath.Dir(path), name, len(tf.Tools)+len(tf.Optional))

	if out == OutputJSON {
		return printCheckJSON(tf, path, results, ok, missing, outdated, unknown)
	}

	// Human output.
	projectLabel := ""
	if tf.Name != "" {
		projectLabel = fmt.Sprintf(" (%s)", tf.Name)
	}
	totalTools := len(tf.Tools) + len(tf.Optional)
	fmt.Fprintf(os.Stderr, "\nChecking %s%s — %d tools (%d required, %d optional)\n\n", path, projectLabel, totalTools, len(tf.Tools), len(tf.Optional))

	// Print required.
	hasRequired := false
	for _, r := range results {
		if r.Optional {
			continue
		}
		if !hasRequired {
			fmt.Fprintln(os.Stderr, "  Required:")
			hasRequired = true
		}
		printCheckLine(r)
	}

	// Print optional.
	hasOptional := false
	for _, r := range results {
		if !r.Optional {
			continue
		}
		if !hasOptional {
			fmt.Fprintln(os.Stderr, "\n  Optional:")
			hasOptional = true
		}
		printCheckLine(r)
	}

	fmt.Fprintf(os.Stderr, "\nResult: %d OK, %d missing, %d outdated, %d unknown\n", ok, missing, outdated, unknown)

	if !teamfile.AllSatisfied(results) {
		return fmt.Errorf("%d tool(s) missing, outdated, or unknown", missing+outdated+unknown)
	}
	fmt.Fprintln(os.Stderr, "All requirements satisfied!")
	return nil
}

func printCheckLine(r teamfile.CheckResult) {
	var icon, ver, constraint string
	switch r.Status {
	case teamfile.StatusOK:
		icon = "✓"
		ver = r.Version
	case teamfile.StatusMissing:
		icon = "✗"
		ver = "—"
	case teamfile.StatusOutdated:
		icon = "⚠"
		ver = r.Version
	case teamfile.StatusUnknown:
		icon = "?"
		ver = "—"
	}
	if r.Tool.Version != "" {
		constraint = fmt.Sprintf("(%s)", r.Tool.Version)
	}
	fmt.Fprintf(os.Stderr, "    %s %-20s %-12s %s\n", icon, r.Tool.Name, ver, constraint)
}

func printCheckJSON(tf *teamfile.TeamFile, path string, results []teamfile.CheckResult, ok, missing, outdated, unknown int) error {
	out := jsonCheckOutput{
		Project:      tf.Name,
		File:         path,
		AllSatisfied: teamfile.AllSatisfied(results),
	}
	out.Summary.OK = ok
	out.Summary.Missing = missing
	out.Summary.Outdated = outdated
	out.Summary.Unknown = unknown

	for _, r := range results {
		status := "ok"
		switch r.Status {
		case teamfile.StatusMissing:
			status = "missing"
		case teamfile.StatusOutdated:
			status = "outdated"
		case teamfile.StatusUnknown:
			status = "unknown"
		}
		out.Results = append(out.Results, jsonCheckResult{
			Name:      r.Tool.Name,
			Required:  r.Tool.Version,
			Installed: r.Version,
			Status:    status,
			Message:   r.Message,
		})
	}

	if err := printJSON(out); err != nil {
		return err
	}

	if !out.AllSatisfied {
		return fmt.Errorf("%d tool(s) missing, outdated, or unknown", missing+outdated+unknown)
	}
	return nil
}
