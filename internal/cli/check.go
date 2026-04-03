package cli

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/nassiharel/clim/internal/detector"
	"github.com/nassiharel/clim/internal/registry"
	"github.com/nassiharel/clim/internal/version"
)

var checkCmd = &cobra.Command{
	Use:   "check <tool>",
	Short: "Check a specific tool's version and update status",
	Args:  cobra.ExactArgs(1),
	RunE:  runCheck,
}

func init() {
	rootCmd.AddCommand(checkCmd)
}

func runCheck(cmd *cobra.Command, args []string) error {
	toolName := args[0]

	// Find the tool in the registry.
	tools := registry.DefaultTools()
	var tool *registry.Tool
	for i := range tools {
		if tools[i].Name == toolName {
			tool = &tools[i]
			break
		}
	}
	if tool == nil {
		// Try partial match on display name.
		for i := range tools {
			if contains(tools[i].DisplayName, toolName) || contains(tools[i].Name, toolName) {
				tool = &tools[i]
				break
			}
		}
	}
	if tool == nil {
		fmt.Fprintf(os.Stderr, "Unknown tool: %s\n\nAvailable tools:\n", toolName)
		for _, t := range tools {
			fmt.Fprintf(os.Stderr, "  %-12s  %s\n", t.Name, t.DisplayName)
		}
		return fmt.Errorf("tool %q not found in registry", toolName)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Detect.
	det := detector.DetectOne(ctx, *tool)

	// Check latest.
	cache := version.LoadCache()
	checker := version.NewHTTPChecker(version.TokenFromEnv())
	lat := checker.Latest(ctx, tool.LatestSource)
	if lat.Error == nil && lat.Version != "" {
		key := cacheKeyFor(*tool)
		cache.Set(key, lat.Version)
		cache.Save()
	}

	// Display.
	fmt.Printf("Tool:       %s (%s)\n", tool.DisplayName, tool.Name)

	if det.Found {
		fmt.Printf("Installed:  %s\n", valueOrDash(det.Version))
		fmt.Printf("Path:       %s\n", det.Path)
	} else {
		fmt.Printf("Installed:  not found\n")
	}

	if lat.Error != nil {
		fmt.Printf("Latest:     error (%v)\n", lat.Error)
	} else {
		fmt.Printf("Latest:     %s\n", lat.Version)
	}

	if det.Found && det.Version != "" && lat.Version != "" {
		status, _ := version.CompareVersions(det.Version, lat.Version)
		fmt.Printf("Status:     %s\n", version.StatusString(status))
	}

	if tool.Homepage != "" {
		fmt.Printf("Homepage:   %s\n", tool.Homepage)
	}

	return nil
}

func cacheKeyFor(tool registry.Tool) string {
	src := tool.LatestSource
	switch src.Type {
	case registry.SourceGitHub:
		return "github:" + src.Repo
	case registry.SourcePyPI:
		return "pypi:" + src.Package
	case registry.SourceNPM:
		return "npm:" + src.Package
	case registry.SourceCustom:
		return "custom:" + src.URLPattern
	default:
		return "unknown:" + tool.Name
	}
}

func valueOrDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

func contains(haystack, needle string) bool {
	return len(needle) > 0 &&
		len(haystack) >= len(needle) &&
		(haystack == needle ||
			strings.Contains(strings.ToLower(haystack), strings.ToLower(needle)))
}
