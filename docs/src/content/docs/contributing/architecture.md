---
title: Architecture
description: Internal architecture and module layout of clim
---

## Module Layout

```
cmd/clim/main.go           Entry point → cli.Execute()
internal/
  build/        Version/Commit/Date (ldflags) + Info(), VersionOnly()
  catalog/      Fetch marketplace.yaml from GitHub, cache locally
  cli/          Cobra commands: list, export, import, open, share, update, etc.
  config/       config.yaml: logging, marketplace, performance, UI prefs
  custompacks/  User-created pack definitions
  detector/     Fallback version detection (Go buildinfo, Windows PE)
  favorites/    Favorites list persistence
  fileutil/     Shared file I/O: AtomicWrite, EnsureDir, ReadYAML, WriteYAML
  finder/       PATH scanning, install source detection
  logging/      slog structured logging + lumberjack file rotation
  manifest/     YAML schema for export/import manifests
  paths/        Single source of truth for all file paths
  pkgmgr/       Package manager queries (installed + latest versions)
  registry/     Tool, Instance, Pack structs; version comparison
  scancache/    Per-host scan cache for installed tools
  selfupdate/   Self-update from GitHub Releases
  service/      ToolService: composition root
  share/        Compact token encode/decode
  tui/          Bubbletea TUI: model, commands, view, styles
marketplace/
  tools/*.yaml  One file per tool definition
  packs/*.yaml  One file per pack definition
```

## Core Architecture

```
ToolService
  ├── ToolCatalog      catalog.LoadOrFetch() → fetch/cache marketplace.yaml
  ├── ToolFinder       PATH scan → detect install source per binary
  └── VersionResolver  pkgmgr queries → installed + latest versions
```

**ToolService** is the composition root that wires together the catalog, finder, and version resolver. It provides two main entry points:

- **`LoadAndResolve()`** — Full scan + version resolution (used by CLI)
- **`LoadCached()` / `LoadAndResolveCached()`** — Cache-first with optional `--refresh` flag

## Data Flow

### TUI Flow

```
Init
  → findToolsCmd           (PATH scan)
  → scanResultMsg          (found tools)
  → resolveToolVersionCmd  (×N, concurrent with semaphore)
  → toolVersionMsg         (×N, version results)
  → done
```

The TUI uses Bubbletea v2's message-passing architecture. Each async operation is a `Cmd` that returns a `Msg`. The `Update` function processes messages and triggers state transitions.

### CLI Flow

```
svc.LoadAndResolve()
  → catalog.LoadOrFetch()   (fetch/cache marketplace)
  → finder.FindTools()      (PATH scan)
  → pkgmgr.Resolve()        (worker pool, concurrent version queries)
  → return tools
```

CLI commands call `svc.LoadAndResolve()` (or the cached variant) which handles everything internally with a worker pool.

## Concurrency Model

- **TUI version resolution:** `resolveSem` channel (capacity 4) limits concurrent subprocess calls
- **`scanGen` counter:** Invalidates stale messages when a rescan is triggered
- **Package manager calls:** `context.WithTimeout` (configurable, default 30s)
- **npm/dpkg caching:** `sync.Once` prevents redundant global queries

## Key Conventions

### Error Handling
- Return `error` last
- Wrap with `%w` for chain
- Use `errors.New()` for static messages
- Never panic on user paths

### Naming
- Bubbletea messages end in `Msg`
- Command factories end in `Cmd`
- Cobra runners: `run<Command>`

### Imports
- stdlib → third-party → internal, separated by blank lines

### Shared Utilities

**`internal/paths`** — Single source for all `~/.config/clim/*` paths:
```go
paths.Config()       // config/config.yaml
paths.Favorites()    // favorites/favorites.yaml
paths.ScanCache()    // cache/scan-cache.yaml
paths.CatalogCache() // marketplace/marketplace-cache.yaml
```

**`internal/fileutil`** — Atomic writes and YAML I/O:
```go
fileutil.AtomicWrite(path, data, 0o644)
fileutil.WriteYAML(path, &obj, "# header\n")
fileutil.ReadYAML(path, &obj)
```

**`internal/registry`** — Tool collection helpers:
```go
registry.SortByName(tools)
registry.ToolMap(tools)
registry.InstalledSet(tools)
```

## Adding a Package Manager

1. Add `InstallSource` constant + command templates in `registry/tool.go`
2. Add to `sourcePriority()`, `SourcesForOS()`, `AllPMStatusForOS()`
3. Implement `xxxInstalledVersion()` / `xxxLatestVersion()` in `pkgmgr/pkgmgr.go`
4. Wire into `installedVersion()` / `latestVersion()` switches
5. Add field to `PackageIDs` struct + YAML tag
6. Add finder source detection in `finder/finder.go`
