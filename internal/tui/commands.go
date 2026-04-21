package tui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"gopkg.in/yaml.v3"

	"github.com/nassiharel/clim/internal/catalog"
	"github.com/nassiharel/clim/internal/manifest"
	"github.com/nassiharel/clim/internal/paths"
	"github.com/nassiharel/clim/internal/registry"
	"github.com/nassiharel/clim/internal/service"
	"github.com/nassiharel/clim/internal/share"
)

// resolveSem limits concurrent version resolution goroutines in the TUI.
// Without this, Bubbletea fires all tools at once, spawning 40+ concurrent
// subprocess calls that overwhelm package managers and cause timeouts.
var resolveSem = make(chan struct{}, 4)

// --- Recommendation types ---

// recommendation represents a tool suggested based on tag/topic/category overlap
// with installed tools, enriched with display metadata.
type recommendation struct {
	toolIdx     int    // index into the tools slice
	score       int    // combined relevance score (higher = more relevant)
	reason      string // sorted installed tool names, e.g. "helm, kubectl, stern"
	category    string // tool's category for display
	description string // from GitHubInfo.Description
	stars       int    // from GitHubInfo.Stars
	matchPct    int    // 0–100 normalized score for display
}

// maxRecommendations caps the number of recommendations shown.
const maxRecommendations = 25

// computeRecommendations ranks not-installed tools by tag/topic overlap with
// installed tools, boosted by category match, GitHub stars, and recency.
func computeRecommendations(tools []registry.Tool) []recommendation {
	// Build tag+topic frequency from installed tools.
	tagFreq := make(map[string]int)
	tagSources := make(map[string][]string) // tag → installed tool names
	installedCats := make(map[string]bool)

	for _, t := range tools {
		if !t.IsInstalled() {
			continue
		}
		if t.Category != "" {
			installedCats[t.Category] = true
		}
		for _, tag := range t.Tags {
			tagFreq[tag]++
			tagSources[tag] = append(tagSources[tag], t.Name)
		}
		// Merge GitHub Topics into the same frequency map.
		if t.GitHubInfo != nil {
			for _, topic := range t.GitHubInfo.Topics {
				tagFreq[topic]++
				tagSources[topic] = append(tagSources[topic], t.Name)
			}
		}
	}

	if len(tagFreq) == 0 {
		return nil
	}

	// Score each not-installed tool.
	var recs []recommendation
	maxScore := 0
	for i, t := range tools {
		if t.IsInstalled() {
			continue
		}
		// Skip archived tools.
		if t.GitHubInfo != nil && t.GitHubInfo.Archived {
			continue
		}
		// Skip tools not installable on this OS.
		if !t.Packages.HasAnyPackageForOS() {
			continue
		}

		score := 0
		matchedTools := make(map[string]struct{})

		// Tag overlap.
		for _, tag := range t.Tags {
			if freq, ok := tagFreq[tag]; ok {
				score += freq
				for _, src := range tagSources[tag] {
					matchedTools[src] = struct{}{}
				}
			}
		}
		// GitHub Topics overlap.
		if t.GitHubInfo != nil {
			for _, topic := range t.GitHubInfo.Topics {
				if freq, ok := tagFreq[topic]; ok {
					score += freq
					for _, src := range tagSources[topic] {
						matchedTools[src] = struct{}{}
					}
				}
			}
		}

		// Category match bonus.
		if t.Category != "" && installedCats[t.Category] {
			score += 2
		}

		// GitHub stars popularity boost.
		if t.GitHubInfo != nil {
			if t.GitHubInfo.Stars > 10000 {
				score += 2
			} else if t.GitHubInfo.Stars > 1000 {
				score += 1
			}
		}

		// Recency boost: pushed within last 6 months.
		if t.GitHubInfo != nil && t.GitHubInfo.PushedAt != "" {
			if pushed, err := time.Parse(time.RFC3339, t.GitHubInfo.PushedAt); err == nil {
				if time.Since(pushed) < 6*30*24*time.Hour {
					score += 1
				}
			}
		}

		if score == 0 {
			continue
		}

		// Build reason from matched installed tool names (sorted, top 3).
		var reasons []string
		for name := range matchedTools {
			reasons = append(reasons, name)
		}
		sort.Strings(reasons)
		if len(reasons) > 3 {
			reasons = reasons[:3]
		}

		desc := ""
		stars := 0
		if t.GitHubInfo != nil {
			desc = t.GitHubInfo.Description
			stars = t.GitHubInfo.Stars
		}

		rec := recommendation{
			toolIdx:     i,
			score:       score,
			reason:      strings.Join(reasons, ", "),
			category:    t.Category,
			description: desc,
			stars:       stars,
		}
		recs = append(recs, rec)
		if score > maxScore {
			maxScore = score
		}
	}

	// Sort by score descending, then name ascending.
	sort.Slice(recs, func(i, j int) bool {
		if recs[i].score != recs[j].score {
			return recs[i].score > recs[j].score
		}
		return tools[recs[i].toolIdx].Name < tools[recs[j].toolIdx].Name
	})

	// Cap at maxRecommendations.
	if len(recs) > maxRecommendations {
		recs = recs[:maxRecommendations]
	}

	// Compute normalized percentages.
	if maxScore > 0 {
		for i := range recs {
			recs[i].matchPct = recs[i].score * 100 / maxScore
			if recs[i].matchPct < 1 {
				recs[i].matchPct = 1
			}
		}
	}

	return recs
}

// --- Scan & version resolution messages ---

type scanResultMsg struct {
	gen         int // scan generation captured at dispatch; stale results are discarded
	tools       []registry.Tool
	catalogInfo *service.CatalogInfo // how the catalog was loaded
	scanInfo    *service.ScanInfo    // cache vs fresh
	err         error                // non-nil if catalog load or PATH scan failed
	// cacheWarning is a non-fatal message shown to the user when the cache
	// was unreadable and had to be discarded (distinct from err, which is
	// reserved for errors that should prevent persisting a new cache).
	cacheWarning string
}

type toolVersionMsg struct {
	index int
	gen   int // scan generation — messages from old scans are discarded
	tool  registry.Tool
}

// --- Single-tool action messages ---

type execFinishedMsg struct {
	toolIdx int
	action  string
	err     error
}

type refreshToolMsg struct {
	toolIdx int
	tool    registry.Tool
}

// --- Action types ---

type pendingAction struct {
	toolIdx int
	action  string
	cmdArgs []string // exec args: [0]=binary, [1:]=arguments (no shell)
}

type sourceChoice struct {
	source  registry.InstallSource
	cmdArgs []string // exec args (no shell)
}

type sourcePicker struct {
	toolIdx int
	action  string
	choices []sourceChoice
}

// toolMenuAction represents one selectable action in the tool action menu.
type toolMenuAction struct {
	label        string        // "Upgrade", "Remove", "Install"
	picker       *sourcePicker // primary action (install or upgrade)
	removePicker *sourcePicker // optional remove action (for installed PM rows)
}

// --- Backup types ---

type backupStatus int

const (
	backupPending backupStatus = iota
	backupRunning
	backupDone
	backupFailed
	backupSkipped
)

type backupItem struct {
	name     string
	display  string
	cmdArgs  []string // install command args (import) or nil (export)
	source   string
	status   backupStatus
	errMsg   string
	selected bool // true = will be installed on confirm (import only)
}

// --- Backup messages ---

type exportFinishedMsg struct {
	path  string
	count int
	err   error
}

type backupPlanMsg struct {
	items     []backupItem
	err       error // non-nil when the entire import failed (bad path, invalid YAML, etc.)
	fromToken bool  // true if this came from a share token import
}

type backupItemDoneMsg struct {
	idx int
	err error
}

// batchUpgradeItemMsg is sent when one batch-upgrade tool finishes.
type batchUpgradeItemMsg struct {
	toolIdx int
	err     error
}

// backupTickMsg advances the animated progress by marking the next pending item as done.
type backupTickMsg struct{}

// --- Sidebar types ---

// sidebarItem represents one entry in the filter sidebar.
type sidebarItem struct {
	label    string // display text ("All", "Cloud", "kubernetes", etc.)
	section  string // "category", "tag", "platform"
	value    string // filter value; "" = show all for that section
	isHeader bool   // true = section header, not selectable
}

// shareFinishedMsg is sent when share token generation completes.
type shareFinishedMsg struct {
	token string
	count int
	err   error
}

type marketplaceRefreshMsg struct {
	result *catalog.RefreshResult
	err    error
}

// --- Scan & version commands ---

// findToolsCmd builds the initial scan command. When force is false it will
// try to serve results from the scan cache (fast path: no version
// resolution). When force is true the cache is invalidated first and a full
// scan runs. The returned message carries the scan generation supplied by
// the caller so stale results from earlier scans can be discarded.
func findToolsCmd(svc *service.ToolService, force bool, gen int) func() scanResultMsg {
	return func() scanResultMsg {
		ctx := context.Background()
		if !force {
			tools, info, scanInfo, err := svc.LoadCached(ctx)
			switch {
			case err == nil:
				return scanResultMsg{gen: gen, tools: tools, catalogInfo: info, scanInfo: scanInfo}
			case os.IsNotExist(err):
				// Cold start — fall through to a fresh scan silently.
			default:
				// Cache exists but is unreadable — ignore it, fresh scan overwrites.
				slog.Warn("scan cache unreadable, will rescan", "error", err)
				tools, info, scanErr := svc.LoadAndScan(ctx)
				return scanResultMsg{
					gen:          gen,
					tools:        tools,
					catalogInfo:  info,
					scanInfo:     &service.ScanInfo{Source: service.ScanSourceFresh},
					err:          scanErr,
					cacheWarning: fmt.Sprintf("scan cache ignored (%v) — rebuilding", err),
				}
			}
		}
		tools, info, err := svc.LoadAndScan(ctx)
		return scanResultMsg{
			gen:         gen,
			tools:       tools,
			catalogInfo: info,
			scanInfo:    &service.ScanInfo{Source: service.ScanSourceFresh},
			err:         err,
		}
	}
}

func resolveToolVersionCmd(svc *service.ToolService, index int, gen int, tool registry.Tool) func() toolVersionMsg {
	return func() toolVersionMsg {
		resolveSem <- struct{}{}        // acquire
		defer func() { <-resolveSem }() // release

		ctx := context.Background()
		if tool.IsInstalled() {
			svc.ResolveOne(ctx, &tool)
		}
		return toolVersionMsg{index: index, gen: gen, tool: tool}
	}
}

// --- Single-tool action commands ---

// toolActionCmd wraps a PM command + post-run pause into a single ExecCommand.
// This keeps the terminal in raw mode during the pause so the user can read output.
type toolActionCmd struct {
	args   []string
	action string
	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer
}

func (c *toolActionCmd) SetStdin(r io.Reader)  { c.stdin = r }
func (c *toolActionCmd) SetStdout(w io.Writer) { c.stdout = w }
func (c *toolActionCmd) SetStderr(w io.Writer) { c.stderr = w }

func (c *toolActionCmd) Run() error {
	cmd := exec.Command(c.args[0], c.args[1:]...)
	cmd.Stdin = c.stdin
	cmd.Stdout = c.stdout
	cmd.Stderr = c.stderr

	runErr := cmd.Run()

	// Log exit code for debugging.
	exitCode := 0
	var exitErr *exec.ExitError
	if errors.As(runErr, &exitErr) {
		exitCode = exitErr.ExitCode()
	}
	slog.Info("tool action finished", "action", c.action, "cmd", c.args, "exitCode", exitCode, "err", runErr)

	// Show result and wait for keypress — terminal is still ours.
	w := c.stderr
	if w == nil {
		w = os.Stderr
	}
	if runErr != nil {
		fmt.Fprintf(w, "\n✗ %s failed (exit code %d)\n", c.action, exitCode)
	} else {
		fmt.Fprintf(w, "\n✓ %s completed (exit code 0)\n", c.action)
	}
	fmt.Fprint(w, "\nPress Enter to return to clim...")

	r := c.stdin
	if r == nil {
		r = os.Stdin
	}
	buf := make([]byte, 64)
	r.Read(buf) //nolint:errcheck

	return runErr
}

func execToolActionCmd(pa pendingAction) tea.Cmd {
	c := &toolActionCmd{
		args:   pa.cmdArgs,
		action: pa.action,
	}
	return tea.Exec(c, func(err error) tea.Msg {
		return execFinishedMsg{
			toolIdx: pa.toolIdx,
			action:  pa.action,
			err:     err,
		}
	})
}

func refreshSingleToolCmd(svc *service.ToolService, idx int, tool registry.Tool) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		refreshed := svc.RefreshTool(ctx, tool)
		return refreshToolMsg{toolIdx: idx, tool: refreshed}
	}
}

// refreshMarketplaceCmd fetches the latest marketplace catalog from GitHub,
// diffs it against the local cache, and returns the result.
func refreshMarketplaceCmd(fetcher catalog.MarketplaceFetcher) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		result, err := catalog.Refresh(ctx, fetcher)
		return marketplaceRefreshMsg{result: result, err: err}
	}
}

// backupsDir returns the path to the backups directory, creating it if needed.
func backupsDir() (string, error) {
	bdir, err := paths.BackupsDir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(bdir, 0o755); err != nil {
		return "", err
	}
	return bdir, nil
}

// --- Export command ---

func exportToolsCmd(tools []registry.Tool) tea.Cmd {
	return func() tea.Msg {
		sorted := make([]registry.Tool, len(tools))
		copy(sorted, tools)
		registry.SortByName(sorted)

		var exported []manifest.Tool
		for _, tool := range sorted {
			if !tool.IsInstalled() {
				continue
			}
			exported = append(exported, manifest.FromRegistryTool(tool))
		}

		m := manifest.Manifest{
			GeneratedBy: "clim export",
			OS:          runtime.GOOS,
			Arch:        runtime.GOARCH,
			Tools:       exported,
		}

		data, err := yaml.Marshal(&m)
		if err != nil {
			return exportFinishedMsg{err: err}
		}

		bdir, err := backupsDir()
		if err != nil {
			return exportFinishedMsg{err: fmt.Errorf("creating backups dir: %w", err)}
		}

		filename := filepath.Join(bdir, fmt.Sprintf("clim-export-%s.yaml", time.Now().Format("2006-01-02")))
		// Avoid silently overwriting an existing export from today.
		if _, err := os.Stat(filename); err == nil {
			filename = filepath.Join(bdir, fmt.Sprintf("clim-export-%s.yaml", time.Now().Format("2006-01-02-150405")))
		}
		header := "# clim — Installed Tools Manifest\n# Generated on " + runtime.GOOS + "/" + runtime.GOARCH + "\n#\n# Reinstall on a new machine:\n#   clim import my-tools.yaml\n#\n\n"

		if err := os.WriteFile(filename, []byte(header+string(data)), 0o644); err != nil {
			return exportFinishedMsg{err: err}
		}

		return exportFinishedMsg{path: filename, count: len(exported)}
	}
}

// --- Import commands ---

// buildImportPlanCmd reads a manifest, scans PATH, and builds a backup plan.
func buildImportPlanCmd(svc *service.ToolService, path string) tea.Cmd {
	return func() tea.Msg {
		data, err := os.ReadFile(path)
		if err != nil {
			return backupPlanMsg{err: fmt.Errorf("reading manifest: %w", err)}
		}

		var m manifest.Manifest
		if err := yaml.Unmarshal(data, &m); err != nil {
			return backupPlanMsg{err: fmt.Errorf("parsing manifest: %w", err)}
		}

		// Load registry and scan PATH.
		ctx := context.Background()
		regTools, _, err := svc.ScanOnly(ctx)
		if err != nil {
			return backupPlanMsg{err: fmt.Errorf("scanning PATH: %w", err)}
		}

		regMap := registry.ToolMap(regTools)

		var items []backupItem
		for _, mt := range m.Tools {
			rt, exists := regMap[mt.Name]

			if exists && rt.IsInstalled() {
				items = append(items, backupItem{
					name:    mt.Name,
					display: mt.DisplayName,
					source:  "—",
					status:  backupSkipped,
					errMsg:  "already installed",
				})
				continue
			}

			// Determine install command.
			var pkgs registry.PackageIDs
			if exists {
				pkgs = rt.Packages
			} else {
				pkgs = registry.PackageIDs{
					Winget: mt.Packages.Winget,
					Choco:  mt.Packages.Choco,
					Scoop:  mt.Packages.Scoop,
					Brew:   mt.Packages.Brew,
					Apt:    mt.Packages.Apt,
					Snap:   mt.Packages.Snap,
					NPM:    mt.Packages.NPM,
				}
			}

			src := pkgs.BestInstallSource()
			// Prefer the source recorded in the manifest if it's available.
			if mt.Source != "" {
				preferred := registry.InstallSource(mt.Source)
				if args := pkgs.InstallArgs(preferred); args != nil {
					src = preferred
				}
			}
			installArgs := pkgs.InstallArgs(src)
			if installArgs == nil {
				reason := "no package for " + runtime.GOOS
				if src == "" && pkgs.HasAnyPackageForOS() {
					reason = "no supported package manager installed"
				}
				items = append(items, backupItem{
					name:    mt.Name,
					display: mt.DisplayName,
					status:  backupSkipped,
					errMsg:  reason,
				})
				continue
			}

			items = append(items, backupItem{
				name:     mt.Name,
				display:  mt.DisplayName,
				cmdArgs:  installArgs,
				source:   string(src),
				status:   backupPending,
				selected: true,
			})
		}

		return backupPlanMsg{items: items}
	}
}

// execBackupInstallCmd suspends the TUI and runs one install command.
func execBackupInstallCmd(idx int, args []string) tea.Cmd {
	cmd := exec.Command(args[0], args[1:]...)
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		return backupItemDoneMsg{idx: idx, err: err}
	})
}

// --- Pack types & commands ---

// packItemStatus tracks the state of a single tool in a pack operation.
type packItemStatus int

const (
	packItemPending packItemStatus = iota
	packItemRunning
	packItemDone
	packItemFailed
	packItemSkipped
)

// packItem represents one tool in a pack install/remove operation.
type packItem struct {
	name    string
	display string
	cmdArgs []string // install or remove command args
	source  string
	status  packItemStatus
	errMsg  string
}

// packItemDoneMsg is sent when one pack tool install/remove finishes.
type packItemDoneMsg struct {
	idx int
	err error
}

// buildPackInstallItems builds the install item list for a pack (synchronous, no tea.Cmd).
func buildPackInstallItems(tools []registry.Tool, pack registry.Pack) []packItem {
	toolMap := make(map[string]*registry.Tool, len(tools))
	for i := range tools {
		toolMap[tools[i].Name] = &tools[i]
	}

	var items []packItem
	for _, name := range pack.ToolNames {
		rt, exists := toolMap[name]

		if exists && rt.IsInstalled() {
			items = append(items, packItem{
				name:    name,
				display: rt.DisplayName,
				source:  "—",
				status:  packItemSkipped,
				errMsg:  "already installed",
			})
			continue
		}

		if !exists {
			items = append(items, packItem{
				name:    name,
				display: name,
				status:  packItemSkipped,
				errMsg:  "not in catalog",
			})
			continue
		}

		src := rt.Packages.BestInstallSource()
		installArgs := rt.Packages.InstallArgs(src)
		if installArgs == nil {
			reason := "no package for " + runtime.GOOS
			if src == "" && rt.Packages.HasAnyPackageForOS() {
				reason = "no supported package manager installed"
			}
			items = append(items, packItem{
				name:    name,
				display: rt.DisplayName,
				status:  packItemSkipped,
				errMsg:  reason,
			})
			continue
		}

		items = append(items, packItem{
			name:    name,
			display: rt.DisplayName,
			cmdArgs: installArgs,
			source:  string(src),
			status:  packItemPending,
		})
	}
	return items
}

// buildPackRemoveItems builds the remove item list for a pack (synchronous, no tea.Cmd).
func buildPackRemoveItems(tools []registry.Tool, pack registry.Pack) []packItem {
	toolMap := make(map[string]*registry.Tool, len(tools))
	for i := range tools {
		toolMap[tools[i].Name] = &tools[i]
	}

	var items []packItem
	for _, name := range pack.ToolNames {
		rt, exists := toolMap[name]

		if !exists {
			items = append(items, packItem{
				name:    name,
				display: name,
				status:  packItemSkipped,
				errMsg:  "not in catalog",
			})
			continue
		}

		if !rt.IsInstalled() {
			items = append(items, packItem{
				name:    name,
				display: rt.DisplayName,
				status:  packItemSkipped,
				errMsg:  "not installed",
			})
			continue
		}

		primary := rt.PrimaryInstance()
		removeArgs := rt.Packages.RemoveArgs(primary.Source)
		if removeArgs == nil {
			items = append(items, packItem{
				name:    name,
				display: rt.DisplayName,
				status:  packItemSkipped,
				errMsg:  "no remove command for " + string(primary.Source),
			})
			continue
		}

		items = append(items, packItem{
			name:    name,
			display: rt.DisplayName,
			cmdArgs: removeArgs,
			source:  string(primary.Source),
			status:  packItemPending,
		})
	}
	return items
}

// execPackItemCmd suspends the TUI and runs one pack tool install/remove command.
func execPackItemCmd(idx int, args []string) tea.Cmd {
	cmd := exec.Command(args[0], args[1:]...)
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		return packItemDoneMsg{idx: idx, err: err}
	})
}

// --- Share token commands ---

// shareToolsCmd generates a compact share token from installed tools.
func shareToolsCmd(tools []registry.Tool) tea.Cmd {
	return func() tea.Msg {
		var names []string
		for _, tool := range tools {
			if tool.IsInstalled() {
				names = append(names, tool.Name)
			}
		}
		if len(names) == 0 {
			return shareFinishedMsg{err: errors.New("no installed tools to share")}
		}

		token, err := share.Encode(names)
		if err != nil {
			return shareFinishedMsg{err: err}
		}
		return shareFinishedMsg{token: token, count: len(names)}
	}
}

// exportFavoritesCmd exports only the favorited tools to a YAML manifest.
func exportFavoritesCmd(tools []registry.Tool, favNames map[string]bool) tea.Cmd {
	return func() tea.Msg {
		var exported []manifest.Tool
		for _, tool := range tools {
			if !favNames[tool.Name] {
				continue
			}
			exported = append(exported, manifest.FromRegistryTool(tool))
		}
		if len(exported) == 0 {
			return exportFinishedMsg{err: errors.New("no favorites to export")}
		}

		sort.Slice(exported, func(i, j int) bool {
			return strings.ToLower(exported[i].Name) < strings.ToLower(exported[j].Name)
		})

		m := manifest.Manifest{
			GeneratedBy: "clim favorites export",
			OS:          runtime.GOOS,
			Arch:        runtime.GOARCH,
			Tools:       exported,
		}

		data, err := yaml.Marshal(&m)
		if err != nil {
			return exportFinishedMsg{err: err}
		}

		bdir, err := backupsDir()
		if err != nil {
			return exportFinishedMsg{err: fmt.Errorf("creating backups dir: %w", err)}
		}

		filename := filepath.Join(bdir, fmt.Sprintf("clim-favorites-%s.yaml", time.Now().Format("2006-01-02")))
		if _, err := os.Stat(filename); err == nil {
			filename = filepath.Join(bdir, fmt.Sprintf("clim-favorites-%s.yaml", time.Now().Format("2006-01-02-150405")))
		}
		header := "# clim — Favorites Manifest\n# Generated on " + runtime.GOOS + "/" + runtime.GOARCH + "\n#\n# Reinstall on a new machine:\n#   clim import favorites.yaml\n#\n\n"

		if err := os.WriteFile(filename, []byte(header+string(data)), 0o644); err != nil {
			return exportFinishedMsg{err: err}
		}

		return exportFinishedMsg{path: filename, count: len(exported)}
	}
}

// shareFavoritesCmd encodes favorite tool names into a share token.
func shareFavoritesCmd(favNames map[string]bool) tea.Cmd {
	return func() tea.Msg {
		var names []string
		for name := range favNames {
			names = append(names, name)
		}
		if len(names) == 0 {
			return shareFinishedMsg{err: errors.New("no favorites to share")}
		}
		sort.Strings(names)

		token, err := share.Encode(names)
		if err != nil {
			return shareFinishedMsg{err: err}
		}
		return shareFinishedMsg{token: token, count: len(names)}
	}
}

// buildTokenImportPlanCmd decodes a share token and builds an import plan.
func buildTokenImportPlanCmd(svc *service.ToolService, token string) tea.Cmd {
	return func() tea.Msg {
		names, err := share.Decode(token)
		if err != nil {
			return backupPlanMsg{err: fmt.Errorf("invalid token: %w", err), fromToken: true}
		}

		// Load registry and scan PATH.
		ctx := context.Background()
		regTools, _, err := svc.ScanOnly(ctx)
		if err != nil {
			return backupPlanMsg{err: fmt.Errorf("scanning PATH: %w", err), fromToken: true}
		}

		regMap := registry.ToolMap(regTools)

		var items []backupItem
		for _, name := range names {
			rt, exists := regMap[name]

			if !exists {
				items = append(items, backupItem{
					name:    name,
					display: name,
					status:  backupSkipped,
					errMsg:  "not in catalog",
				})
				continue
			}

			if rt.IsInstalled() {
				items = append(items, backupItem{
					name:    rt.Name,
					display: rt.DisplayName,
					source:  "—",
					status:  backupSkipped,
					errMsg:  "already installed",
				})
				continue
			}

			src := rt.Packages.BestInstallSource()
			installArgs := rt.Packages.InstallArgs(src)
			if installArgs == nil {
				reason := "no package for " + runtime.GOOS
				if src == "" && rt.Packages.HasAnyPackageForOS() {
					reason = "no supported package manager installed"
				}
				items = append(items, backupItem{
					name:    rt.Name,
					display: rt.DisplayName,
					status:  backupSkipped,
					errMsg:  reason,
				})
				continue
			}

			items = append(items, backupItem{
				name:     rt.Name,
				display:  rt.DisplayName,
				cmdArgs:  installArgs,
				source:   string(src),
				status:   backupPending,
				selected: true,
			})
		}

		return backupPlanMsg{items: items, fromToken: true}
	}
}

// execBatchUpgradeCmd suspends the TUI and runs one upgrade command.
func execBatchUpgradeCmd(toolIdx int, args []string) tea.Cmd {
	cmd := exec.Command(args[0], args[1:]...)
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		return batchUpgradeItemMsg{toolIdx: toolIdx, err: err}
	})
}

// humaniseCacheAge renders a cache-write timestamp as a short, human-friendly
// relative string ("just now", "3m ago", "2h ago", "yesterday", "4d ago").
// Returns "" for a zero timestamp so callers can fall back to a generic label.
func humaniseCacheAge(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	d := time.Since(t)
	switch {
	case d < 45*time.Second:
		return "just now"
	case d < 90*time.Second:
		return "1m ago"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 2*time.Hour:
		return "1h ago"
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	case d < 48*time.Hour:
		return "yesterday"
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}
