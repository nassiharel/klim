---
title: "klim info"
description: Show everything about a tool — versions, packages, references, GitHub info
---

`klim info <tool>` is the CLI counterpart to the TUI's tool detail
page. It shows everything klim knows about a tool: every detected
installation, available package managers across all sources, GitHub
project metadata, project / pack references, and related installed
tools.

## Usage

```bash
klim info <tool> [flags]
```

## Flags

| Flag | Description |
|------|-------------|
| `--output` | `text` (default) or `json` |

`klim info` always runs a fresh scan: it walks PATH once to build the
catalog state, then re-checks PATH for the requested tool and resolves
its package-manager versions. Catalog-wide version resolution is
skipped, so a single `klim info <tool>` is much cheaper than the
previous behaviour that fanned out package-manager queries for every
tool in the marketplace. There is no cache to bypass.

## Examples

```bash
klim info kubectl                     # human-readable
klim info terraform --output json     # machine-readable for scripts
```

## Output

```
kubectl  (Containers)  ★ 3.3k
  Issue tracker and mirror of kubectl code

  ✗ Not installed

  Available via:
    winget  Kubernetes.kubectl
    choco   kubernetes-cli
    scoop   kubectl
    brew    kubernetes-cli
    snap    kubectl

  GitHub:
    Repo:      https://github.com/kubernetes/kubectl
    Stats:     ★ 3.3k stars   ⑂ 997 forks
    License:   Apache-2.0
    Topics:    k8s-sig-cli, k8s-staging
    Last push: 3 days ago

  Tags: kubernetes, cli

  Referenced by:
    • .klim.yaml (required >=1.28) — /home/me/myproject/.klim.yaml
    • Pack "Kubernetes Starter" (k8s-starter)

  Related installed tools: kubectx
```

## JSON Output

`--output json` returns the same data as a structured payload. Real
shape (taken from the live `kubectl` catalog entry):

```json
{
  "name": "kubectl",
  "display_name": "kubectl",
  "category": "Containers",
  "tags": ["kubernetes", "cli"],
  "installed": false,
  "update_available": false,
  "instances": [],
  "packages": [
    {"source": "winget", "id": "Kubernetes.kubectl"},
    {"source": "choco",  "id": "kubernetes-cli"},
    {"source": "scoop",  "id": "kubectl"},
    {"source": "brew",   "id": "kubernetes-cli"},
    {"source": "snap",   "id": "kubectl"}
  ],
  "github": {
    "slug": "kubernetes/kubectl",
    "url": "https://github.com/kubernetes/kubectl",
    "stars": 3300,
    "forks": 997,
    "license": "Apache-2.0",
    "topics": ["k8s-sig-cli", "k8s-staging"],
    "last_push": "2026-04-26T..."
  },
  "references": [],
  "related_tools": [],
  "warnings": []
}
```

The collection fields (`tags`, `instances`, `packages`, `references`,
`related_tools`, `warnings`) are always present as arrays — `[]` when
empty, never `null`.

## Errors

If the tool name is not in the catalog and a close match exists, klim
suggests it:

```
$ klim info kubctl
Error: tool "kubctl" not found in catalog (did you mean "kubectl"?)
```

## See Also

- [`klim why`](/reference/commands/why) — Where (and why) a tool is referenced — focused on the dependency map rather than full metadata.
- [`klim list`](/reference/commands/list) — Every installed tool, summary table.
- [`klim search`](/reference/commands/search) — Full-text search across the marketplace.
