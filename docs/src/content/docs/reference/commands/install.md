---
title: "clim install"
description: Install one or more tools or packs via the system package manager
---

`clim install` installs one or more tools (positional) and/or every tool
in one or more packs (`--pack`). Each tool is installed via its
preferred system package manager — clim does not bundle any binaries.

## Usage

```bash
clim install [tool...] [flags]
```

At least one positional tool name **or** `--pack` is required.

## Source precedence

clim picks the package manager for each tool using this precedence
(highest wins):

1. `--source <pm>` flag (per invocation)
2. `defaults.preferred_source` in `config.yaml` (global default)
3. OS-priority fallback — first available manager from the per-OS
   priority list (`brew → npm` on macOS, `winget → choco → scoop → npm`
   on Windows, `apt → snap → brew → npm` on Linux)

If the preferred source has no package id for a particular tool, clim
falls through to the next level rather than failing.

## Flags

| Flag | Short | Description |
|------|-------|-------------|
| `--source <pm>` | | Package manager: `winget`, `choco`, `scoop`, `brew`, `apt`, `snap`, `npm` |
| `--pack <name>` | | Pack name to expand into a tool list (repeatable) |
| `--dry-run` | | Print the plan without executing |
| `--yes` | `-y` | Skip the confirmation prompt |
| `--refresh` | | Ignore the scan cache and rescan PATH |
| `--output <fmt>` | | `text` (default) or `json` |

## Examples

```bash
# Install two tools using the OS-default package manager
clim install jq fzf

# Install everything in a curated pack
clim install --pack go-dev

# Force a specific package manager and skip the prompt
clim install jq --source brew --yes

# Combine multiple packs and preview the plan
clim install --pack rust-dev --pack web-dev --dry-run

# Machine-readable output for CI / scripts
clim install jq --output json
```

## Behavior

For each target:

- **Already installed** → skipped (silent in text mode, listed under
  `skipped: already_installed` in JSON).
- **Not in catalog** → reported as a warning, skipped.
- **No package on this OS** → reported, skipped.
- **No package manager available** → reported, skipped.
- Otherwise → install command runs, output streams live to your
  terminal.

After execution clim invalidates its scan cache so subsequent commands
(`clim list`, `clim info`, `clim doctor`) rescan PATH.

## Exit codes

| Code | Meaning |
|------|---------|
| 0 | All targets succeeded (or were already installed) |
| 1 | Runtime error |
| 2 | Usage error (unknown source, unknown pack, no targets) |
| 3 | At least one install failed |

## JSON output

`--output json` writes a single object to stdout (text summary still
goes to stderr until execution starts). Schema:

```json
{
  "action": "install",
  "dry_run": false,
  "planned": [
    {
      "name": "jq",
      "display": "jq",
      "source": "brew",
      "cmd": ["brew", "install", "jq"]
    }
  ],
  "succeeded": ["jq"],
  "failed": [],
  "skipped": [
    { "name": "fzf", "reason": "already_installed" }
  ]
}
```

`--output=json` implies non-interactive — there's no prompt.

## See also

- [`clim upgrade`](./upgrade) — bring installed tools to the latest version
- [`clim remove`](./remove) — uninstall tools
- [`clim import`](./import) — bulk install from a manifest file
- [`clim config`](./config) — set `defaults.preferred_source`
