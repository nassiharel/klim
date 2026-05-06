---
title: "klim tools"
description: Manage the tool catalog
---

Manage the local tool catalog cache.

## Usage

```bash
klim tools <subcommand>
```

## Subcommands

### tools path

Print the path to the local catalog cache file:

```bash
klim tools path
# Output: /home/user/.config/klim/marketplace/marketplace-cache.yaml

klim tools path --output json
# {"cache_path": "/home/user/.config/klim/marketplace/marketplace-cache.yaml"}
```

Accepts `--output text` (default) or `--output json`.

## About the Catalog

The tool catalog is fetched from GitHub at runtime and cached locally. It contains definitions for 110+ developer tools with their package manager IDs, categories, tags, and metadata.

The catalog source of truth is the `marketplace/` directory in the klim repository. Individual tool YAML files are assembled into a single `marketplace.yaml` by CI and published to the `marketplace` branch.

## See Also

- [Adding Tools](../../guides/adding-tools.mdx) — How to contribute to the catalog
- [Adding Tools guide](../../guides/adding-tools.mdx) — Browsing the catalog in the TUI
