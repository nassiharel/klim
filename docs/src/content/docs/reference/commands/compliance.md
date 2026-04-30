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

## Flags

| Flag | Commands | Description |
|------|----------|-------------|
| `--policy` | check, show, init | Path to policy file (overrides config) |
| `--json` | check | Machine-readable JSON output |
| `--refresh` | check | Force fresh scan |

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

The policy file is resolved in this order:
1. `--policy` flag
2. `compliance.policy` in config.yaml
3. `.clim-policy.yaml` in current directory

## Examples

```bash
# Generate a sample policy
clim compliance init

# Check compliance
clim compliance check

# Check with explicit policy file
clim compliance check --policy /path/to/policy.yaml

# JSON output for CI
clim compliance check --json

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
