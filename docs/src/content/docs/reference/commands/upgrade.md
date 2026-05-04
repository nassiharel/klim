---
title: "clim upgrade"
description: Upgrade installed tools to the latest available version
---

`clim upgrade` brings installed tools to the latest version reported by
their package manager. Source precedence and flag set match
[`clim install`](./install).

## Usage

```bash
clim upgrade [tool...] [flags]
```

At least one positional tool name **or** `--pack` is required.

## Behavior per target

| State | Outcome |
|-------|---------|
| Installed and update available | upgrade |
| Installed and already at latest | skipped (`up_to_date`) |
| Not installed | skipped (`not_installed`) — use `clim install` |
| Not in catalog | reported, skipped |

`clim upgrade --pack <name>` is therefore safe to run on machines that
have only some of the pack's tools — missing tools are skipped, no
auto-install happens.

## Flags

Same as [`clim install`](./install#flags):
`--source`, `--pack` (repeatable), `--dry-run`, `--yes`/`-y`,
`--refresh`, `--output`.

## Examples

```bash
# Upgrade a single tool
clim upgrade jq

# Upgrade everything in a pack
clim upgrade --pack go-dev

# Force a specific manager
clim upgrade jq --source brew --yes

# Dry-run a multi-pack upgrade
clim upgrade --pack rust-dev --pack web-dev --dry-run

# JSON for scripts
clim upgrade --pack go-dev --output json --yes
```

## Exit codes

Same as `clim install`: 0 OK, 1 runtime error, 2 usage error,
3 partial failure.

## See also

- [`clim install`](./install)
- [`clim remove`](./remove)
- [`clim update`](./update) — upgrade clim itself
