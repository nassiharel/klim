---
title: "clim init"
description: Generate a .clim.yaml from project files
---

Scan your project directory to detect which CLI tools it uses, then generate a `.clim.yaml` team manifest.

## Usage

```bash
clim init [flags]
```

## Flags

| Flag | Description |
|------|-------------|
| `--all` | Include all installed tools (skip project detection) |
| `--min-version` | Include minimum version constraints (`>=X.Y`) |
| `--name` | Project name for the manifest |
| `--force` | Overwrite an existing `.clim.yaml` (clim refuses by default to protect a team-shared file). When `--force` is passed but no tools are detected (and an existing manifest is present), clim refuses rather than silently writing an empty manifest or leaving a stale manifest in place. |

## Detection

clim scans your project for tool references in:

- **Dockerfiles** — `FROM`, `RUN` commands
- **package.json** — scripts, devDependencies
- **go.mod** — Go module dependencies
- **CI workflows** — GitHub Actions, GitLab CI, CircleCI
- **Helm charts** — Chart.yaml, values.yaml
- **Terraform** — .tf files
- **Bicep** — .bicep files
- **pyproject.toml** — Python project config
- **Makefile** — build targets
- **docker-compose.yaml** — service definitions
- And 30+ more file types

Only tools that are both detected AND installed are included, so versions can be pinned accurately.

## Examples

```bash
# Auto-detect from project files
clim init

# Include all installed tools (no detection)
clim init --all

# Pin minimum versions
clim init --min-version

# Set project name
clim init --name my-project

# Overwrite an existing .clim.yaml
clim init --force
```

## Output

Creates a `.clim.yaml` file in the current directory:

```yaml
name: my-project
tools:
  - name: kubectl
    version: ">=1.28"
  - name: helm
  - name: docker
optional:
  - name: k9s
```

If you keep `.clim.yaml` as a symbolic link (e.g. to a shared template), `clim init --force` updates the link's target rather than replacing the symlink with a regular file.

## See Also

- [clim check](/reference/commands/check) — Validate against .clim.yaml
- [clim generate](/reference/commands/generate) — Generate CI/container configs from .clim.yaml
- [Team Manifests guide](/guides/team-manifests)
