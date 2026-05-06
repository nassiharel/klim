---
title: "klim remove"
description: Remove installed tools via the system package manager
---

`klim remove` uninstalls tools using the package manager they were
originally installed from (detected via the `klim list` scan), or the
`--source` you specify. Falls back to OS-priority when the installed
source isn't recorded. Source precedence overall matches
[`klim install`](./install).

## Usage

```bash
klim remove [tool...] [flags]
```

At least one positional tool name **or** `--pack` is required.

## Behavior per target

| State | Outcome |
|-------|---------|
| Installed | remove |
| Not installed | skipped (`not_installed`) |
| `klim` itself | refused — use the OS uninstaller for klim |
| Not in catalog | reported, skipped |

The self-protection refuses to remove the binary named `klim`, so
`klim remove klim` never runs the underlying package manager.

## Flags

Same as [`klim install`](./install#flags):
`--source`, `--pack` (repeatable), `--dry-run`, `--yes`/`-y`,
`--refresh`, `--output`.

## Examples

```bash
# Remove a single tool
klim remove jq

# Remove every installed tool in a pack
klim remove --pack go-developer --yes

# Pin the package manager
klim remove jq fzf --source brew --dry-run
```

## Exit codes

Same as `klim install`: 0 OK, 1 runtime error, 2 usage error,
3 partial failure.

## See also

- [`klim install`](./install)
- [`klim upgrade`](./upgrade)
