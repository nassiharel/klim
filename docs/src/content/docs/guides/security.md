---
title: Doctor & Audit
description: Diagnose environment issues and audit your toolchain for security concerns
---

klim includes two complementary health-check features: **Doctor** for environment diagnostics and **Audit** for security/compliance analysis.

## Doctor

The doctor checks your environment for common issues that can cause confusion or break tools.

### CLI

```bash
klim security health
```

### What It Checks

- **Duplicate PATH entries** — same directory listed multiple times
- **Broken PATH entries** — directories that don't exist, aren't accessible, or aren't directories
- **Conflicting versions** — same tool installed at multiple PATH locations with different versions
- **Missing package managers** — PMs that could manage your tools but aren't installed
- **Stale cache** — scan cache older than 7 days
- **Unresolved versions** — installed tools where version couldn't be determined
- **Outdated tools** — summary of available updates

### TUI

Press `9` to open the Security tab. Issues are grouped by category with color-coded severity:

- 🔴 **Error** — something is broken
- 🟡 **Warning** — potential problem
- 🔵 **Info** — suggestion or note

## Audit

The audit analyzes your installed tools for security and compliance concerns.

### CLI

```bash
# Human-readable report
klim security audit

# CycloneDX 1.5 SBOM
klim security audit --sbom > sbom.json
```

### What It Checks

- **Unmanaged installs** — tools from unknown sources, not tracked by any PM
- **Archived projects** — upstream GitHub repo marked as archived
- **Stale projects** — no upstream activity in 12+ months
- **Missing versions** — can't verify security status
- **Outdated tools** — updates available

It also generates a **license inventory** showing the distribution of licenses across your toolchain.

### TUI

In the Security tab, press Tab or → to switch to the **Audit** sub-tab. It shows the same findings as `klim security audit` with color-coded severity and a license summary.

### SBOM Generation

The `--sbom` flag generates a [CycloneDX 1.5](https://cyclonedx.org/) JSON document suitable for compliance pipelines:

```bash
# Generate and save
klim security audit --sbom > sbom.json

# Pipe to a compliance tool
klim security audit --sbom | cyclonedx-cli validate --input-format json
```

## CI Integration

Both commands support JSON output and meaningful exit codes:

```yaml
# GitHub Actions example
- name: Environment health check
  run: klim security health --output json

- name: Security audit
  run: klim security audit --output json
```
