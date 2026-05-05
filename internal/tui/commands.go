package tui

import (
	"bufio"
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
	"github.com/nassiharel/clim/internal/compliance"
	"github.com/nassiharel/clim/internal/config"
	"github.com/nassiharel/clim/internal/fileutil"
	"github.com/nassiharel/clim/internal/manifest"
	"github.com/nassiharel/clim/internal/paths"
	"github.com/nassiharel/clim/internal/recommend"
	"github.com/nassiharel/clim/internal/registry"
	"github.com/nassiharel/clim/internal/service"
	"github.com/nassiharel/clim/internal/share"
)

// resolveSem limits concurrent version resolution goroutines in the TUI.
// Without this, Bubbletea fires all tools at once, spawning 40+ concurrent
// subprocess calls that overwhelm package managers and cause timeouts.
var resolveSem = make(chan struct{}, 4)

// recommendation is the TUI-local alias for recommend.Recommendation.
// The shared algorithm lives in internal/recommend so the web UI can
// share it; the alias keeps existing TUI call sites compact.
type recommendation = recommend.Recommendation

// computeRecommendations is a thin wrapper that delegates to
// recommend.Compute. Kept as a function to minimise churn at call
// sites; new code should call recommend.Compute directly.
func computeRecommendations(tools []registry.Tool) []recommendation {
	return recommend.Compute(tools)
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

// complianceLoadedMsg arrives when the async URL fetch initiated by
// loadComplianceURLCmd completes. It carries the freshly fetched
// policy; the receiver must rebuild the index against its CURRENT
// tools slice — using a snapshot taken at command-dispatch time
// would regress to pre-action state if the user installed/upgraded/
// removed a tool while the fetch was in flight.
type complianceLoadedMsg struct {
	policy   *compliance.Policy
	errorMsg string
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

// SetStdin sets the stdin for the tool action command.
func (c *toolActionCmd) SetStdin(r io.Reader) { c.stdin = r }

// SetStdout sets the stdout for the tool action command.
func (c *toolActionCmd) SetStdout(w io.Writer) { c.stdout = w }

// SetStderr sets the stderr for the tool action command.
func (c *toolActionCmd) SetStderr(w io.Writer) { c.stderr = w }

// Run executes the tool action command.
func (c *toolActionCmd) Run() error {
	// Apply os.Std* fallbacks so command I/O works even if Bubble Tea
	// didn't set the fields (nil stdio would discard output).
	stdin := c.stdin
	if stdin == nil {
		stdin = os.Stdin
	}
	stdout := c.stdout
	if stdout == nil {
		stdout = os.Stdout
	}
	stderr := c.stderr
	if stderr == nil {
		stderr = os.Stderr
	}

	// Clear screen before running command so previous exec output doesn't persist.
	_, _ = fmt.Fprint(stdout, "\033[2J\033[H") // ANSI: clear screen + cursor home

	cmd := exec.Command(c.args[0], c.args[1:]...)
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	runErr := cmd.Run()

	// Log exit code for debugging.
	var exitErr *exec.ExitError
	hasExitCode := errors.As(runErr, &exitErr)
	exitCode := -1
	if hasExitCode {
		exitCode = exitErr.ExitCode()
	} else if runErr == nil {
		exitCode = 0
	}
	slog.Info("tool action finished", "action", c.action, "cmd", c.args, "exitCode", exitCode, "err", runErr)

	// Show result and wait for keypress — terminal is still ours.
	if runErr != nil {
		if hasExitCode {
			_, _ = fmt.Fprintf(stderr, "\n✗ %s failed (exit code %d)\n", c.action, exitCode)
		} else {
			_, _ = fmt.Fprintf(stderr, "\n✗ %s failed: %s\n", c.action, runErr)
		}
		if hint := actionFailureHint(c.args, exitCode); hint != "" {
			_, _ = fmt.Fprintf(stderr, "\n%s\n", hint)
		}
	} else {
		_, _ = fmt.Fprintf(stderr, "\n✓ %s completed (exit code 0)\n", c.action)
	}
	_, _ = fmt.Fprint(stderr, "\nPress Enter to return to clim...")

	// Read until newline so buffered stdin doesn't skip the pause.
	br := bufio.NewReader(stdin)
	_, _ = br.ReadString('\n')

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

// loadComplianceURLCmd asynchronously fetches the compliance policy
// from cfg.Compliance.URL (with cache + auto-refresh) and returns a
// complianceLoadedMsg carrying just the parsed Policy. The Index is
// deliberately NOT pre-built here — the receiver builds it against
// its current tools slice so a tool change in flight isn't lost.
//
// Returns nil when there's nothing to fetch (no URL configured) so
// callers can unconditionally tea.Batch it.
func loadComplianceURLCmd(cfg *config.Config) tea.Cmd {
	if cfg == nil || cfg.Compliance.URL == "" {
		return nil
	}
	url := cfg.Compliance.URL
	autoRefresh := cfg.Compliance.AutoRefresh
	maxAge := cfg.Compliance.RefreshInterval.Duration
	return func() tea.Msg {
		ctx := context.Background()
		fetcher := &compliance.HTTPFetcher{URL: url}
		opts := compliance.LoadOptions{}
		if autoRefresh && maxAge > 0 {
			opts.MaxAge = maxAge
		}
		policy, _, err := compliance.LoadOrFetch(ctx, fetcher, opts)
		if err != nil {
			return complianceLoadedMsg{errorMsg: fmt.Sprintf("Failed to load policy from %s: %v", url, err)}
		}
		return complianceLoadedMsg{policy: policy}
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

		if err := fileutil.AtomicWrite(filename, []byte(header+string(data)), 0o644); err != nil {
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

		if err := fileutil.AtomicWrite(filename, []byte(header+string(data)), 0o644); err != nil {
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
