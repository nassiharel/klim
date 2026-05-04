<p align="center">
  <img src="assets/logo.svg" alt="clim logo" width="480">
</p>

<h1 align="center">clim</h1>

<p align="center">
  <strong>One command to discover, inspect, and upgrade every CLI on your system.</strong>
</p>

<p align="center">
  <a href="https://github.com/nassiharel/clim/releases/latest"><img src="https://img.shields.io/github/v/release/nassiharel/clim?style=flat-square" alt="Release"></a>
  <a href="https://github.com/nassiharel/clim/actions"><img src="https://img.shields.io/github/actions/workflow/status/nassiharel/clim/ci.yml?style=flat-square" alt="CI"></a>
  <a href="https://github.com/nassiharel/clim/actions/workflows/codeql.yml"><img src="https://img.shields.io/github/actions/workflow/status/nassiharel/clim/codeql.yml?style=flat-square&label=CodeQL" alt="CodeQL"></a>
  <a href="https://goreportcard.com/report/github.com/nassiharel/clim"><img src="https://img.shields.io/badge/go%20report-A+-brightgreen?style=flat-square" alt="Go Report Card"></a>
  <a href="https://pkg.go.dev/github.com/nassiharel/clim"><img src="https://img.shields.io/badge/godoc-reference-blue?style=flat-square" alt="Go Reference"></a>
  <a href="https://github.com/nassiharel/clim/releases"><img src="https://img.shields.io/github/downloads/nassiharel/clim/total?style=flat-square" alt="Downloads"></a>
  <a href="LICENSE"><img src="https://img.shields.io/github/license/nassiharel/clim?style=flat-square" alt="License"></a>
</p>

---

## Screenshots

<p align="center">
  <img src="assets/tui-installed.png" alt="Installed tab" width="720">
</p>

> The TUI shows all detected tools with version status, install source, and upgrade availability. Navigate between tabs to discover new tools, manage updates, and export your toolchain.

---

## Features

### 🔍 Discover & Install Tools
Browse 110+ curated developer tools from one place. Filter by category, tag, or platform. Install anything with one keypress — clim picks the right package manager for your OS.

### 🛠️ Scriptable Install / Upgrade / Remove
`clim install jq fzf`, `clim upgrade --pack go-dev`, `clim remove jq` — install/upgrade/remove tools and packs from the command line, with `--source` to pin a package manager, `--pack` for bundles, `--dry-run` to preview, and `--output json` for CI.

### 📦 Packs — Curated Tool Bundles
Install entire toolchains in one shot. Cloud Essentials, K8s Starter, Python Developer — pick a pack and go. See which packs you've already completed with visual progress gauges.

### 🎒 Create Your Own Pack
Hand-pick tools from the marketplace, give your pack a name, and save it. Share it with teammates or use it to set up your next machine.

### 📋 Team Manifests (`.clim.yaml`)
Drop a `.clim.yaml` in your repo to define required and optional tools with version constraints. `clim check` validates every developer's environment — in the terminal or CI. `clim init` scans your project files (Dockerfile, go.mod, package.json, CI workflows, Helm charts, Terraform, Bicep, and 30+ more) and generates one automatically. `clim generate` produces GitHub Actions workflows, Dockerfiles, and devcontainer.json from your manifest. Manage multiple projects from the TUI's Project tab — add tools, install missing dependencies, and keep everyone in sync.

### 🔄 Keep Everything Up to Date
See which tools have updates at a glance. Batch-upgrade everything with one keystroke, or pick and choose. clim queries native package managers — no custom registries.

### 💾 Backup & Restore Your Toolchain
Export your installed tools to a portable manifest. Import it on a new machine — same tools, same setup, zero guesswork. Backups are saved automatically so you never lose your setup.

### 🔗 Share Your Toolchain
Generate a compact share token and paste it in Slack, Teams, or email. Recipients run `clim share open <token>` to get your exact toolchain. No files to send.

### 🖥️ Move Between OSes
Installed everything on macOS? Export and import on your new Linux box. clim maps each tool to the best available package manager on the target OS — winget, brew, apt, choco, scoop, snap, or npm.

### 📊 Dashboard
See your entire dev environment at a glance: installed vs available, update status, package manager breakdown, category distribution, pack completion, and recent backups — all with visual gauges and charts.

### ⚡ Smart Recommendations
clim analyzes your installed tools and suggests related ones you might like, ranked by tag overlap. Discover tools you didn't know existed.

### ⚙️ Built-in Config Editor
Tune clim from inside the TUI — log level, refresh interval, concurrency, default tab, sidebar position. Toggle, cycle, type, save. No need to find and edit a YAML file.

### 🩺 Environment Doctor
Run `clim doctor` to diagnose your environment — detects duplicate and broken PATH entries, conflicting tool versions (multiple installations), missing package managers, stale caches, and unresolved versions. JSON output for CI with `--json`. TUI Doctor tab shows color-coded issues with fix suggestions.

### 🐚 Shell Integration
Native tab completion for bash, zsh, fish, and PowerShell via `clim shell completion`. Shell hooks via `clim shell hook` that auto-check `.clim.yaml` when you `cd` into a project — like nvm/direnv for your entire toolchain.

### 🔀 Environment Diff
Compare your local tools against a colleague's manifest or share token with `clim diff`. See which tools match, differ in version, or are missing on either side — the "works on my machine" killer.

### 🔐 Security Audit & SBOM
Run `clim audit` to flag unmanaged installs, archived projects, stale repos, and missing versions. Get a license inventory across your toolchain. Generate a CycloneDX SBOM with `clim audit --sbom` for compliance pipelines.

### ⚡ Auto-Install Shims
Create lightweight shims with `clim proxy add kubectl terraform` that auto-install tools on first use. Run a shimmed tool that isn't installed — clim installs it transparently via the best available package manager, then runs it. Like `npx` but for any CLI tool.

### 🎓 Onboarding Wizard
Run `clim onboard` to get role-based tool recommendations. Pick your role (web, devops, data, mobile, systems, security) and get a curated list of tools ranked by relevance, with one-click batch install.

### 🔍 Tool Provenance
Run `clim why kubectl` to see where a tool is referenced — which projects require it, which packs include it, related tools, and all available package managers.

### 🔔 Update Watch
Run `clim watch` to check all tools for updates. Designed for cron or Task Scheduler — use `--json` for scripted notifications.

### 🧪 Tool Playground
Run `clim try bat -- README.md` to install a tool temporarily, run it, then choose whether to keep or remove it. Try before you commit.

### 🔒 Native Package Managers Only
clim never installs anything itself. It delegates to the package managers you already have — winget, brew, apt, choco, scoop, snap, npm. What you see is what your system runs.

### 📡 Custom Marketplaces
Add extra marketplace URLs to extend the tool catalog with your own or community tool definitions. Tools from extra sources are merged with the default catalog. Manage with `clim marketplace add/remove/list` or configure `extra_urls` in config.yaml.

---


## Why not just use an agent with skills?

> *"Cool — but I have an AI agent. It can run shell commands. Can't it do all of this?"*

It can, for one-off tasks. But "agent + shell access" and clim solve different problems, and at any non-trivial scale the differences compound. Honest case for both:

### Where an agent does fine
- One-shot exploratory installs on your laptop (*"set me up for this React project"*).
- Translating fuzzy intent into commands (*"I need something to query JSON"* → `jq`).
- Composing multi-step plans across unfamiliar tools.

### Where you want clim

#### Determinism
- Reproducible across machines, days, models. `clim install --pack go-dev --output json` exits 3 the same way every time. *"Hey agent, set up the pack"* is a coin flip on prompt, model, and weather.
- A regression test can pin clim's output. It cannot pin an LLM's output.

#### Speed and cost
- `clim list` is **sub-second**, free, offline. The agent equivalent is 5–30 seconds, costs tokens, and dies on a plane / VPN-only / air-gapped corp net.
- Multiply by 10 000 engineers running it daily — that's ~$millions/yr in tokens for an answer a single binary stat already had.
- Battery, data, and laptop fan all thank you.

#### Trust boundary
- clim runs a **curated, public catalog** (110+ vetted entries) through **native package managers**. Every install is `winget install Stedolan.jq` — auditable, signed by the OS package manager.
- A shell-enabled agent will happily `curl … | bash` whatever a Stack Overflow answer / README / GitHub Issue suggests. That is a SOC2 audit-finding waiting to happen.
- **Prompt injection**: a malicious tool's README on GitHub can convince an agent to install something else / exfiltrate secrets / `rm -rf`. clim's catalog is git-versioned source code; READMEs aren't executable.

#### Compliance as code, not as vibes
- `.clim-policy.yaml` defines blocked tools, allowed licenses, allowed install sources, required versions. `compliance.block_installs: true` makes them **hard refusals**, not "the agent will probably honor it."
- Policy is fetched from a versioned URL with caching, signed cache validation, and an audit trail.
- Try writing *"never install GPL-licensed CLIs"* as a system prompt and proving to an auditor that it always holds. You can't.

#### Privacy
- Your installed-tools list, your project's `.clim.yaml`, your company's compliance policy never leave the machine.
- "Ask the agent what's in `~/bin`" sends that information to a third-party model provider every time.
- Air-gapped / regulated environments simply cannot run agent-only workflows.

#### Stateful, portable artifacts
- **`.clim.yaml`** in a repo: a versioned, lintable contract every developer + CI follows.
- **Manifest exports** & **share tokens**: paste `clim:abc123…` in Slack — recipient runs one command and gets the same toolchain. No backend, no auth, no model.
- **`clim trail`**: git-style content-addressable history of your dev environment, diffable across machines and across time. Hashes survive process restarts; agent memory does not.
- These all outlive the chat session.

#### Scriptability and CI
- Agents in CI = non-determinism + token spend per build + flaky tests. Most teams ban them from the critical path.
- `RUN clim import team.yaml --yes` works in 100 000 container builds with zero model calls.
- `--output json` has a **stable, versioned schema**. LLM JSON output drifts the moment the model updates.

#### Offline / cold-start
- Fresh box, no API key, no network: `clim list` still works. Cached catalog + native PM queries are enough.
- Onboarding day-one engineers who don't yet have agent access is a real UX.

#### Explainability
- `clim why jq` answers *"why is this on my machine?"* in milliseconds, citing the team manifest, pack, or project that requires it. An agent has to *guess* (and won't remember last week).
- `clim diff <other-machine-token>` shows **exactly** which tool versions differ between two environments. Agent diffs are prose.

#### Discoverability for humans
- A user who doesn't know `kubectx`, `direnv`, or `gh` exists can't ask the agent for them by name.
- The TUI / `clim browser` marketplace surfaces the **long tail of tools you didn't know you needed** — categories, tags, "for you" recommendations based on what's already on your box.

#### Catalog freshness
- LLM training cutoffs lag months/years. clim's catalog updates **daily** via the `marketplace` branch and ships new tool entries via PRs anyone can audit.
- A new tool released yesterday is in the catalog today. The agent will tell you about it next year.

#### Bounded resource use
- `clim` cannot, under any circumstance, spend $10 000 on tokens overnight in a runaway loop.
- It cannot leak its system prompt or its catalog because it has neither.

#### Agents *should call clim*
The honest framing: **clim is a primitive an agent should be using, not competing with.**
> *"Hey agent, my dev box should have the go-dev pack."*
> Agent: `clim install --pack go-dev --output json` → parses the JSON.

The agent supplies intent translation. clim supplies the deterministic, auditable, cached primitive. Without something like clim, every agent has to re-implement PATH scanning, source detection, version comparison, and per-OS package-manager dispatch in throwaway shell heredocs — badly, every time.

### One-line summary
> **Agents handle the long tail of judgment calls. clim handles the short head of operations you want to be the same every time.**

You don't replace `apt`, `winget`, or `npm` with an agent — you let agents call them. clim is the missing piece between *agent* and *apt* / *brew* / *winget*: the contract layer that makes the answer the same every time, on every machine, in every CI pipeline, for every user, today and a year from now.

---


## Usage

### Interactive TUI

```bash
clim
```

Launches a full-screen interactive interface with 9 tabs. Tools are detected and version-checked concurrently -- results stream in as they arrive.

## Architecture

**Version sources** -- versions are queried from native package managers, not HTTP APIs:

| Package Manager | Platforms | Used for |
|-----------------|-----------|----------|
| winget | Windows | Installed + latest versions |
| Chocolatey | Windows | Installed + latest versions |
| Homebrew | macOS, Linux | Installed + latest versions |
| apt / dpkg | Debian/Ubuntu | Installed + latest versions |
| snap | Linux | Installed + latest versions |
| npm | All | Installed + latest versions |

**Self-update**: `clim update` queries the GitHub Releases API for the latest `nassiharel/clim` release, downloads the correct platform archive, and replaces the binary in-place using a rename-swap strategy.

---

## Use Cases

### Developers
- Get a quick snapshot of your local environment
- Know which tools need updating before starting a project

### DevOps / Platform Teams
- Standardize developer environments across a team
- Export a toolchain manifest and import it on new machines

### Security Teams
- Detect outdated CLI tools that may contain vulnerabilities
- Verify tools are running from expected paths

### CI/CD Pipelines
- Validate tool versions before execution
- Fail fast if a required tool is missing or outdated

---

## Configuration

### Tool Catalog

The user's tool catalog (customizations, enabled/disabled state, custom tools) is stored at:

| OS | Path |
|----|------|
| macOS | `~/Library/Application Support/clim/marketplace-cache.yaml` |
| Linux | `~/.config/clim/marketplace-cache.yaml` |
| Windows | `%AppData%\clim\marketplace-cache.yaml` |

The catalog is fetched from GitHub and cached locally for offline use.

Edit the user catalog with `clim tools edit` or directly. User customizations (enabled/disabled state, custom tools) are preserved across updates.

### Config File

clim's configuration file (`config.yaml`) lives in the same directory as the tool catalog. It controls GitHub source, marketplace refresh behavior, concurrency, timeouts, and UI preferences. All values are optional with sensible defaults.

```bash
clim config path   # show config.yaml location
clim config edit   # open config.yaml in $EDITOR
```

---

## Troubleshooting

| Problem | Solution |
|---------|----------|
| `clim: command not found` | Ensure install directory is in `$PATH`. Run `which clim` (macOS/Linux) or `where clim` (Windows) to check. |
| Tool not detected | Verify binary is in `$PATH` with `which <tool>` / `where <tool>`. Run `clim` then press `r` to refresh. |
| Permission denied on upgrade | Package manager may need elevated privileges. Use `sudo` (Linux/macOS) or run as Administrator (Windows). |
| Stale version info | Run `clim doctor` to diagnose, or delete local cache (`clim tools path` shows location) and relaunch to re-fetch from GitHub. |
| Self-update fails | Download manually from [Releases](https://github.com/nassiharel/clim/releases/latest) and replace binary. |

---

## Contributing

Contributions are welcome! Please read [CONTRIBUTING.md](./CONTRIBUTING.md) before submitting a pull request.

See [AGENTS.md](./AGENTS.md) for detailed architecture documentation.

## Roadmap

- Add more package managers (pip, gem, cargo, asdf, etc)

## License

MIT
