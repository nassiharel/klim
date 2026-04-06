package tui

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"gopkg.in/yaml.v3"

	"github.com/nassiharel/clim/internal/detector"
	"github.com/nassiharel/clim/internal/finder"
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
	cmdStr  string
}

type sourceChoice struct {
	source registry.InstallSource
	cmd    string
}

type sourcePicker struct {
	toolIdx int
	action  string
	choices []sourceChoice
}

// --- Transfer types ---

type transferStatus int

const (
	transferPending transferStatus = iota
	transferRunning
	transferDone
	transferFailed
	transferSkipped
)

type transferItem struct {
	name    string
	display string
	cmd     string // install command (import) or "" (export)
	source  string
	status  transferStatus
	errMsg  string
}

// --- Transfer messages ---

type exportFinishedMsg struct {
	path  string
	count int
	err   error
}

type transferPlanMsg struct {
	items []transferItem
}

type transferItemDoneMsg struct {
	idx int
	err error
}

// --- Scan & version commands ---

func findToolsCmd() func() scanResultMsg {
	return func() scanResultMsg {
		tools := registry.DefaultTools()
		err := finder.FindAll(tools)
		return scanResultMsg{tools: tools, err: err}
	}
}

func resolveToolVersionCmd(index int, tool registry.Tool) func() toolVersionMsg {
	return func() toolVersionMsg {
		if tool.IsInstalled() && !tool.Disabled {
			pkgmgr.ResolveOne(&tool)
			detector.EnrichOne(&tool)
		}
		return toolVersionMsg{index: index, tool: tool}
	}
}

// --- Single-tool action commands ---

func execToolActionCmd(pa pendingAction) tea.Cmd {
	cmd := buildShellCmd(pa.cmdStr)
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

func buildShellCmd(cmdStr string) *exec.Cmd {
	if runtime.GOOS == "windows" {
		return exec.Command("cmd", "/C", cmdStr)
	}
	return exec.Command("sh", "-c", cmdStr)
}

// --- Export manifest types ---

type exportManifest struct {
	GeneratedBy string           `yaml:"generated_by"`
	OS          string           `yaml:"os"`
	Arch        string           `yaml:"arch"`
	Tools       []exportToolItem `yaml:"tools"`
}

type exportToolItem struct {
	Name        string             `yaml:"name"`
	DisplayName string             `yaml:"display_name"`
	Version     string             `yaml:"version,omitempty"`
	Source      string             `yaml:"source,omitempty"`
	Category    string             `yaml:"category"`
	Packages    exportToolPackages `yaml:"packages,omitempty"`
}

type exportToolPackages struct {
	Winget string `yaml:"winget,omitempty"`
	Choco  string `yaml:"choco,omitempty"`
	Brew   string `yaml:"brew,omitempty"`
	Apt    string `yaml:"apt,omitempty"`
	Snap   string `yaml:"snap,omitempty"`
	NPM    string `yaml:"npm,omitempty"`
}

// --- Export command ---

func exportToolsCmd(tools []registry.Tool) tea.Cmd {
	return func() tea.Msg {
		sorted := make([]registry.Tool, len(tools))
		copy(sorted, tools)
		sort.Slice(sorted, func(i, j int) bool {
			return strings.ToLower(sorted[i].Name) < strings.ToLower(sorted[j].Name)
		})

		var exported []exportToolItem
		for _, tool := range sorted {
			if !tool.IsInstalled() || tool.Disabled {
				continue
			}
			primary := tool.PrimaryInstance()
			exported = append(exported, exportToolItem{
				Name:        tool.Name,
				DisplayName: tool.DisplayName,
				Version:     primary.Version,
				Source:      string(primary.Source),
				Category:    tool.Category,
				Packages: exportToolPackages{
					Winget: tool.Packages.Winget,
					Choco:  tool.Packages.Choco,
					Brew:   tool.Packages.Brew,
					Apt:    tool.Packages.Apt,
					Snap:   tool.Packages.Snap,
					NPM:    tool.Packages.NPM,
				},
			})
		}

		manifest := exportManifest{
			GeneratedBy: "clim export",
			OS:          runtime.GOOS,
			Arch:        runtime.GOARCH,
			Tools:       exported,
		}

		data, err := yaml.Marshal(&manifest)
		if err != nil {
			return exportFinishedMsg{err: err}
		}

		filename := fmt.Sprintf("clim-export-%s.yaml", time.Now().Format("2006-01-02"))
		header := "# clim — Installed Tools Manifest\n# Generated on " + runtime.GOOS + "/" + runtime.GOARCH + "\n#\n# Reinstall on a new machine:\n#   clim import " + filename + "\n#\n\n"

		if err := os.WriteFile(filename, []byte(header+string(data)), 0o644); err != nil {
			return exportFinishedMsg{err: err}
		}

		return exportFinishedMsg{path: filename, count: len(exported)}
	}
}

// --- Import commands ---

// buildImportPlanCmd reads a manifest, scans PATH, and builds a transfer plan.
func buildImportPlanCmd(path string) tea.Cmd {
	return func() tea.Msg {
		data, err := os.ReadFile(path)
		if err != nil {
			return transferPlanMsg{items: []transferItem{{
				name: "error", display: "Error", status: transferFailed,
				errMsg: fmt.Sprintf("reading manifest: %v", err),
			}}}
		}

		var manifest exportManifest
		if err := yaml.Unmarshal(data, &manifest); err != nil {
			return transferPlanMsg{items: []transferItem{{
				name: "error", display: "Error", status: transferFailed,
				errMsg: fmt.Sprintf("parsing manifest: %v", err),
			}}}
		}

		// Load registry and scan PATH.
		regTools := registry.DefaultTools()
		if err := finder.FindAll(regTools); err != nil {
			return transferPlanMsg{items: []transferItem{{
				name: "error", display: "Error", status: transferFailed,
				errMsg: fmt.Sprintf("scanning PATH: %v", err),
			}}}
		}

		regMap := make(map[string]*registry.Tool, len(regTools))
		for i := range regTools {
			regMap[regTools[i].Name] = &regTools[i]
		}

		var items []transferItem
		for _, mt := range manifest.Tools {
			rt, exists := regMap[mt.Name]

			if exists && rt.IsInstalled() {
				items = append(items, transferItem{
					name:    mt.Name,
					display: mt.DisplayName,
					source:  "—",
					status:  transferSkipped,
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
			installCmd := pkgs.InstallCmd(src)
			if installCmd == "" {
				items = append(items, transferItem{
					name:    mt.Name,
					display: mt.DisplayName,
					status:  transferSkipped,
					errMsg:  "no package for " + runtime.GOOS,
				})
				continue
			}

			items = append(items, transferItem{
				name:    mt.Name,
				display: mt.DisplayName,
				cmd:     installCmd,
				source:  string(src),
				status:  transferPending,
			})
		}

		return transferPlanMsg{items: items}
	}
}

// execTransferInstallCmd suspends the TUI and runs one install command.
func execTransferInstallCmd(idx int, cmdStr string) tea.Cmd {
	cmd := buildShellCmd(cmdStr)
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		return transferItemDoneMsg{idx: idx, err: err}
	})
}
