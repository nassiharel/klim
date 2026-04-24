package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/nassiharel/clim/internal/progress"
	"github.com/nassiharel/clim/internal/teamfile"
)

var checkFileFlag string
var checkJSONFlag bool

var checkCmd = &cobra.Command{
	Use:   "check",
	Short: "Check installed tools against .clim.yaml requirements",
	Long: `Validate that all tools required by the project's .clim.yaml are
installed and meet version constraints.

Searches for .clim.yaml in the current directory and parent directories.
Use --file to specify an explicit path.

Exit codes:
  0  All requirements satisfied
  1  One or more tools missing or outdated

Usage:
  clim check                      # auto-find .clim.yaml
  clim check --file path/to/.clim.yaml
  clim check --json               # machine-readable output for CI`,
	RunE: runCheck,
}

func init() {
	checkCmd.Flags().StringVarP(&checkFileFlag, "file", "f", "", "Path to .clim.yaml (default: auto-detect)")
	checkCmd.Flags().BoolVar(&checkJSONFlag, "json", false, "Output results as JSON")
	rootCmd.AddCommand(checkCmd)
}

// jsonCheckResult is the JSON output schema.
type jsonCheckResult struct {
	Name       string `json:"name"`
	Required   string `json:"required,omitempty"`
	Installed  string `json:"installed,omitempty"`
	Status     string `json:"status"` // "ok", "missing", "outdated"
	Message    string `json:"message"`
}

type jsonCheckOutput struct {
	Project  string            `json:"project,omitempty"`
	File     string            `json:"file"`
	Results  []jsonCheckResult `json:"results"`
	Summary  struct {
		OK       int `json:"ok"`
		Missing  int `json:"missing"`
		Outdated int `json:"outdated"`
	} `json:"summary"`
	AllSatisfied bool `json:"all_satisfied"`
}

func runCheck(cmd *cobra.Command, args []string) error {
	// Find .clim.yaml.
	path := checkFileFlag
	if path == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("getting working directory: %w", err)
		}
		path = teamfile.Find(cwd)
		if path == "" {
			return fmt.Errorf("no .clim.yaml found (searched from %s to root)", cwd)
		}
	}

	// Parse.
	tf, err := teamfile.Parse(path)
	if err != nil {
		return err
	}

	// Scan installed tools and resolve versions (needed for version constraints).
	sp := progress.New("Scanning installed tools...")
	tools, _, err := svc.LoadAndResolve(cmd.Context())
	if err != nil {
		sp.Fail(err.Error())
		return err
	}
	sp.Done("Tools scanned")

	// Check.
	results := teamfile.Check(tf, tools)
	ok, missing, outdated := teamfile.Summary(results)

	if checkJSONFlag {
		return printCheckJSON(tf, path, results, ok, missing, outdated)
	}

	// Human output.
	projectLabel := ""
	if tf.Name != "" {
		projectLabel = fmt.Sprintf(" (%s)", tf.Name)
	}
	fmt.Fprintf(os.Stderr, "\nChecking %s%s — %d tools\n\n", path, projectLabel, len(tf.Tools))

	for _, r := range results {
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
		}
		if r.Tool.Version != "" {
			constraint = fmt.Sprintf("(%s)", r.Tool.Version)
		}
		fmt.Fprintf(os.Stderr, "  %s %-20s %-12s %s\n", icon, r.Tool.Name, ver, constraint)
	}

	fmt.Fprintf(os.Stderr, "\nResult: %d OK, %d missing, %d outdated\n", ok, missing, outdated)

	if !teamfile.AllSatisfied(results) {
		return fmt.Errorf("%d tool(s) missing or outdated", missing+outdated)
	}
	fmt.Fprintln(os.Stderr, "All requirements satisfied!")
	return nil
}

func printCheckJSON(tf *teamfile.TeamFile, path string, results []teamfile.CheckResult, ok, missing, outdated int) error {
	out := jsonCheckOutput{
		Project:      tf.Name,
		File:         path,
		AllSatisfied: teamfile.AllSatisfied(results),
	}
	out.Summary.OK = ok
	out.Summary.Missing = missing
	out.Summary.Outdated = outdated

	for _, r := range results {
		status := "ok"
		switch r.Status {
		case teamfile.StatusMissing:
			status = "missing"
		case teamfile.StatusOutdated:
			status = "outdated"
		}
		out.Results = append(out.Results, jsonCheckResult{
			Name:      r.Tool.Name,
			Required:  r.Tool.Version,
			Installed: r.Version,
			Status:    status,
			Message:   r.Message,
		})
	}

	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))

	if !out.AllSatisfied {
		return fmt.Errorf("%d tool(s) missing or outdated", missing+outdated)
	}
	return nil
}
