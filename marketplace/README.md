# klim marketplace

This directory is the **single source of truth** for every tool and pack klim knows
about. It is assembled by CI into the top-level `marketplace.yaml` and published to
the `marketplace` branch, which the CLI fetches at runtime.

```
marketplace/
  tools/             one YAML file per tool        (238 today)
  packs/             one YAML file per curated pack  (27 today)
  agent-marketplaces/ agent ecosystem sources
  categories.yaml    allowed tool categories (canonical order)
  tags.yaml          known tags
  tool-template.yaml copy this to add a tool
```

> **Never edit the root `marketplace.yaml` by hand** — it is auto-generated. Edit the
> individual files here instead.

## Add a tool in one PR

The fastest way to grow klim. No Go required.

1. Copy the template:
   ```bash
   cp marketplace/tool-template.yaml marketplace/tools/<name>.yaml
   ```
2. Fill in the fields (required: `name`, `display_name`, `category`, `binary_names`).
   Add the package IDs for whichever managers ship the tool.
3. Validate:
   ```bash
   make marketplace-validate
   ```
4. Open a PR. CI validates automatically and publishes the catalog on merge.

Don't want to write YAML? Open an [Add a tool issue](https://github.com/nassiharel/klim/issues/new?template=add-tool.yml)
and fill in the form — a maintainer will turn it into a PR.

### Example (`marketplace/tools/ripgrep.yaml`)

```yaml
name: ripgrep
display_name: ripgrep
category: CLI
tags: [search, cli]
binary_names: [rg]
packages:
    winget: BurntSushi.ripgrep.MSVC
    scoop: ripgrep
    choco: ripgrep
    brew: ripgrep
    apt: ripgrep
github: BurntSushi/ripgrep
```

## Add a pack

A pack is a named bundle of tools. Copy an existing file in `packs/`:

```yaml
name: my-pack
display_name: My Pack
description: What this bundle is for.
tools:
    - ripgrep
    - fzf
    - gh
```

Every tool listed must exist in `tools/`. Run `make marketplace-validate` to check
cross-references.
