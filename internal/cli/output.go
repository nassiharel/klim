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
// machine-readable output. klim adopts `--output={text,json,yaml}` as the
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
// getter resolves the active format at run time and validates that the
// requested value is one this command supports — unsupported or unknown
// values are returned as a *UsageError so they map to ExitUsage (2).
//
// Default format is OutputText. If the user passes both --json and
// --output=text, --json wins (the deprecated alias is treated as an
// explicit opt-in to JSON).
func addOutputFlag(cmd *cobra.Command, supported ...OutputFormat) func() (OutputFormat, error) {
	if len(supported) == 0 {
		supported = []OutputFormat{OutputText, OutputJSON}
	}
	state := &outputFlagState{output: string(OutputText)}

	names := make([]string, len(supported))
	for i, s := range supported {
		names[i] = string(s)
	}
	supportedList := strings.Join(names, "|")
	cmd.Flags().StringVar(&state.output, "output", string(OutputText),
		"output format: "+supportedList)

	if containsFormat(supported, OutputJSON) {
		cmd.Flags().BoolVar(&state.json, "json", false, "output results as JSON (deprecated)")
		_ = cmd.Flags().MarkDeprecated("json", "use --output=json instead")
	}

	return func() (OutputFormat, error) {
		if state.json {
			// --json is gated by `containsFormat(supported, OutputJSON)` above.
			return OutputJSON, nil
		}
		raw := strings.ToLower(strings.TrimSpace(state.output))
		var resolved OutputFormat
		switch raw {
		case "", "text", "human":
			resolved = OutputText
		case "json":
			resolved = OutputJSON
		case "yaml", "yml":
			resolved = OutputYAML
		default:
			return "", &UsageError{Err: fmt.Errorf("invalid --output %q (supported: %s)", state.output, supportedList)}
		}
		if !containsFormat(supported, resolved) {
			return "", &UsageError{Err: fmt.Errorf("--output=%s is not supported by this command (supported: %s)", resolved, supportedList)}
		}
		return resolved, nil
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
