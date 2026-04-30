---
title: Configuration Reference
description: Complete reference for config.yaml options
---

clim uses a `config.yaml` file for persistent settings. The file is created with defaults on first run.

## File Location

| OS | Path |
|----|------|
| macOS | `~/Library/Application Support/clim/config/config.yaml` |
| Linux | `~/.config/clim/config/config.yaml` |
| Windows | `%AppData%\clim\config\config.yaml` |

Find the exact path on your system:

```bash
clim config path
```

## Editing

```bash
# Open in your editor
clim config edit

# Or edit in the TUI (Config tab, press 8)
```

## Options

### Logging

```yaml
logging:
  level: "debug"      # Log level: debug, info, warn, error
  file: true          # Write logs to file
  verbose: false      # Also log to stderr
```

| Option | Default | Description |
|--------|---------|-------------|
| `level` | `"debug"` | Minimum log level. Set to `"warn"` or `"error"` to reduce noise. |
| `file` | `true` | Write structured logs to the log file. |
| `verbose` | `false` | Additionally output logs to stderr (useful for debugging). |

Log file location:
- macOS: `~/Library/Application Support/clim/logs/clim.log`
- Linux: `~/.config/clim/logs/clim.log`
- Windows: `%AppData%\clim\logs\clim.log`

### Marketplace

```yaml
marketplace:
  url: "https://raw.githubusercontent.com/nassiharel/clim/marketplace/marketplace.yaml"
  auto_refresh: false
  refresh_interval: "24h"
```

| Option | Default | Description |
|--------|---------|-------------|
| `url` | GitHub raw URL | URL to fetch the marketplace catalog from. |
| `auto_refresh` | `false` | Automatically refresh the catalog on startup if stale. |
| `refresh_interval` | `"24h"` | How often to consider the cached catalog stale. |

### Performance

```yaml
performance:
  concurrency: 0
  command_timeout: "30s"
```

| Option | Default | Description |
|--------|---------|-------------|
| `concurrency` | `0` (auto) | Number of concurrent version queries. `0` = `runtime.NumCPU()`. |
| `command_timeout` | `"30s"` | Timeout for each package manager subprocess call. |

### UI

```yaml
ui:
  default_tab: "installed"
  show_path: true
  sidebar_right: false
```

| Option | Default | Description |
|--------|---------|-------------|
| `default_tab` | `"installed"` | Tab shown on startup. Options: `installed`, `favorites`, `updates`, `marketplace`, `backup`, `project`, `dashboard`, `config`. |
| `show_path` | `true` | Show binary path in tool list. |
| `sidebar_right` | `false` | Place the filter sidebar on the right side instead of left. |

## Defaults

If `config.yaml` doesn't exist, clim uses sensible defaults and writes the default file on first run. Deleting the file resets all settings.
