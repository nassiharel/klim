---
title: "clim info"
description: Show everything about a tool — versions, packages, references, GitHub info
---

`clim info <tool>` is the CLI counterpart to the TUI's tool detail
page. It shows everything clim knows about a tool: every detected
installation, available package managers across all sources, GitHub
project metadata, project / pack references, and related installed
tools.

## Usage

```bash
clim info <tool> [flags]
```

## Flags

| Flag | Description |
|------|-------------|
| `--refresh` | Force a fresh scan (ignore cache) |
| `--output` | `text` (default) or `json` |

## Examples

```bash
clim info kubectl                     # human-readable
clim info terraform --output json     # machine-readable for scripts
clim info bat --refresh               # bypass cache
```

## Output

```
kubectl  (Containers)  ★ 109k  ⬆ Update available
  Production-Grade Container Scheduling and Management

  ✓ Installed: 1.28.4 (brew) at /usr/local/bin/kubectl
  ⬆ Update available: 1.28.4 → 1.31.0

  Available via:
    winget   Kubernetes.kubectl
    choco    kubernetes-cli
    scoop    kubectl
    brew     kubernetes-cli
    apt      kubectl

  GitHub:
    Repo:      https://github.com/kubernetes/kubernetes
    Stats:     ★ 109k stars   ⑂ 39.2k forks
    License:   Apache-2.0
    Topics:    kubernetes, containers, orchestration
    Last push: 2 day(s) ago

  Tags: containers, k8s, devops

  Referenced by:
    • .clim.yaml (required >=1.28) — /home/me/myproject/.clim.yaml
    • Pack "K8s Essentials" (k8s-essentials)

  Related installed tools: helm, k9s, istioctl
```

## JSON Output

`--output json` returns the same data as a structured payload:

```json
{
  "name": "kubectl",
  "display_name": "kubectl",
  "category": "Containers",
  "tags": ["containers", "k8s", "devops"],
  "installed": true,
  "update_available": true,
  "latest": "1.31.0",
  "instances": [
    {"path": "/usr/local/bin/kubectl", "version": "1.28.4", "source": "brew"}
  ],
  "packages": [
    {"source": "winget", "id": "Kubernetes.kubectl"},
    {"source": "brew",   "id": "kubernetes-cli"}
  ],
  "github": {
    "slug": "kubernetes/kubernetes",
    "url": "https://github.com/kubernetes/kubernetes",
    "stars": 109000,
    "license": "Apache-2.0",
    "topics": ["kubernetes", "containers", "orchestration"],
    "last_push": "2026-05-01T..."
  },
  "references": [
    {
      "kind": "teamfile",
      "path": "/home/me/myproject/.clim.yaml",
      "required": true,
      "version_constraint": ">=1.28"
    }
  ],
  "related_tools": ["helm", "k9s", "istioctl"],
  "warnings": []
}
```

The collection fields (`tags`, `instances`, `packages`, `references`,
`related_tools`, `warnings`) are always present as arrays — `[]` when
empty, never `null`.

## Errors

If the tool name is not in the catalog and a close match exists, clim
suggests it:

```
$ clim info kubctl
Error: tool "kubctl" not found in catalog (did you mean "kubectl"?)
```

## See Also

- [`clim why`](/reference/commands/why) — Where (and why) a tool is referenced — focused on the dependency map rather than full metadata.
- [`clim list`](/reference/commands/list) — Every installed tool, summary table.
- [`clim search`](/reference/commands/search) — Full-text search across the marketplace.
