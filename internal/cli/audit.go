package cli

import (
	"fmt"
	"os"
	"runtime"
	"sort"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/nassiharel/clim/internal/audit"
	"github.com/nassiharel/clim/internal/progress"
	"github.com/nassiharel/clim/internal/registry"
	"github.com/nassiharel/clim/internal/service"
)

var auditRefreshFlag bool
var auditSBOMFlag bool
var auditOutput func() OutputFormat

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
	auditOutput = addOutputFlag(auditCmd, OutputText, OutputJSON)
	auditCmd.Flags().BoolVar(&auditRefreshFlag, "refresh", false, "Force fresh scan (ignore cache)")
	auditCmd.Flags().BoolVar(&auditSBOMFlag, "sbom", false, "Generate CycloneDX SBOM instead of audit report")
	// Registered in root.go with command group.
}

// auditFinding is an alias for the shared audit.Finding type (used in JSON output).
type auditFinding = audit.Finding

type auditReport struct {
	Findings []audit.Finding `json:"findings"`
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

	// Count installed.
	var installedCount int
	for _, t := range tools {
		if t.IsInstalled() {
			installedCount++
		}
	}

	// Use shared audit logic.
	findings, licenses := audit.Analyze(tools)
	warnings, infos := audit.CountBySeverity(findings)

	if auditOutput() == OutputJSON {
		return printAuditJSON(findings, installedCount, warnings, infos, licenses)
	}

	// Human output.
	fmt.Fprintf(os.Stderr, "\nAuditing %d installed tools\n\n", installedCount)

	if len(findings) == 0 {
		fmt.Fprintln(os.Stderr, "  ✓ No issues found — your toolchain looks clean!")
	} else {
		w := tabwriter.NewWriter(os.Stderr, 0, 0, 2, ' ', 0)
		_, _ = fmt.Fprintln(w, "  SEVERITY\tTOOL\tCATEGORY\tMESSAGE")
		_, _ = fmt.Fprintln(w, "  --------\t----\t--------\t-------")
		for _, f := range findings {
			icon := "ℹ"
			if f.Severity == "warning" {
				icon = "⚠"
			}
			_, _ = fmt.Fprintf(w, "  %s\t%s\t%s\t%s\n", icon, f.Tool, f.Category, f.Message)
		}
		_ = w.Flush()
	}

	// License summary.
	fmt.Fprintf(os.Stderr, "\n  Licenses:\n")
	sortedLicenses := sortMapByCount(licenses)
	for _, lc := range sortedLicenses {
		fmt.Fprintf(os.Stderr, "    %-20s %d tool(s)\n", lc.name, lc.count)
	}

	fmt.Fprintf(os.Stderr, "\nResult: %d warning(s), %d info(s) across %d tools\n", warnings, infos, installedCount)

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

	if err := printJSON(report); err != nil {
		return err
	}

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
	Type        string        `json:"type"`
	Name        string        `json:"name"`
	Version     string        `json:"version,omitempty"`
	Description string        `json:"description,omitempty"`
	Licenses    []cdxLicense  `json:"licenses,omitempty"`
	ExternalRef []cdxExtRef   `json:"externalReferences,omitempty"`
	Properties  []cdxProperty `json:"properties,omitempty"`
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

	if err := printJSON(bom); err != nil {
		return err
	}
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
