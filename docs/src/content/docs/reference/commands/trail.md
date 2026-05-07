---
title: "klim trail"
description: Git for your dev environment — capture, log, show, diff, prune toolchain history
---

`klim trail` records every captured state of your toolchain as a
content-addressed snapshot, exposing git-style history inspection.

Two captures of an identical environment share storage on disk — only
a new history entry is appended.

## Usage

```bash
klim trail <subcommand>
```

## Subcommands

| Subcommand | Description |
|---|---|
| `klim trail capture` | Record the current toolchain as a new entry (forces a fresh PATH scan by default) |
| `klim trail log` | Show entries newest-first, with `@<index>` and short ref columns |
| `klim trail show <ref>` | Display the toolchain at a specific entry |
| `klim trail diff <ref> [<ref>]` | Compare two entries (defaults second arg to `HEAD`) |
| `klim trail prune` | Trim the trail and GC orphan objects |

## Refs

A `<ref>` can be:

- `HEAD` — the newest entry (alias: `latest`)
- `HEAD~N` — N entries back from `HEAD`
- `@<index>` — exact 0-based entry index
- a content hash — full 64-char or 7+ char prefix (must be unambiguous)
- an entry's `--label` (must be unique)

## Examples

```bash
# Tag the env before risky changes.
klim trail capture --label before-kubectl-upgrade

# After upgrading kubectl, what changed?
klim trail diff before-kubectl-upgrade

# Show the full toolchain at a specific point.
klim trail show HEAD~3

# Newest 5 entries, structured for scripts.
klim trail log --limit 5 --output json

# Trim to the 50 newest entries; orphan objects removed automatically.
klim trail prune --keep 50
```

## JSON Output

`log`, `show`, and `diff` accept `--output json` for scripting:

```bash
$ klim trail show HEAD --output json
{
  "entry": {
    "index": 1,
    "object": "1638d6421104...",
    "time": "2026-05-03T07:33:51.671Z",
    "op": "capture",
    "label": "same",
    "summary": "no changes"
  },
  "snapshot": {
    "schema_version": 1,
    "os": "windows",
    "arch": "amd64",
    "tools": [...]
  }
}
```

`diff` returns four tool-keyed collections — `added`, `removed`,
`version_changed`, `source_changed` — always as arrays (`[]` when
empty). When the two snapshots come from different platforms it also
emits an optional `platform_change` object with `from_os` / `to_os` /
`from_arch` / `to_arch` so cross-machine diffs aren't reported as "no
changes":

```json
"platform_change": {
  "from_os": "linux",
  "to_os": "darwin",
  "from_arch": "amd64",
  "to_arch": "arm64"
}
```

The field is omitted entirely when both snapshots share the same OS
and architecture.

## Storage layout

```
~/.klim/trail/
├── HEAD                   # newest entry index, single ASCII line
├── log.yaml               # ordered list of entries
├── log.lock               # cross-process advisory lock
└── objects/
    └── <aa>/<bb...>.yaml  # content-addressed snapshot bodies (sha256)
```

The snapshot body is hashed in canonical form (tools sorted, no
timestamp / label / op, **no per-machine paths**), so two captures of
an identical environment hash to the same `ObjectID` and dedupe
automatically — even across machines, which is forward-compatible
with `klim sync`.

The trail YAML format is read with **strict decoding** —
`yaml.KnownFields(true)` plus an explicit `schema_version`. A trail
written by a future, incompatible version of klim is rejected with a
"newer klim wrote this trail; upgrade klim" error, and a corrupted /
hand-edited log without `schema_version` is also rejected.

## Capture defaults

`klim trail capture` performs a fresh PATH scan by default so the
recorded snapshot matches your current toolchain — not whatever the
scan cache last saw. Pass `--refresh=false` to reuse the on-disk scan
cache (useful only when chaining klim commands and you want them to
see exactly the same view).

`--label` must be unique. Re-using an existing label fails fast rather
than creating an ambiguous label that would break `klim trail
show <label>`.

## What's NOT in the current release

- **Auto-capture** on install / upgrade / remove — coming next.
- **`klim trail revert`** — needs a real design pass for non-destructive
  default vs full convergence, and partial-failure modeling. Coming
  after auto-capture.
- **`klim trail bisect`** — a future addition.

## See Also

- [`klim export`](./export.md) — One-shot YAML manifest of installed tools.
- [`klim diff`](./diff.md) — Compare your env to an external manifest or share token.
- [`klim security`](./security.md) — Security / compliance / SBOM audit.
