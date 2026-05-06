---
title: "klim watch"
description: Check for available tool updates
---

Check all installed tools for available updates. Designed to run periodically via cron or Task Scheduler.

## Usage

```bash
klim watch [flags]
```

## Flags

| Flag | Description |
|------|-------------|
| `--json` | Machine-readable JSON output |

## Examples

```bash
# Human-readable output
klim watch

# JSON for scripting
klim watch --json
```

## Scheduling

```bash
# Cron (daily at 9am)
0 9 * * * klim watch --json >> ~/.config/klim/watch.log

# Windows Task Scheduler
schtasks /create /tn "klim-watch" /tr "klim watch" /sc daily /st 09:00
```

## Output

```
4 update(s) available:

  ⬆ gh                   2.74.2 → 2.92.0  (winget)
  ⬆ git                  2.53.0 → 2.54.0  (winget)
  ⬆ node                 24.14.1 → 24.15.0  (winget)
  ⬆ terraform            1.6.5 → 1.7.0  (scoop)

Run 'klim' to upgrade interactively.
```

## Note

`klim watch` always performs a fresh scan (equivalent to `--refresh`) to ensure results are authoritative.

## See Also

- [Batch Updates guide](../../guides/batch-updates.md) — Upgrade tools in the TUI
