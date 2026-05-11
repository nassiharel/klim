---
title: klim rollback
description: Produce a plan that restores a saved checkpoint. Read-only by default — review the diff before applying.
---

`klim rollback <name>` compares the current toolchain to a saved checkpoint and emits a plan that would restore it. The command is **read-only** — review the diff first, then run the suggested commands manually (or `klim apply` when the rollback only requires upgrades).

## Synopsis

```
klim rollback <checkpoint> [flags]
```

We deliberately don't auto-execute the rollback. PM-specific downgrades can fail in ways that need human inspection (formula versions may have moved, registry entries may differ, native modules may need rebuild), and silently running them would obscure exactly which step broke.

## Sections

Rollback emits the same sections as `klim plan`:

- **Planned changes** — grouped by PM. Upgrades + downgrades + installs (for tools removed since the checkpoint) + optional removes (with `--remove-extras`).
- **Upgrade confidence** — 0-100% per change with the per-factor breakdown.
- **Risk analysis** — heuristic warnings (downgrades carry their own risk class).
- **Disk impact** — estimated cache delta.
- **Estimated time** — pessimistic wall-clock estimate.

## Flags

| Flag | Description |
|---|---|
| `--refresh` | Force a fresh scan, ignoring the cache. |
| `--remove-extras` | Also propose removing tools that were installed *after* the checkpoint (i.e. tools not in the snapshot). Off by default so a rollback only restores known tools without touching new arrivals. |
| `--output {text,json}` | Output format. |

## Example

```
$ klim rollback before-k8s-upgrade
Rollback plan to checkpoint "before-k8s-upgrade" (captured 2026-05-11 16:35)

Planned changes:

  brew:
    kubectl 1.32.0 -> 1.31.0    (confidence: 72%)

Upgrade confidence:
  kubectl upgrade confidence: 72%
    -8  1.32.0 → 1.31.0 adds new features that may shift defaults
    -20 kubectl ±1 minor version of the API server is supported; further skew breaks features

Risk analysis:
  ⚠ kubectl: confirm your cluster's API server version supports 1.31

Disk impact:
  +50MB cache

Estimated time:
  20s

Review the plan above, then apply the suggested commands manually
(or run `klim apply` for upgrade-only rollbacks).
```

Note: a rollback that downgrades a tool reuses the same upgrade-confidence
formula — there is **no** rollback-specific "+N for reverting to a known
state" boost today. The captured PATH lives in the checkpoint file for
audit / manual inspection but is not yet replayed by `klim rollback`;
the rollback plan only addresses tool versions. Restoring PATH itself is
on the roadmap.

## Related

- [`klim checkpoint`](./checkpoint.md) — capture a snapshot first
- [`klim plan`](./plan.md) — same plan output for forward changes
- [`klim apply`](./apply.md) — captures a `pre-apply-<UTC>` checkpoint automatically before running
