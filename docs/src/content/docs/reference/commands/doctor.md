---
title: "clim doctor"
description: Check environment health and diagnose common issues
---

Run environment diagnostics to detect PATH problems, conflicting tool installations, missing package managers, stale caches, and available updates.

## Usage

```bash
clim doctor [flags]
```

## Flags

| Flag | Description |
|------|-------------|
| `--json` | Machine-readable JSON output |
| `--refresh` | Force fresh scan, ignoring cache |

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | No errors found (warnings and info may still be reported) |
| 1 | One or more errors detected |

## Checks Performed

| Check | Severity | What it detects |
|-------|----------|-----------------|
| Duplicate PATH | Warning | Same directory appearing multiple times in PATH |
| Broken PATH | Warning | Missing, inaccessible, or non-directory PATH entries |
| Conflicting versions | Warning | Same tool installed at multiple locations with different versions |
| Missing PMs | Info | Package managers that could manage your tools but aren't installed |
| Stale cache | Info | Scan cache older than 7 days |
| Unresolved versions | Warning | Installed tools where version couldn't be determined |
| Outdated tools | Info | Summary of tools with available updates |

## Examples

```bash
# Human-readable output
clim doctor

# JSON output for CI pipelines
clim doctor --json

# Force fresh scan
clim doctor --refresh
```

## TUI

The Doctor tab (key `9`) shows the same diagnostics in a scrollable, color-coded view. Sub-tabs switch between Doctor diagnostics and Audit findings.

## See Also

- [Doctor & Audit guide](/guides/doctor-audit) — Detailed walkthrough
- [clim audit](/reference/commands/audit) — Security audit and SBOM
