---
title: "klim install"
description: Install one or more tools or packs via the system package manager
---

`klim install` installs one or more tools (positional) and/or every tool
in one or more packs (`--pack`). Each tool is installed via its
preferred system package manager — klim does not bundle any binaries.

## Usage

```bash
klim install [tool...] [flags]
```

At least one positional tool name **or** `--pack` is required.

## Source precedence

klim picks the package manager for each tool using this precedence
(highest wins):

1. `--source <pm>` flag (per invocation)
2. `defaults.preferred_source` in `config.yaml` (global default)
3. OS-priority fallback — first available manager from the per-OS
   priority list (`brew → npm` on macOS, `winget → choco → scoop → npm`
   on Windows, `apt → snap → brew → npm` on Linux)

If the preferred source has no package id for a particular tool, klim
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
klim install jq fzf

# Install everything in a curated pack
klim install --pack go-dev

# Force a specific package manager and skip the prompt
klim install jq --source brew --yes

# Combine multiple packs and preview the plan
klim install --pack rust-dev --pack web-dev --dry-run

# Machine-readable output for CI / scripts
klim install jq --output json
```

## Behavior

For each target:

- **Already installed** → skipped (listed under "Already installed" in
  the plan summary, and as `skipped: already_installed` in JSON).
- **Not in catalog** → reported as a warning, skipped.
- **No package on this OS** → reported, skipped.
- **No package manager available** → reported, skipped.
- Otherwise → install command runs, output streams live to your
  terminal.

After execution klim invalidates its scan cache so subsequent commands
(`klim list`, `klim info`, `klim security health`) rescan PATH.

## Exit codes

| Code | Meaning |
|------|---------|
| 0 | All targets succeeded (or were already installed) |
| 1 | Runtime error |
| 2 | Usage error (unknown source, unknown pack, no targets) |
| 3 | At least one install failed |

## JSON output

`--output json` writes a single object to stdout. Human-readable
progress (the plan summary, package-manager output, success/failure
markers) goes to stderr — stdout is reserved for the final JSON
payload so it remains parseable for scripts and CI.

Schema (every field always present — empty arrays / `false` instead of missing keys for scripts and CI):

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
  ],
  "errors": []
}
```

`--output=json` implies non-interactive — there's no prompt.

## See also

- [`klim upgrade`](./upgrade) — bring installed tools to the latest version
- [`klim remove`](./remove) — uninstall tools
- [`klim import`](./import) — bulk install from a manifest file
- [`klim config`](./config) — set `defaults.preferred_source`
