---
title: "klim diff"
description: Compare your installed tools against a manifest or share token
---

Compare your local tool environment against a reference to find differences in tool presence, versions, and sources.

## Usage

```bash
klim diff <manifest.yaml | share-token> [flags]
```

## Flags

| Flag | Description |
|------|-------------|
| `--refresh` | Force fresh scan, ignoring cache |
| `--output` | Output format: `text` (default) or `json` |

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
klim diff colleague-tools.yaml

# Compare against a share token
klim diff "klim:v1:H4sIAAAA..."

# Force fresh scan for accurate comparison
klim diff tools.yaml --refresh
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

## JSON Output

`--output json` emits a structured report with canonical status keys (`match`, `differs`, `local_only`, `remote_only`), the original CLI argument as `target`, and a human-friendly `target_label`:

```json
{
  "target": "colleague-tools.yaml",
  "target_label": "colleague-tools.yaml (linux/amd64)",
  "summary": {
    "match": 1,
    "differs": 1,
    "local_only": 1,
    "remote_only": 1
  },
  "entries": [
    {
      "name": "git",
      "local_version": "2.53.0",
      "local_source": "winget",
      "remote_version": "99.0.0",
      "remote_source": "winget",
      "status": "differs"
    }
  ]
}
```

`entries` is always an array (empty when no diffs). The exit-code semantics are unchanged: `--output json` with differences still exits 1.

## See Also

- [klim export](/reference/commands/export) — Export your tools to a manifest
- [klim share](/reference/commands/share) — Generate a share token
