---
title: "clim search"
description: Search the tool marketplace
---

Search the tool marketplace by name, description, category, tags, or GitHub topics. Results are ranked by relevance and GitHub stars.

## Usage

```bash
clim search <query> [flags]
```

## Flags

| Flag | Short | Description |
|------|-------|-------------|
| `--category` | `-c` | Filter by category |
| `--limit` | `-n` | Max results to show (default: 15) |

## How Matching Works

The search engine scores each tool against your query by matching:

| Field | Weight | Example |
|-------|--------|---------|
| Exact name match | Highest | `clim search jq` → exact hit |
| Partial name match | High | `clim search kube` → kubectl, kubectx |
| Category | Medium | `clim search cloud` → Cloud tools |
| Tags | Medium | `clim search encryption` → age, sops |
| GitHub topics | Medium | `clim search ci` → act, gh |
| Description | Low | `clim search "json processor"` → jq, yq |

Results are then boosted by GitHub star count for popular tools.

## Examples

```bash
# Find JSON tools
clim search json

# Multi-word search
clim search "kubernetes dashboard"

# Filter by category
clim search cli --category Security

# Limit results
clim search cloud -n 5
```

## TUI

The TUI search (press `/` on any tab) uses the same search engine. It matches against descriptions and GitHub topics in addition to names, categories, and tags.

## See Also

- [clim onboard](/reference/commands/onboard) — Role-based tool recommendations
- [Tool Catalog](/marketplace/catalog) — Browse all tools
