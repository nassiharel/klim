---
title: Favorites
description: How to favorite tools and manage your favorites list
---

Favorites let you bookmark your most important tools for quick access, export, and sharing.

## Favoriting Tools

Press `*` on any tool in the **Installed**, **Updates**, or **Discover** tabs to toggle its favorite status. Favorited tools show a ★ indicator.

## Favorites Tab

Switch to the **★ Favorites** tab (press `2`) to see all your favorited tools in one place.

### Favorites Keybindings

| Key | Action |
|-----|--------|
| `*` | Unfavorite selected tool |
| `e` | Export favorites to a YAML manifest |
| `s` | Generate a share token |
| `x` | Clear all favorites (with y/n confirmation) |

## Export Favorites

Press `e` on the Favorites tab to export your favorited tools to a YAML manifest file. This creates a portable file you can use with `klim import` on another machine.

## Share Favorites

Press `s` to generate a compact share token that encodes your favorited tools. Send this token to a colleague — they run:

```bash
klim open <token>
```

to install the same set of tools.

## Storage

Favorites are stored at:
- **macOS:** `~/Library/Application Support/klim/favorites/favorites.yaml`
- **Linux:** `~/.config/klim/favorites/favorites.yaml`
- **Windows:** `%AppData%\klim\favorites\favorites.yaml`

The file contains a simple list of tool names.
