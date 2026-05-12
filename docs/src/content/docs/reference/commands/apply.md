---
title: klim apply
description: Execute the changes klim plan proposes, wrapped in an auto-checkpoint and post-apply validation.
---

`klim apply` runs the changes `klim plan` would output, wrapped in a safety net so you can trust the result.

## Synopsis

```
klim apply [tool...] [flags]
```

## Lifecycle

Every `klim apply` invocation runs four phases:

1. **Pre-apply scan + checkpoint** — captures a named snapshot of the current toolchain to `~/.klim/checkpoints/pre-apply-<UTC>.yaml`. Roll back any time with `klim rollback pre-apply-<UTC>`.
2. **Apply** — runs the same logic as `klim upgrade` for the targets.
3. **Postcheck** — re-scans and validates the resulting state through four parallel checks:
   - **Shell resolution** — every installed tool still resolves via `exec.LookPath`.
   - **Binary validation** — each binary stats + responds to `--version`/`-V`/`version`/`-v`/`--help`. The probe is generous: any non-zero output counts as "works". Probes the pre-apply binary too so a pre-existing breakage doesn't masquerade as a regression.
   - **PATH consistency** — no new missing/duplicate PATH entries (reported as `warn`, not `fail`).
   - **Manager integrity** — every package manager that owns at least one installed tool responds to a fast probe. Missing PMs are reported as `skip`, not `fail`.
4. **Auto-rollback affordance** — on regression, prints the exact `klim rollback <name>` command needed to restore the pre-apply state. We deliberately do **not** auto-execute the rollback (downgrades are PM-specific and racy; you keep control).

Failures classify regressions vs. pre-existing issues: only tools that were working pre-apply and are broken now trip the rollback prompt. Pre-existing problems are surfaced as warnings.

## Flags

| Flag | Description |
|---|---|
| `--no-checkpoint` | Skip the pre-apply checkpoint capture. |
| `--no-postcheck` | Skip the post-apply validation pass. |
| `--postcheck-concurrency N` | Max parallel binary probes during postcheck. `0` = `runtime.NumCPU`. |
| `--postcheck-budget DURATION` | Wall-clock ceiling for the postcheck pass. Default `60s`. Probes still in-flight when the budget elapses are reported as `skip`. |
| `--output {text,json}` | Output format. |
| `--dry-run` | Print the plan and exit without executing. |
| `--yes` | Skip the per-tool confirmation prompt. |
| `--source <pm>` | Force a specific package manager. |

Use `klim upgrade` if you want the apply step with **none** of the wrapper behaviour.

## Exit codes

| Code | Meaning |
|---|---|
| `0` | Apply succeeded and every postcheck passed. |
| `3` | Apply succeeded but postcheck detected one or more regressions. A `klim rollback <name>` command is printed; rerun the apply after rolling back, or use `--no-postcheck` to accept the warnings. |

## Example

```
$ klim apply kubectl
💾 Pre-apply checkpoint saved: pre-apply-20260511-163045
   File:    ~/.klim/checkpoints/pre-apply-20260511-163045.yaml
   Restore: klim rollback pre-apply-20260511-163045

✓ kubectl 1.31.0 → 1.32.0 (brew)

Postcheck (3.2s):
  ✓ shell resolution     149 installed tool(s) resolve via PATH  (0s)
  ✓ binary validation    149 binary(ies) verified  (2.8s)
  ⚠ PATH consistency     2 PATH issue(s)  (0s)
        - duplicate at position 28 (first seen at 22): C:\Program Files\nodejs\
        - missing directory: C:\.tools\.npm-global
  ✓ manager integrity    1 package manager(s) healthy  (0.4s)
```

## Related

- [`klim plan`](./plan.md) — preview the changes
- [`klim checkpoint`](./checkpoint.md) — manage named snapshots
- [`klim rollback`](./rollback.md) — produce a plan that restores a checkpoint
- [`klim upgrade`](./upgrade.md) — apply without checkpoint/postcheck
