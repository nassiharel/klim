---
title: "clim upgrade"
description: Upgrade installed tools to the latest available version
---

`clim upgrade` brings installed tools to the latest version reported by
their package manager. Flag set matches [`clim install`](./install);
source precedence differs slightly so the upgrade runs through the
package manager the tool was actually installed from (see below).

## Source precedence

For an installed tool, clim picks the package manager in this order:

1. `--source <pm>` flag (per invocation), if it maps to a package id
   for this tool
2. `defaults.preferred_source` in `config.yaml`, if it maps to a
   package id
3. The tool's installed package manager (detected during the PATH scan)
4. `BestInstallSource()` — last-ditch OS-priority fallback

That ordering avoids the surprise of running `winget upgrade jq` on a
jq that was installed via scoop. The same precedence applies to
[`clim remove`](./remove).

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
