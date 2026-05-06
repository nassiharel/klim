---
title: Dashboard
description: View aggregate stats and breakdowns of your developer tools
---

The **Dashboard** tab (press `7`) provides a high-level overview of your developer tool ecosystem.

![klim Dashboard tab with environment score, tool coverage, GitHub highlights, package managers chart, and category breakdown](../../../assets/examples/klim-dashboard-tui.png)

## What It Shows

### Summary Gauges

- **Installed** — Total number of detected tools
- **Up to Date** — Tools at the latest version
- **Updates Available** — Tools with newer versions
- **Favorited** — Number of favorited tools

### Category Breakdown

Distribution of your tools by category (Cloud, CLI, Containers, Database, IaC, Security, etc.) with counts and visual bars.

### Platform Coverage

How many tools are available on each platform (macOS, Linux, Windows) from the marketplace.

### Tag Cloud

Most common tags across your installed tools.

### Install Source Distribution

Breakdown by package manager source — how many tools come from brew, winget, apt, scoop, npm, etc.

## Keybindings

| Key | Action |
|-----|--------|
| `↑` / `↓` | Scroll |
| `Home` | Jump to top |
| `r` | Refresh data |

## When to Use

The Dashboard is useful for:
- Getting a quick snapshot of your development environment
- Identifying tools that need attention (updates available)
- Understanding your tool distribution across categories and sources
- Reporting on team tool standardization
