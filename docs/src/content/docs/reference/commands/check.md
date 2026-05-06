---
title: "klim check"
description: Validate installed tools against .klim.yaml requirements
---

Check that all tools required by the project's `.klim.yaml` are installed and meet version constraints.

## Usage

```bash
klim check [flags]
```

## Flags

| Flag | Short | Description |
|------|-------|-------------|
| `--file` | `-f` | Path to .klim.yaml (default: auto-detect) |
| `--json` | | Machine-readable JSON output |
| `--refresh` | | Force fresh scan, ignoring cache |

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | All requirements satisfied |
| 1 | One or more tools missing or outdated |

## Examples

```bash
# Auto-find .klim.yaml in current/parent directories
klim check

# Specify explicit path
klim check --file path/to/.klim.yaml

# Machine-readable output for CI
klim check --json
```

## Output

```
✓ node        22.11.0   (required: >=20.0.0)
✗ docker      not found (required: >=24.0.0)
✓ kubectl     1.31.0
✓ terraform   1.7.2     (required: >=1.5.0)

1 tool missing or outdated. Exit code: 1
```

## CI Usage

```yaml
# GitHub Actions
- name: Verify developer tools
  run: |
    curl -fsSL https://raw.githubusercontent.com/nassiharel/klim/main/install.sh | bash
    klim check --json
```

## See Also

- [Team Manifests guide](/guides/team-manifests) — How to set up .klim.yaml
