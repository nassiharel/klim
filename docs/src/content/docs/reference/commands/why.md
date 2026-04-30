---
title: "clim why"
description: Show why a tool is needed and where it's referenced
---

Show all references to a tool across projects, packs, and your system.

## Usage

```bash
clim why <tool>
```

## What It Shows

- **Install status** — version, source, binary path, available updates
- **Project references** — which `.clim.yaml` files require or optionally include it
- **Pack references** — which marketplace and custom packs include it
- **Available packages** — all package manager IDs for the tool
- **Related tools** — installed tools in the same category/tags

## Examples

```bash
clim why kubectl
```

```
kubectl (K8s)
  Kubernetes command-line tool

  ✓ Installed: 1.28.0 (brew) at /usr/local/bin/kubectl

  Referenced by:
    • .clim.yaml (required >=1.28) — /home/user/myproject/.clim.yaml
    • Pack "K8s Essentials" (k8s-essentials)
    • Custom pack "My Stack" (my-stack)

  Available via: winget: Kubernetes.kubectl, brew: kubernetes-cli, apt: kubectl
  Related installed tools: helm, k9s, istioctl
```

## TUI

The tool detail view (press Enter on any tool) includes a **"Referenced By"** section showing the same project and pack references.

## See Also

- [clim check](/reference/commands/check) — Validate project requirements
- [Team Manifests guide](/guides/team-manifests)
