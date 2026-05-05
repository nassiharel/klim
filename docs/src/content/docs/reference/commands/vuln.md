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
| `--fail-on {none,low,medium,high,critical}` | from `config.yaml` (default `high`) | Exit non-zero if any finding meets or exceeds this severity. |
| `--force-refresh-vulns` | `false` | Bypass the local cache and re-query OSV.dev. With this flag, a fetch failure is **not** masked by stale cache fallback — useful in CI. |
| `--url <url>` | from `config.yaml` (default `https://api.osv.dev`) | Override the OSV.dev endpoint (testing / mirrors). Note: cache is keyed by URL, so a one-shot override writes to a different cache file than passive surfaces (`clim info`, web `/security`) read. |

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

Results are cached at `~/.config/clim/vuln/cache-<sha256-prefix>.yaml`.
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
