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
| `--force` | Overwrite an existing `.clim.yaml` (clim refuses by default to protect a team-shared file). When `--force` is overwriting an existing manifest, clim refuses to write an empty result on three paths: (a) `--all` is used and no tools are installed at all; (b) project detection ran but matched no project files; (c) project detection found tools but none of them are installed yet. The existing manifest is preserved untouched in all three cases. A dangling `.clim.yaml` symlink also counts as "existing" for the safety check; `--force` is required to write through it (the symlink itself is preserved — see *Symlinks* below). |

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

## Symlinks

If you keep `.clim.yaml` as a symbolic link (e.g. to a shared template), `clim init --force` writes through the link to the target file rather than replacing the link with a regular file. This works even when the link is dangling — the target file is created on first write **as long as the target's parent directory already exists**. A link like `.clim.yaml → ../shared/missing/manifest.yaml` will fail with `ENOENT` if `../shared/missing/` doesn't exist; clim does not auto-create parent directories under shared mounts. Symlink chains are followed by the OS up to its own limit (Linux 40 hops, Windows configurable); cycles surface as the OS-level error your platform produces (e.g. `ELOOP` on Linux, `ERROR_CANT_RESOLVE_FILENAME` on Windows) — clim doesn't translate these.

## Permissions and metadata

When a `.clim.yaml` already exists as a regular file (or as a symlink to an existing target), `clim init --force` preserves its current mode bits, ownership, ACLs (POSIX and Windows), extended attributes, and inode — the file is rewritten in place rather than replaced. A manually-restricted manifest (e.g. `chmod 600 .clim.yaml` because the manifest contains sensitive tool/version data) keeps those bits across re-inits. Hardlinks pointing at the manifest stay live. For freshly-created manifests clim *requests* mode `0644`; the actual mode is whatever the OS produces after applying the process umask (typically `0022`, giving `0644`) — on a system with `umask 0077` you'll see `0600` instead, and Windows does not honor POSIX bits 1:1. Note: when `--force` overwrites a *dangling* symlink, the target file is being created for the first time, so there is no prior metadata to preserve and the requested mode + umask rule applies.

## See Also

- [clim check](/reference/commands/check) — Validate against .clim.yaml
- [clim generate](/reference/commands/generate) — Generate CI/container configs from .clim.yaml
- [Team Manifests guide](/guides/team-manifests)
