---
title: klim security
description: Umbrella for klim's security commands — audit, vulnerability scan, and compliance.
---

`klim security` groups every command that helps you reason about the
safety of your toolchain. With no arguments it prints a summary across
all subcommands; otherwise, dispatch to a specific check.

Environment health (PATH conflicts, multi-installs, missing PMs,
stale caches) lives under the top-level [`klim health`](./health.md)
command — it's a separate concern from supply-chain security.

## Synopsis

```
klim security                       # aggregated summary
klim security audit                 # archived/stale/license findings
klim security vuln                  # CVE/GHSA scan via OSV.dev
klim security compliance            # validate against a policy
```

`klim audit` and `klim compliance` are **not** top-level commands.
Use the `klim security <sub>` form.

## Subcommands

### `klim security audit`

Static analysis on the installed catalog. Flags archived upstreams,
tools without a recent release, license red flags, and missing
publishers.

### `klim security vuln`

Queries [OSV.dev](https://osv.dev) for known vulnerabilities affecting
the installed versions of every tool that maps to a supported
ecosystem. Coverage today is **npm only** — OSV.dev rejects the
`Homebrew` and `GitHub` ecosystems with HTTP 400, so brew-only and
GitHub-slug-only tools are listed under `skipped`. See the dedicated
[`klim security vuln`](./vuln.md) reference for full
flag documentation.

Exit codes: `0` = clean or `--fail-on` not set, `1` = vuln lookup
hard-failed (network, OSV down, etc.), `3` = findings meet or
exceed `--fail-on`.

### `klim security compliance`

Validates the installed toolchain against a policy file. Policies are
fetched from `compliance.url` in `config.yaml` and cached locally.

The `max_vuln_severity` policy field reads the local vulnerability
cache populated by `klim security vuln` and adds a violation for any
tool whose worst severity meets or exceeds the threshold. The gate
silently skips when the cache is empty — `klim install` won't fail
just because the user hasn't run a vuln scan. Run a fresh scan in
CI to enforce the gate strictly.

## Output convention

All `klim security` commands print human-readable progress to stderr
and machine-readable payloads (`--output json`) to stdout. See
[configuration reference](../configuration.md).

## Related

- [`klim health`](./health.md) — environment health & PATH diagnostics
- [`klim security vuln`](./vuln.md) — vulnerability scan reference
- [`klim score`](./score.md) — composite security score per tool
- [`klim trail`](./trail.md) — change history (every install/upgrade)

