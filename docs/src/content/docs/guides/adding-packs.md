---
title: Adding Packs
description: How to add a curated tool pack to the klim marketplace
---

Packs are curated bundles of related tools. They help developers quickly install a complete toolset for a specific workflow.

## Pack Definition Format

Each pack is defined in `marketplace/packs/<name>.yaml`:

```yaml
name: cloud-essentials
display_name: Cloud Essentials
description: Core tools for cloud development
icon: ☁️
tools:
  - az
  - aws
  - gcloud
  - terraform
  - kubectl
```

### Fields

| Field | Required | Description |
|-------|----------|-------------|
| `name` | Yes | Unique identifier (lowercase, hyphens) |
| `display_name` | Yes | Human-readable name |
| `description` | Yes | What this pack is for |
| `icon` | No | Emoji icon for the TUI |
| `tools` | Yes | List of tool names (must exist in marketplace) |

## Examples

```yaml
# marketplace/packs/k8s-starter.yaml
name: k8s-starter
display_name: K8s Starter
description: Everything you need to get started with Kubernetes
icon: ☸️
tools:
  - kubectl
  - helm
  - k9s
  - kubectx
  - kustomize
```

```yaml
# marketplace/packs/python-developer.yaml
name: python-developer
display_name: Python Developer
description: Essential tools for Python development
icon: 🐍
tools:
  - python
  - pip
  - poetry
  - ruff
  - mypy
```

## Adding a Pack

1. Create a YAML file at `marketplace/packs/<name>.yaml`
2. List only tools that already exist in `marketplace/tools/`
3. Run `make marketplace-validate` to verify
4. Submit a PR

## Validation

CI validates that:
- All referenced tools exist in the marketplace
- Pack names are unique
- Required fields are present

## Custom Packs

Users can also create personal packs from the TUI (Backup tab → Create Pack). These are stored locally and not part of the marketplace. See [Backup & Restore](./backup-restore.mdx) for details.
