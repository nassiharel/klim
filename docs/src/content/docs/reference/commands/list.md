---
title: "klim list"
description: List developer tools with versions, sources, and update status
---

List installed developer tools with version info, install sources, and update status.

## Usage

```bash
klim list [flags]
```

## Flags

| Flag | Short | Description |
|------|-------|-------------|
| `--category` | `-c` | Filter by category (e.g., Cloud, CLI, Containers) |
| `--source` | | Filter by install source (brew, winget, apt, etc.) |
| `--categories` | | Print available category names and exit |
| `--refresh` | | Force fresh scan, ignoring on-disk cache |

## Examples

```bash
# List all installed tools
klim list

# Filter by category
klim list --category Cloud

# Filter by install source
klim list --source brew

# Combine filters
klim list --category IaC --source brew

# Show available categories
klim list --categories

# Force fresh scan
klim list --refresh
```

## Output

Each line shows:
- Status indicator (✓ up to date, ⬆ update available)
- Tool name
- Installed version
- Install source in parentheses
- Display name
- Latest version (if update available)

```
✓ az          2.67.0    (brew)     Azure CLI
⬆ docker      24.0.7    (manual)   Docker CLI             → 27.1.0
✓ git         2.47.0    (brew)     Git version control
```

## Caching

By default, `klim list` uses a cached scan result for fast startup. Use `--refresh` to force a fresh PATH scan and version resolution.
