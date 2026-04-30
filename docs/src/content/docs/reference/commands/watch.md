---
title: "clim watch"
description: Check for available tool updates
---

Check all installed tools for available updates. Designed to run periodically via cron or Task Scheduler.

## Usage

```bash
clim watch [flags]
```

## Flags

| Flag | Description |
|------|-------------|
| `--json` | Machine-readable JSON output |

## Examples

```bash
# Human-readable output
clim watch

# JSON for scripting
clim watch --json
```

## Scheduling

```bash
# Cron (daily at 9am)
0 9 * * * clim watch --json >> ~/.config/clim/watch.log

# Windows Task Scheduler
schtasks /create /tn "clim-watch" /tr "clim watch" /sc daily /st 09:00
```

## Output

```
4 update(s) available:

  ⬆ gh                   2.74.2 → 2.92.0  (winget)
  ⬆ git                  2.53.0 → 2.54.0  (winget)
  ⬆ node                 24.14.1 → 24.15.0  (winget)
  ⬆ terraform            1.6.5 → 1.7.0  (scoop)

Run 'clim' to upgrade interactively.
```

## Note

`clim watch` always performs a fresh scan (equivalent to `--refresh`) to ensure results are authoritative.

## See Also

- [Batch Updates guide](/guides/batch-updates) — Upgrade tools in the TUI
