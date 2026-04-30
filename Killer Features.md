Killer Features — Ranked by Impact

## 🏆 Tier 1 — Game-Changers

📋 **Team Manifests (.clim.yaml)** ✅ SHIPPED — project-aware tool detection, `clim check`/`clim init`, multi-project management, required + optional tools, CI integration with `--json` output, 70+ file type detection.

🩺 **`clim doctor` — Universal Environment Health Check** ✅ SHIPPED — Detects conflicting tool versions (multiple installations with different versions), broken/missing/inaccessible PATH directories, duplicate PATH entries, missing package managers, stale scan cache, unresolved versions, and outdated tools summary. CLI: `clim doctor` with `--json` and `--refresh` flags. TUI: dedicated Doctor tab (key 9) with scrollable color-coded output. Exit code 0 = no errors, 1 = errors found.

🐚 **Shell Integration — Hook + Completions** ✅ SHIPPED — (1) **Hook:** `eval "$(clim hook bash)"` (zsh/fish/powershell). Auto-runs `clim check` when you `cd` into a project with `.clim.yaml` — only shows output when tools are missing or outdated. The nvm/direnv model. (2) **Completions:** `clim completion bash|zsh|fish|powershell` — native tab completion for all commands, flags, and tool names via Cobra.

🔀 **`clim diff` — Environment Comparison** ✅ SHIPPED — Compare your installed tools against a manifest file (`clim diff my-tools.yaml`) or share token (`clim diff clim:v1:abc...`). Side-by-side table showing tool name, local version/source, remote version/source, and status (✓ match / ≠ differs / ← local only / → remote only). Exit code 0 = match, 1 = differences found. Supports `--refresh` for fresh scan.

⚡ **`clim proxy` — Auto-Install Shims** ✅ SHIPPED — `clim proxy setup` creates a managed shims directory. `clim proxy add kubectl terraform` creates lightweight shims that auto-install tools on first use via the best available package manager. Shims delegate to `clim proxy run` which finds the real binary (skipping the shims dir), or installs if missing, then executes transparently. Supports `setup`, `add`, `remove`, `list` subcommands. Cross-platform (`.cmd` on Windows, shell scripts on Unix).

🔐 **`clim audit` — Security, Compliance & SBOM** ✅ SHIPPED — Audits installed tools for: unmanaged installs (manual/unknown source), archived upstream repos, stale projects (no activity 12+ months), missing version info, outdated tools. Reports license inventory across your toolchain. `clim audit --sbom` generates CycloneDX 1.5 SBOM with tool metadata, licenses, source paths, and VCS references. `--json` for CI pipelines. Exit code 0 = clean, 1 = warnings found.

📸 **Environment Snapshots & Profiles** ✅ SHIPPED — `clim snapshot save/list/show/delete` for timestamped snapshots of installed tools. `clim snapshot profile save/list/show/delete` for named profiles ("work", "personal"). Snapshots stored under `~/.config/clim/snapshots/`, profiles under `~/.config/clim/profiles/`. Fuzzy name matching for show/delete. Built on the existing manifest format.

## 🥈 Tier 2 — Strong Differentiators

🎓 **`clim onboard` — Interactive Setup Wizard** ✅ SHIPPED — 6 dev roles (web, devops, data, mobile, systems, security). Scores marketplace tools by category/tag overlap + GitHub stars. Shows top 15 recommendations with descriptions. `clim onboard devops --list` for preview, or interactive mode with install prompt. Bulk installs via best available PM.

🔍 **`clim why <tool>` — Reverse Dependency Map** ✅ SHIPPED — Shows install status, version info, all references across .clim.yaml projects and packs, available package managers, and related installed tools by tag/category overlap.

🔔 **`clim watch` — Update Monitor** ✅ SHIPPED — `clim watch` does a fresh scan and reports all available updates. `--json` for machine-readable output. Designed for cron/Task Scheduler integration. Always forces a fresh scan for authoritative results.

🏎️ **`clim benchmark` — PM Speed Comparison** — `clim benchmark terraform` → "scoop: install 4.2s, query 0.8s ★ fastest / winget: install 12.1s, query 2.3s". Recommendation: "Switch terraform to scoop for 2.9x faster installs."

🧪 **`clim try` — Tool Playground** ✅ SHIPPED — `clim try bat -- README.md` installs a tool, runs it with args, then asks "Keep or remove?". `--keep` flag to skip the prompt. If already installed, just runs it. Cleanup uses the correct PM remove command.

## 🧠 Tier 3 — Visionary / Long-term

🏅 **`clim score` — Environment Health Score** ✅ SHIPPED — Single 0-100 metric combining tool freshness (30pts), doctor health (25pts), audit findings (20pts), compliance (15pts), and managed sources (10pts). Grade scale A+ to F. CLI with `--json` for CI and `--badge` for shields.io URL. TUI Dashboard shows score gauge. Gamifies environment management.

📡 **Plugin System for Custom PMs** — Allow enterprises to add internal package managers (Artifactory, internal registries). Simple interface: `InstalledVersion()`, `LatestVersion()`, `InstallCmd()`. Custom marketplace URLs are ✅ SHIPPED via `clim marketplace add <url>` — multiple marketplace YAML sources are merged at load time. Full PM plugin interface is future work.

📊 **Smart History Analysis** — Opt-in: analyze shell history. "You ran `jq` 47 times last month but it's not in your favorites." Suggest tools based on actual usage, not just what's installed. "You haven't used terraform in 45 days. Remove?"

🤖 **AI Tool Discovery** — "I need to process JSON" → suggests jq, gron, fx, jless. Natural language search over the marketplace. Could use embeddings on tool descriptions + tags.

🏗️ **`clim generate`** ✅ SHIPPED — Auto-generate CI/container configs from `.clim.yaml`: `clim generate github-action` (workflow with install + verify steps), `clim generate dockerfile` (apt/brew/npm installs), `clim generate devcontainer` (VS Code / GitHub Codespaces with Dev Container Features mapping). Resolves tool names to package IDs from the marketplace. Supports `--output` for file writing and `--base` for custom Docker images.