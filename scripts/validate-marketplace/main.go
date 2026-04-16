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
	"bytes"
	"errors"
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

	var errs []string

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
		errs = append(errs, validateToolFile(f, toolNames, allowedCategories, allowedTags)...)
	}

	// --- Validate packs ---
	packFiles, err := filepath.Glob(filepath.Join(*dir, "packs", "*.yaml"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: globbing pack files: %v\n", err)
		os.Exit(1)
	}

	packNames := make(map[string]string) // name → file path
	for _, f := range packFiles {
		errs = append(errs, validatePackFile(f, packNames, toolNames)...)
	}

	// --- Report ---
	if len(errs) > 0 {
		fmt.Fprintf(os.Stderr, "Marketplace validation failed with %d error(s):\n\n", len(errs))
		for _, e := range errs {
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
		return nil, errors.New("categories.yaml has no categories")
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
		return nil, errors.New("tags.yaml has no tags")
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
	var errs []string
	rel := filepath.Base(path)

	data, err := os.ReadFile(path)
	if err != nil {
		return []string{rel + ": cannot read file: " + err.Error()}
	}

	var tool toolDef
	if err := yaml.Unmarshal(data, &tool); err != nil {
		return []string{rel + ": invalid YAML: " + err.Error()}
	}

	// Check for unknown fields by re-parsing with strict mode.
	var strict toolDef
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&strict); err != nil {
		errs = append(errs, rel+": unknown field(s): "+err.Error())
	}

	// Required fields.
	if tool.Name == "" {
		errs = append(errs, rel+": missing required field 'name'")
	}
	if tool.DisplayName == "" {
		errs = append(errs, rel+": missing required field 'display_name'")
	}
	if tool.Category == "" {
		errs = append(errs, rel+": missing required field 'category'")
	}
	if len(tool.BinaryNames) == 0 {
		errs = append(errs, rel+": missing required field 'binary_names' (must have at least one)")
	}

	// Name format.
	if tool.Name != "" && !validName.MatchString(tool.Name) {
		errs = append(errs, fmt.Sprintf("%s: name %q is invalid (must be lowercase alphanumeric + hyphens)", rel, tool.Name))
	}

	// Filename must match name.
	if tool.Name != "" && rel != tool.Name+".yaml" {
		errs = append(errs, fmt.Sprintf("%s: filename must match name field (expected %s.yaml)", rel, tool.Name))
	}

	// Category must be from allowed set.
	if tool.Category != "" && !allowedCategories[tool.Category] {
		cats := sortedKeys(allowedCategories)
		errs = append(errs, fmt.Sprintf("%s: invalid category %q (allowed: %s)", rel, tool.Category, strings.Join(cats, ", ")))
	}

	// Tags must be from allowed set, no duplicates within a tool.
	seenTags := make(map[string]bool, len(tool.Tags))
	for _, tag := range tool.Tags {
		if !allowedTags[tag] {
			errs = append(errs, fmt.Sprintf("%s: unknown tag %q (add it to tags.yaml first)", rel, tag))
		}
		if seenTags[tag] {
			errs = append(errs, fmt.Sprintf("%s: duplicate tag %q", rel, tag))
		}
		seenTags[tag] = true
	}

	// Must have at least one package manager.
	if !hasAnyPackage(tool.Packages) {
		errs = append(errs, rel+": must define at least one package manager in 'packages'")
	}

	// Duplicate detection.
	if tool.Name != "" {
		if prev, exists := seen[tool.Name]; exists {
			errs = append(errs, fmt.Sprintf("%s: duplicate tool name %q (also in %s)", rel, tool.Name, filepath.Base(prev)))
		} else {
			seen[tool.Name] = path
		}
	}

	return errs
}

func validatePackFile(path string, seenPacks, toolNames map[string]string) []string {
	var errs []string
	rel := filepath.Base(path)

	data, err := os.ReadFile(path)
	if err != nil {
		return []string{rel + ": cannot read file: " + err.Error()}
	}

	var pack packDef
	if err := yaml.Unmarshal(data, &pack); err != nil {
		return []string{rel + ": invalid YAML: " + err.Error()}
	}

	// Check for unknown fields.
	var strict packDef
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&strict); err != nil {
		errs = append(errs, rel+": unknown field(s): "+err.Error())
	}

	// Required fields.
	if pack.Name == "" {
		errs = append(errs, rel+": missing required field 'name'")
	}
	if pack.DisplayName == "" {
		errs = append(errs, rel+": missing required field 'display_name'")
	}
	if len(pack.Tools) == 0 {
		errs = append(errs, rel+": missing required field 'tools' (must have at least one)")
	}

	// Name format.
	if pack.Name != "" && !validName.MatchString(pack.Name) {
		errs = append(errs, fmt.Sprintf("%s: name %q is invalid (must be lowercase alphanumeric + hyphens)", rel, pack.Name))
	}

	// Filename must match name.
	if pack.Name != "" && rel != pack.Name+".yaml" {
		errs = append(errs, fmt.Sprintf("%s: filename must match name field (expected %s.yaml)", rel, pack.Name))
	}

	// All referenced tools must exist, no duplicates within a pack.
	seenTools := make(map[string]bool, len(pack.Tools))
	for _, toolName := range pack.Tools {
		if _, exists := toolNames[toolName]; !exists {
			errs = append(errs, fmt.Sprintf("%s: references unknown tool %q", rel, toolName))
		}
		if seenTools[toolName] {
			errs = append(errs, fmt.Sprintf("%s: duplicate tool reference %q", rel, toolName))
		}
		seenTools[toolName] = true
	}

	// Duplicate detection.
	if pack.Name != "" {
		if prev, exists := seenPacks[pack.Name]; exists {
			errs = append(errs, fmt.Sprintf("%s: duplicate pack name %q (also in %s)", rel, pack.Name, filepath.Base(prev)))
		} else {
			seenPacks[pack.Name] = path
		}
	}

	return errs
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
