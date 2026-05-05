---
title: clim env
description: Generate, share, and apply environment fingerprints between machines.
---

`clim env` captures the shape of your clim-managed environment
(installed tools, favorites, custom packs, available package
managers, clim version, OS, audit/security counts) into a portable
artifact you can share via chat or commit to git.

## Synopsis

```
clim env [flags]                   # print compact token to stdout
clim env --output yaml > file.yaml # rich YAML form
clim env show <token-or-file>      # decode + pretty-print
clim env diff <token-or-file>      # diff vs current environment
clim env apply <token-or-file>     # reproduce locally
```

## Privacy

A profile contains:

- installed tools (catalog name, version, install source, category)
- favorites
- custom packs you've defined (name + tool list)
- which package managers are available on `$PATH`
- `clim` version + commit
- OS, architecture, and (best-effort) distro
- observational audit/security counts (warning/info totals; per-tool clean/watch/risk/unknown bucket counts)

Deliberately **not** captured:

- hostname, username
- absolute paths or any file contents
- environment variables
- catalog metadata that would identify the host (no IPs, no ssh fingerprints, etc.)

The artifact is deterministic from environment state alone (modulo `generated_at`), so passing it to a coworker leaks nothing beyond the inventory above. The hash recorded in the profile is recomputed by `clim env diff` from the decoded content, so an edited token can't fake a "matching hash".

## Output formats

| Flag | Output |
| --- | --- |
| (default, or `--output text`) | `clim:env:v1:<base64>` token, single line, paste-friendly |
| `--output yaml` | Multi-line YAML document |
| `--output json` | JSON document |

A typical 30-tool env produces a token under 1 KB after gzip+base64.

## Examples

```bash
# Copy the current env token to the clipboard (macOS).
clim env | pbcopy

# Save the rich YAML for a code review.
clim env --output yaml > my-env.yaml

# Inspect a coworker's token before applying it.
clim env show 'clim:env:v1:H4sIAAAAAAAA...'

# What would change if I applied this?
clim env diff 'clim:env:v1:H4sIAAAAAAAA...'

# Reproduce locally — installs tools that aren't already there,
# adds favorites, registers custom packs.
clim env apply 'clim:env:v1:H4sIAAAAAAAA...'

# Apply from a YAML file with no prompts (CI mode).
clim env apply ./my-env.yaml --yes
```

## What's in a Profile

```yaml
schema_version: 1
clim:
  version: v1.2.3
  commit: 97b8857
generated_at: "2026-05-05T17:30:00Z"
hash: f4a7b9c1de32     # first 12 hex of sha256 over canonical payload
                       # (excluding hash + generated_at), so the same
                       # env regenerated still has the same hash

os:
  goos: linux
  arch: amd64
  distro: "Ubuntu 22.04"

package_managers:      # which PM binaries exist on $PATH
  brew: true
  apt: true
  npm: true
  snap: false
  winget: false
  scoop: false
  choco: false

tools:                 # installed tools — name + version + source + category
  - name: jq
    version: "1.7"
    source: brew
    category: CLI

favorites: [fzf, jq, ripgrep]

packs:                 # user's custom packs only (marketplace packs
                       # are supplied by the catalog, not the env)
  - name: my-cli
    tools: [fzf, jq, ripgrep]

security:              # observational; never gates apply
  audit_warnings: 3
  audit_infos: 1
  verdicts:
    clean: 12
    watch: 4
    risk: 1
    unknown: 0
```

## Apply semantics

`clim env apply` is best-effort:

1. **Tools** — runs the same install plan as `clim import`; tools
   already installed locally are skipped, tools without a
   compatible package manager (e.g. apt-only on Windows) appear in
   the report as `skipped` rather than failing.
2. **Favorites** — additive merge; never removes existing favorites.
3. **Custom packs** — additive; existing packs with the same name
   are kept; to replace one, edit it out of `~/.config/clim/marketplace/custom-packs.yaml` (or use the TUI's My Packs tab to delete) and re-run apply.

Cross-OS gaps surface as informational entries, never as errors.

## Exit codes

| Code | Meaning |
| --- | --- |
| 0 | Success (or nothing to do) |
| 1 | Runtime failure (file I/O, scan, internal error) |
| 2 | Usage error: missing/invalid arg, or **malformed token** — invalid prefix, unknown schema version, oversize, corrupt base64/gzip/yaml, schema-version mismatch |
| 3 | Partial failure — some installs failed but others succeeded |

## Related

- [`clim share`](/reference/commands/share/) — narrower token (tool names only) for chat
- [`clim export`](/reference/commands/export/) — full YAML manifest (tools + OS + arch)
- [`clim import`](/reference/commands/import/) — install from a manifest file
