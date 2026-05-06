// assemble-marketplace reads individual tool and pack YAML files from
// marketplace/tools/ and marketplace/packs/, and assembles them into a
// single marketplace.yaml matching the format the CLI expects.
//
// Usage:
//
//	go run ./internal/marketplace/assemble
//	go run ./internal/marketplace/assemble -o marketplace.yaml
//	go run ./internal/marketplace/assemble -dir ./marketplace
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/nassiharel/klim/internal/registry"
)

type assembledFile struct {
	Tools []registry.ToolDef `yaml:"tools"`
	Packs []packDef          `yaml:"packs"`
}

type packDef struct {
	Name        string   `yaml:"name"`
	DisplayName string   `yaml:"display_name"`
	Description string   `yaml:"description,omitempty"`
	Tools       []string `yaml:"tools"`
}

type categoriesFile struct {
	Categories []string `yaml:"categories"`
}

func main() {
	dir := flag.String("dir", "marketplace", "path to the marketplace directory")
	output := flag.String("o", "", "output file (default: stdout)")
	fetchGitHub := flag.Bool("fetch-github", false, "fetch repository metadata (stars, description, etc.) from the GitHub API for tools with a `github` field")
	ghConcurrency := flag.Int("github-concurrency", 4, "maximum concurrent GitHub API requests when -fetch-github is set")
	ghTimeout := flag.Duration("github-timeout", 5*time.Minute, "overall timeout for GitHub enrichment when -fetch-github is set")
	ghStrict := flag.Bool("github-strict", false, "fail the build if any GitHub API lookup fails (default: warn and continue)")
	fallbackFile := flag.String("fallback", "", "path to a previously assembled marketplace.yaml; github_info from it is used for tools where a fresh fetch fails or -fetch-github is not set (required)")
	flag.Parse()

	if *fallbackFile == "" {
		fatal("-fallback is required; pass the path to the previous marketplace.yaml (use /dev/null or NUL if none exists)")
	}

	categoryOrder, err := loadCategoryOrder(filepath.Join(*dir, "categories.yaml"))
	if err != nil {
		fatal("%v", err)
	}

	// --- Load tools ---
	toolFiles, err := filepath.Glob(filepath.Join(*dir, "tools", "*.yaml"))
	if err != nil {
		fatal("globbing tool files: %v", err)
	}
	if len(toolFiles) == 0 {
		fatal("no tool files found in %s/tools/", *dir)
	}

	var tools []registry.ToolDef
	for _, f := range toolFiles {
		data, err := os.ReadFile(f)
		if err != nil {
			fatal("reading %s: %v", f, err)
		}
		var t registry.ToolDef
		if err := yaml.Unmarshal(data, &t); err != nil {
			fatal("parsing %s: %v", f, err)
		}
		tools = append(tools, t)
	}

	sort.SliceStable(tools, func(i, j int) bool {
		ci := categoryRank(categoryOrder, tools[i].Category)
		cj := categoryRank(categoryOrder, tools[j].Category)
		if ci != cj {
			return ci < cj
		}
		return strings.ToLower(tools[i].Name) < strings.ToLower(tools[j].Name)
	})

	// --- Load packs ---
	packFiles, err := filepath.Glob(filepath.Join(*dir, "packs", "*.yaml"))
	if err != nil {
		fatal("globbing pack files: %v", err)
	}

	var packs []packDef
	for _, f := range packFiles {
		data, err := os.ReadFile(f)
		if err != nil {
			fatal("reading %s: %v", f, err)
		}
		var p packDef
		if err := yaml.Unmarshal(data, &p); err != nil {
			fatal("parsing %s: %v", f, err)
		}
		packs = append(packs, p)
	}

	sort.SliceStable(packs, func(i, j int) bool {
		return strings.ToLower(packs[i].Name) < strings.ToLower(packs[j].Name)
	})

	// --- Optionally enrich with GitHub metadata ---
	// Load fallback github_info from a previously assembled file (e.g. from
	// the marketplace branch). This is used for tools where a fresh fetch
	// fails or -fetch-github is not set.
	fallbackInfo, err := loadFallbackGitHubInfo(*fallbackFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: loading fallback file: %v\n", err)
	} else if len(fallbackInfo) > 0 {
		fmt.Fprintf(os.Stderr, "Loaded fallback github_info for %d tools from %s\n", len(fallbackInfo), *fallbackFile)
	}

	if *fetchGitHub {
		fetcher := newGitHubFetcher()
		if fetcher.token == "" {
			fmt.Fprintln(os.Stderr, "note: GITHUB_TOKEN is not set; GitHub API requests will use the 60/hour unauthenticated limit")
		}
		ctx, cancel := context.WithTimeout(context.Background(), *ghTimeout)
		defer cancel()
		if err := enrichWithGitHub(ctx, tools, fetcher, *ghConcurrency, *ghStrict); err != nil {
			fatal("github enrichment: %v", err)
		}
	}

	// Apply fallback: for any tool that still has no useful GitHubInfo, use
	// the previously cached data so we never regress from "has metadata" to
	// "no metadata" just because a fetch failed or was skipped.
	if len(fallbackInfo) > 0 {
		applied := 0
		for i := range tools {
			if !tools[i].GitHubInfo.IsUseful() {
				if fb, ok := fallbackInfo[tools[i].Name]; ok {
					tools[i].GitHubInfo = fb
					applied++
				}
			}
		}
		if applied > 0 {
			fmt.Fprintf(os.Stderr, "Applied fallback github_info to %d tools\n", applied)
		}
	}

	// --- Assemble ---
	assembled := assembledFile{Tools: tools, Packs: packs}

	data, err := yaml.Marshal(&assembled)
	if err != nil {
		fatal("marshaling assembled marketplace: %v", err)
	}

	header := fmt.Sprintf("# klim — Tool Marketplace\n# Auto-generated from %s/tools/ and %s/packs/.\n# Do not edit this file directly — edit individual files instead.\n\n", *dir, *dir)
	content := []byte(header + string(data))

	if *output != "" {
		if err := os.WriteFile(*output, content, 0o644); err != nil {
			fatal("writing %s: %v", *output, err)
		}
		fmt.Fprintf(os.Stderr, "Assembled %d tools + %d packs → %s\n", len(tools), len(packs), *output)
	} else {
		if _, err := os.Stdout.Write(content); err != nil {
			fatal("writing to stdout: %v", err)
		}
	}
}

func loadCategoryOrder(path string) (map[string]int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading categories.yaml: %w", err)
	}
	var cf categoriesFile
	if err := yaml.Unmarshal(data, &cf); err != nil {
		return nil, fmt.Errorf("parsing categories.yaml: %w", err)
	}
	if len(cf.Categories) == 0 {
		return nil, errors.New("categories.yaml has no categories")
	}
	m := make(map[string]int, len(cf.Categories))
	for i, c := range cf.Categories {
		m[c] = i
	}
	return m, nil
}

func categoryRank(order map[string]int, cat string) int {
	if rank, ok := order[cat]; ok {
		return rank
	}
	return 999
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "error: "+format+"\n", args...)
	os.Exit(1)
}
