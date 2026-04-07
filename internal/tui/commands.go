package tui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"gopkg.in/yaml.v3"

	"github.com/nassiharel/clim/internal/detector"
	"github.com/nassiharel/clim/internal/finder"
	"github.com/nassiharel/clim/internal/manifest"
	"github.com/nassiharel/clim/internal/pkgmgr"
	"github.com/nassiharel/clim/internal/registry"
)

// --- Scan & version resolution messages ---

type scanResultMsg struct {
	tools []registry.Tool
	err   error // non-nil if PATH scanning failed
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

// toolInfoMsg is sent when tool metadata has been fetched.
type toolInfoMsg struct {
	toolIdx int
	info    *registry.ToolInfo
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
	label  string        // "Upgrade", "Remove", "Install"
	picker *sourcePicker // resolved sources for this action
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
	name    string
	display string
	cmdArgs []string // install command args (import) or nil (export)
	source  string
	status  backupStatus
	errMsg  string
}

// --- Backup messages ---

type exportFinishedMsg struct {
	path  string
	count int
	err   error
}

type backupPlanMsg struct {
	items []backupItem
}

type backupItemDoneMsg struct {
	idx int
	err error
}

// backupTickMsg advances the animated progress by marking the next pending item as done.
type backupTickMsg struct{}

// --- Scan & version commands ---

func findToolsCmd() func() scanResultMsg {
	return func() scanResultMsg {
		tools := registry.DefaultTools()
		err := finder.FindAll(tools)
		return scanResultMsg{tools: tools, err: err}
	}
}

func resolveToolVersionCmd(index int, gen int, tool registry.Tool) func() toolVersionMsg {
	return func() toolVersionMsg {
		if tool.IsInstalled() && !tool.Disabled {
			pkgmgr.ResolveOne(&tool)
			detector.EnrichOne(&tool)
		}
		return toolVersionMsg{index: index, gen: gen, tool: tool}
	}
}

// --- Single-tool action commands ---

func execToolActionCmd(pa pendingAction) tea.Cmd {
	cmd := exec.Command(pa.cmdArgs[0], pa.cmdArgs[1:]...)
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		return execFinishedMsg{
			toolIdx: pa.toolIdx,
			action:  pa.action,
			err:     err,
		}
	})
}

func refreshSingleToolCmd(idx int, tool registry.Tool) tea.Cmd {
	return func() tea.Msg {
		singleTool := []registry.Tool{tool}
		_ = finder.FindAll(singleTool) // best-effort: user already warned on initial scan
		tool = singleTool[0]
		if tool.IsInstalled() {
			pkgmgr.ResolveOne(&tool)
			detector.EnrichOne(&tool)
		}
		return refreshToolMsg{toolIdx: idx, tool: tool}
	}
}

// fetchToolInfoCmd fetches rich metadata for a tool in the background.
func fetchToolInfoCmd(idx int, tool registry.Tool) tea.Cmd {
	return func() tea.Msg {
		pkgmgr.FetchToolInfo(&tool)
		return toolInfoMsg{toolIdx: idx, info: tool.Info}
	}
}

// --- Export command ---

func exportToolsCmd(tools []registry.Tool) tea.Cmd {
	return func() tea.Msg {
		sorted := make([]registry.Tool, len(tools))
		copy(sorted, tools)
		sort.Slice(sorted, func(i, j int) bool {
			return strings.ToLower(sorted[i].Name) < strings.ToLower(sorted[j].Name)
		})

		var exported []manifest.Tool
		for _, tool := range sorted {
			if !tool.IsInstalled() || tool.Disabled {
				continue
			}
			primary := tool.PrimaryInstance()
			exported = append(exported, manifest.Tool{
				Name:        tool.Name,
				DisplayName: tool.DisplayName,
				Version:     primary.Version,
				Source:      string(primary.Source),
				Category:    tool.Category,
				Packages: manifest.Packages{
					Winget: tool.Packages.Winget,
					Choco:  tool.Packages.Choco,
					Brew:   tool.Packages.Brew,
					Apt:    tool.Packages.Apt,
					Snap:   tool.Packages.Snap,
					NPM:    tool.Packages.NPM,
				},
			})
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

		filename := fmt.Sprintf("clim-export-%s.yaml", time.Now().Format("2006-01-02"))
		header := "# clim — Installed Tools Manifest\n# Generated on " + runtime.GOOS + "/" + runtime.GOARCH + "\n#\n# Reinstall on a new machine:\n#   clim import " + filename + "\n#\n\n"

		if err := os.WriteFile(filename, []byte(header+string(data)), 0o644); err != nil {
			return exportFinishedMsg{err: err}
		}

		if abs, err := filepath.Abs(filename); err == nil {
			filename = abs
		}

		return exportFinishedMsg{path: filename, count: len(exported)}
	}
}

// --- Import commands ---

// buildImportPlanCmd reads a manifest, scans PATH, and builds a backup plan.
func buildImportPlanCmd(path string) tea.Cmd {
	return func() tea.Msg {
		data, err := os.ReadFile(path)
		if err != nil {
			return backupPlanMsg{items: []backupItem{{
				name: "error", display: "Error", status: backupFailed,
				errMsg: fmt.Sprintf("reading manifest: %v", err),
			}}}
		}

		var m manifest.Manifest
		if err := yaml.Unmarshal(data, &m); err != nil {
			return backupPlanMsg{items: []backupItem{{
				name: "error", display: "Error", status: backupFailed,
				errMsg: fmt.Sprintf("parsing manifest: %v", err),
			}}}
		}

		// Load registry and scan PATH.
		regTools := registry.DefaultTools()
		if err := finder.FindAll(regTools); err != nil {
			return backupPlanMsg{items: []backupItem{{
				name: "error", display: "Error", status: backupFailed,
				errMsg: fmt.Sprintf("scanning PATH: %v", err),
			}}}
		}

		regMap := make(map[string]*registry.Tool, len(regTools))
		for i := range regTools {
			regMap[regTools[i].Name] = &regTools[i]
		}

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
				name:    mt.Name,
				display: mt.DisplayName,
				cmdArgs: installArgs,
				source:  string(src),
				status:  backupPending,
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
