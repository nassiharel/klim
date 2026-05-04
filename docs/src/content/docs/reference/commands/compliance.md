---
title: "clim compliance"
description: Validate installed tools against a company compliance policy
---

Check installed tools against a compliance policy file that defines allowed sources, licenses, blocked tools, and required tools.

## Subcommands

| Command | Description |
|---------|-------------|
| `clim compliance check` | Validate tools against the policy |
| `clim compliance show` | Show the current policy details |
| `clim compliance init` | Generate a sample `.clim-policy.yaml` |
| `clim compliance refresh` | Force-refetch the policy from `compliance.url` and update the local cache |

## Flags

| Flag | Commands | Description |
|------|----------|-------------|
| `--policy` | check, show, init | Path to a local policy file (overrides config) |
| `--url` | check, refresh | Remote policy URL (overrides `compliance.url`) |
| `--output {text,json}` | check | Output format (canonical) |
| `--json` | check | Machine-readable JSON output (deprecated alias for `--output=json`) |
| `--refresh` | check | Force fresh tool scan (PATH rescan) |
| `--force-refresh-policy` | check | Force re-fetch policy from the URL even if the cache is still fresh |

## Policy File Format

```yaml
name: "My Company Policy"
description: "Tool compliance rules for engineering"

# Only allow these install sources
allowed_sources: [winget, brew, apt, scoop, npm]

# Only allow these licenses
allowed_licenses: [MIT, Apache-2.0, BSD-2-Clause, BSD-3-Clause, ISC]

# Block these licenses
blocked_licenses: [AGPL-3.0, GPL-3.0]

# Explicitly blocked tools
blocked_tools: [nmap]

# Tools that must be installed
required_tools:
  - name: git
  - name: gh
    version: ">=2.40"
```

## Policy Resolution

The policy is resolved in this order:
1. `--policy` flag (always a local file)
2. `--url` flag (remote, with caching)
3. `compliance.url` in `config.yaml` (remote, with caching)
4. `compliance.policy` in `config.yaml` (local file)
5. `~/.config/clim/compliance/policy.yaml` (default global location)

When a remote URL is used, clim caches the fetched policy at
`~/.config/clim/compliance/policy-cache.yaml`. The cache is kept fresh
according to `compliance.auto_refresh` and `compliance.refresh_interval`
in `config.yaml`. Failed fetches always fall back to the previous
cache rather than poison it — see *Cache safety* below.

## Cache safety

- The HTTP fetcher rejects non-`http`/`https` URLs (incl. `file://`)
  to keep an attacker-controlled `compliance.url` from reading local
  files; redirects are similarly re-validated and capped at 3.
- A freshly fetched payload is parsed as YAML *before* it replaces the
  cache, so a transient login page / proxy error / malformed response
  cannot break later compliance checks.
- The cache is written via temp file + rename so other clim processes
  cannot observe a half-written or empty file.

## Examples

```bash
# Generate a sample policy
clim compliance init

# Check compliance
clim compliance check

# Check with explicit policy file
clim compliance check --policy /path/to/policy.yaml

# Use a remote policy URL ad-hoc (no config edit required)
clim compliance check --url https://policies.example.com/clim.yaml

# JSON output for CI
clim compliance check --output json

# Force re-fetch the policy from the configured URL
clim compliance refresh

# Or re-fetch from an ad-hoc URL
clim compliance refresh --url https://policies.example.com/clim.yaml

# View current policy
clim compliance show
```

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | All tools comply, or only warning-level findings |
| 1 | One or more error-level violations found |

## Violation Rules

| Rule | Severity | Description |
|------|----------|-------------|
| `blocked_tool` | Error | Tool is explicitly blocked |
| `disallowed_source` | Error | Installed via a non-approved PM |
| `required_missing` | Error | Required tool not installed |
| `required_version` | Error | Required tool version not met |
| `disallowed_license` | Warning | License not in approved list |
| `blocked_license` | Error | License explicitly blocked |

## CI Usage

```yaml
- name: Compliance check
  run: clim compliance check --policy .clim-policy.yaml --json
```

## See Also

- [clim audit](/reference/commands/audit) — Security audit and SBOM
- [clim check](/reference/commands/check) — Validate .clim.yaml requirements
