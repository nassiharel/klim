---
title: clim security vuln
description: Scan installed tools for known vulnerabilities (CVE / GHSA) via OSV.dev.
---

`clim security vuln` queries [OSV.dev](https://osv.dev) for known
vulnerabilities affecting the installed versions of your tools. It
caches results locally so repeated runs are fast and offline-tolerant.

## Synopsis

```
clim security vuln [flags]
```

## Examples

```bash
# Plain run — uses cache when fresh, refreshes when stale
clim security vuln

# Force a fresh fetch
clim security vuln --force-refresh-vulns

# Fail (exit 3) on any High or Critical finding — useful in CI
clim security vuln --fail-on high

# Machine-readable
clim security vuln --output json
```

## Flags

| Flag | Default | Description |
| --- | --- | --- |
| `--output {text,json}` | `text` | Output format. JSON goes to stdout; human progress to stderr. |
| `--fail-on {none,low,medium,high,critical}` | from `config.yaml` (default `high`) | Exit with code 3 if any finding meets or exceeds this severity. |
| `--force-refresh-vulns` | `false` | Bypass the local cache and re-query OSV.dev. |
| `--vuln-url <url>` | `https://api.osv.dev` | Override the OSV.dev endpoint (testing / mirrors). |

## Severity model

OSV records vary in how they express severity. clim collapses every
finding into one of four buckets:

- **Critical** — CVSS ≥ 9.0 or labeled `CRITICAL` by the source DB
- **High** — CVSS 7.0–8.9 or `HIGH`
- **Medium** — CVSS 4.0–6.9 or `MODERATE`/`MEDIUM`
- **Low** — CVSS < 4.0 or `LOW`
- **Unknown** — CVSS missing/unparseable

Vector-only CVSS scores (no numeric base) are reported as Unknown;
clim does not ship a CVSS calculator.

## Coverage

`clim security vuln` only scans tools with a recognized OSV ecosystem
mapping:

- npm globals (mapped via `packages.npm`)
- Homebrew formulas (`packages.brew`) — best-effort, mapped to OSV
  Homebrew namespace
- Tools with a `github: owner/repo` annotation in the marketplace
  catalog

Other package managers (winget, scoop, choco, apt, snap) are listed
under `skipped` in the JSON output and noted with a reason.

## Cache

Results are cached at
`~/.config/clim/cache/vulns/<sha256-of-url>.yaml`. The cache is
keyed by OSV URL (allowing private mirrors). On fetch failure the
last successful payload is used (stale-fallback) and the operation
prints a warning to stderr.

## JSON schema (excerpt)

```json
{
  "fetched_at": "2026-05-02T12:34:56Z",
  "source": "https://api.osv.dev",
  "matches": [
    {
      "tool": "left-pad",
      "installed_ver": "1.3.0",
      "vulnerabilities": [
        {
          "id": "GHSA-xxxx-yyyy-zzzz",
          "severity": "high",
          "summary": "...",
          "fixed_in": "1.3.1",
          "url": "https://github.com/advisories/GHSA-..."
        }
      ]
    }
  ],
  "skipped": [
    { "tool": "winget-only-tool", "reason": "no OSV ecosystem mapping" }
  ]
}
```

## Configuration

```yaml
# ~/.config/clim/config.yaml
vuln:
  url: https://api.osv.dev
  auto_refresh: true
  refresh_interval: 24h
  fail_on_severity: high
```

## Related

- [`clim security`](/reference/commands/security/) — umbrella reference
- [`clim score`](/reference/commands/score/) — composite per-tool score (folds in vuln data)
