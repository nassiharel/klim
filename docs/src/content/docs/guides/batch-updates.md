---
title: Batch Updates
description: Upgrade multiple tools at once with the Updates tab
---

The **Updates** tab (press `3`) shows all tools that have newer versions available and lets you upgrade them in batch.

## How It Works

clim compares your installed versions against the latest versions available from your package managers. Tools with available updates appear in this tab with both current and latest versions displayed.

## Keybindings

| Key | Action |
|-----|--------|
| `↑` / `↓` | Navigate |
| `Space` | Toggle tool for upgrade |
| `a` | Select all tools |
| `u` | Upgrade all selected tools |
| `Enter` | View tool detail |
| `f` | Filter by category |

## Upgrade Workflow

1. Switch to the **Updates** tab (press `3`)
2. Review the list of outdated tools
3. Press `Space` to select individual tools, or `a` to select all
4. Press `u` to start the batch upgrade
5. clim runs the appropriate package manager command for each tool

## Upgrade Commands

clim delegates upgrades to native package managers:

| Source | Command |
|--------|---------|
| Homebrew | `brew upgrade <package>` |
| winget | `winget upgrade <package>` |
| apt | `sudo apt install --only-upgrade <package>` |
| Chocolatey | `choco upgrade <package>` |
| Scoop | `scoop update <package>` |
| snap | `sudo snap refresh <package>` |
| npm | `npm update -g <package>` |

After upgrading, clim automatically rescans to verify the new versions.

## CLI Alternative

For non-interactive upgrades, you can use the standard package manager commands directly. clim's `list` command shows which tools need updating:

```bash
# Show all tools with update status
clim list

# Filter to a specific category
clim list --category Cloud
```
