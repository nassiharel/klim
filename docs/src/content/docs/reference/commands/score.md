---
title: "klim score"
description: Calculate your environment health score (0-100)
---

Compute a single health score for your dev environment by combining tool freshness, doctor diagnostics, audit findings, compliance status, and source management into a 0-100 metric.

## Usage

```bash
klim score [flags]
```

## Flags

| Flag | Description |
|------|-------------|
| `--json` | Output as JSON |
| `--badge` | Output shields.io badge URL |
| `--refresh` | Force fresh scan |

## Scoring Categories

| Category | Max Points | What it measures |
|----------|-----------|-----------------|
| Tools up to date | 30 | Percentage of tools at latest version |
| Doctor health | 25 | Errors (-5 each) and warnings (-2 each) from doctor |
| Audit clean | 20 | Warnings (-3) and infos (-1) from audit |
| Compliance | 15 | Policy violations (errors -5, warnings -2) |
| Managed sources | 10 | Unmanaged/manual tools (-3 each) |

## Grade Scale

| Grade | Score |
|-------|-------|
| A+ | 95-100 |
| A | 90-94 |
| B | 80-89 |
| C | 70-79 |
| D | 60-69 |
| F | 0-59 |

## Examples

```bash
# Human-readable score card
klim score

# JSON for CI pipelines
klim score --json

# shields.io badge URL for README
klim score --badge
```

## Badge

Add to your README:

```markdown
![klim score](https://img.shields.io/badge/klim%20score-85%2F100%20A-yellowgreen)
```

Generate your actual badge URL with:
```bash
klim score --badge
```

## TUI

The score is shown in the **Dashboard** tab as a prominent gauge with grade.

## See Also

- [klim security health](./security.md) — Environment diagnostics
- [klim security audit](./security.md) — Security audit
- [klim security compliance](./security.md) — Policy validation
