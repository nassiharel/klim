---
title: "klim why"
description: Show why a tool is needed and where it's referenced
---

Show all references to a tool across projects, packs, and your system.

## Usage

```bash
klim why <tool> [flags]
```

## Flags

| Flag | Description |
|------|-------------|
| `--output` | Output format: `text` (default) or `json` |

## What It Shows

- **Install status** — version, source, binary path, available updates
- **Project references** — which `.klim.yaml` files require or optionally include it
- **Pack references** — which marketplace and custom packs include it
- **Available packages** — all package manager IDs for the tool
- **Related tools** — installed tools in the same category/tags

## Examples

```bash
klim why kubectl
```

```
kubectl (K8s)
  Kubernetes command-line tool

  ✓ Installed: 1.28.0 (brew) at /usr/local/bin/kubectl

  Referenced by:
    • .klim.yaml (required >=1.28) — /home/user/myproject/.klim.yaml
    • Pack "K8s Essentials" (k8s-essentials)
    • Custom pack "My Stack" (my-stack)

  Available via: winget: Kubernetes.kubectl, brew: kubernetes-cli, apt: kubectl
  Related installed tools: helm, k9s, istioctl
```

## JSON Output

`--output json` returns a structured report. All collection fields (`references`, `available_via`, `related_tools`, `warnings`) are always present as arrays — `[]` when empty, never `null`:

```json
{
  "name": "kubectl",
  "display_name": "kubectl",
  "category": "K8s",
  "installed": true,
  "version": "1.28.0",
  "source": "brew",
  "path": "/usr/local/bin/kubectl",
  "references": [
    {
      "kind": "teamfile",
      "path": "/home/user/myproject/.klim.yaml",
      "required": true,
      "version_constraint": ">=1.28"
    },
    {
      "kind": "pack",
      "name": "k8s-essentials",
      "display_name": "K8s Essentials"
    }
  ],
  "available_via": [
    {"source": "winget", "id": "Kubernetes.kubectl"},
    {"source": "brew",   "id": "kubernetes-cli"}
  ],
  "related_tools": ["helm", "k9s", "istioctl"],
  "warnings": []
}
```

If `.klim.yaml` files (in or above CWD, or registered via the project registry) fail to parse, the parse error is surfaced in `warnings` rather than silently dropped.

## TUI

The tool detail view (press Enter on any tool) includes a **"Referenced By"** section showing the same project and pack references.

## See Also

- [klim check](/reference/commands/check) — Validate project requirements
- [Team Manifests guide](/guides/team-manifests)
