# Discoverable Agent Marketplaces

Each YAML file in this directory describes one well-known third-party
agent marketplace that klim surfaces in the **Agents → Marketplaces**
sub-tab. They appear as `Installed=false` entries the user can install
via the detail page's **Add to library** action.

Files in this directory are assembled into `marketplace.yaml` by CI
(via `internal/marketplace/assemble`) alongside tool and pack
definitions. The CLI reads them from the `agent_marketplaces:` section
of the cached marketplace.yaml at runtime — the same fetch/cache
mechanism used for tools and packs.

## Schema

```yaml
id: openai-codex-plugin-cc            # optional, kebab-case; defaults to name
name: openai-codex-plugin-cc          # required, used for de-dup
display_name: OpenAI Codex Plugin     # optional, shown in detail page
description: |                        # optional
  OpenAI's Codex plugin marketplace for Claude Code
providers:                            # required, ≥1
  - claude-code
  - copilot-cli
owner: openai                         # optional
url: https://github.com/openai/codex-plugin-cc
install_spec: openai/codex-plugin-cc  # at least one of install_spec or url required
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
2. `make marketplace-validate` — the validator checks required fields.
3. `make marketplace-assemble` — bake it into `marketplace.yaml`.
4. `go test ./internal/agents/catalog/...` — verify expansion logic.
