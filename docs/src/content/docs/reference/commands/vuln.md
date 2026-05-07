---
title: klim security vuln
description: Scan installed tools for known vulnerabilities (CVE / GHSA) via OSV.dev.
---

`klim security vuln` queries [OSV.dev](https://osv.dev) for known
vulnerabilities affecting the installed versions of your tools. It
caches results locally so repeated runs are fast and offline-tolerant.

## Synopsis

```
klim security vuln [flags]
```

## Examples

```bash
# Plain run — uses cache when fresh, refreshes when stale
klim security vuln

# Force a fresh fetch
klim security vuln --force-refresh-vulns

# Fail (exit code 3) on any High or Critical finding — useful in CI
klim security vuln --fail-on high

# Machine-readable
klim security vuln --output json
```

## Flags

| Flag | Default | Description |
| --- | --- | --- |
| `--output {text,json}` | `text` | Output format. JSON goes to stdout; human progress to stderr. |
| `--fail-on {low,medium,high,critical}` | from `config.yaml` (default: empty = never fail) | Exit code 3 if any finding meets or exceeds this severity. |
| `--force-refresh-vulns` | `false` | Bypass the local cache and re-query OSV.dev. With this flag, a fetch failure is **not** masked by stale cache fallback — useful in CI. |
| `--url <url>` | from `config.yaml` (default `https://api.osv.dev`) | Override the OSV.dev endpoint (testing / mirrors). Note: cache is keyed by URL, so a one-shot override writes to a different cache file than passive surfaces (`klim info`, web `/security`) read. |
| `--refresh` | `false` | Force a fresh PATH scan instead of using the scan cache. |

## Severity model

OSV records vary in how they express severity. klim collapses every
finding into one of four buckets:

- **Critical** — CVSS ≥ 9.0 or labeled `CRITICAL` by the source DB
- **High** — CVSS 7.0–8.9 or `HIGH`
- **Medium** — CVSS 4.0–6.9 or `MODERATE`/`MEDIUM`
- **Low** — CVSS < 4.0 or `LOW`
- **Unknown** — CVSS missing/unparseable

Vector-only CVSS scores (no numeric base) are reported as Unknown;
klim does not ship a CVSS calculator.

## Coverage

`klim security vuln` only scans tools with a recognized OSV ecosystem
mapping. Currently that's:

- **npm globals** — mapped via `packages.npm` in the marketplace catalog

OSV.dev does **not** accept `Homebrew` or `GitHub` as query
ecosystems (the API returns HTTP 400 "Invalid ecosystem"), so brew
formulas and GitHub-by-slug tools are listed under `skipped`. We're
tracking adding Go modules / PyPI / crates / RubyGems support as
catalog metadata grows.

Tools installed via winget, scoop, choco, apt, snap, or brew without
a parallel npm id will appear under `skipped` with a reason.

## Cache

Results are cached at `~/.klim/vuln/cache-<sha256-prefix>.yaml`.
The cache file is keyed by OSV URL (allowing private mirrors); the
default endpoint and any custom `vuln.url` get separate files. On
fetch failure the last successful payload is used (stale-fallback)
unless `--force-refresh-vulns` is set, in which case the fetch error
is propagated.

## JSON schema (excerpt)

```json
{
  "scanned_at": "2026-05-02T12:34:56Z",
  "tools_scanned": 14,
  "source": "https://api.osv.dev",
  "matches": [
    {
      "tool": "yarn",
      "installed_version": "1.22.0",
      "coord": { "ecosystem": "npm", "package": "yarn", "version": "1.22.0" },
      "vulnerabilities": [
        {
          "id": "GHSA-xxxx-yyyy-zzzz",
          "severity": "HIGH",
          "summary": "...",
          "fixed_in": "1.22.1",
          "url": "https://github.com/advisories/GHSA-..."
        }
      ]
    }
  ],
  "skipped": [
    { "tool": "winget-only-tool", "reason": "no OSV-queryable ecosystem (only npm packages currently supported)" }
  ]
}
```

## Configuration

```yaml
# ~/.klim/config/config.yaml
vuln:
  url: https://api.osv.dev
  auto_refresh: true
  refresh_interval: 24h
  fail_on_severity: high
```

## Related

- [`klim security`](./security.md) — umbrella reference
- [`klim score`](./score.md) — composite per-tool score (folds in vuln data)
