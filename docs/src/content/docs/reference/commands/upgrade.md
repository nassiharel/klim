---
title: "klim upgrade"
description: Upgrade installed tools to the latest available version
---

`klim upgrade` brings installed tools to the latest version reported by
their package manager. Flag set matches [`klim install`](./install);
source precedence differs slightly so the upgrade runs through the
package manager the tool was actually installed from (see below).

## Source precedence

For an installed tool, klim picks the package manager in this order:

1. `--source <pm>` flag (per invocation), if it maps to a package id
   for this tool
2. `defaults.preferred_source` in `config.yaml`, if it maps to a
   package id
3. The tool's installed package manager (detected during the PATH scan)
4. `BestInstallSource()` — last-ditch OS-priority fallback

That ordering avoids the surprise of running `winget upgrade jq` on a
jq that was installed via scoop. The same precedence applies to
[`klim remove`](./remove).

## Usage

```bash
klim upgrade [tool...] [flags]
```

At least one positional tool name **or** `--pack` is required.

## Behavior per target

| State | Outcome |
|-------|---------|
| Installed and update available | upgrade |
| Installed and already at latest | skipped (`up_to_date`) |
| Not installed | skipped (`not_installed`) — use `klim install` |
| Not in catalog | reported, skipped |

`klim upgrade --pack <name>` is therefore safe to run on machines that
have only some of the pack's tools — missing tools are skipped, no
auto-install happens.

## Flags

Same as [`klim install`](./install#flags):
`--source`, `--pack` (repeatable), `--dry-run`, `--yes`/`-y`,
`--refresh`, `--output`.

## Examples

```bash
# Upgrade a single tool
klim upgrade jq

# Upgrade everything in a pack
klim upgrade --pack go-dev

# Force a specific manager
klim upgrade jq --source brew --yes

# Dry-run a multi-pack upgrade
klim upgrade --pack rust-dev --pack web-dev --dry-run

# JSON for scripts
klim upgrade --pack go-dev --output json --yes
```

## Exit codes

Same as `klim install`: 0 OK, 1 runtime error, 2 usage error,
3 partial failure.

## See also

- [`klim install`](./install)
- [`klim remove`](./remove)
- [`klim update`](./update) — upgrade klim itself
