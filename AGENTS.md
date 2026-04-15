# AGENTS.md — clim

> Reference for AI agents and humans working on this codebase.

## What is clim

Cross-platform developer-tool manager. Discovers, inspects, upgrades 70+ CLI tools via native package managers (winget, brew, apt, choco, snap, npm). Written in Go 1.25, Bubbletea v2 TUI, Cobra CLI.

Module: `github.com/nassiharel/clim`

## Project Structure

```
cmd/clim/main.go          Entry point → cli.Execute()
marketplace/                Modular tool catalog (source of truth)
  tools/*.yaml              One file per tool definition
  packs/*.yaml              One file per pack definition
scripts/                    Marketplace assembly & validation
  assemble-marketplace/     Combines individual files → marketplace.yaml
  validate-marketplace/     Schema validation, uniqueness, cross-references
install.sh                 Linux/macOS installer script
install.ps1                Windows PowerShell installer script
Makefile                   build / test / lint / cover / clean
.goreleaser.yml            Cross-platform release (darwin/linux/windows × amd64/arm64)

internal/
  build/       Version/Commit/Date (ldflags) + Info(), VersionOnly()
  catalog/     Fetch marketplace.yaml from GitHub, cache locally, diff, refresh
  cli/         Cobra commands: list, export, import, open, share, update, tools, config
  config/      config.yaml: logging, marketplace URL, performance, UI prefs
  detector/    Fallback version detection (Go buildinfo, Windows PE resources)
  finder/      PATH scanning, install source detection (brew/winget/apt/manual)
  logging/     slog structured logging + lumberjack file rotation
  manifest/    YAML schema for export/import manifests
  pkgmgr/      Package manager queries (installed + latest versions)
  registry/    Tool, Instance, PackageIDs, Pack structs; version comparison
  selfupdate/  Self-update from GitHub Releases (download → extract → replace)
  service/     ToolService: composition root wiring catalog + finder + resolver
  share/       Compact token encode/decode for sharing tool lists
  tui/         Bubbletea Model (model.go), commands (commands.go), rendering (view.go), styles
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

**Marketplace:** individual tool/pack YAML files in `marketplace/` are assembled into a single `marketplace.yaml` by CI and published to the `marketplace` branch. The CLI fetches from `https://raw.githubusercontent.com/nassiharel/clim/marketplace/marketplace.yaml`, cached at `~/.config/clim/marketplace-cache.yaml`. User customizations in `~/.config/clim/marketplace.yaml` are preserved via `mergeToolDefs()`.

## TUI Tabs

| Tab | Content |
|---|---|
| Installed | All detected tools with version status (✓ / ⬆) |
| Updates | Tools with available upgrades; batch upgrade support |
| Discover | Sub-tabs: **Tools** (marketplace), **Packs** (curated bundles), **For You** (smart recommendations) |
| Backup | Export/import toolchain; share tokens |
| Config | View config.yaml path and settings |

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
```

Run `make marketplace-validate` to check. CI validates on every PR and publishes the assembled catalog on merge to `main`.

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
make marketplace-validate  # validate marketplace/tools/ and marketplace/packs/
make marketplace-assemble  # assemble → marketplace.yaml
```

**Test patterns:** table-driven, `httptest.NewServer`, in-memory archives, `t.TempDir()`, `atomic.Pointer` for `pmAvailableFunc` test hook.

## CI/CD

| Workflow | Trigger | Does |
|---|---|---|
| `ci.yml` | push/PR | lint, test (Linux/macOS/Windows matrix), govulncheck |
| `marketplace.yml` | push/PR (marketplace/**) | validate individual files, assemble, publish to marketplace branch |
| `release.yml` | `v*` tag | GoReleaser → GitHub Release, Homebrew tap, deb/rpm, SBOM |
| `codeql.yml` | push/weekly | Security analysis |

## Gotchas

- **`marketplace/` is the single source of truth.** No tool definitions in Go code. Individual YAML files are assembled into `marketplace.yaml` by CI.
- **Never edit root `marketplace.yaml` directly** — it's auto-generated. Edit files in `marketplace/tools/` and `marketplace/packs/` instead.
- **Catalog is fetched at runtime**, not embedded. No network + no cache = catalog failure.
- **`config.yaml` is optional.** `config.Load()` returns defaults if missing; writes defaults on first run.
- **Version comparison stops at first non-numeric segment.** `"2.53.0.windows.1"` → `[2, 53, 0]`.
- **`VersionsMatch` handles PE padding** (`1400` ≈ `14`), `CompareVersions` does not.
- **Windows cannot delete a running exe.** Self-update leaves `.old`, cleaned up next launch.
- **TUI `pending` counter:** reset with `scanGen++` on rescan to invalidate stale in-flight messages.
