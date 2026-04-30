//go:build ignore

// Generates the marketplace catalog page for the docs site by fetching
// the assembled marketplace.yaml from the published marketplace branch.
// Includes GitHub-enriched metadata (stars, descriptions, licenses).
//
// Run: go run docs/scripts/gen-marketplace-page.go
package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const marketplaceURL = "https://raw.githubusercontent.com/nassiharel/clim/marketplace/marketplace.yaml"

type githubInfo struct {
	Stars       int    `yaml:"stars"`
	Description string `yaml:"description"`
	License     string `yaml:"license"`
}

type toolDef struct {
	Name        string      `yaml:"name"`
	DisplayName string      `yaml:"display_name"`
	Category    string      `yaml:"category"`
	Tags        []string    `yaml:"tags"`
	GitHub      string      `yaml:"github"`
	GitHubInfo  *githubInfo `yaml:"github_info"`
	Packages    struct {
		Winget string `yaml:"winget"`
		Brew   string `yaml:"brew"`
		Apt    string `yaml:"apt"`
		Scoop  string `yaml:"scoop"`
		Choco  string `yaml:"choco"`
		Snap   string `yaml:"snap"`
		NPM    string `yaml:"npm"`
	} `yaml:"packages"`
}

type packDef struct {
	Name        string   `yaml:"name"`
	DisplayName string   `yaml:"display_name"`
	Description string   `yaml:"description"`
	Tools       []string `yaml:"tools"`
}

func main() {
	fmt.Fprintf(os.Stderr, "Fetching marketplace from %s...\n", marketplaceURL)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(marketplaceURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error fetching marketplace: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		fmt.Fprintf(os.Stderr, "marketplace returned %s\n", resp.Status)
		os.Exit(1)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading response: %v\n", err)
		os.Exit(1)
	}

	var mp struct {
		Tools []toolDef `yaml:"tools"`
		Packs []packDef `yaml:"packs"`
	}
	if err := yaml.Unmarshal(data, &mp); err != nil {
		fmt.Fprintf(os.Stderr, "error parsing marketplace: %v\n", err)
		os.Exit(1)
	}

	tools := mp.Tools
	packs := mp.Packs

	for i := range tools {
		if tools[i].DisplayName == "" {
			tools[i].DisplayName = tools[i].Name
		}
	}
	sort.Slice(tools, func(i, j int) bool {
		return strings.ToLower(tools[i].Name) < strings.ToLower(tools[j].Name)
	})
	sort.Slice(packs, func(i, j int) bool {
		return strings.ToLower(packs[i].Name) < strings.ToLower(packs[j].Name)
	})

	// Group by category.
	byCategory := map[string][]toolDef{}
	for _, t := range tools {
		byCategory[t.Category] = append(byCategory[t.Category], t)
	}

	type catEntry struct {
		name  string
		tools []toolDef
	}
	var cats []catEntry
	for k, v := range byCategory {
		cats = append(cats, catEntry{k, v})
	}
	sort.Slice(cats, func(i, j int) bool {
		if len(cats[i].tools) != len(cats[j].tools) {
			return len(cats[i].tools) > len(cats[j].tools)
		}
		return cats[i].name < cats[j].name
	})

	// Platform counts.
	var brewCount, wingetCount, aptCount, scoopCount, chocoCount, snapCount, npmCount int
	for _, t := range tools {
		if t.Packages.Brew != "" {
			brewCount++
		}
		if t.Packages.Winget != "" {
			wingetCount++
		}
		if t.Packages.Apt != "" {
			aptCount++
		}
		if t.Packages.Scoop != "" {
			scoopCount++
		}
		if t.Packages.Choco != "" {
			chocoCount++
		}
		if t.Packages.Snap != "" {
			snapCount++
		}
		if t.Packages.NPM != "" {
			npmCount++
		}
	}

	// Generate page.
	var b strings.Builder

	b.WriteString(fmt.Sprintf(`---
title: Tool Catalog
description: Browse all %d developer tools available in the clim marketplace
---

{/* Auto-generated — do not edit manually */}
{/* Regenerate: go run docs/scripts/gen-marketplace-page.go */}

`, len(tools)))

	b.WriteString(fmt.Sprintf("## 📦 %d Tools · %d Packs · %d Categories\n\n", len(tools), len(packs), len(cats)))

	// Platform coverage.
	b.WriteString("### Platform Coverage\n\n")
	b.WriteString("| Package Manager | Tools |\n")
	b.WriteString("|----------------|-------|\n")
	b.WriteString(fmt.Sprintf("| 🍺 Homebrew | %d |\n", brewCount))
	b.WriteString(fmt.Sprintf("| 📦 winget | %d |\n", wingetCount))
	b.WriteString(fmt.Sprintf("| 🪣 Scoop | %d |\n", scoopCount))
	b.WriteString(fmt.Sprintf("| 🍫 Chocolatey | %d |\n", chocoCount))
	b.WriteString(fmt.Sprintf("| 🐧 apt | %d |\n", aptCount))
	b.WriteString(fmt.Sprintf("| 📌 snap | %d |\n", snapCount))
	b.WriteString(fmt.Sprintf("| 📗 npm | %d |\n", npmCount))
	b.WriteString("\n---\n\n")

	// Packs section.
	b.WriteString("## 🎒 Curated Packs\n\n")
	for _, p := range packs {
		b.WriteString(fmt.Sprintf("### %s\n\n", p.DisplayName))
		b.WriteString(fmt.Sprintf("_%s_\n\n", p.Description))
		b.WriteString("**Tools:** ")
		for i, t := range p.Tools {
			if i > 0 {
				b.WriteString(" · ")
			}
			b.WriteString(fmt.Sprintf("`%s`", t))
		}
		b.WriteString("\n\n")
	}

	b.WriteString("---\n\n")

	// Tools by category.
	b.WriteString("## 🔍 Tools by Category\n\n")

	for _, cat := range cats {
		b.WriteString(fmt.Sprintf("### %s (%d)\n\n", cat.name, len(cat.tools)))

		hasGitHub := false
		for _, t := range cat.tools {
			if t.GitHubInfo != nil && t.GitHubInfo.Stars > 0 {
				hasGitHub = true
				break
			}
		}

		if hasGitHub {
			b.WriteString("| Tool | Description | Platforms | Stars | License |\n")
			b.WriteString("|------|-------------|-----------|-------|--------|\n")
		} else {
			b.WriteString("| Tool | Platforms | GitHub |\n")
			b.WriteString("|------|-----------|--------|\n")
		}

		// Sort by stars within category.
		sorted := make([]toolDef, len(cat.tools))
		copy(sorted, cat.tools)
		sort.Slice(sorted, func(i, j int) bool {
			si, sj := 0, 0
			if sorted[i].GitHubInfo != nil {
				si = sorted[i].GitHubInfo.Stars
			}
			if sorted[j].GitHubInfo != nil {
				sj = sorted[j].GitHubInfo.Stars
			}
			return si > sj
		})

		for _, t := range sorted {
			platforms := platformBadges(t)
			name := t.DisplayName
			if t.GitHub != "" {
				name = fmt.Sprintf("[%s](https://github.com/%s)", t.DisplayName, t.GitHub)
			}

			if hasGitHub {
				desc, stars, license := "", "", ""
				if t.GitHubInfo != nil {
					if t.GitHubInfo.Description != "" {
						desc = truncate(t.GitHubInfo.Description, 60)
					}
					if t.GitHubInfo.Stars > 0 {
						stars = formatStars(t.GitHubInfo.Stars)
					}
					if t.GitHubInfo.License != "" {
						license = t.GitHubInfo.License
					}
				}
				b.WriteString(fmt.Sprintf("| **%s** | %s | %s | %s | %s |\n", name, desc, platforms, stars, license))
			} else {
				github := ""
				if t.GitHub != "" {
					github = t.GitHub
				}
				b.WriteString(fmt.Sprintf("| **%s** | %s | %s |\n", name, platforms, github))
			}
		}
		b.WriteString("\n")
	}

	// Write file.
	outPath := "docs/src/content/docs/marketplace/catalog.mdx"
	if err := os.WriteFile(outPath, []byte(b.String()), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "error writing %s: %v\n", outPath, err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "✓ Generated %s (%d tools, %d packs, %d categories)\n", outPath, len(tools), len(packs), len(cats))
}

func platformBadges(t toolDef) string {
	var badges []string
	if t.Packages.Brew != "" {
		badges = append(badges, "🍺")
	}
	if t.Packages.Winget != "" {
		badges = append(badges, "📦")
	}
	if t.Packages.Apt != "" {
		badges = append(badges, "🐧")
	}
	if t.Packages.Scoop != "" {
		badges = append(badges, "🪣")
	}
	if t.Packages.Choco != "" {
		badges = append(badges, "🍫")
	}
	if t.Packages.Snap != "" {
		badges = append(badges, "📌")
	}
	if t.Packages.NPM != "" {
		badges = append(badges, "📗")
	}
	return strings.Join(badges, " ")
}

func formatStars(n int) string {
	if n >= 1000 {
		return fmt.Sprintf("⭐ %.1fk", float64(n)/1000)
	}
	return fmt.Sprintf("⭐ %d", n)
}

func truncate(s string, max int) string {
	s = strings.ReplaceAll(s, "|", "\\|")
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}
