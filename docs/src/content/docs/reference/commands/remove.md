---
title: "clim remove"
description: Remove installed tools via the system package manager
---

`clim remove` uninstalls tools using their original package manager
(or the `--source` you specify). Source precedence and flag set match
[`clim install`](./install).

## Usage

```bash
clim remove [tool...] [flags]
```

At least one positional tool name **or** `--pack` is required.

## Behavior per target

| State | Outcome |
|-------|---------|
| Installed | remove |
| Not installed | skipped silently (`not_installed`) |
| `clim` itself | refused — use the OS uninstaller for clim |
| Not in catalog | reported, skipped |

The self-protection refuses to remove the binary named `clim`, so
`clim remove clim` never runs the underlying package manager.

## Flags

Same as [`clim install`](./install#flags):
`--source`, `--pack` (repeatable), `--dry-run`, `--yes`/`-y`,
`--refresh`, `--output`.

## Examples

```bash
# Remove a single tool
clim remove jq

# Remove every installed tool in a pack
clim remove --pack go-dev --yes

# Pin the package manager
clim remove jq fzf --source brew --dry-run
```

## Exit codes

Same as `clim install`: 0 OK, 1 runtime error, 2 usage error,
3 partial failure.

## See also

- [`clim install`](./install)
- [`clim upgrade`](./upgrade)
