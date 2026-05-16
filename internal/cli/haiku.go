package cli

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/nassiharel/klim/internal/haiku"
	"github.com/nassiharel/klim/internal/registry"
)

var (
	haikuSeed     int64
	haikuOutputFn func() (OutputFormat, error)
)

var haikuCmd = &cobra.Command{
	Use:   "haiku <tool>",
	Short: "Generate a 5-7-5 haiku for a tool",
	Long: `Produce a small 5-7-5 syllable poem about a tool, derived
locally from the tool's catalog metadata (name, category, description,
tags). The output is deterministic — running the command twice gives
the same haiku — unless you pass --seed for variety.

This command uses klim's normal catalog loader: a stale or missing
catalog cache may trigger a refresh from GitHub the same way every
other klim command does. There is no agent and no LLM in the loop —
generation is pure template + syllable counting.

Examples:
  klim haiku terraform
  klim haiku kubectl --seed 42
  klim haiku git --output json`,
	Args: requireArgs(1, "klim haiku <tool>"),
	RunE: runHaiku,
}

func init() {
	haikuCmd.Flags().Int64Var(&haikuSeed, "seed", 0, "override the deterministic seed for variety")
	haikuOutputFn = addOutputFlag(haikuCmd, OutputText, OutputJSON, OutputYAML)
	// Registered in root.go.
}

// haikuReport is the structured shape for --output json|yaml.
// Seed always reports the resolved int64 used to generate Lines
// (NOT the user-supplied --seed override), so a downstream consumer
// can reproduce the exact same haiku by passing it back.
//
// SeedString is the same value rendered as a decimal string. JSON
// numbers larger than 2^53 lose precision in JavaScript consumers,
// and FNV-derived seeds routinely sit in the 2^62 range, so we
// include both: number form for the typed case, string form for
// JS-safe round-tripping.
type haikuReport struct {
	Tool       string    `json:"tool" yaml:"tool"`
	Seed       int64     `json:"seed" yaml:"seed"`
	SeedString string    `json:"seed_string" yaml:"seed_string"`
	Lines      [3]string `json:"lines" yaml:"lines"`
}

func runHaiku(cmd *cobra.Command, args []string) error {
	out, err := haikuOutputFn()
	if err != nil {
		return err
	}
	name := args[0]

	svc := svcFrom(cmd)
	tools, _, err := svc.Catalog.LoadTools(cmd.Context())
	if err != nil {
		return fmt.Errorf("klim haiku: %w", err)
	}
	tool, ok := registry.ToolMap(tools)[name]
	if !ok {
		// Even unknown tools should get *some* haiku — but a typo is
		// more likely than a deliberate ask for an unknown name, so
		// we surface a "did-you-mean" hint rather than guessing.
		return notFoundError(name, closestToolName(tools, name))
	}

	h := haiku.Generate(haiku.Tool{
		Name:        tool.Name,
		DisplayName: tool.DisplayName,
		Category:    tool.Category,
		Tags:        append([]string(nil), tool.Tags...),
		Description: toolHaikuDescription(tool),
	}, haiku.Options{Seed: haikuSeed})

	switch out {
	case OutputJSON:
		return printJSON(haikuReport{Tool: tool.Name, Seed: h.Seed, SeedString: strconv.FormatInt(h.Seed, 10), Lines: h.Lines})
	case OutputYAML:
		return printYAML(haikuReport{Tool: tool.Name, Seed: h.Seed, SeedString: strconv.FormatInt(h.Seed, 10), Lines: h.Lines})
	}

	fmt.Println(h.String())
	return nil
}

// toolHaikuDescription gathers description text from anywhere we can
// find it on the tool — GitHubInfo.Description takes precedence
// because catalog descriptions are often a single noun.
func toolHaikuDescription(t *registry.Tool) string {
	if t == nil {
		return ""
	}
	if t.GitHubInfo != nil && t.GitHubInfo.Description != "" {
		return t.GitHubInfo.Description
	}
	return ""
}
