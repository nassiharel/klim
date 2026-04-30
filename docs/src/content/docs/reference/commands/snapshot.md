---
title: "clim snapshot"
description: Save and restore environment snapshots and named profiles
---

Manage timestamped snapshots of your installed tools and named profiles for switching between configurations.

## Subcommands

### Snapshots

| Command | Description |
|---------|-------------|
| `clim snapshot save [label]` | Save current tool state as a snapshot |
| `clim snapshot list` | List saved snapshots |
| `clim snapshot show <name>` | Show tools in a snapshot |
| `clim snapshot delete <name>` | Delete a snapshot |

### Profiles

| Command | Description |
|---------|-------------|
| `clim snapshot profile save <name>` | Save current state as a named profile |
| `clim snapshot profile list` | List saved profiles |
| `clim snapshot profile show <name>` | Show a profile's tools |
| `clim snapshot profile delete <name>` | Delete a profile |

## Snapshots vs Profiles

- **Snapshots** are timestamped — for point-in-time backups before upgrades or experiments
- **Profiles** are named — for switching between configurations ("work", "personal", "client-x")

## Examples

```bash
# Save before a big upgrade
clim snapshot save "before-k8s-upgrade"

# List all snapshots
clim snapshot list

# View what was in a snapshot
clim snapshot show before-k8s-upgrade

# Save a named profile
clim snapshot profile save work

# List profiles
clim snapshot profile list

# View a profile
clim snapshot profile show work
```

## Storage

- Snapshots: `~/.config/clim/snapshots/<timestamp>-<label>.yaml`
- Profiles: `~/.config/clim/profiles/<name>.yaml`

Both use the same YAML manifest format as `clim export`, so snapshots can also be used with `clim diff` and `clim import`.

## Name Matching

The `show` and `delete` commands support fuzzy matching — you can use a label, prefix, suffix, or substring:

```bash
clim snapshot show before-k8s    # matches "2026-04-30T...-before-k8s-upgrade"
clim snapshot show upgrade       # also matches
```

## See Also

- [clim export](/reference/commands/export) — Export tools to a manifest file
- [clim diff](/reference/commands/diff) — Compare against a snapshot or manifest
- [Backup & Restore guide](/guides/backup-restore)
