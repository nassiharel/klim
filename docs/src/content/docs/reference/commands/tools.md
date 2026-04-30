---
title: "clim tools"
description: Manage the tool catalog
---

Manage the local tool catalog cache.

## Usage

```bash
clim tools <subcommand>
```

## Subcommands

### tools path

Print the path to the local catalog cache file:

```bash
clim tools path
# Output: /home/user/.config/clim/marketplace/marketplace-cache.yaml
```

## About the Catalog

The tool catalog is fetched from GitHub at runtime and cached locally. It contains definitions for 110+ developer tools with their package manager IDs, categories, tags, and metadata.

The catalog source of truth is the `marketplace/` directory in the clim repository. Individual tool YAML files are assembled into a single `marketplace.yaml` by CI and published to the `marketplace` branch.

## See Also

- [Adding Tools](/guides/adding-tools) — How to contribute to the catalog
- [Marketplace guide](/guides/marketplace) — Browsing the catalog in the TUI
