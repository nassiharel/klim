---
title: klim plan
description: Terraform-plan-style preview of toolchain changes, with confidence scoring, risk analysis, disk impact, and time estimates.
---

`klim plan` is a read-only preview of every change `klim apply` would make. Modelled on `terraform plan` — produce a plan, review it, then execute it.

## Synopsis

```
klim plan [tool...] [flags]
```

## Sections

The text output is structured into five sections:

| Section | Purpose |
|---|---|
| **Planned changes** | Grouped by package manager (`brew:`, `winget:`, `npm:`, …). Each line is a single transition (install / upgrade / remove) with the from-version and to-version. |
| **Upgrade confidence** | 0-100% per upgrade, with the full factor breakdown (semver delta, tool-specific fragility, plugin ecosystem detection, foundational-runtime size, community-signal placeholder). |
| **Risk analysis** | Heuristic warnings (kubectl client-server skew, Terraform provider lockfile, native modules, build-cache invalidation, venv orphaning, container restarts). |
| **Disk impact** | Estimated `+N MB cache` plus `-N MB reclaimable` from old runtimes detected in `tool.Instances`. |
| **Estimated time** | Pessimistic per-PM wall-clock estimate summed across the change set. |

## Flags

| Flag | Description |
|---|---|
| `--output {text,json}` | Output format. JSON emits the full `plan.Plan` schema. |
| `--refresh` | Force a fresh scan, ignoring the local cache. |
| `--file <path>` | Plan against a `.klim.yaml` target manifest (installs missing tools, upgrades where required). |
| `--detailed-exitcode` | Exit `3` when changes are pending (mirrors `terraform plan -detailed-exitcode`). Default exits `0` either way so casual runs don't fail. |

## Confidence scoring

Each upgrade row carries a confidence score derived from the following factors (each contributes a delta that's printed alongside the row):

- **Semver delta** — patch `-2`, minor `-8`, major `-25`.
- **Tool-specific fragility** — kubectl `-20` (client-server skew), node `-15` (native modules), python `-15` (venv coupling), docker `-10`, terraform `-8` (provider lockfile), go/rust `-5` (build-cache invalidation).
- **Plugin ecosystem** — when related plugins/tools are locally installed (helm + kubectl, tflint + terraform, npm + node, …), confidence drops further proportional to the count.
- **Foundational runtime ecosystem** — upgrading node/python/go/ruby/rust/java while you have 30+ tools installed adds a `-5` penalty (ripple risk).
- **Community signal** (placeholder) — reserved slot for future live integration with GitHub issue data; emits a `0`-delta factor today so users see the slot exists.

A score below 70% suggests reviewing the breaking-change notes before applying.

## Example

```
$ klim plan
Planned changes:

  brew:
    terraform 1.8 -> 1.9    (confidence: 92%)
    kubectl 1.31 -> 1.32    (confidence: 48%)

  npm:
    @anthropic-ai/claude-code 1.2 -> 1.4    (confidence: 98%)

Upgrade confidence:
  Terraform upgrade confidence: 92%
    -8  1.8 → 1.9 adds new features that may shift defaults
  kubectl upgrade confidence: 48%
    -8  1.31 → 1.32 adds new features that may shift defaults
    -20 kubectl ±1 minor version of the API server is supported; further skew breaks features
    -10 plugin ecosystem detected (helm, kustomize, k9s installed)
    -14 foundational runtime + 149 tools — upgrade ripples through many of them

Risk analysis:
  ⚠ kubectl: confirm your cluster's API server version supports 1.32
  ℹ terraform: provider lockfile may need refresh — run terraform init -upgrade

Disk impact:
  +1.2GB cache
  -400MB old runtimes removable

Estimated time:
  4m 20s
```

## Related

- [`klim apply`](./apply.md) — execute the plan with safety wrapper
- [`klim checkpoint`](./checkpoint.md) — capture a named snapshot before applying
- [`klim rollback`](./rollback.md) — produce a plan that restores a checkpoint
- [`klim upgrade`](./upgrade.md) — bare-bones upgrade with no plan/checkpoint wrapper
