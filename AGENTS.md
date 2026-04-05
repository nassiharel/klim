# AGENTS.md — clim Codebase Guide

> **Purpose:** Authoritative reference for AI coding agents (and humans) working on this repository.  
> Covers architecture, conventions, data-flow, testing rules, and contribution guidelines.

---

## Table of Contents

1. [Project Overview](#1-project-overview)
2. [Repository Layout](#2-repository-layout)
3. [Architecture & Data Flow](#3-architecture--data-flow)
4. [Package Reference](#4-package-reference)
5. [Key Types & Interfaces](#5-key-types--interfaces)
6. [Coding Conventions](#6-coding-conventions)
7. [Adding a New Tool](#7-adding-a-new-tool)
8. [Adding a New Version Source](#8-adding-a-new-version-source)
9. [Testing](#9-testing)
10. [Build & Local Dev](#10-build--local-dev)
11. [CI/CD & Release](#11-cicd--release)
12. [Dependencies](#12-dependencies)
13. [Environment Variables](#13-environment-variables)
14. [Cache](#14-cache)
15. [Known Constraints & Gotchas](#15-known-constraints--gotchas)

---

## 1. Project Overview

**clim** is a cross-platform developer-tool manager written in Go.
Module path: `github.com/nassiharel/clim` · Go: `1.25`

It does three things:

| Mode | Trigger | What it does |
|---|---|---|
| Interactive TUI | `clim` (stdout is a TTY) | Bubbletea full-screen table: detect + latest version concurrently, keyboard nav, filter, refresh |
| Non-interactive list | `clim list` or piped | Tab-aligned table to stdout via `text/tabwriter` |
| Single-tool | `clim check <name>` / `clim upgrade <name>` | Detects one tool, queries its latest version, or runs the native package-manager upgrade command |

Supported tools (defined in `internal/registry/tool.go`):

`az`, `azd`, `gh`, `copilot`, `kubectl`, `docker`, `terraform`, `helm`, `go`, `node`, `python`, `git`

---

## 2. Repository Layout

```
clim/
├── cmd/
│   └── clim/
│       └── main.go               # Entry point — calls cli.Execute()
├── internal/
│   ├── build/
│   │   └── build.go              # Version/Commit/Date vars (ldflags) + Info()
│   ├── cli/
│   │   ├── root.go               # rootCmd (cobra), Execute(), --no-color flag
│   │   ├── check.go              # `clim check <tool>` subcommand
│   │   ├── list.go               # `clim list` subcommand + runList() (used as TTY fallback)
│   │   ├── upgrade.go            # `clim upgrade <tool>` subcommand
│   │   └── version.go            # `clim version` subcommand — prints build.Info()
│   ├── detector/
│   │   ├── detector.go           # DetectOne, DetectAll, ParseVersion
│   │   └── detector_test.go      # Table-driven ParseVersion tests (16 cases)
│   ├── registry/
│   │   ├── tool.go               # Tool struct + DefaultTools() — the tool catalogue
│   │   └── source.go             # VersionSource struct + SourceType constants
│   ├── tui/
│   │   ├── model.go              # Bubbletea Model, ToolRow, NewModel, Init, Update, View, Run
│   │   ├── commands.go           # DetectionCompleteMsg, LatestVersionMsg, tea.Cmd factories
│   │   ├── view.go               # renderView, renderRow, renderHeader, renderSeparator, renderHelp
│   │   └── styles.go             # lipgloss colour palette and style variables
│   ├── updater/
│   │   └── updater.go            # Upgrade, InstallCmd, UpgradeCmd, toUpgradeCmd
│   └── version/
│       ├── checker.go            # Checker interface, HTTPChecker, CheckAll, cacheKey, TokenFromEnv
│       ├── compare.go            # Status type, CompareVersions, StatusString, StatusIcon
│       ├── cache.go              # Cache struct, LoadCache, Get, Set, Save (1-hour TTL, JSON on disk)
│       ├── github.go             # latestGitHub — GitHub Releases API
│       ├── pypi.go               # latestPyPI — PyPI JSON API
│       ├── npm.go                # latestNPM — npm registry API
│       └── custom.go             # latestCustom — go.dev, nodejs.org, endoflife.date
├── .github/
│   └── workflows/
│       ├── ci.yml                # Build + test (3-OS matrix) + lint/vet on every push/PR
│       └── release.yml           # GoReleaser on v* tag push
├── .golangci.yml                 # standard linters + misspell + gofmt; excludes dist/ bin/
├── .goreleaser.yml               # CGO_ENABLED=0, trimpath, ldflags, darwin+linux+windows amd64/arm64
├── .gitignore
├── go.mod
├── go.sum
├── Makefile
├── LICENSE
└── README.md
```

---

## 3. Architecture & Data Flow

### 3.1 Concurrency Model

Everything that touches the filesystem or network is run concurrently using **goroutines + `sync.WaitGroup`**.
There are no channels used for results — goroutines write into a pre-allocated results slice by index, which is safe because each goroutine owns its own index slot.

```
DefaultTools()
    │
    ├──[parallel]──► DetectAll  ──► []DetectionResult  (exec.LookPath + run binary)
    │
    └──[parallel]──► CheckAll   ──► []LatestVersion    (HTTP APIs, cache-first)
```

In the TUI the same work is done via **Bubbletea `tea.Cmd`** functions (one `detectToolCmd` + one `checkLatestCmd` per tool), which return `tea.Msg` values (`DetectionCompleteMsg`, `LatestVersionMsg`) processed in `Model.Update`.

### 3.2 TUI Message Flow

```
Model.Init()
  └─► tea.Batch(spinner.Tick, detectToolCmd×N, checkLatestCmd×N)
        │
        ▼
  goroutines run concurrently in bubbletea's event loop
        │
        ▼
  DetectionCompleteMsg{Index, Result}  ──► Update() ──► row.DetectDone=true, recalculateStatus()
  LatestVersionMsg{Index, Result}      ──► Update() ──► row.LatestDone=true, recalculateStatus()
```

`recalculateStatus` waits until **both** `DetectDone` and `LatestDone` are true for a row before calling `CompareVersions`.

### 3.3 Version Comparison Pipeline

```
installed string  ──► strings.TrimPrefix("v")
latest string     ──► semver.ParseTolerant()  ──► iv.GTE(lv) ? StatusUpToDate : StatusUpgradable
```

Uses `github.com/blang/semver/v4` with `ParseTolerant` (handles missing patch component, e.g. `"1.23"`).

### 3.4 Cache

- Loaded at startup by `version.LoadCache()` (reads `/clim/cache.json`).
- Thread-safe via `sync.RWMutex`.
- TTL = **1 hour** (constant `cacheTTL` in `cache.go`).
- Written to disk by `cache.Save()` at program exit (TUI: after `p.Run()`; list/check: after table flush).
- Cache keys: `"github:owner/repo"`, `"pypi:package"`, `"npm:package"`, `"custom:urlPattern"`.

---

## 4. Package Reference

| Package | Responsibility | Public surface |
|---|---|---|
| `cmd/clim` | Binary entry point | `main()` |
| `internal/build` | Compile-time metadata | `Version`, `Commit`, `Date`, `Info()` |
| `internal/registry` | Tool catalogue, source descriptors | `Tool`, `VersionSource`, `SourceType`, `DefaultTools()` |
| `internal/detector` | Binary detection, version parsing | `DetectionResult`, `DetectOne()`, `DetectAll()`, `ParseVersion()` |
| `internal/version` | Latest version checks, caching, comparison | `Checker`, `HTTPChecker`, `LatestVersion`, `Cache`, `Status`, `CompareVersions()`, `StatusString()`, `StatusIcon()`, `CheckAll()`, `LoadCache()`, `TokenFromEnv()` |
| `internal/updater` | Platform-specific upgrade execution | `Upgrade()`, `UpgradeCmd()`, `InstallCmd()` |
| `internal/tui` | Bubbletea interactive UI | `Model`, `ToolRow`, `NewModel()`, `Run()` |
| `internal/cli` | Cobra command tree | `Execute()`, subcommands |

All packages live under `internal/` — nothing is exported for external consumption.

---

## 5. Key Types & Interfaces

### `registry.Tool` (`internal/registry/tool.go`)

```go
type Tool struct {
    Name         string              // short CLI id: "az", "gh"
    DisplayName  string              // human label: "Azure CLI"
    BinaryNames  []string            // binaries tried in order: ["python3", "python"]
    VersionArgs  []string            // args to pass: ["--version"]
    VersionRegex string              // one capture group
    LatestSource VersionSource       // where to check for updates
    InstallCmds  map[string][]string // runtime.GOOS -> command
    Homepage     string
}
```

### `registry.VersionSource` (`internal/registry/source.go`)

```go
type SourceType string

const (
    SourceGitHub SourceType = "github"
    SourcePyPI   SourceType = "pypi"
    SourceNPM    SourceType = "npm"
    SourceCustom SourceType = "custom"
)

type VersionSource struct {
    Type       SourceType
    Repo       string // "owner/repo" for GitHub
    Package    string // for PyPI / npm
    URLPattern string // for Custom -- URL used to dispatch in custom.go
}
```

### `detector.DetectionResult` (`internal/detector/detector.go`)

```go
type DetectionResult struct {
    Found   bool
    Version string // empty if not parseable
    Path    string // absolute path to binary
    Error   error  // non-fatal (e.g. regex miss)
}
```

### `version.Checker` interface (`internal/version/checker.go`)

```go
type Checker interface {
    Latest(ctx context.Context, source registry.VersionSource) LatestVersion
}

type LatestVersion struct {
    Version string
    Error   error
}
```

`HTTPChecker` is the only concrete implementation. Its `baseURL` field is empty in production but overridable for testing (not yet wired to any tests — future opportunity).

### `version.Status` (`internal/version/compare.go`)

```go
type Status int

const (
    StatusLoading      Status = iota // 0 -- waiting for results
    StatusUpToDate                   // 1 -- installed >= latest
    StatusUpgradable                 // 2 -- latest > installed
    StatusNotInstalled               // 3 -- binary not found
    StatusError                      // 4 -- parse or API failure
)
```

### `version.Cache` (`internal/version/cache.go`)

```go
type Cache struct {
    path    string
    mu      sync.RWMutex
    entries map[string]CacheEntry // key -> {Version, FetchedAt}
}
```

`LoadCache()` always returns a non-nil `*Cache` (empty on any OS/IO error).

### `tui.Model` / `tui.ToolRow` (`internal/tui/model.go`)

```go
type ToolRow struct {
    Tool         registry.Tool
    InstalledVer string
    LatestVer    string
    Path         string
    Status       version.Status
    DetectDone   bool
    LatestDone   bool
}

type Model struct {
    tools         []ToolRow
    cursor        int
    spinner       spinner.Model
    filterInput   textinput.Model
    filtering     bool
    filterText    string
    filteredIndex []int          // maps visible row index -> tools slice index
    loading       int            // countdown: starts at len(tools)*2
    width, height int
    ctx           context.Context
    checker       version.Checker
    cache         *version.Cache
    quitting      bool
}
```

`loading` starts at `len(tools) * 2` (one detect + one latest per tool). The spinner renders while `loading > 0`.

---

## 6. Coding Conventions

### 6.1 Error Handling

- Functions return `error` as the last return value. Never panic on user-facing paths.
- Wrap errors with `fmt.Errorf("context: %w", err)` — the `%w` verb is used consistently throughout.
- Non-fatal errors (e.g. a tool binary exists but its version cannot be parsed) are stored in `DetectionResult.Error` or `LatestVersion.Error` and surfaced as `StatusError`, never causing a crash.
- Cache `Save()` and `json.Unmarshal` errors during load are silently discarded (`_ = ...`) — caching is best-effort.
- HTTP errors always include the source name in the message, e.g. `"github %s: status %d"`, `"pypi %s: status %d"`.

### 6.2 Naming

- Types: `PascalCase`. Acronyms follow Go convention: `HTTPChecker`, not `HttpChecker`.
- Functions/methods: `camelCase` for unexported, `PascalCase` for exported.
- Private helpers use descriptive names: `toUpgradeCmd`, `extractSemver`, `cacheKeyForTool`.
- Cobra `RunE` functions are named `run<CommandName>` (e.g. `runCheck`, `runList`, `runUpgrade`).
- Bubbletea message types end in `Msg`: `DetectionCompleteMsg`, `LatestVersionMsg`.
- Bubbletea `tea.Cmd` factory functions end in `Cmd`: `detectToolCmd`, `checkLatestCmd`.

### 6.3 Import Grouping

Three groups, separated by blank lines, in this order:

```go
import (
    // 1. Standard library
    "context"
    "fmt"
    "os"

    // 2. Third-party
    tea "charm.land/bubbletea/v2"
    "github.com/spf13/cobra"

    // 3. Internal (this module)
    "github.com/nassiharel/clim/internal/registry"
    "github.com/nassiharel/clim/internal/version"
)
```

### 6.4 Comment Style

- Every exported type and function has a doc comment beginning with its name.
- Unexported helpers may have shorter comments or inline `//` comments.
- Section dividers within a file use `// --- Section name ---`.
- Inline comments explain non-obvious behaviour (e.g. "Some tools write version info even on non-zero exit. Combine stdout and stderr for regex matching.").

### 6.5 Cobra Subcommands

- Each subcommand lives in its own file (`check.go`, `list.go`, `upgrade.go`, `version.go`).
- The `var xxxCmd = &cobra.Command{...}` variable is unexported.
- Registration happens in the file's own `func init()` calling `rootCmd.AddCommand(xxxCmd)`.
- `SilenceUsage: true` and `SilenceErrors: true` are set on `rootCmd` — errors are printed manually via `fmt.Fprintln(os.Stderr, err)`.

### 6.6 Concurrency Patterns

- Pre-allocate result slices (`make([]T, len(input))`) so goroutines write by index without locking.
- Always `defer wg.Done()` immediately after `wg.Add(1)`.
- Pass loop variables explicitly into goroutine closures: `go func(idx int, t registry.Tool) {...}(i, tool)`.
- Use `context.WithTimeout` at the call site, not inside library functions.

### 6.7 TUI Patterns (Bubbletea v2)

- `Model.View()` returns `tea.View` (not `string`) with `AltScreen: true`.
- All rendering lives in `view.go`; all message/command logic lives in `model.go` and `commands.go`.
- Styles are defined as package-level `var` blocks in `styles.go` using `lipgloss.NewStyle()`.
- The `filteredIndex` slice is the single source of truth for visible rows; `cursor` indexes into it.

### 6.8 Formatting & Linting

- **`gofmt`** — enforced by CI (`gofmt -l .` must produce no output).
- **`go vet`** — enforced by CI.
- **`golangci-lint`** — uses `standard` linters plus `misspell` and `gofmt`; excludes `dist/` and `bin/`.
- Run locally: `make lint` (requires `golangci-lint` in PATH).

