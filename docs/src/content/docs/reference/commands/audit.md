---
title: "clim audit"
description: Audit installed tools for security and compliance issues
---

Analyze installed tools for security and compliance concerns, and optionally generate a CycloneDX SBOM.

## Usage

```bash
clim audit [flags]
```

## Flags

| Flag | Description |
|------|-------------|
| `--json` | Machine-readable JSON output |
| `--refresh` | Force fresh scan, ignoring cache |
| `--sbom` | Generate CycloneDX 1.5 SBOM instead of audit report |

## Audit Checks

| Category | Severity | Description |
|----------|----------|-------------|
| Unmanaged | Warning | Tools installed from unknown/manual sources |
| No Version | Warning | Version could not be determined |
| Archived | Warning | Upstream GitHub repository is archived |
| Stale | Info | No upstream activity in 12+ months |
| Outdated | Info | Newer version available |

The report also includes a **license inventory** showing the distribution of licenses across your toolchain.

## Examples

```bash
# Human-readable audit report
clim audit

# JSON output for CI
clim audit --json

# Generate CycloneDX 1.5 SBOM
clim audit --sbom > sbom.json

# Force fresh scan for authoritative results
clim audit --refresh
```

## SBOM Output

The `--sbom` flag generates a [CycloneDX 1.5](https://cyclonedx.org/) JSON document containing:

- Tool name, version, and description
- License information (from GitHub metadata)
- VCS references (GitHub repository URLs)
- Install source and binary path (as custom properties)

## TUI

Audit findings are shown in the Doctor tab under the **Audit** sub-tab (use Tab/←/→ to switch).

## See Also

- [Doctor & Audit guide](/guides/doctor-audit) — Detailed walkthrough
- [clim doctor](/reference/commands/doctor) — Environment health check
