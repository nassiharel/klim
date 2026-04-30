package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"sort"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/nassiharel/clim/internal/progress"
	"github.com/nassiharel/clim/internal/registry"
	"github.com/nassiharel/clim/internal/service"
)

var auditJSONFlag bool
var auditRefreshFlag bool
var auditSBOMFlag bool

var auditCmd = &cobra.Command{
	Use:   "audit",
	Short: "Audit installed tools for security and compliance issues",
	Long: `Analyze installed tools for security and compliance concerns:

  • Unmanaged installs — tools from unknown/manual sources
  • Archived projects — tools whose upstream repos are archived
  • Stale projects — tools with no upstream activity in 12+ months
  • Missing version info — tools that can't be version-tracked
  • License inventory — report licenses across your toolchain
  • Outdated tools — tools with available updates

Use --sbom to generate a CycloneDX SBOM (Software Bill of Materials).

Exit codes:
  0  No warnings
  1  One or more findings reported`,
	RunE: runAudit,
}

func init() {
	auditCmd.Flags().BoolVar(&auditJSONFlag, "json", false, "Output results as JSON")
	auditCmd.Flags().BoolVar(&auditRefreshFlag, "refresh", false, "Force fresh scan (ignore cache)")
	auditCmd.Flags().BoolVar(&auditSBOMFlag, "sbom", false, "Generate CycloneDX SBOM instead of audit report")
	rootCmd.AddCommand(auditCmd)
}

// auditFinding represents a single audit issue.
type auditFinding struct {
	Severity string `json:"severity"` // "warning", "info"
	Tool     string `json:"tool"`
	Category string `json:"category"`
	Message  string `json:"message"`
}

type auditReport struct {
	Findings []auditFinding `json:"findings"`
	Summary  struct {
		TotalInstalled int            `json:"total_installed"`
		Warnings       int            `json:"warnings"`
		Infos          int            `json:"infos"`
		Licenses       map[string]int `json:"licenses"`
	} `json:"summary"`
}

func runAudit(cmd *cobra.Command, args []string) error {
	sp := progress.New("Scanning installed tools...")
	tools, _, scanInfo, err := svc.LoadAndResolveCached(cmd.Context(), auditRefreshFlag)
	if err != nil {
		sp.Fail(err.Error())
		return err
	}
	if scanInfo != nil && scanInfo.Source == service.ScanSourceCache {
		sp.Done("Loaded from cache")
	} else {
		sp.Done("Tools scanned")
	}

	if auditSBOMFlag {
		return generateSBOM(tools)
	}

	// Collect only installed tools.
	var installed []registry.Tool
	for _, t := range tools {
		if t.IsInstalled() {
			installed = append(installed, t)
		}
	}

	var findings []auditFinding
	licenses := make(map[string]int)

	for _, t := range installed {
		primary := t.PrimaryInstance()
		if primary == nil {
			continue
		}

		// Unmanaged installs.
		if primary.Source == registry.SourceManual {
			findings = append(findings, auditFinding{
				Severity: "warning",
				Tool:     t.Name,
				Category: "Unmanaged",
				Message:  fmt.Sprintf("Installed from unknown source at %s — not tracked by any package manager", primary.Path),
			})
		}

		// Missing version.
		if primary.Version == "" && primary.Source != registry.SourceManual {
			findings = append(findings, auditFinding{
				Severity: "warning",
				Tool:     t.Name,
				Category: "No Version",
				Message:  "Version could not be determined — cannot verify security status",
			})
		}

		// Archived project.
		if t.GitHubInfo != nil && t.GitHubInfo.Archived {
			findings = append(findings, auditFinding{
				Severity: "warning",
				Tool:     t.Name,
				Category: "Archived",
				Message:  "Upstream repository is archived — no longer receiving security updates",
			})
		}

		// Stale project (no push in 12+ months).
		if t.GitHubInfo != nil && t.GitHubInfo.PushedAt != "" {
			if pushed, err := time.Parse(time.RFC3339, t.GitHubInfo.PushedAt); err == nil {
				age := time.Since(pushed)
				if age > 365*24*time.Hour {
					months := int(age.Hours() / 24 / 30)
					findings = append(findings, auditFinding{
						Severity: "info",
						Tool:     t.Name,
						Category: "Stale",
						Message:  fmt.Sprintf("Last upstream activity was %d months ago", months),
					})
				}
			}
		}

		// Outdated.
		if t.HasUpdate() {
			findings = append(findings, auditFinding{
				Severity: "info",
				Tool:     t.Name,
				Category: "Outdated",
				Message:  fmt.Sprintf("Update available: %s → %s", primary.Version, t.Latest),
			})
		}

		// Collect license.
		if t.GitHubInfo != nil && t.GitHubInfo.License != "" {
			licenses[t.GitHubInfo.License]++
		} else {
			licenses["Unknown"]++
		}
	}

	// Sort findings by severity then tool name.
	sort.Slice(findings, func(i, j int) bool {
		if findings[i].Severity != findings[j].Severity {
			return findings[i].Severity < findings[j].Severity // "info" < "warning"
		}
		return findings[i].Tool < findings[j].Tool
	})

	var warnings, infos int
	for _, f := range findings {
		switch f.Severity {
		case "warning":
			warnings++
		case "info":
			infos++
		}
	}

	if auditJSONFlag {
		return printAuditJSON(findings, len(installed), warnings, infos, licenses)
	}

	// Human output.
	fmt.Fprintf(os.Stderr, "\nAuditing %d installed tools\n\n", len(installed))

	if len(findings) == 0 {
		fmt.Fprintln(os.Stderr, "  ✓ No issues found — your toolchain looks clean!")
	} else {
		w := tabwriter.NewWriter(os.Stderr, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "  SEVERITY\tTOOL\tCATEGORY\tMESSAGE")
		fmt.Fprintln(w, "  --------\t----\t--------\t-------")
		for _, f := range findings {
			icon := "ℹ"
			if f.Severity == "warning" {
				icon = "⚠"
			}
			fmt.Fprintf(w, "  %s\t%s\t%s\t%s\n", icon, f.Tool, f.Category, f.Message)
		}
		_ = w.Flush()
	}

	// License summary.
	fmt.Fprintf(os.Stderr, "\n  Licenses:\n")
	sortedLicenses := sortMapByCount(licenses)
	for _, lc := range sortedLicenses {
		fmt.Fprintf(os.Stderr, "    %-20s %d tool(s)\n", lc.name, lc.count)
	}

	fmt.Fprintf(os.Stderr, "\nResult: %d warning(s), %d info(s) across %d tools\n", warnings, infos, len(installed))

	if warnings > 0 {
		return fmt.Errorf("%d warning(s) found", warnings)
	}
	return nil
}

func printAuditJSON(findings []auditFinding, total, warnings, infos int, licenses map[string]int) error {
	report := auditReport{Findings: findings}
	report.Summary.TotalInstalled = total
	report.Summary.Warnings = warnings
	report.Summary.Infos = infos
	report.Summary.Licenses = licenses

	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))

	if warnings > 0 {
		return fmt.Errorf("%d warning(s) found", warnings)
	}
	return nil
}

// --- SBOM generation (CycloneDX 1.5 JSON) ---

type cdxBOM struct {
	BOMFormat    string         `json:"bomFormat"`
	SpecVersion  string         `json:"specVersion"`
	Version      int            `json:"version"`
	SerialNumber string         `json:"serialNumber,omitempty"`
	Metadata     cdxMetadata    `json:"metadata"`
	Components   []cdxComponent `json:"components"`
}

type cdxMetadata struct {
	Timestamp string  `json:"timestamp"`
	Tools     cdxTool `json:"tools"`
}

type cdxTool struct {
	Components []cdxToolComponent `json:"components"`
}

type cdxToolComponent struct {
	Type    string `json:"type"`
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}

type cdxComponent struct {
	Type        string          `json:"type"`
	Name        string          `json:"name"`
	Version     string          `json:"version,omitempty"`
	Description string          `json:"description,omitempty"`
	Licenses    []cdxLicense    `json:"licenses,omitempty"`
	ExternalRef []cdxExtRef     `json:"externalReferences,omitempty"`
	Properties  []cdxProperty   `json:"properties,omitempty"`
}

type cdxLicense struct {
	License cdxLicenseID `json:"license"`
}

type cdxLicenseID struct {
	Name string `json:"name"`
}

type cdxExtRef struct {
	Type string `json:"type"`
	URL  string `json:"url"`
}

type cdxProperty struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

func generateSBOM(tools []registry.Tool) error {
	bom := cdxBOM{
		BOMFormat:   "CycloneDX",
		SpecVersion: "1.5",
		Version:     1,
		Metadata: cdxMetadata{
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			Tools: cdxTool{
				Components: []cdxToolComponent{{
					Type: "application",
					Name: "clim",
				}},
			},
		},
	}

	for _, t := range tools {
		if !t.IsInstalled() {
			continue
		}
		primary := t.PrimaryInstance()
		if primary == nil {
			continue
		}

		comp := cdxComponent{
			Type:    "application",
			Name:    t.Name,
			Version: primary.Version,
		}

		if t.GitHubInfo != nil {
			if t.GitHubInfo.Description != "" {
				comp.Description = t.GitHubInfo.Description
			}
			if t.GitHubInfo.License != "" {
				comp.Licenses = []cdxLicense{{
					License: cdxLicenseID{Name: t.GitHubInfo.License},
				}}
			}
		}

		if t.GitHubSlug != "" {
			comp.ExternalRef = append(comp.ExternalRef, cdxExtRef{
				Type: "vcs",
				URL:  "https://github.com/" + t.GitHubSlug,
			})
		}

		comp.Properties = []cdxProperty{
			{Name: "clim:source", Value: string(primary.Source)},
			{Name: "clim:path", Value: primary.Path},
			{Name: "clim:os", Value: runtime.GOOS},
			{Name: "clim:arch", Value: runtime.GOARCH},
		}

		bom.Components = append(bom.Components, comp)
	}

	data, err := json.MarshalIndent(bom, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	fmt.Fprintf(os.Stderr, "\n%d components in SBOM\n", len(bom.Components))
	return nil
}

type nameCount struct {
	name  string
	count int
}

func sortMapByCount(m map[string]int) []nameCount {
	result := make([]nameCount, 0, len(m))
	for k, v := range m {
		result = append(result, nameCount{k, v})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].count != result[j].count {
			return result[i].count > result[j].count
		}
		return result[i].name < result[j].name
	})
	return result
}
