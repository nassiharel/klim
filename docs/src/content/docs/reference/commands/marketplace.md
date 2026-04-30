---
title: "clim marketplace"
description: Manage extra marketplace URLs
---

Add, remove, and list extra marketplace URLs. Extra marketplaces extend the default tool catalog with additional tool definitions.

## Subcommands

| Command | Description |
|---------|-------------|
| `clim marketplace list` | Show primary and extra marketplace URLs |
| `clim marketplace add <url>` | Add an extra marketplace URL |
| `clim marketplace remove <url>` | Remove an extra marketplace URL |

## How It Works

Extra marketplace URLs point to YAML files with the same format as the default `marketplace.yaml`. Tools from extra sources are **merged** with the default catalog — if an extra marketplace defines a tool with the same name as a default tool, the extra version takes priority.

Extra marketplaces are cached locally (per-URL) and respect the same `auto_refresh` / `refresh_interval` settings as the primary marketplace.

## Examples

```bash
# List all marketplace sources
clim marketplace list

# Add a team-internal marketplace
clim marketplace add https://raw.githubusercontent.com/myorg/tools/main/marketplace.yaml

# Remove a marketplace
clim marketplace remove https://example.com/old-tools.yaml
```

## Configuration

Extra URLs are stored in `config.yaml`:

```yaml
marketplace:
  extra_urls:
    - https://raw.githubusercontent.com/myorg/tools/main/marketplace.yaml
    - https://example.com/my-custom-tools.yaml
```

## Creating a Custom Marketplace

A custom marketplace YAML has the same format as clim's built-in catalog:

```yaml
tools:
  - name: my-internal-tool
    display_name: My Internal Tool
    category: Internal
    tags: [internal, devops]
    binary_names: [my-tool]
    packages:
      brew: "myorg/tap/my-tool"

packs:
  - name: my-team-pack
    display_name: My Team Pack
    description: Tools our team uses daily
    tools: [my-internal-tool, kubectl, terraform]
```

## See Also

- [Marketplace guide](/guides/marketplace)
- [Adding Tools guide](/guides/adding-tools)
- [config.yaml Reference](/reference/configuration)
