package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
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
// addOutputFlag registers the canonical `--output {text,json,yaml}`
// flag on cmd plus the deprecated `--json` alias when JSON is a
// supported format. The flag is registered on cmd.Flags() (i.e.
// NOT persistent) so each command opts in explicitly; for parent
// commands whose subcommands also need access to the flag, use
// addPersistentOutputFlag below.
//
// Default format is OutputText. If the user passes both --json and
// --output=text, --json wins (the deprecated alias is treated as an
// explicit opt-in to JSON).
func addOutputFlag(cmd *cobra.Command, supported ...OutputFormat) func() (OutputFormat, error) {
	return addOutputFlagOn(cmd.Flags(), cmd, supported...)
}

// addPersistentOutputFlag is the variant for parent commands whose
// subcommands need to see the same --output flag. It registers the
// flag on cmd.PersistentFlags() so `klim parent sub --output=json`
// works as users expect (vs. requiring `klim parent --output=json sub`).
func addPersistentOutputFlag(cmd *cobra.Command, supported ...OutputFormat) func() (OutputFormat, error) {
	return addOutputFlagOn(cmd.PersistentFlags(), cmd, supported...)
}

func addOutputFlagOn(flags *pflag.FlagSet, cmd *cobra.Command, supported ...OutputFormat) func() (OutputFormat, error) {
	if len(supported) == 0 {
		supported = []OutputFormat{OutputText, OutputJSON}
	}
	state := &outputFlagState{output: string(OutputText)}

	names := make([]string, len(supported))
	for i, s := range supported {
		names[i] = string(s)
	}
	supportedList := strings.Join(names, "|")
	flags.StringVar(&state.output, "output", string(OutputText),
		"output format: "+supportedList)

	if containsFormat(supported, OutputJSON) {
		flags.BoolVar(&state.json, "json", false, "output results as JSON (deprecated)")
		_ = flags.MarkDeprecated("json", "use --output=json instead")
	}
	_ = cmd // retained for future hooks (e.g. cmd-scoped logging)

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
// Routing path: marshal to JSON first, decode into a generic
// interface{}, then YAML-marshal that. This is intentional — many
// structured-output reports in klim only carry `json:` tags, and
// yaml.v3 doesn't honour those (it lowercases the field name and
// ignores `,omitempty`). Round-tripping through JSON preserves the
// exact JSON schema (key names, omitempty, custom MarshalJSON
// implementations), so `klim ... --output yaml` produces a YAML
// document with the same keys a JSON consumer would see.
func printYAML(v any) error {
	jsonBytes, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("encoding YAML: %w", err)
	}
	var generic any
	if err := json.Unmarshal(jsonBytes, &generic); err != nil {
		return fmt.Errorf("encoding YAML (decode): %w", err)
	}
	b, err := yaml.Marshal(generic)
	if err != nil {
		return fmt.Errorf("encoding YAML: %w", err)
	}
	if _, err := os.Stdout.WriteString(string(b)); err != nil {
		return fmt.Errorf("writing YAML: %w", err)
	}
	return nil
}

// printStructured dispatches to printJSON or printYAML based on the
// caller's resolved OutputFormat. Calling with OutputText (or any
// non-structured format) is a programming error: this helper returns
// an error in that case so the misuse is visible but doesn't crash
// the process.
func printStructured(format OutputFormat, v any) error {
	switch format {
	case OutputJSON:
		return printJSON(v)
	case OutputYAML:
		return printYAML(v)
	default:
		return fmt.Errorf("printStructured: unsupported format %q", format)
	}
}
