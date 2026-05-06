---
title: "klim generate"
description: Generate CI/container configs from .klim.yaml
---

Auto-generate CI and container configuration files from your `.klim.yaml` tool requirements. The generated files use package IDs from the klim marketplace to produce install commands for each tool.

## Usage

```bash
klim generate <github-action|dockerfile|devcontainer> [flags]
```

## Generators

| Format | Description | Output |
|--------|-------------|--------|
| `github-action` | GitHub Actions workflow | Reusable workflow with install + verify steps |
| `dockerfile` | Dockerfile | Multi-stage with apt/brew/npm installs |
| `devcontainer` | devcontainer.json | VS Code / GitHub Codespaces config with features |

## Flags

| Flag | Short | Description |
|------|-------|-------------|
| `--file` | `-f` | Path to .klim.yaml (default: auto-detect) |
| `--output` | `-o` | Write to file instead of stdout |
| `--base` | | Base image for Dockerfile (default: ubuntu:24.04) |

## Examples

```bash
# Generate GitHub Actions workflow
klim generate github-action

# Generate and save to file
klim generate github-action -o .github/workflows/setup-tools.yml

# Generate Dockerfile with custom base image
klim generate dockerfile --base ubuntu:22.04 -o Dockerfile.tools

# Generate devcontainer.json
klim generate devcontainer -o .devcontainer/devcontainer.json
```

## Dev Container Features

The devcontainer generator automatically maps known tools to official [Dev Container Features](https://containers.dev/features):

| Tool | Feature |
|------|---------|
| kubectl | `ghcr.io/devcontainers/features/kubectl-helm-minikube` |
| terraform | `ghcr.io/devcontainers/features/terraform` |
| node | `ghcr.io/devcontainers/features/node` |
| python | `ghcr.io/devcontainers/features/python` |
| go | `ghcr.io/devcontainers/features/go` |
| gh | `ghcr.io/devcontainers/features/github-cli` |
| az | `ghcr.io/devcontainers/features/azure-cli` |
| docker | `ghcr.io/devcontainers/features/docker-in-docker` |

Tools without a known feature are installed via `postCreateCommand`.

## See Also

- [klim init](/reference/commands/init) — Generate .klim.yaml from project files
- [klim check](/reference/commands/check) — Validate tool requirements
- [Team Manifests guide](/guides/team-manifests)
