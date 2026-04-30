---
title: Marketplace
description: Browse and install tools from the clim marketplace
---

The **Discover** tab (press `4`) lets you browse the full clim marketplace — over 110 curated developer tools organized into categories.

## Sub-tabs

The Discover tab has three sub-tabs:

### Tools
Browse all tools in the marketplace catalog. Each tool shows:
- Name and description
- Category and tags
- GitHub stars (if available)
- Install status (installed or not)

Press `i` to install a tool directly from the marketplace.

### Packs
Curated bundles of related tools. Examples:
- **Cloud Essentials** — az, aws, gcloud, terraform
- **K8s Starter** — kubectl, helm, k9s, kubectx
- **Python Developer** — python, pip, poetry, ruff

View pack contents and install all tools in a pack at once.

### For You
Smart recommendations based on your currently installed tools. clim analyzes what you have and suggests related tools you might find useful.

## Sorting

Press `s` to toggle between:
- **Sort by name** — alphabetical order
- **Sort by stars** — GitHub stars (most popular first)

## Filtering

Press `f` to open the filter sidebar, then filter by:
- **Category** — Cloud, CLI, Containers, Database, IaC, Security, etc.
- **Platform** — macOS, Linux, Windows
- **Tags** — specific technology tags

## Installing Tools

1. Navigate to a tool and press `Enter` to see its detail card
2. The detail view shows available package manager options for your OS
3. Press `Enter` on the install action to install via your preferred package manager

clim delegates all installation to native package managers (brew, winget, apt, scoop, choco, snap, npm) — it never installs binaries directly.

## Marketplace Source

The tool catalog is fetched from GitHub and cached locally. It refreshes automatically based on your `marketplace.refresh_interval` setting (default: 24 hours). Press `r` to force a refresh.

To add a tool to the marketplace, see [Adding Tools](/guides/adding-tools).
