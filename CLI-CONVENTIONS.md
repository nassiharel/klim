# klim CLI conventions

This document codifies how klim's CLI surface behaves so that scripts and
humans can rely on consistent output, exit codes, and flag names.

## Streams

* **stdout** — primary, machine-relevant data. Manifests, JSON/YAML output,
  resolved paths, share tokens, etc. Designed to be pipeable.
* **stderr** — prose for humans. Progress spinners, summaries, warnings,
  hints (“X tools exported”), and errors. Never machine-parsed by klim
  itself.
* **errors** are always written to stderr with an `Error:` prefix.

This split is deliberate: `klim export > my-tools.yaml` works as a Unix user
expects, while the human-readable summary still prints to the terminal.

## Output format

Every command that produces structured data accepts:

```
--output text|json|yaml
```

* Default is `text` (human-readable).
* `--json` is supported as a deprecated alias for `--output=json` and prints
  a deprecation warning.
* When `--output=json` (or `yaml`) is set, only the structured payload is
  written to stdout; prose stays on stderr.
* `--output=yaml` emits a YAML document on stdout. Commands that don't
  support YAML return a usage error and exit 2 — they do **not** silently
  fall back to text.
* Unknown values (e.g. `--output=jsno`) are usage errors and exit 2.

Currently supports `--output={json,yaml}` (wired via `addOutputFlag`):
`audit`, `badge`, `check`, `compliance check`, `diff`, `doctor`, `haiku`,
`info`, `list`, `score`, `search`, `share`, `tools path`, `trail log`,
`trail show`, `trail diff`, `watch`, `why`, and `config marketplace list`.
(`export` already emits YAML by design.) `graph` is text-only — its
output is a force-directed terminal drawing, not structured data.

## Exit codes

| Code | Meaning |
| --- | --- |
| 0 | Success |
| 1 | Runtime error (network failure, file IO, etc.) |
| 2 | Usage error — bad flags, missing/extra arguments, unknown command, unsupported `--output` value. Cobra's own flag-parse errors are wrapped via `SetFlagErrorFunc` and "unknown command/flag" errors that escape that hook are detected by message prefix in `cli.Run`. |
| 3 | Partial failure (multi-item operation where some items failed, e.g. `klim import` with one or more install failures) |

Commands like `audit`, `compliance check`, `check`, and `diff` may also exit
non-zero (1) to signal "findings present" — see each command's `--help`.

## Flags

* Common flags use a consistent name across commands:
  * `--refresh` — force fresh PATH scan + version resolution
  * `--output` — output format (see above)
  * `--verbose` — enable verbose logging (root-level persistent flag)
  * `--file`, `-f` — input file path
* Boolean negation is not used; instead provide an explicit positive flag.
* Short flags are reserved for high-frequency options (`-c` for category,
  `-n` for limit, `-f` for file, `-y` for yes, `-v` for version).

## Help & examples

Every command must have:
* `Short` — one-line summary (≤ 80 chars)
* `Long` — multi-line description with at least one usage example
* Examples that show common invocations

## Errors

All errors are wrapped with context (`fmt.Errorf("doing X: %w", err)`) when
they cross a layer boundary. Never panic in CLI flows. Use the typed
`UsageError` and `PartialFailureError` from `internal/cli/errors.go` to
control the exit code.
