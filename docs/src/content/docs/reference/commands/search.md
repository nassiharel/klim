---
title: "klim search"
description: Search the tool marketplace
---

Search the tool marketplace by name, description, category, tags, or GitHub topics. Results are ranked by relevance and GitHub stars.

## Usage

```bash
klim search <query> [flags]
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
| Exact name match | Highest | `klim search jq` → exact hit |
| Partial name match | High | `klim search kube` → kubectl, kubectx |
| Category | Medium | `klim search cloud` → Cloud tools |
| Tags | Medium | `klim search encryption` → age, sops |
| GitHub topics | Medium | `klim search ci` → act, gh |
| Description | Low | `klim search "json processor"` → jq, yq |

Results are then boosted by GitHub star count for popular tools.

## Examples

```bash
# Find JSON tools
klim search json

# Multi-word search
klim search "kubernetes dashboard"

# Filter by category
klim search cli --category Security

# Limit results
klim search cloud -n 5
```

## TUI

The TUI search (press `/` on any tab) uses the same search engine. It matches against descriptions and GitHub topics in addition to names, categories, and tags.

## See Also

- [klim onboard](/reference/commands/onboard) — Role-based tool recommendations
- [Tool Catalog](/marketplace/catalog) — Browse all tools
