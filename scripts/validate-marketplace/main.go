// validate-marketplace validates individual tool and pack YAML files
// in the marketplace/ directory. It checks schema correctness, naming
// conventions, cross-references, and uniqueness.
//
// Usage:
//
//	go run ./scripts/validate-marketplace
//	go run ./scripts/validate-marketplace -dir ./marketplace
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// toolDef mirrors the YAML structure for a single tool file.
type toolDef struct {
	Name        string     `yaml:"name"`
	DisplayName string     `yaml:"display_name"`
	Category    string     `yaml:"category"`
	Tags        []string   `yaml:"tags,omitempty"`
	BinaryNames []string   `yaml:"binary_names"`
	Packages    packageDef `yaml:"packages"`
}

type packageDef struct {
	Winget string `yaml:"winget,omitempty"`
	Choco  string `yaml:"choco,omitempty"`
	Brew   string `yaml:"brew,omitempty"`
	Apt    string `yaml:"apt,omitempty"`
	Snap   string `yaml:"snap,omitempty"`
	NPM    string `yaml:"npm,omitempty"`
}

// packDef mirrors the YAML structure for a single pack file.
type packDef struct {
	Name        string   `yaml:"name"`
	DisplayName string   `yaml:"display_name"`
	Description string   `yaml:"description,omitempty"`
	Tools       []string `yaml:"tools"`
}

// categoriesFile mirrors marketplace/categories.yaml.
type categoriesFile struct {
	Categories []string `yaml:"categories"`
}

// tagsFile mirrors marketplace/tags.yaml.
type tagsFile struct {
	Tags []string `yaml:"tags"`
}

// validName matches lowercase alphanumeric + hyphens (e.g., "docker-compose", "k9s").
var validName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

func main() {
	dir := flag.String("dir", "marketplace", "path to the marketplace directory")
	flag.Parse()

	// Load allowed categories from categories.yaml.
	allowedCategories, err := loadCategories(filepath.Join(*dir, "categories.yaml"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	// Load allowed tags from tags.yaml.
	allowedTags, err := loadTags(filepath.Join(*dir, "tags.yaml"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	var errors []string

	// --- Validate tools ---
	toolFiles, err := filepath.Glob(filepath.Join(*dir, "tools", "*.yaml"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: globbing tool files: %v\n", err)
		os.Exit(1)
	}

	if len(toolFiles) == 0 {
		fmt.Fprintf(os.Stderr, "error: no tool files found in %s/tools/\n", *dir)
		os.Exit(1)
	}

	toolNames := make(map[string]string) // name → file path (for duplicate detection)
	for _, f := range toolFiles {
		errs := validateToolFile(f, toolNames, allowedCategories, allowedTags)
		errors = append(errors, errs...)
	}

	// --- Validate packs ---
	packFiles, err := filepath.Glob(filepath.Join(*dir, "packs", "*.yaml"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: globbing pack files: %v\n", err)
		os.Exit(1)
	}

	packNames := make(map[string]string) // name → file path
	for _, f := range packFiles {
		errs := validatePackFile(f, packNames, toolNames)
		errors = append(errors, errs...)
	}

	// --- Report ---
	if len(errors) > 0 {
		fmt.Fprintf(os.Stderr, "Marketplace validation failed with %d error(s):\n\n", len(errors))
		for _, e := range errors {
			fmt.Fprintf(os.Stderr, "  - %s\n", e)
		}
		fmt.Fprintln(os.Stderr)
		os.Exit(1)
	}

	fmt.Printf("Marketplace validated: %d tools, %d packs. All checks passed.\n",
		len(toolFiles), len(packFiles))
}

// loadCategories reads categories.yaml and returns a set of allowed category names.
func loadCategories(path string) (map[string]bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading categories.yaml: %w", err)
	}
	var cf categoriesFile
	if err := yaml.Unmarshal(data, &cf); err != nil {
		return nil, fmt.Errorf("parsing categories.yaml: %w", err)
	}
	if len(cf.Categories) == 0 {
		return nil, fmt.Errorf("categories.yaml has no categories")
	}
	m := make(map[string]bool, len(cf.Categories))
	for _, c := range cf.Categories {
		if m[c] {
			return nil, fmt.Errorf("categories.yaml: duplicate category %q", c)
		}
		m[c] = true
	}
	return m, nil
}

// loadTags reads tags.yaml and returns a set of allowed tag names.
func loadTags(path string) (map[string]bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading tags.yaml: %w", err)
	}
	var tf tagsFile
	if err := yaml.Unmarshal(data, &tf); err != nil {
		return nil, fmt.Errorf("parsing tags.yaml: %w", err)
	}
	if len(tf.Tags) == 0 {
		return nil, fmt.Errorf("tags.yaml has no tags")
	}
	m := make(map[string]bool, len(tf.Tags))
	for _, t := range tf.Tags {
		if m[t] {
			return nil, fmt.Errorf("tags.yaml: duplicate tag %q", t)
		}
		m[t] = true
	}
	return m, nil
}

func validateToolFile(path string, seen map[string]string, allowedCategories, allowedTags map[string]bool) []string {
	var errors []string
	rel := filepath.Base(path)

	data, err := os.ReadFile(path)
	if err != nil {
		return []string{fmt.Sprintf("%s: cannot read file: %v", rel, err)}
	}

	var tool toolDef
	if err := yaml.Unmarshal(data, &tool); err != nil {
		return []string{fmt.Sprintf("%s: invalid YAML: %v", rel, err)}
	}

	// Check for unknown fields by re-parsing with strict mode.
	var strict toolDef
	dec := yaml.NewDecoder(strings.NewReader(string(data)))
	dec.KnownFields(true)
	if err := dec.Decode(&strict); err != nil {
		errors = append(errors, fmt.Sprintf("%s: unknown field(s): %v", rel, err))
	}

	// Required fields.
	if tool.Name == "" {
		errors = append(errors, fmt.Sprintf("%s: missing required field 'name'", rel))
	}
	if tool.DisplayName == "" {
		errors = append(errors, fmt.Sprintf("%s: missing required field 'display_name'", rel))
	}
	if tool.Category == "" {
		errors = append(errors, fmt.Sprintf("%s: missing required field 'category'", rel))
	}
	if len(tool.BinaryNames) == 0 {
		errors = append(errors, fmt.Sprintf("%s: missing required field 'binary_names' (must have at least one)", rel))
	}

	// Name format.
	if tool.Name != "" && !validName.MatchString(tool.Name) {
		errors = append(errors, fmt.Sprintf("%s: name %q is invalid (must be lowercase alphanumeric + hyphens)", rel, tool.Name))
	}

	// Filename must match name.
	expectedFilename := tool.Name + ".yaml"
	if tool.Name != "" && rel != expectedFilename {
		errors = append(errors, fmt.Sprintf("%s: filename must match name field (expected %s)", rel, expectedFilename))
	}

	// Category must be from allowed set.
	if tool.Category != "" && !allowedCategories[tool.Category] {
		cats := sortedKeys(allowedCategories)
		errors = append(errors, fmt.Sprintf("%s: invalid category %q (allowed: %s)", rel, tool.Category, strings.Join(cats, ", ")))
	}

	// Tags must be from allowed set, no duplicates within a tool.
	seenTags := make(map[string]bool, len(tool.Tags))
	for _, tag := range tool.Tags {
		if !allowedTags[tag] {
			errors = append(errors, fmt.Sprintf("%s: unknown tag %q (add it to tags.yaml first)", rel, tag))
		}
		if seenTags[tag] {
			errors = append(errors, fmt.Sprintf("%s: duplicate tag %q", rel, tag))
		}
		seenTags[tag] = true
	}

	// Must have at least one package manager.
	if !hasAnyPackage(tool.Packages) {
		errors = append(errors, fmt.Sprintf("%s: must define at least one package manager in 'packages'", rel))
	}

	// Duplicate detection.
	if tool.Name != "" {
		if prev, exists := seen[tool.Name]; exists {
			errors = append(errors, fmt.Sprintf("%s: duplicate tool name %q (also in %s)", rel, tool.Name, filepath.Base(prev)))
		} else {
			seen[tool.Name] = path
		}
	}

	return errors
}

func validatePackFile(path string, seenPacks, toolNames map[string]string) []string {
	var errors []string
	rel := filepath.Base(path)

	data, err := os.ReadFile(path)
	if err != nil {
		return []string{fmt.Sprintf("%s: cannot read file: %v", rel, err)}
	}

	var pack packDef
	if err := yaml.Unmarshal(data, &pack); err != nil {
		return []string{fmt.Sprintf("%s: invalid YAML: %v", rel, err)}
	}

	// Check for unknown fields.
	var strict packDef
	dec := yaml.NewDecoder(strings.NewReader(string(data)))
	dec.KnownFields(true)
	if err := dec.Decode(&strict); err != nil {
		errors = append(errors, fmt.Sprintf("%s: unknown field(s): %v", rel, err))
	}

	// Required fields.
	if pack.Name == "" {
		errors = append(errors, fmt.Sprintf("%s: missing required field 'name'", rel))
	}
	if pack.DisplayName == "" {
		errors = append(errors, fmt.Sprintf("%s: missing required field 'display_name'", rel))
	}
	if len(pack.Tools) == 0 {
		errors = append(errors, fmt.Sprintf("%s: missing required field 'tools' (must have at least one)", rel))
	}

	// Name format.
	if pack.Name != "" && !validName.MatchString(pack.Name) {
		errors = append(errors, fmt.Sprintf("%s: name %q is invalid (must be lowercase alphanumeric + hyphens)", rel, pack.Name))
	}

	// Filename must match name.
	expectedFilename := pack.Name + ".yaml"
	if pack.Name != "" && rel != expectedFilename {
		errors = append(errors, fmt.Sprintf("%s: filename must match name field (expected %s)", rel, expectedFilename))
	}

	// All referenced tools must exist.
	for _, toolName := range pack.Tools {
		if _, exists := toolNames[toolName]; !exists {
			errors = append(errors, fmt.Sprintf("%s: references unknown tool %q", rel, toolName))
		}
	}

	// Duplicate detection.
	if pack.Name != "" {
		if prev, exists := seenPacks[pack.Name]; exists {
			errors = append(errors, fmt.Sprintf("%s: duplicate pack name %q (also in %s)", rel, pack.Name, filepath.Base(prev)))
		} else {
			seenPacks[pack.Name] = path
		}
	}

	return errors
}

func hasAnyPackage(p packageDef) bool {
	return p.Winget != "" || p.Choco != "" || p.Brew != "" ||
		p.Apt != "" || p.Snap != "" || p.NPM != ""
}

func sortedKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
