---
title: "clim diff"
description: Compare your installed tools against a manifest or share token
---

Compare your local tool environment against a reference to find differences in tool presence, versions, and sources.

## Usage

```bash
clim diff <manifest.yaml | share-token> [flags]
```

## Flags

| Flag | Description |
|------|-------------|
| `--refresh` | Force fresh scan, ignoring cache |

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Environments match |
| 1 | Differences found |

## Status Indicators

| Status | Meaning |
|--------|---------|
| ✓ match | Tool present on both sides with same version |
| ≠ differs | Tool present on both sides but versions differ |
| ← local only | Tool installed locally but not in reference |
| → remote only | Tool in reference but not installed locally |

## Examples

```bash
# Compare against a manifest file
clim diff colleague-tools.yaml

# Compare against a share token
clim diff "clim:v1:H4sIAAAA..."

# Force fresh scan for accurate comparison
clim diff tools.yaml --refresh
```

## Output

```
TOOL      LOCAL                  REMOTE           STATUS
----      -----                  ------           ------
git       2.53.0 (winget)        99.0.0 (winget)  ≠ differs
gh        2.74.2 (winget)        2.74.2 (winget)  ✓ match
kubectl   —                      1.28.0 (brew)    → remote only
node      24.14.1 (winget)       —                ← local only

Result: 1 match, 1 differ, 1 local only, 1 remote only
```

## See Also

- [clim export](/reference/commands/export) — Export your tools to a manifest
- [clim share](/reference/commands/share) — Generate a share token
