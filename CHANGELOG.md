# Changelog

All notable changes to klim are documented here.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

Per-release notes (with the full commit log and downloadable binaries) are published on the
[GitHub Releases](https://github.com/nassiharel/klim/releases) page, generated automatically by
GoReleaser from [Conventional Commits](https://www.conventionalcommits.org/). This file
summarises the highlights.

## [Unreleased]

### Changed
- **BREAKING:** reorganized the CLI into noun-first command groups. Every command now lives under a
  group — e.g. `klim install` → `klim tool install`, `klim check` → `klim project check`,
  `klim apply` → `klim plan apply`, `klim trail` → `klim env trail`,
  `klim export` / `klim import` → `klim share export` / `klim share import`,
  `klim health` → `klim doctor`, `klim score` → `klim security score`,
  `klim agents` → `klim agent`, and `klim proxy` → `klim shell proxy`. Tool self-management
  (`klim update`, `klim version`, `klim browser`) stays at the top level. There are no
  backward-compatible aliases — the old top-level names are removed.

### Added
- `CODE_OF_CONDUCT.md` (Contributor Covenant 2.1).
- `CHANGELOG.md` (this file).
- `marketplace/tool-template.yaml`, `marketplace/README.md`, and an "Add a tool" issue form for
  a one-PR marketplace contribution path.
- `examples/` directory with ready-to-use `.klim.yaml` files.

### Changed
- Repositioned the README and website around the cross-platform installer story; refreshed the
  tool/pack counts (238 tools, 27 packs).

## [0.1.5] — see GitHub Releases

Marketplace growth, agent-management surface (`klim agent`), health/PATH diagnostics, and
plan/apply/rollback safety net. Full notes:
<https://github.com/nassiharel/klim/releases/tag/v0.1.5>

## [0.1.0] — initial public releases

Cross-platform tool discovery and install over native package managers, the Bubbletea TUI,
`.klim.yaml` project contract, and export/import. Full history:
<https://github.com/nassiharel/klim/releases>

[Unreleased]: https://github.com/nassiharel/klim/compare/v0.1.5...HEAD
[0.1.5]: https://github.com/nassiharel/klim/releases/tag/v0.1.5
[0.1.0]: https://github.com/nassiharel/klim/releases/tag/v0.1.0
