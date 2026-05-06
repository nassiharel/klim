---
title: Environment Diff
description: Compare one developer environment against another
---

Use `klim diff` to compare your installed tools against a manifest file or share token. It turns "works on my machine" into a concrete environment delta you can fix, review, or enforce in CI.

## Comparing Against a Manifest

Export your tools on one machine, then diff on another:

```bash
# Machine A: export
klim export > alice-tools.yaml

# Machine B: compare
klim diff alice-tools.yaml
```

## Comparing Against a Share Token

Share tokens are compact strings you can paste in Slack or Teams:

```bash
# Generate a token
klim share
# → klim:v1:H4sIAAAA...

# Compare on another machine
klim diff "klim:v1:H4sIAAAA..."
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

`klim diff` returns exit code 1 when differences are found:

```yaml
- name: Check environment matches baseline
  run: klim diff baseline-tools.yaml
```

## See Also

- [Backup & Restore](./backup-restore.mdx) — Export and import tool manifests
- [klim diff reference](../reference/commands/diff.md)
