---
title: "klim config marketplace"
description: Manage extra marketplace URLs
---

Add, remove, and list extra marketplace URLs. Extra marketplaces extend the default tool catalog with additional tool definitions.

## Subcommands

| Command | Description |
|---------|-------------|
| `klim config marketplace list` | Show primary and extra marketplace URLs |
| `klim config marketplace add <url>` | Add an extra marketplace URL |
| `klim config marketplace remove <url>` | Remove an extra marketplace URL |

## How It Works

Extra marketplace URLs point to YAML files with the same format as the default `marketplace.yaml`. Tools from extra sources are **merged** with the default catalog — if an extra marketplace defines a tool with the same name as a default tool, the extra version takes priority.

Extra marketplaces are cached locally (per-URL) and respect the same `auto_refresh` / `refresh_interval` settings as the primary marketplace.

## Examples

```bash
# List all marketplace sources
klim config marketplace list

# Same, as JSON for scripts
klim config marketplace list --output json
# {"primary": "https://...", "extra": []}

# Add a team-internal marketplace
klim config marketplace add https://raw.githubusercontent.com/myorg/tools/main/marketplace.yaml

# Remove a marketplace
klim config marketplace remove https://example.com/old-tools.yaml
```

`klim config marketplace list` accepts `--output text` (default) or `--output json`. In JSON mode, a config-load failure is surfaced as an error rather than silently falling back to default URLs.

## Configuration

Extra URLs are stored in `config.yaml`:

```yaml
marketplace:
  extra_urls:
    - https://raw.githubusercontent.com/myorg/tools/main/marketplace.yaml
    - https://example.com/my-custom-tools.yaml
```

## Creating a Custom Marketplace

A custom marketplace YAML has the same format as klim's built-in catalog:

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
