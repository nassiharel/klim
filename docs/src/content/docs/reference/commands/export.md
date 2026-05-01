---
title: "clim export"
description: Export tools to stdout, snapshots, or named profiles
---

Export detected tools to YAML. Without a subcommand, prints to stdout. Includes snapshot and profile management for saved exports.

## Usage

```bash
clim export [flags]           # print manifest to stdout
clim export save [label]      # save as timestamped snapshot
clim export list              # list saved snapshots
clim export show <name>       # show a snapshot
clim export delete <name>     # delete a snapshot
clim export profile <command> # manage named profiles
```

## Flags

| Flag | Commands | Description |
|------|----------|-------------|
| `--refresh` | export | Force fresh scan, ignoring on-disk cache |

## Subcommands

### Snapshots (Saved Exports)

| Command | Description |
|---------|-------------|
| `clim export save [label]` | Save current tool state as a timestamped snapshot |
| `clim export list` | List saved snapshots |
| `clim export show <name>` | Show tools in a snapshot |
| `clim export delete <name>` | Delete a snapshot |

### Profiles (Named Snapshots)

| Command | Description |
|---------|-------------|
| `clim export profile save <name>` | Save current state as a named profile |
| `clim export profile list` | List saved profiles |
| `clim export profile show <name>` | Show a profile's tools |
| `clim export profile delete <name>` | Delete a profile |

## Snapshots vs Profiles

- **Snapshots** are timestamped — for point-in-time backups before upgrades or experiments
- **Profiles** are named — for switching between configurations ("work", "personal", "client-x")

## Examples

```bash
# Export to stdout
clim export

# Save to file
clim export > my-tools.yaml

# Force fresh scan before export
clim export --refresh > my-tools.yaml

# Save before a big upgrade
clim export save "before-k8s-upgrade"

# List all snapshots
clim export list

# View what was in a snapshot
clim export show before-k8s-upgrade

# Save a named profile
clim export profile save work

# List profiles
clim export profile list

# Import on another machine
clim import my-tools.yaml
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

## Storage

- Snapshots: `~/.config/clim/snapshots/<timestamp>-<label>.yaml`
- Profiles: `~/.config/clim/profiles/<name>.yaml`

Both use the same YAML manifest format, so snapshots can also be used with `clim diff` and `clim import`.

## Name Matching

The `show` and `delete` commands support fuzzy matching — you can use a label, prefix, suffix, or substring:

```bash
clim export show before-k8s    # matches "2026-04-30T...-before-k8s-upgrade"
clim export show upgrade       # also matches
```

## See Also

- [`clim import`](/reference/commands/import) — Install tools from a manifest
- [`clim share`](/reference/commands/share) — Generate a compact share token
- [`clim diff`](/reference/commands/diff) — Compare against a snapshot or manifest
