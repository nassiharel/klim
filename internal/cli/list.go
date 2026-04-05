package cli

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/nassiharel/clim/internal/detector"
	"github.com/nassiharel/clim/internal/latest"
	"github.com/nassiharel/clim/internal/scanner"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all executables found on PATH with versions and update status",
	RunE:  runList,
}

func runList(cmd *cobra.Command, args []string) error {
	cfg, err := loadConfig()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// Phase A: scan PATH for all executables.
	fmt.Fprintln(os.Stderr, "Scanning PATH...")
	tools, err := scanner.ScanPATH(cfg)
	if err != nil {
		return fmt.Errorf("scanning PATH: %w", err)
	}

	if len(tools) == 0 {
		fmt.Fprintln(os.Stderr, "No executables found on PATH.")
		return nil
	}

	// Phase B: detect installed versions (file metadata, no execution).
	fmt.Fprintln(os.Stderr, "Reading version info...")
	detector.DetectAll(tools, runtime.NumCPU()*2)

	// Phase C: check latest versions for known tools.
	fmt.Fprintln(os.Stderr, "Checking for updates...")
	cache := latest.DefaultCache()
	latest.CheckAll(tools, cache, runtime.NumCPU())
	cache.Save()

	// Phase D: render table.
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "TOOL\tVERSION\tLATEST\tSTATUS\tPATH")
	fmt.Fprintln(w, "────\t───────\t──────\t──────\t────")

	for _, tool := range tools {
		name := tool.DisplayName
		ver := valueOr(tool.Version, "—")
		lat := valueOr(tool.LatestVersion, "")
		status := computeStatus(tool.Version, tool.LatestVersion)
		path := tool.Path
		if len(path) > 45 {
			path = "..." + path[len(path)-42:]
		}

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", name, ver, lat, status, path)
	}

	w.Flush()
	fmt.Fprintf(os.Stderr, "\n%d executables found.\n", len(tools))

	return nil
}

// computeStatus compares installed vs latest versions.
func computeStatus(installed, latest string) string {
	if installed == "" {
		return "?"
	}
	if latest == "" {
		return "" // no latest source — nothing to compare
	}
	if versionEqual(installed, latest) {
		return "✓ up to date"
	}
	return "⬆ update"
}

// versionEqual checks if two version strings refer to the same version,
// accounting for trailing build numbers and .0 suffixes.
// e.g. "2.53.0.2" matches "2.53.0", "7.6.0.500" matches "7.6.0".
func versionEqual(a, b string) bool {
	na := normalizeVer(a)
	nb := normalizeVer(b)
	if na == nb {
		return true
	}
	// Check if one is a prefix of the other (handles extra build segments).
	if len(na) > len(nb) {
		return strings.HasPrefix(na, nb+".")
	}
	return strings.HasPrefix(nb, na+".")
}

// normalizeVer strips trailing ".0" segments for comparison.
func normalizeVer(v string) string {
	for {
		if len(v) >= 2 && v[len(v)-2:] == ".0" {
			v = v[:len(v)-2]
		} else {
			break
		}
	}
	return v
}

func valueOr(val, fallback string) string {
	if val == "" {
		return fallback
	}
	return val
}
