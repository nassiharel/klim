# Discoverable Agent Marketplaces

Each YAML file in this directory describes one well-known third-party
agent marketplace that klim surfaces in the **Agents → Marketplaces**
sub-tab. They appear as `Installed=false` entries the user can install
via the detail page's **Add to library** action.

Files in this directory are embedded into the klim binary at build
time (`marketplace/marketplaces/embed.go` uses `//go:embed` and the
catalog loader in `internal/agents/catalog/discoverable.go` reads
from that embedded FS), so adding or changing an entry requires
re-building.

## Schema

```yaml
id: openai-codex-plugin-cc            # required, kebab-case
name: openai-codex-plugin-cc          # required, used for de-dup
display_name: OpenAI Codex Plugin     # optional, shown in detail page
description: |                        # optional
  OpenAI's Codex plugin marketplace for Claude Code
providers:                            # required, ≥1
  - claude-code
  - copilot-cli
owner: openai                         # optional
url: https://github.com/openai/codex-plugin-cc
install_spec: openai/codex-plugin-cc  # required for "Add to library"
source: catalog-claude                # optional; provider-derived if omitted
```

### Provider IDs

| ProviderID      | Source value           |
|-----------------|------------------------|
| `claude-code`   | `catalog-claude`       |
| `copilot-cli`   | `catalog-copilot`      |
| `mcp-registry`  | `catalog-mcp-registry` |

A marketplace listed under multiple `providers:` is expanded into one
`Marketplace` entry per provider during catalog load. Each expansion
is independently de-duplicated against the provider's locally
registered marketplaces during the snapshot merge.

## Adding a new entry

1. Drop a new `<name>.yaml` file here.
2. `go test ./internal/agents/catalog/...` — the loader validates the
   YAML at startup; the test asserts every file parses and lists at
   least one provider.
3. `go build ./...` to bake it into the binary.
