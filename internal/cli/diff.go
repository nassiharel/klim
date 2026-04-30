package cli

import (
	"bytes"
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/nassiharel/clim/internal/manifest"
	"github.com/nassiharel/clim/internal/progress"
	"github.com/nassiharel/clim/internal/service"
	"github.com/nassiharel/clim/internal/share"
)

var diffRefreshFlag bool

var diffCmd = &cobra.Command{
	Use:   "diff <manifest.yaml | share-token>",
	Short: "Compare your installed tools against a manifest or share token",
	Long: `Compare your local tool environment against a reference:

  clim diff my-tools.yaml            # compare against a manifest file
  clim diff clim:v1:abc123...        # compare against a share token

Shows which tools match, differ in version, or are missing on either side.

Exit codes:
  0  Environments match
  1  Differences found`,
	Args: cobra.ExactArgs(1),
	RunE: runDiff,
}

func init() {
	diffCmd.Flags().BoolVar(&diffRefreshFlag, "refresh", false, "Force fresh scan (ignore cache)")
	rootCmd.AddCommand(diffCmd)
}

// diffEntry holds the comparison data for a single tool.
type diffEntry struct {
	name          string
	localVersion  string
	localSource   string
	remoteVersion string
	remoteSource  string
	status        string // "✓ match", "≠ differs", "← local only", "→ remote only"
}

func runDiff(cmd *cobra.Command, args []string) error {
	target := args[0]

	// Load remote/target tools.
	remoteName, remoteTools, err := loadDiffTarget(target)
	if err != nil {
		return err
	}

	// Load local tools.
	sp := progress.New("Scanning installed tools...")
	tools, _, scanInfo, err := svc.LoadAndResolveCached(cmd.Context(), diffRefreshFlag)
	if err != nil {
		sp.Fail(err.Error())
		return err
	}
	if scanInfo != nil && scanInfo.Source == service.ScanSourceCache {
		sp.Done("Loaded from cache")
	} else {
		sp.Done("Tools scanned")
	}

	// Build local map (only installed tools).
	localMap := make(map[string]manifest.Tool)
	for _, t := range tools {
		if !t.IsInstalled() {
			continue
		}
		localMap[t.Name] = manifest.FromRegistryTool(t)
	}

	// Build remote map.
	remoteMap := make(map[string]manifest.Tool)
	for _, t := range remoteTools {
		remoteMap[t.Name] = t
	}

	// Collect all unique tool names.
	allNames := make(map[string]bool)
	for name := range localMap {
		allNames[name] = true
	}
	for name := range remoteMap {
		allNames[name] = true
	}

	sorted := make([]string, 0, len(allNames))
	for name := range allNames {
		sorted = append(sorted, name)
	}
	sort.Strings(sorted)

	// Compare.
	var entries []diffEntry
	var matches, differs, localOnly, remoteOnly int

	for _, name := range sorted {
		local, hasLocal := localMap[name]
		remote, hasRemote := remoteMap[name]

		var e diffEntry
		e.name = name

		switch {
		case hasLocal && hasRemote:
			e.localVersion = local.Version
			e.localSource = local.Source
			e.remoteSource = remote.Source
			// Compare using raw versions first, then format for display.
			if versionsEqual(local.Version, remote.Version) {
				e.status = "✓ match"
				matches++
			} else {
				e.status = "≠ differs"
				differs++
			}
			e.remoteVersion = remote.Version
			if e.remoteVersion == "" {
				e.remoteVersion = "—"
			}
		case hasLocal && !hasRemote:
			e.localVersion = local.Version
			e.localSource = local.Source
			e.remoteVersion = "—"
			e.remoteSource = ""
			e.status = "← local only"
			localOnly++
		case !hasLocal && hasRemote:
			e.localVersion = "—"
			e.localSource = ""
			e.remoteVersion = remote.Version
			e.remoteSource = remote.Source
			e.status = "→ remote only"
			remoteOnly++
		}

		entries = append(entries, e)
	}

	// Print results.
	fmt.Fprintf(os.Stderr, "\nComparing local vs %s\n\n", remoteName)

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "TOOL\tLOCAL\tREMOTE\tSTATUS")
	fmt.Fprintln(w, "----\t-----\t------\t------")

	for _, e := range entries {
		localCol := e.localVersion
		if e.localSource != "" {
			localCol += " (" + e.localSource + ")"
		}
		remoteCol := e.remoteVersion
		if e.remoteSource != "" {
			remoteCol += " (" + e.remoteSource + ")"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", e.name, localCol, remoteCol, e.status)
	}
	_ = w.Flush()

	fmt.Fprintf(os.Stderr, "\nResult: %d match, %d differ, %d local only, %d remote only\n",
		matches, differs, localOnly, remoteOnly)

	if differs > 0 || localOnly > 0 || remoteOnly > 0 {
		return fmt.Errorf("%d difference(s) found", differs+localOnly+remoteOnly)
	}
	return nil
}

// loadDiffTarget parses the diff target — either a manifest file or a share token.
// Returns a display name, the list of remote tools, and any error.
func loadDiffTarget(target string) (string, []manifest.Tool, error) {
	// Check if it's a share token.
	if strings.HasPrefix(target, "clim:") {
		names, err := share.Decode(target)
		if err != nil {
			return "", nil, fmt.Errorf("decoding share token: %w", err)
		}
		// Share tokens only carry names — no versions.
		var tools []manifest.Tool
		for _, name := range names {
			tools = append(tools, manifest.Tool{Name: name})
		}
		return "share token", tools, nil
	}

	// Try as a manifest file.
	data, err := os.ReadFile(target)
	if err != nil {
		return "", nil, fmt.Errorf("reading %s: %w", target, err)
	}

	var m manifest.Manifest
	dec := yaml.NewDecoder(bytes.NewReader(data))
	if err := dec.Decode(&m); err != nil {
		return "", nil, fmt.Errorf("parsing manifest %s: %w", target, err)
	}

	label := target
	if m.OS != "" || m.Arch != "" {
		label = fmt.Sprintf("%s (%s/%s)", target, m.OS, m.Arch)
	}
	return label, m.Tools, nil
}

// versionsEqual compares a local version against a remote/reference version.
// An empty remote version is treated as unknown and considered a match
// (for example, share tokens only carry tool names). An empty local
// version does not match a non-empty remote version.
func versionsEqual(localVersion, remoteVersion string) bool {
	if remoteVersion == "" {
		return true
	}
	if localVersion == "" {
		return false
	}
	localVersion = strings.TrimPrefix(localVersion, "v")
	remoteVersion = strings.TrimPrefix(remoteVersion, "v")
	return localVersion == remoteVersion
}
