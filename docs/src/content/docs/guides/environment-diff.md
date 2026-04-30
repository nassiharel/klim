---
title: Environment Diff
description: Compare your tools against another developer's environment
---

Use `clim diff` to compare your installed tools against a manifest file or share token. This is the "works on my machine" killer — quickly identify what's different between two environments.

## Comparing Against a Manifest

Export your tools on one machine, then diff on another:

```bash
# Machine A: export
clim export > alice-tools.yaml

# Machine B: compare
clim diff alice-tools.yaml
```

## Comparing Against a Share Token

Share tokens are compact strings you can paste in Slack or Teams:

```bash
# Generate a token
clim share
# → clim:v1:H4sIAAAA...

# Compare on another machine
clim diff "clim:v1:H4sIAAAA..."
```

:::note
Share tokens carry only tool names (no versions), so version comparisons show "—" for the remote side and all present tools show as matching.
:::

## Reading the Output

```
TOOL      LOCAL                  REMOTE           STATUS
----      -----                  ------           ------
git       2.53.0 (winget)        2.54.0 (winget)  ≠ differs
gh        2.74.2 (winget)        2.74.2 (winget)  ✓ match
kubectl   —                      1.28.0 (brew)    → remote only
node      24.14.1 (winget)       —                ← local only
```

| Status | Meaning |
|--------|---------|
| ✓ match | Same tool, same version |
| ≠ differs | Same tool, different versions |
| ← local only | You have it, they don't |
| → remote only | They have it, you don't |

## CI Usage

`clim diff` returns exit code 1 when differences are found:

```yaml
- name: Check environment matches baseline
  run: clim diff baseline-tools.yaml
```

## See Also

- [Backup & Restore](/guides/backup-restore) — Export and import tool manifests
- [clim diff reference](/reference/commands/diff)
