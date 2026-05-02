package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// OutputFormat names the canonical output formats supported by commands.
//
// AGENTS.md ("CLI Standards"): commands should expose `--output` for
// machine-readable output. clim adopts `--output={text,json,yaml}` as the
// canonical flag and keeps `--json` as a deprecated alias for backward
// compatibility.
type OutputFormat string

// Output formats accepted by the canonical --output flag.
const (
	OutputText OutputFormat = "text"
	OutputJSON OutputFormat = "json"
	OutputYAML OutputFormat = "yaml"
)

// outputFlagState holds the raw flag values for a single command.
type outputFlagState struct {
	output string
	json   bool // deprecated alias
}

// addOutputFlag attaches the canonical --output flag to cmd and (when
// OutputJSON is in supported) the deprecated --json alias. The returned
// getter resolves the active format at run time.
//
// Default format is OutputText. If the user passes both --json and
// --output=text, --json wins (the deprecated alias is treated as an
// explicit opt-in to JSON).
func addOutputFlag(cmd *cobra.Command, supported ...OutputFormat) func() OutputFormat {
	if len(supported) == 0 {
		supported = []OutputFormat{OutputText, OutputJSON}
	}
	state := &outputFlagState{output: string(OutputText)}

	names := make([]string, len(supported))
	for i, s := range supported {
		names[i] = string(s)
	}
	cmd.Flags().StringVar(&state.output, "output", string(OutputText),
		"output format: "+strings.Join(names, "|"))

	if containsFormat(supported, OutputJSON) {
		cmd.Flags().BoolVar(&state.json, "json", false, "output results as JSON (deprecated)")
		_ = cmd.Flags().MarkDeprecated("json", "use --output=json instead")
	}

	return func() OutputFormat {
		if state.json {
			return OutputJSON
		}
		switch strings.ToLower(strings.TrimSpace(state.output)) {
		case "json":
			return OutputJSON
		case "yaml", "yml":
			return OutputYAML
		case "", "text", "human":
			return OutputText
		}
		// Unknown value — fall back to text rather than erroring late.
		return OutputText
	}
}

func containsFormat(list []OutputFormat, want OutputFormat) bool {
	for _, f := range list {
		if f == want {
			return true
		}
	}
	return false
}

// printJSON marshals v to stdout as indented JSON, with a trailing newline.
// Errors during marshalling are returned wrapped.
func printJSON(v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding JSON: %w", err)
	}
	if _, err := fmt.Fprintln(os.Stdout, string(b)); err != nil {
		return fmt.Errorf("writing JSON: %w", err)
	}
	return nil
}

// printYAML marshals v to stdout as YAML.
//
//nolint:unused // Reserved for future commands that adopt --output=yaml.
func printYAML(v any) error {
	b, err := yaml.Marshal(v)
	if err != nil {
		return fmt.Errorf("encoding YAML: %w", err)
	}
	if _, err := os.Stdout.WriteString(string(b)); err != nil {
		return fmt.Errorf("writing YAML: %w", err)
	}
	return nil
}
