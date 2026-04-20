# AGENTS.md — clim

> Reference for AI agents and humans working on this codebase.

## What is clim

Cross-platform developer-tool manager. Discovers, inspects, upgrades 70+ CLI tools via native package managers (winget, scoop, brew, apt, choco, snap, npm). Written in Go 1.25, Bubbletea v2 TUI, Cobra CLI.

Module: `github.com/nassiharel/clim`

## Project Structure

```
cmd/clim/main.go          Entry point → cli.Execute()
marketplace/                Modular tool catalog (source of truth)
  tools/*.yaml              One file per tool definition
  packs/*.yaml              One file per pack definition
  marketplace/   Marketplace assembly & validation scripts
    assemble/    Combines individual files → marketplace.yaml
    validate/    Schema validation, uniqueness, cross-references
install.sh                 Linux/macOS installer script
install.ps1                Windows PowerShell installer script
Makefile                   build / test / lint / cover / clean
.goreleaser.yml            Cross-platform release (darwin/linux/windows × amd64/arm64)

internal/
  build/       Version/Commit/Date (ldflags) + Info(), VersionOnly()
  catalog/     Fetch marketplace.yaml from GitHub, cache locally, diff, refresh
  cli/         Cobra commands: list, export, import, open, share, update, tools, config
  config/      config.yaml: logging, marketplace URL, performance, UI prefs
  custompacks/ User-created pack definitions → ~/.config/clim/marketplace/custom-packs.yaml
  detector/    Fallback version detection (Go buildinfo, Windows PE resources)
  favorites/   Favorites list persistence → ~/.config/clim/favorites/favorites.yaml
  fileutil/    Shared file I/O: AtomicWrite, EnsureDir, ReadYAML, WriteYAML
  finder/      PATH scanning, install source detection (brew/winget/scoop/apt/manual)
  logging/     slog structured logging + lumberjack file rotation
  manifest/    YAML schema for export/import manifests + FromRegistryTool converter
  paths/       Single source of truth for all ~/.config/clim/* paths
  pkgmgr/      Package manager queries (installed + latest versions)
  registry/    Tool, Instance, PackageIDs, Pack structs; version comparison; SortByName, ToolMap, InstalledSet helpers
  scancache/   Per-host scan cache: installed/not, paths, versions → ~/.config/clim/scan-cache.yaml
  selfupdate/  Self-update from GitHub Releases (download → extract → replace)
  service/     ToolService: composition root wiring catalog + finder + resolver
  share/       Compact token encode/decode for sharing tool lists
  tui/         Bubbletea Model (model.go), commands (commands.go), rendering (view.go), favorites (favorites.go), styles
```

## Architecture

```
ToolService
  ├── ToolCatalog     catalog.LoadOrFetch() → fetch/cache marketplace.yaml from GitHub
  ├── ToolFinder      PATH scan → detect install source per binary
  └── VersionResolver pkgmgr queries → installed + latest versions per tool
```

**TUI flow:** `Init → findToolsCmd → scanResultMsg → resolveToolVersionCmd ×N → toolVersionMsg ×N → done`

**CLI flow:** `svc.LoadAndResolve()` — single call, internal worker pool.

**Marketplace:** individual tool/pack YAML files in `marketplace/` are assembled into a single `marketplace.yaml` by CI and published to the `marketplace` branch. The CLI fetches from `https://raw.githubusercontent.com/nassiharel/clim/marketplace/marketplace.yaml` and caches locally at `~/.config/clim/marketplace-cache.yaml`.

## TUI Tabs

| Tab | Key | Content |
|---|---|---|
| Installed | 1 | All detected tools with version status (✓ / ⬆). Press `*` to favorite. |
| ★ Favorites | 2 | Favorited tools. `e` export, `s` share, `x` clear all, `*` unfavorite. |
| Updates | 3 | Tools with available upgrades; batch upgrade support |
| Discover | 4 | Sub-tabs: **Tools** (marketplace), **Packs** (curated bundles), **For You** (smart recommendations) |
| Backup | 5 | Export/import toolchain; share tokens; My Packs; My Backups |
| Dashboard | 6 | Aggregate stats, gauges, category/tag/platform breakdowns |
| Config | 7 | View/edit config.yaml settings |

**Filter sidebar:** Category / Platform / Tag filters with counters. Configurable left/right position (`config.yaml → ui.sidebar_right`).

## Key Conventions

**Errors:** Return `error` last. Wrap with `%w`. Use `errors.New()` for static messages. Never panic on user paths.

**Naming:** Bubbletea messages end in `Msg`, command factories end in `Cmd`. Cobra runners: `run<Command>`.

**Imports:** stdlib → third-party → internal, separated by blank lines.

**Concurrency:**
- TUI version resolution: `resolveSem` channel (capacity 4) limits concurrent subprocesses
- `scanGen` counter invalidates stale messages on rescan
- Package manager calls use `context.WithTimeout` (configurable, default 30s)
- npm globals cached via `sync.Once`; dpkg cached via `sync.Once`

**Formatting:** `gofmt`, `go vet`, `golangci-lint` (v2+) enforced in CI.

## Adding a Tool

Create `marketplace/tools/mytool.yaml`:
```yaml
name: mytool
display_name: My Tool
category: IaC
tags: [automation, infrastructure]
binary_names: [mytool]
packages:
  brew: "mytool"
  winget: "Publisher.MyTool"
  scoop: "mytool"
# Optional: when set, the marketplace assemble workflow fetches repository
# metadata (stars, description, homepage, license, topics, last push) from
# the GitHub REST API and emits it as `github_info:` in the published
# marketplace.yaml. Never set `github_info` by hand — it is build-time only.
github: owner/repo
```

Run `make marketplace-validate` to check. CI validates on every PR and publishes the assembled catalog on merge to `main`. GitHub enrichment is enabled automatically in the `marketplace.yml` workflow (`-fetch-github`), authenticated with the workflow's `GITHUB_TOKEN`. To preview enrichment locally, export a token and run `go run ./internal/marketplace/assemble -fetch-github -o marketplace.yaml`.

## Adding a Package Manager

1. Add `InstallSource` constant + command templates in `registry/tool.go`
2. Add to `sourcePriority()`, `SourcesForOS()`, `AllPMStatusForOS()`
3. Implement `xxxInstalledVersion()` / `xxxLatestVersion()` in `pkgmgr/pkgmgr.go`
4. Wire into `installedVersion()` / `latestVersion()` switches
5. Add field to `PackageIDs` struct + YAML tag
6. Add finder source detection in `finder/finder.go`

## Build & Test

```bash
make all          # lint + test + build (default)
make build        # bin/clim with version ldflags
make test         # go test -race -count=1 ./...
make lint         # golangci-lint run
make cover        # HTML coverage report
make marketplace-validate  # validate marketplace/ tool and pack files
make marketplace-assemble  # assemble → marketplace.yaml

# Opt-in live check — probes every package ID against the real PM
# (winget/choco/scoop/brew/apt/snap/npm). Skips PMs whose binary isn't on
# PATH, so it's safe to run locally; on CI it runs via the
# marketplace-livecheck workflow across Win/macOS/Linux runners.
go test -tags=integration -timeout=40m ./internal/marketplace/livecheck/...
```

**Test patterns:** table-driven, `httptest.NewServer`, in-memory archives, `t.TempDir()`, `atomic.Pointer` for `pmAvailableFunc` test hook.

## CI/CD

| Workflow | Trigger | Does |
|---|---|---|
| `ci.yml` | push/PR | lint, test (Linux/macOS/Windows matrix), govulncheck |
| `marketplace.yml` | push/PR (marketplace/**) | validate individual files, assemble, publish to marketplace branch |
| `marketplace-livecheck.yml` | weekly / manual | probes every package ID against live winget/choco/scoop/brew/apt/snap/npm (Linux/macOS/Windows matrix) — external-network dependent, not PR-gating |
| `release.yml` | `v*` tag | GoReleaser → GitHub Release, Homebrew tap, deb/rpm, SBOM |
| `codeql.yml` | push/weekly | Security analysis |

## Gotchas

- **`marketplace/` is the single source of truth.** No tool definitions in Go code. Individual YAML files are assembled into `marketplace.yaml` by CI.
- **Never edit root `marketplace.yaml` directly** — it's auto-generated. Edit files in `marketplace/tools/` and `marketplace/packs/` instead.
- **Catalog is fetched at runtime**, not embedded. No network + no cache = catalog failure.
- **Scan cache (`~/.config/clim/scan-cache.yaml`) is user-controlled.** Written after every successful scan; loaded on startup to skip PATH scan + version resolution. Only installed tools are persisted (a missing entry means "not installed"). Invalidated by TUI `r` key or CLI `--refresh` flag. In the TUI, mutating actions (install/upgrade/remove) trigger `startScan` which rescans and rewrites the cache. In the CLI, `clim import` calls `svc.InvalidateScanCache()` after install attempts so later `clim list` / `clim export` runs rescan automatically.
- **`config.yaml` is optional.** `config.Load()` returns defaults if missing; writes defaults on first run.
- **Version comparison stops at first non-numeric segment.** `"2.53.0.windows.1"` → `[2, 53, 0]`.
- **`VersionsMatch` handles PE padding** (`1400` ≈ `14`), `CompareVersions` does not.
- **Windows cannot delete a running exe.** Self-update leaves `.old`, cleaned up next launch.
- **TUI `pending` counter:** reset with `scanGen++` on rescan to invalidate stale in-flight messages.

## Shared Utilities

**DO NOT duplicate file I/O, path resolution, or data conversion.** Use the shared packages:

### `internal/paths` — All config/data file paths

Single source for every `~/.config/clim/*` path. Never call `os.UserConfigDir()` directly.

```go
paths.Config()       // config/config.yaml
paths.Favorites()    // favorites/favorites.yaml
paths.CustomPacks()  // marketplace/custom-packs.yaml
paths.ScanCache()    // scan-cache.yaml
paths.CatalogCache() // marketplace/marketplace-cache.yaml
paths.BackupsDir()   // backups/
paths.LogFile()      // logs/clim.log
paths.Join("x","y")  // arbitrary sub-path
```

### `internal/fileutil` — Atomic writes and YAML I/O

```go
fileutil.AtomicWrite(path, data, 0o644)       // temp+rename, Windows-safe
fileutil.WriteYAML(path, &obj, "# header\n")  // marshal + atomic write + EnsureDir
fileutil.ReadYAML(path, &obj)                 // returns (found bool, err)
fileutil.EnsureDir(path)                      // MkdirAll on parent
```

### `internal/registry` — Tool collection helpers

```go
registry.SortByName(tools)    // case-insensitive alphabetical sort
registry.ToolMap(tools)       // map[string]*Tool by name
registry.InstalledSet(tools)  // map[string]bool of installed tool names
```

### `internal/manifest` — Registry-to-manifest conversion

```go
manifest.FromRegistryTool(tool)  // registry.Tool → manifest.Tool (with version/source from PrimaryInstance)
```

## Favorites Feature

Favorites persist at `~/.config/clim/favorites/favorites.yaml` (simple list of tool names). The `internal/favorites` package provides `Load`, `Save`, `Add`, `Remove`, `Toggle`, `Contains`, `Set`.

TUI integration:
- `*` key toggles favorite on any tool-list tab (Installed, Favorites, Updates, Discover)
- ★ indicator shown next to favorited tools on all tabs
- Favorites tab: `e` export to YAML manifest, `s` share via token, `x` clear all (y/n confirm)
- `m.favoriteNames map[string]bool` in-memory set, loaded at startup via `favorites.Set()`
