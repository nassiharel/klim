---
title: clim security
description: Umbrella for clim's security commands — health checks, audit, vulnerability scan, and compliance.
---

`clim security` groups every command that helps you reason about the
safety of your toolchain. With no arguments it prints a summary across
all subcommands; otherwise, dispatch to a specific check.

## Synopsis

```
clim security                       # aggregated summary
clim security health                # PATH / shell / network diagnostics
clim security audit                 # archived/stale/license findings
clim security vuln                  # CVE/GHSA scan via OSV.dev
clim security compliance            # validate against a policy
```

`clim audit`, `clim doctor`, and `clim compliance` are **not**
top-level commands. Use the `clim security <sub>` form.

## Subcommands

### `clim security health`

Environment diagnostics. Verifies PATH integrity, shell hook
installation, network reachability of the marketplace and OSV.dev,
and detects PATH-shadowing where a user-writable directory shadows a
system tool.

Flags: `--output {text,json}` (default text).

### `clim security audit`

Static analysis on the installed catalog. Flags archived upstreams,
tools without a recent release, license red flags, and missing
publishers.

### `clim security vuln`

Queries [OSV.dev](https://osv.dev) for known vulnerabilities affecting
the installed versions of every tool that maps to a supported
ecosystem (npm, Homebrew formula, GitHub-by-slug). See the dedicated
[`clim security vuln`](/reference/commands/vuln/) reference for full
flag documentation.

Exit codes: `0` = clean, non-zero when findings meet or exceed
`--fail-on` (default `high`).

### `clim security compliance`

Validates the installed toolchain against a policy file. Policies are
fetched from `compliance.url` in `config.yaml` and cached locally.
The `max_vuln_severity` policy field cross-references the local
vulnerability cache populated by `clim security vuln`.

## Output convention

All `clim security` commands print human-readable progress to stderr
and machine-readable payloads (`--output json`) to stdout. See
[CLI conventions](/reference/cli-conventions/).

## Related

- [`clim security vuln`](/reference/commands/vuln/) — vulnerability scan reference
- [`clim score`](/reference/commands/score/) — composite security score per tool
- [`clim trail`](/reference/commands/trail/) — change history (every install/upgrade)
