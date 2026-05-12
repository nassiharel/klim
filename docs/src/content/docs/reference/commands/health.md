---
title: klim health
description: Environment health diagnostics and PATH conflict visualization.
---

`klim health` inspects your local development environment for issues
that aren't supply-chain risks but still cause real friction: duplicate
or broken `PATH` entries, multiple installations of the same tool
across different package managers, missing PMs that own tools you
have installed, stale scan caches, and unresolved versions.

It runs entirely offline against the data klim already collected
during its last scan (`--refresh` forces a fresh one).

## Synopsis

```
klim health                  # all diagnostics, grouped by category
klim health path             # PATH-conflict visualization (Active vs Shadowed)
```

`klim health` is the top-level home for environment checks. Supply-chain
checks (vulnerabilities, archived upstreams, compliance) live under
[`klim security`](./security.md).

## `klim health`

Runs the full diagnostic suite and prints findings grouped by
category (PATH, Tools, Package Managers, Cache). Each finding has a
severity (`error` / `warning` / `info`), a one-line title, optional
detail, and an actionable fix suggestion.

Flags:

| Flag | Description |
|------|-------------|
| `--output {text,json}` | Output format. Default `text` to stderr; `json` writes the structured `{issues, summary, healthy}` schema to stdout. |
| `--refresh`            | Force a fresh PATH scan, ignoring the local cache. |

Exit codes: `0` = no errors found (warnings and info still possible),
`1` = one or more `error`-severity findings.

## `klim health path`

A focused view of PATH-shadowing situations. For every tool with more
than one binary on PATH it shows:

- **Active** — the copy that actually resolves first (your shell's
  `which` answer).
- **Shadowed** — every other copy, in PATH order, with each one's
  recorded version and source package manager.
- **Version conflict** — flagged whenever the shadowed copies report
  different versions from the active one.
- **Privilege risk** — flagged when the active copy sits in a
  user-writable directory that comes before a system directory
  containing another copy (the classic local privilege-escalation
  pattern: drop a malicious binary into `~/.local/bin/sudo` and you
  shadow the real one).

Two presentations of the same model:

| Section | Layout |
|---------|--------|
| `By tool`   | Active row + Shadowed rows per tool. |
| `By PATH dir` | Each `$PATH` entry in order, with the tools it provides and whether this directory wins or loses the lookup. Directories are annotated `[missing]`, `[duplicate]`, `[user-writable]`, `[system]`. |

The same data backs the **Health → PATH** TUI tab, which adds
interactive uninstall (`u`) of a shadowed copy through its detected
package manager.

Flags:

| Flag | Description |
|------|-------------|
| `--output {text,json}` | Output format. JSON emits the full `pathconflict.Report` schema (`by_tool`, `by_dir`). |
| `--refresh`            | Force a fresh PATH scan. |

Exit codes: `0` = no conflicts, `1` = at least one tool has differing
versions across PATH copies.

## Example

```
$ klim health path
  By tool — 2 tool(s) with multiple PATH copies, 3 shadowed total

  Node.js  ⚠ version conflict
    ✓ active   (20.0.0, manual)   /home/u/.nvm/bin/node
    ⊘ shadowed (18.0.0, brew)     /usr/local/bin/node
        → brew uninstall node
    ⊘ shadowed (16.0.0, apt)      /usr/bin/node
        → sudo apt remove nodejs
  ...
```

## Interactive fixes (TUI)

The TUI's Health → Issues sub-tab makes every diagnostic interactive: `↑/↓` selects an issue, `f` (or Enter) opens a fix wizard. The wizard:

1. Shows the issue summary plus the proposed command in a bordered code block.
2. Offers labelled buttons depending on the action kind:
   - `CopyCommand` — Run command · Copy to clipboard · Cancel
   - `JumpPathView` — Open PATH view · Cancel
   - `Rescan` — Rescan now · Cancel
   - `JumpUpdates` — Open Updates tab · Cancel
3. On Run, spawns `powershell -NoProfile -Command` (Windows) or `sh -c` (POSIX), streams output, transitions to a Done state with ✓/✗.
4. After success, dismisses and runs a **PATH-only refresh** (re-walks PATH + re-diagnoses; no version resolution) so the issue list reflects the fix in milliseconds, not seconds.

### PATH backups

Every fix that touches `$PATH` (duplicate-removal, missing-dir cleanup, reorder) writes a backup of the current PATH to `~/.klim/backups/path/path-<UTC>.yaml` **before** the command runs. On Windows the persistent User PATH from the registry is captured too. The Done state shows the saved file path and exposes a **Restore previous PATH** button that runs the platform-specific restore command (also through the same modal so you see what's about to happen).

#### CLI access

```
klim health path-backups list                    # browse every backup
klim health path-backups show <name>             # inspect one backup
klim health path-backups restore-cmd <name>      # print the restore command
```

The `<name>` argument accepts the bare filename, the full file path, or an unambiguous prefix. `restore-cmd` emits the restore command to stdout (so it can be piped) and a reminder to stderr — review the command before pasting it into your shell.

The backup files are plain YAML and can also be opened with any text editor. Format:

```yaml
timestamp: 2026-05-11T16:35:12Z
trigger: doctor.fix
issue: Duplicate PATH entry
goos: windows
path: "C:\\Windows;..."
user_path: "C:\\Users\\..."   # Windows only
command: "$new = ..."          # the command that was about to run
```

## Related

- [`klim security`](./security.md) — supply-chain checks (vuln, audit, compliance)
- [`klim score`](./score.md) — composite per-tool security score
- [`klim plan`](./plan.md) — preview pending changes with confidence scoring
- [`klim apply`](./apply.md) — execute changes with auto-checkpoint and postcheck
