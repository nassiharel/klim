# AGENTS.md — clim Codebase Guide

> **Purpose:** Authoritative reference for AI coding agents (and humans) working on this repository.  
> Covers architecture, conventions, data-flow, testing rules, and contribution guidelines.

---

## Table of Contents

1. [Project Overview](#1-project-overview)
2. [Repository Layout](#2-repository-layout)
3. [Architecture & Data Flow](#3-architecture--data-flow)
4. [Package Reference](#4-package-reference)
5. [Key Types](#5-key-types)
6. [Coding Conventions](#6-coding-conventions)
7. [Adding a New Tool](#7-adding-a-new-tool)
8. [Adding a New Package Manager](#8-adding-a-new-package-manager)
9. [Testing](#9-testing)
10. [Build & Local Dev](#10-build--local-dev)
11. [CI/CD & Release](#11-cicd--release)
12. [Dependencies](#12-dependencies)
13. [Known Constraints & Gotchas](#13-known-constraints--gotchas)

---

## 1. Project Overview

**clim** is a cross-platform developer-tool manager written in Go.
Module path: `github.com/nassiharel/clim` · Go: `1.25`

It does three things:

| Mode | Trigger | What it does |
|---|---|---|
| Interactive TUI | `clim` (stdout is a TTY) | Bubbletea full-screen with 6 tabs: Installed, Updates, Discover, Disabled, Backup, Config |
| Non-interactive list | `clim list` or piped | Tab-aligned table to stdout via `text/tabwriter` |
| Subcommands | `clim export/import/open/share/update/tools/config/version` | Specific operations |

Supported tools are defined in `marketplace.yaml` (70+ curated tools). The catalog is fetched from GitHub at runtime, cached locally, and merged with the user's customizations on startup.

---

## 2. Repository Layout

```
clim/
├── cmd/
│   └── clim/
│       └── main.go               # Entry point — calls cli.Execute()
├── marketplace.yaml               # Curated tool catalog (70+ tools, all config)
├── internal/
│   ├── build/
│   │   └── build.go              # Version/Commit/Date vars (ldflags) + Info(), VersionOnly()
│   ├── catalog/
│   │   └── catalog.go            # GitHubFetcher, LoadOrFetch, cache, Diff, Refresh
│   ├── cli/
│   │   ├── root.go               # rootCmd (cobra), Execute(), TUI vs list dispatch
│   │   ├── list.go               # `clim list` subcommand + runList() (used as TTY fallback)
│   │   ├── version.go            # `clim version` subcommand — prints build.Info()
│   │   ├── tools.go              # `clim tools` + `clim tools path/edit` subcommands
│   │   ├── config.go             # `clim config` + `clim config path/edit` subcommands
│   │   ├── export.go             # `clim export` subcommand
│   │   ├── import.go             # `clim import <file>` subcommand
│   │   ├── open.go               # `clim open <token>` subcommand — install from share token
│   │   ├── share.go              # `clim share` subcommand — generate share token
│   │   ├── installplan.go        # Shared install plan types and execution helpers
│   │   └── update.go             # `clim update` subcommand — self-update from GitHub Releases
│   ├── config/
│   │   └── config.go             # Config struct, YAML load/save, defaults, Duration type
│   ├── detector/
│   │   ├── detector.go           # Fallback binary version detection (Go buildinfo)
│   │   ├── pe_windows.go         # Windows PE resource version extraction
│   │   └── pe_stub.go            # PE stub for non-Windows
│   ├── finder/
│   │   ├── finder.go             # PATH scanning, install source detection
│   │   ├── path_windows.go       # Windows registry PATH merging
│   │   └── path_other.go         # Unix PATH handling
│   ├── manifest/
│   │   └── manifest.go           # YAML schema for export/import manifests
│   ├── pkgmgr/
│   │   └── pkgmgr.go             # Package manager queries (winget/brew/apt/choco/snap/npm)
│   ├── registry/
│   │   ├── known.go              # DefaultToolsFromBytes(): parses catalog + merges with user YAML
│   │   ├── tool.go               # Tool, Instance, PackageIDs structs; command builders
│   │   └── version.go            # VersionsMatch(), CompareVersions()
│   ├── selfupdate/
│   │   ├── selfupdate.go         # Update orchestrator: check → download → extract → replace
│   │   ├── github.go             # GitHub Releases API client
│   │   ├── archive.go            # tar.gz / zip extraction
│   │   ├── replace.go            # Cross-platform binary replacement (rename-swap)
│   │   ├── replace_unix.go       # Unix cleanup helper
│   │   └── replace_windows.go    # Windows cleanup helper
│   ├── service/
│   │   └── service.go            # ToolService: composition root wiring catalog, finder, resolver
│   ├── share/
│   │   └── share.go              # Compact token encode/decode for sharing tool lists
│   └── tui/
│       ├── model.go              # Bubbletea Model, Init/Update, all key handling
│       ├── commands.go           # tea.Cmd factories (find, resolve, exec, export, import)
│       ├── view.go               # All rendering (tabs, rows, detail, backup, config)
│       ├── styles.go             # lipgloss color palette and style variables
│       └── clipboard.go          # Clipboard interface + system clipboard wrapper
│
├── go.mod                        # Module deps
├── go.sum
├── Makefile                      # build/test/lint/run/clean targets
├── .goreleaser.yml               # Multi-platform release config
├── .golangci.yml                 # Linter config
├── .go-version                   # Go 1.25.9
├── README.md                     # User-facing docs
├── CONTRIBUTING.md               # Dev workflow docs
├── AGENTS.md                     # This file
└── marketplace.yaml              # Source-of-truth tool catalog (also fetched from GitHub at runtime)
```

---

## 3. Architecture & Data Flow

### 3.1 Tool Discovery & Version Resolution

All CLI commands and the TUI access tool discovery through **`ToolService`** (`internal/service`), which composes a `ToolCatalog`, `ToolFinder`, and `VersionResolver` behind clean interfaces.

```
                              ToolService
                                  │
                    ┌─────────────┼─────────────┐
                    ▼             ▼              ▼
              ToolCatalog    ToolFinder    VersionResolver
              (Catalog)       (Finder)      (Resolver)
                    │             │              │
                    ▼             ▼              ▼
            DefaultCatalog   PathFinder    PackageManagerResolver
            LoadOrFetch()    LookPath      ├── query installed version via PM
            fetch/cache      detect src    ├── query latest version via PM
            marketplace.yaml               └── detector.EnrichOne() fallback
```

**Pipelines exposed by `ToolService`:**

| Method | Pipeline | Used by |
|---|---|---|
| `LoadAndResolve()` | Catalog → Finder → Resolver (includes detector enrichment) | `clim list`, `clim export` |
| `ScanOnly()` | Catalog → Finder (no version resolution) | `clim import`, `clim open`, `clim share` |
| `LoadAndScan()` | Catalog → Finder (sorted, no versions) | TUI initial scan phase |
| `ResolveOne()` | Resolver for a single tool | TUI per-tool version resolution |
| `RefreshTool()` | Finder → Resolver for a single tool | TUI after install/upgrade/remove |

The **TUI** fires individual `tea.Cmd` goroutines per tool for PATH scanning (`findToolsCmd` → `svc.LoadAndScan`) and version resolution (`resolveToolVersionCmd` → `svc.ResolveOne`), counting down `m.pending` as results arrive.

The **CLI** calls `svc.LoadAndResolve()` which internally runs `Finder.FindAll` → `Resolver.ResolveVersions` (bounded worker pool with detector enrichment per tool).

### 3.2 TUI Message Flow

```
Model.Init()
  └─► tea.Batch(spinner.Tick, findToolsCmd)
        │
        ▼
  findToolsCmd runs svc.LoadAndScan and returns scanResultMsg
        │
        ▼
  scanResultMsg ──► Update() ──► dispatch resolveToolVersionCmd per installed tool
        │
        ▼
  toolVersionMsg×N ──► Update() ──► decrement m.pending, update tool in place
        │
        ▼
  pending == 0 ──► phase=2 (done), stop spinner
```

### 3.3 Version Comparison

Uses custom numeric segment comparison in `registry/version.go`:

- `parseSegments("1.23.14")` → `[1, 23, 14]` (stops at first non-numeric segment)
- `VersionsMatch` handles trailing `.0` segments and PE version padding (×100)
- `CompareVersions` returns -1/0/1 for segment-by-segment comparison

### 3.4 Marketplace YAML: Fetch, Cache & Merge

The `marketplace.yaml` in the repository is the source of truth, but it is **not embedded** into the binary. Instead, it is fetched at runtime from GitHub and cached locally by the `internal/catalog` package.

**Flow:**

```
GitHubFetcher.Fetch()                    marketplace-cache.yaml (local cache)
  │ GET raw.githubusercontent.com            │
  │ /nassiharel/clim/main/marketplace.yaml   │
  └──────────────┬───────────────────────────┘
                 │
           catalog.LoadOrFetch()
           1. Try reading marketplace-cache.yaml
           2. If valid → use it
           3. If missing/corrupt → fetch from GitHub → write cache
                 │
                 ▼
      registry.DefaultToolsFromBytes(data)
           1. Parse cached YAML as catalog definitions
           2. Read user YAML (~/.config/clim/marketplace.yaml)
           3. mergeToolDefs(): catalog is authority for display_name,
              category, binary_names, tags; user is authority for
              enabled flag and user-added custom tools
           4. User file is rewritten if anything changed
```

**Cache vs user file:**

| File | Path | Purpose |
|---|---|---|
| Remote cache | `~/.config/clim/marketplace-cache.yaml` | Last-fetched catalog from GitHub |
| User file | `~/.config/clim/marketplace.yaml` | User customizations (enabled/disabled, custom tools) |

**Refresh:** `catalog.Refresh()` re-fetches from GitHub, diffs against the user's file, and updates the cache. The merge in `registry.DefaultToolsFromBytes` incorporates new tools on the next load.

### 3.5 Self-Update Flow

`clim update` uses the `internal/selfupdate` package:

```
build.VersionOnly() ──► GitHub Releases API ──► CompareVersions()
                                                    │
                                              current >= latest → "Already up to date!"
                                              current < latest  ↓
                                        AssetURL(rel, GOOS, GOARCH)
                                                    │
                                              download archive
                                                    │
                                              ExtractBinary (tar.gz or zip)
                                                    │
                                              ReplaceBinary (rename-swap)
                                              1. write .new
                                              2. rename current → .old
                                              3. rename .new → current
                                              4. delete .old (best-effort)
```

---

## 4. Package Reference

| Package | Responsibility | Key exports |
|---|---|---|
| `cmd/clim` | Binary entry point | `main()` |
| `internal/build` | Compile-time metadata | `Version`, `Commit`, `Date`, `Info()`, `VersionOnly()` |
| `internal/catalog` | Fetch, cache, diff, refresh marketplace from GitHub | `GitHubFetcher`, `LoadOrFetch()`, `CachePath()`, `Diff()`, `Refresh()`, `RefreshResult` |
| `internal/cli` | Cobra command tree | `Execute()`, subcommands: `list`, `version`, `tools`, `config`, `export`, `import`, `open`, `share`, `update` |
| `internal/config` | Configuration file management | `Config`, `Default()`, `Load()`, `MustLoad()`, `Path()`, `Duration` |
| `internal/registry` | Tool catalogue, version comparison | `Tool`, `Instance`, `PackageIDs`, `DefaultToolsFromBytes()`, `VersionsMatch()`, `CompareVersions()`, `ToolsPath()`, `SetToolEnabled()` |
| `internal/service` | Composition root wiring catalog, finder, resolver | `ToolService`, `New()`, `NewWithConfig()`, `LoadAndResolve()`, `ScanOnly()`, `LoadAndScan()`, `ResolveOne()`, `RefreshTool()` |
| `internal/finder` | PATH scanning, source detection | `ToolFinder`, `NewFinder()`, `PathFinder`, `FindAll(tools)` |
| `internal/pkgmgr` | Package manager queries + detector enrichment | `VersionResolver`, `NewResolver()`, `PackageManagerResolver`, `ResolveVersions()`, `ResolveOne()`, `FetchToolInfo()` |
| `internal/detector` | Fallback version extraction | `EnrichOne(tool)` |
| `internal/manifest` | YAML schema for export/import | `Manifest`, `Tool`, `Packages` |
| `internal/share` | Compact token encode/decode for tool sharing | `Encode(names)`, `Decode(token)` |
| `internal/selfupdate` | Self-update from GitHub Releases | `Update(ctx, version, opts)`, `Result`, `Options` |
| `internal/tui` | Bubbletea interactive UI | `Model`, `NewModel()`, `Run()` |

All packages live under `internal/` — nothing is exported for external consumption.

---

## 5. Key Types

### `registry.Tool` (`internal/registry/tool.go`)

```go
type Tool struct {
    Name        string       // internal key: "az", "gh", "kubectl"
    DisplayName string       // human label: "Azure CLI", "GitHub CLI"
    Category    string       // e.g. "Cloud", "VCS", "Editor", "Shell"
    BinaryNames []string     // binaries searched on PATH: ["python3", "python"]
    Packages    PackageIDs   // package manager IDs
    Instances   []Instance   // found installations, PATH order (index 0 = active)
    Latest      string       // latest version string from package manager
    LatestFrom  string       // which PM reported the latest version
    Disabled    bool         // true = hidden from clim UI
    Info        *ToolInfo    // lazily fetched rich metadata (may be nil)
    InfoFetched bool         // true once a fetch attempt completed
}
```

**Key methods:** `PrimaryInstance()`, `InstalledVersion()`, `IsInstalled()`, `HasUpdate()`

### `registry.Instance`

```go
type Instance struct {
    Path    string        // absolute path to binary
    Version string        // installed version (may be empty)
    Source  InstallSource // detected install source: "brew", "winget", "apt", "manual"
}
```

### `registry.PackageIDs`

```go
type PackageIDs struct {
    Winget string `yaml:"winget,omitempty"`
    Choco  string `yaml:"choco,omitempty"`
    Brew   string `yaml:"brew,omitempty"`
    Apt    string `yaml:"apt,omitempty"`
    Snap   string `yaml:"snap,omitempty"`
    NPM    string `yaml:"npm,omitempty"`
}
```

**Key methods:** `InstallArgs(source)`, `UpgradeArgs(source)`, `RemoveArgs(source)`, `BestInstallSource()`, `HasAnyPackageForOS()`

### `selfupdate.Result`

```go
type Result struct {
    CurrentVersion string
    LatestVersion  string
    Updated        bool // true if a new binary was installed
}
```

**Key method:** `UpdateAvailable()` — reports whether a newer version exists

### `tui.Model` (`internal/tui/model.go`)

```go
type Model struct {
    tools         []registry.Tool
    cursor        int
    activeTab     int           // tabInstalled..tabConfig (6 tabs)
    phase         int           // 0=scanning, 1=resolving, 2=done
    pending       int           // tools still resolving versions
    filteredIndex []int         // visible row indices into tools
    // ... plus detail view, backup, filter, spinner, layout state
}
```

The TUI uses Bubbletea v2. All rendering lives in `view.go`; state updates and key handling in `model.go`; async commands in `commands.go`; styles in `styles.go`.

---

## 6. Coding Conventions

### 6.1 Error Handling

- Functions return `error` as the last return value. Never panic on user-facing paths.
- Wrap errors with `fmt.Errorf("context: %w", err)` — the `%w` verb is used consistently.
- Non-fatal errors in package manager queries return empty strings, never crash.
- Use `errors.New()` for static error messages, `fmt.Errorf()` only when wrapping or interpolating.

### 6.2 Output Routing

- **Progress/status messages** → `os.Stderr` (so piped output stays clean)
- **Data output** → `os.Stdout`

### 6.3 Naming

- Types: `PascalCase`. Acronyms follow Go convention: `HTTPClient`, not `HttpClient`.
- Functions/methods: `camelCase` for unexported, `PascalCase` for exported.
- Cobra `RunE` functions: `run<CommandName>` (e.g. `runList`, `runImport`, `runUpdate`).
- Bubbletea message types end in `Msg`: `scanResultMsg`, `toolVersionMsg`, `exportFinishedMsg`.
- Bubbletea `tea.Cmd` factory functions end in `Cmd`: `findToolsCmd`, `resolveToolVersionCmd`.

### 6.4 Import Grouping

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
    "github.com/nassiharel/clim/internal/build"
    "github.com/nassiharel/clim/internal/registry"
)
```

### 6.5 Cobra Subcommands

- Each subcommand lives in its own file (`list.go`, `export.go`, `update.go`, etc.).
- The `var xxxCmd = &cobra.Command{...}` variable is unexported.
- Registration happens in the file's own `func init()` calling `rootCmd.AddCommand(xxxCmd)`.
- `SilenceUsage: true` and `SilenceErrors: true` are set on `rootCmd` — errors are printed manually via `fmt.Fprintln(os.Stderr, err)`.
- Use `RunE` (not `Run`) so errors propagate through `Execute()`.

### 6.6 Concurrency Patterns

- Use `sync.WaitGroup` + semaphore channel for bounded concurrency in batch operations.
- Always `defer wg.Done()` immediately after `wg.Add(1)`.
- Pass loop variables explicitly into goroutine closures.
- Package manager subprocess calls use `context.WithTimeout` with a configurable timeout (default 10s via `config.yaml`).

### 6.7 TUI Patterns (Bubbletea v2)

- `Model.View()` returns `tea.View` (not `string`) with `AltScreen: true`.
- All rendering lives in `view.go`; all message/command logic lives in `model.go` and `commands.go`.
- Styles are defined as package-level `var` blocks in `styles.go` using `lipgloss.NewStyle()`.
- The `filteredIndex` slice is the single source of truth for visible rows; `cursor` indexes into it.
- External process execution (install/upgrade/remove) uses `tea.ExecProcess` to suspend the TUI.

### 6.8 Formatting & Linting

- **`gofmt`** — enforced by CI (`gofmt -l .` must produce no output).
- **`go vet`** — enforced by CI.
- **`golangci-lint`** — uses `standard` linters plus `misspell`, `gofmt`, `errcheck`, `gocritic`, `perfsprint`; excludes `dist/` and `bin/`.
- Run locally: `make lint` (requires `golangci-lint` v2+ in PATH).

---

## 7. Adding a New Tool

1. **Add the tool to `marketplace.yaml`:**
   ```yaml
   - name: mytool
     display_name: My Tool
     category: DevOps
     enabled: true
     binary_names: [mytool]
     packages:
       brew: "mytool"
       apt: "mytool"
       winget: "Publisher.MyTool"
   ```

2. That's it. The tool is automatically detected, version-checked, and manageable via the TUI and CLI on the next run. The catalog fetched from GitHub is merged with the user's local copy.

3. **Custom user-only tools** — users can add tools to their local `marketplace.yaml` that aren't in the upstream catalog. These are preserved across updates.

---

## 8. Adding a New Package Manager

1. Add a new `InstallSource` constant in `registry/tool.go` (e.g. `SourcePacman`).
2. Add command templates to the `commandTemplates` map in `tool.go`.
3. Add the source to the OS-priority switch blocks: `sourcePriority()`, `SourcesForOS()`, `AllPMStatusForOS()`.
4. Implement `pacmanInstalledVersion()` and `pacmanLatestVersion()` functions in `pkgmgr/pkgmgr.go`.
5. Wire them into `installedVersion()` and `latestVersion()` switch statements.
6. Add the package field to `PackageIDs` struct and its YAML tag.
7. Add the field to `pkgID()` switch, `PackageIDs` in `manifest/manifest.go`, and the `Packages` mapping in `export.go` and `commands.go`.
8. Add finder source detection in `finder/finder.go` (path-based heuristics).

---

## 9. Testing

- Run all tests: `make test` (includes `-race` detector)
- Run a single package: `go test ./internal/selfupdate/ -v -count=1`
- Coverage report: `make cover` → opens `coverage.html`

### Test patterns used:
- **Table-driven tests** for version comparison, path truncation, PM output parsing
- **`httptest.NewServer`** for GitHub API tests in `selfupdate`
- **In-memory archives** (tar.gz/zip built in test) for extraction tests
- **`t.TempDir()`** for filesystem operations (binary replacement)
- **`pmAvailableFunc`** test hook for overriding package manager availability detection

### Test files:
- `internal/registry/version_test.go`, `tool_test.go`, `known_test.go`
- `internal/pkgmgr/pkgmgr_test.go`
- `internal/finder/finder_test.go`
- `internal/share/share_test.go`
- `internal/selfupdate/selfupdate_test.go`

---

## 10. Build & Local Dev

```bash
make build          # compile to bin/clim with version ldflags
make run            # build and run
make test           # tests with -race
make lint           # golangci-lint
make tidy           # check go.mod tidiness
make vulncheck      # govulncheck
make cover          # HTML coverage report
make clean          # remove bin/ dist/ coverage files
make all            # lint + test + build (default)
```

**Version injection** via ldflags:
```
-X github.com/nassiharel/clim/internal/build.Version=$(git describe --tags)
-X github.com/nassiharel/clim/internal/build.Commit=$(git rev-parse --short HEAD)
-X github.com/nassiharel/clim/internal/build.Date=$(date -u +%Y-%m-%dT%H:%M:%SZ)
```

When installed via `go install`, version info is automatically read from Go module metadata (no ldflags needed).

---

## 11. CI/CD & Release

| Workflow | Trigger | What it does |
|---|---|---|
| `ci.yml` | Push to `main`, any PR | Lint, tidy check, test (Linux/macOS/Windows matrix), govulncheck |
| `release.yml` | Push of `v*` tag | GoReleaser: cross-compile, GitHub Release, Homebrew tap update, deb/rpm packages, SBOM |
| `codeql.yml` | Push to `main`, PRs, weekly | Static security analysis (Go) |
| `dependabot.yml` | Weekly Monday | Auto-PRs for Go module updates and GitHub Actions version bumps |

**Release process:**
1. Tag: `git tag v1.x.x`
2. Push: `git push origin v1.x.x`
3. GitHub Actions builds for darwin/linux/windows × amd64/arm64, creates GitHub Release, updates Homebrew formula

---

## 12. Dependencies

| Dependency | Version | Purpose |
|---|---|---|
| `github.com/spf13/cobra` | v1.10.2 | CLI command framework |
| `charm.land/bubbletea/v2` | v2.0.2 | Terminal UI framework |
| `charm.land/bubbles/v2` | v2.1.0 | TUI components (spinner, text input, progress) |
| `charm.land/lipgloss/v2` | v2.0.2 | Terminal styling |
| `github.com/atotto/clipboard` | v0.1.4 | System clipboard access (share token copy) |
| `golang.org/x/term` | v0.41.0 | TTY detection |
| `gopkg.in/yaml.v3` | v3.0.1 | YAML parsing for marketplace, manifests, and config |

All direct dependencies are pure Go (CGO_ENABLED=0). No C libraries, fully static binaries. The `clipboard` package shells out to OS clipboard utilities (`xclip`/`xsel`/`pbcopy`/`clip.exe`) at runtime.

---

## 13. Known Constraints & Gotchas

- **`marketplace.yaml` is the single source of truth for tools.** Do not hardcode tool definitions in Go files. All tool metadata (names, binary names, package IDs) must go in the YAML.
- **The marketplace catalog is fetched from GitHub at runtime**, not embedded into the binary. `internal/catalog` manages fetching, caching (`marketplace-cache.yaml`), and diffing. If no cache exists and the network is unavailable, catalog loading will fail. The user's `marketplace.yaml` holds only customizations (enabled/disabled, custom tools) and is never overwritten wholesale — `mergeToolDefs` preserves user changes.
- **`config.yaml` is optional.** `internal/config.Load()` returns sensible defaults if the file is missing. On first run it writes a commented default file. All configuration (GitHub owner/repo/branch, concurrency, timeouts, UI preferences) lives here.
- **Package manager queries are synchronous subprocesses** with a configurable timeout (default 10 seconds, set via `config.yaml`). If a PM hangs, the timeout kills it and returns an empty string.
- **Version comparison stops at the first non-numeric segment.** `"2.53.0.windows.1"` is compared as `[2, 53, 0]`. This is intentional — non-numeric suffixes are platform metadata, not version info.
- **`VersionsMatch` handles PE padding** (e.g. `1400` ≈ `14`) but `CompareVersions` does not. Callers that need PE-aware comparison should use `VersionsMatch` as a guard first (as `HasUpdate()` does).
- **The TUI calls `svc.ResolveOne` per tool** (individual goroutines via Bubbletea commands), while the **CLI calls `svc.LoadAndResolve`** which internally uses `Resolver.ResolveVersions` (worker pool). Both reach the same result via different concurrency models.
- **Windows cannot delete a running executable.** The self-update `ReplaceBinary` leaves a `.old` file that is cleaned up on the next invocation.
- **`sync.Once` in `pmAvailability`** caches package manager availability permanently for the process lifetime. Tests must set `pmAvailableFunc` before any real call.
