---
title: "clim export"
description: Export installed tools to a portable YAML manifest
---

Export all detected tools to YAML, suitable for reinstalling on another machine.

## Usage

```bash
clim export [flags]
```

## Flags

| Flag | Description |
|------|-------------|
| `--refresh` | Force fresh scan, ignoring on-disk cache |

## Examples

```bash
# Save to file
clim export > my-tools.yaml

# Print to stdout
clim export

# Force fresh scan before export
clim export --refresh > my-tools.yaml
```

## Output Format

The YAML manifest includes all installed tools with their versions and package IDs:

```yaml
tools:
  - name: az
    version: "2.67.0"
    source: brew
    packages:
      brew: azure-cli
      winget: Microsoft.AzureCLI
      choco: azure-cli
  - name: docker
    version: "24.0.7"
    source: manual
    packages:
      brew: docker
      winget: Docker.DockerDesktop
      choco: docker-desktop
```

## Cross-Platform Portability

The manifest is **cross-platform** — it contains package IDs for all supported package managers. When imported on a different OS, clim automatically picks the best available package manager.

## See Also

- [`clim import`](/reference/commands/import) — Install tools from a manifest
- [`clim share`](/reference/commands/share) — Generate a compact share token
