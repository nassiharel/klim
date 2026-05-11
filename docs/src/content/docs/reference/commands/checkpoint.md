---
title: klim checkpoint
description: Save / list / show / delete named toolchain snapshots that klim rollback can later restore.
---

`klim checkpoint` captures the currently-installed tool versions (and `$PATH`) under a named YAML snapshot. A snapshot can later be restored declaratively through `klim rollback <name>`.

## Synopsis

```
klim checkpoint <name>           Capture a new checkpoint.
klim checkpoint list             List every checkpoint.
klim checkpoint show <name>      Print a checkpoint's tools and versions.
klim checkpoint delete <name>    Remove a checkpoint.
```

## Storage

Each checkpoint is stored at `~/.klim/checkpoints/<name>.yaml` as human-readable YAML you can review or hand-edit before rolling back. Names are validated to be filename-safe (letters, digits, dots, dashes, underscores; no path traversal).

## What's captured

| Field | Purpose |
|---|---|
| `name` | Unique identifier for `klim rollback`. |
| `description` | Optional free-text (set with `--description`). |
| `created_at` | UTC timestamp. |
| `goos` | OS at capture time — used by the rollback command builder. |
| `tools` | Every installed tool with `version`, `source`, and `path`. Only installed tools are recorded. |
| `path` | The `$PATH` value at capture time. Stored for audit / inspection only — `klim rollback` does **not** currently replay it; tool-version restoration is the only diff applied today. |

## Flags

`klim checkpoint <name>` accepts:

| Flag | Description |
|---|---|
| `--description STRING`, `-d STRING` | Free-text description stored with the snapshot. |
| `--output {text,json}` | Output format for the capture summary. |

## Auto-checkpoints

`klim apply` automatically captures a checkpoint named `pre-apply-<UTC>` before running. They appear in `klim checkpoint list` alongside any manual checkpoints — feel free to delete old ones with `klim checkpoint delete pre-apply-<UTC>` once you no longer need the safety net.

## Example

```
$ klim checkpoint before-k8s-upgrade --description "K8s control plane upgrade prep"

✓ Captured checkpoint "before-k8s-upgrade" with 149 tool(s)
  Saved to ~/.klim/checkpoints/before-k8s-upgrade.yaml
  Roll back any time with: klim rollback before-k8s-upgrade

$ klim checkpoint list
NAME                       CREATED            TOOLS  DESCRIPTION
before-k8s-upgrade         2026-05-11 16:35   149    K8s control plane upgrade prep
pre-apply-20260511-163045  2026-05-11 16:30   149    Automatic pre-apply snapshot

$ klim checkpoint show before-k8s-upgrade
Checkpoint: before-k8s-upgrade
Description: K8s control plane upgrade prep
Created:     2026-05-11 16:35:12
Platform:    windows
Tools:       149

TOOL          VERSION      SOURCE
azure-cli     2.85.0       winget
helm          3.16.2       winget
kubectl       1.31.0       brew
…
```

## Related

- [`klim rollback`](./rollback.md) — produce a plan that restores a checkpoint
- [`klim apply`](./apply.md) — captures `pre-apply-<UTC>` automatically
- [`klim trail`](./trail.md) — content-addressed history (different mental model: trail is an append-only log; checkpoint is a named save state)
