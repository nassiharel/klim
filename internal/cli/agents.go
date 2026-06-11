package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/nassiharel/klim/internal/agents"
	"github.com/nassiharel/klim/internal/agents/catalog"
	"github.com/nassiharel/klim/internal/agents/providers/claudecode"
	"github.com/nassiharel/klim/internal/agents/providers/copilotcli"
)

// newAgentsService builds the AgentService used by every `klim agents`
// subcommand. Kept as a function so future tests can swap in fakes.
var newAgentsService = func() *agents.Service {
	svc := agents.NewService(4,
		claudecode.New(),
		copilotcli.New(),
	)
	svc.RemoteCatalog = catalogAdapter{f: catalog.New()}
	return svc
}

// surfaceSnapshotWarnings writes any snap.Warnings to stderr with a
// `agents: ` prefix. This is the CLI-side equivalent of the library
// fold: `hydrateSessionExtras` no longer writes to stderr directly
// (that corrupts the TUI's screen), so each CLI command that loads
// a snapshot routes warnings to stderr explicitly here. No-op when
// snap is nil or has no warnings. The TUI ignores this helper and
// surfaces snap.Warnings through its status row instead.
func surfaceSnapshotWarnings(snap *agents.Snapshot) {
	if snap == nil || len(snap.Warnings) == 0 {
		return
	}
	for _, w := range snap.Warnings {
		fmt.Fprintln(os.Stderr, "agents: "+w)
	}
}

// catalogAdapter bridges catalog.Fetcher to agents.RemoteCatalog. The
// agents package defines the RemoteCatalog interface so it never needs
// to import the catalog package (which would create a cycle since
// catalog imports agents for its types).
type catalogAdapter struct{ f *catalog.Fetcher }

// FetchAll fetches every configured marketplace and adapts the result
// into the agents.RemoteCatalogResult shape the service expects.
func (a catalogAdapter) FetchAll(ctx context.Context) []agents.RemoteCatalogResult {
	in := a.f.FetchAll(ctx)
	out := make([]agents.RemoteCatalogResult, 0, len(in))
	for _, r := range in {
		out = append(out, agents.RemoteCatalogResult{
			SourceName:   r.Source.Name,
			Plugins:      r.Plugins,
			Marketplaces: r.Marketplaces,
			Err:          r.Err,
		})
	}
	return out
}

// agentsCmd is the top-level umbrella. With no args it runs `agents list`.
var agentsCmd = &cobra.Command{
	Use:     "agents",
	Short:   "Browse and manage agent plugins, skills, MCPs, and sessions",
	GroupID: "tools",
	Long: `agents discovers and manages the agent-tooling ecosystem across
multiple agent CLIs (Claude Code, GitHub Copilot CLI, and more).

It surfaces five entity types — marketplaces, plugins, skills, MCPs, and
sessions — and lets you search, browse, install, launch, and remove them
through a single set of subcommands.

Examples:
  klim agents                       # list everything detected on this host
  klim agents search react          # global fuzzy search across all entities
  klim agents plugins list
  klim agents launch --provider claude-code --skill summarize
  klim agents launch --print-only --session claude:home%2Fuser%2Frepo`,
	RunE: func(cmd *cobra.Command, args []string) error { return runAgentsList(cmd, args, "") },
}

// ---------------- list ----------------

var (
	agentsListType      string
	agentsListProvider  string
	agentsListInstalled bool
	agentsListAvailable bool
	agentsListSearch    string
	agentsListRefresh   bool
)

var agentsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List discovered agent entities",
	RunE:  func(cmd *cobra.Command, args []string) error { return runAgentsList(cmd, args, "") },
}

// ---------------- search ----------------

var agentsSearchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Fuzzy search across all entities (use type:query for scoped search)",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		query := strings.Join(args, " ")
		return runAgentsSearch(cmd, query)
	},
}

// ---------------- marketplaces / plugins / skills / mcps / sessions ----------------

var agentsMarketplacesCmd = &cobra.Command{Use: "marketplaces", Aliases: []string{"market"}, Short: "Manage agent marketplaces"}
var agentsPluginsCmd = &cobra.Command{Use: "plugins", Aliases: []string{"plugin"}, Short: "Manage agent plugins"}
var agentsSkillsCmd = &cobra.Command{Use: "skills", Aliases: []string{"skill"}, Short: "Browse agent skills"}
var agentsMCPsCmd = &cobra.Command{Use: "mcps", Aliases: []string{"mcp"}, Short: "Manage MCP servers"}

// agentsSessionsCmd is the umbrella for `klim agents sessions …`.
// With no args it launches the focused sessions dashboard (Bubbletea
// TUI) when stdout is a TTY, falling back to a one-shot list when the
// output is piped — this keeps `klim agents sessions | head` and other
// scripting use cases working unchanged.
//
// Explicit subcommands (resume, view, tail, stats, files, star,
// unstar, group, delete) handle the rest of the dashboard surface.
var agentsSessionsCmd = &cobra.Command{
	Use:     "sessions",
	Aliases: []string{"session"},
	Short:   "List, resume, view, and manage agent sessions",
	Long: `sessions inspects Claude Code and Copilot CLI session
transcripts and presents them as a glanceable dashboard.

Bare ` + "`klim agents sessions`" + ` opens a TUI dashboard when stdout
is a TTY; when piped (or when stdout is not a terminal) it runs the
equivalent of ` + "`klim agents sessions list`" + ` so existing
pipelines keep working.

Examples:
  klim agents sessions
  klim agents sessions list --status waiting --since 2h
  klim agents sessions view claude:3b4dc369-…
  klim agents sessions tail claude:3b4dc369-…
  klim agents sessions stats --output json
  klim agents sessions files --top 10
  klim agents sessions star claude:3b4dc369-…
  klim agents sessions group set klim=Klim`,
	RunE: runAgentsSessionsDefault,
}

// ---------------- launch ----------------

var (
	agentsLaunchProvider  string
	agentsLaunchSkill     string
	agentsLaunchSession   string
	agentsLaunchPlugin    string
	agentsLaunchCwd       string
	agentsLaunchPrompt    string
	agentsLaunchPrintOnly bool
)

var agentsLaunchCmd = &cobra.Command{
	Use:   "launch",
	Short: "Launch an agent session (skill, plugin, or saved session)",
	RunE:  runAgentsLaunch,
}

// ---------------- refresh / doctor ----------------

var agentsRefreshCmd = &cobra.Command{
	Use:   "refresh",
	Short: "Invalidate the agents scan cache and rescan",
	RunE: func(cmd *cobra.Command, args []string) error {
		svc := newAgentsService()
		svc.Invalidate()
		ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
		defer cancel()
		snap, err := svc.LoadAll(ctx, agents.LoadOpts{Refresh: true})
		if err == nil {
			surfaceSnapshotWarnings(snap)
			fmt.Fprintln(os.Stderr, "agents: cache refreshed")
		}
		return err
	},
}

var agentsDoctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Diagnose provider detection and cache freshness",
	RunE:  runAgentsDoctor,
}

var agentsListFormatGetter func() (OutputFormat, error)
var agentsSearchFormatGetter func() (OutputFormat, error)

func init() {
	// list
	agentsListCmd.Flags().StringVar(&agentsListType, "type", "", "filter by entity type: marketplace|plugin|skill|mcp|session")
	// PR #77 review #3: --provider is declared as a persistent flag on
	// the parent (agentsCmd) below; we no longer redeclare it here so
	// passing `klim agents list --provider X` and `klim agents --provider X list`
	// hit the same flag binding instead of fighting over the variable.
	agentsListCmd.Flags().BoolVar(&agentsListInstalled, "installed", false, "show only installed entities (plugins/MCPs)")
	agentsListCmd.Flags().BoolVar(&agentsListAvailable, "available", false, "show only available (non-installed) catalog entries")
	agentsListCmd.Flags().StringVar(&agentsListSearch, "search", "", "filter by fuzzy match (same as `klim agents search …`)")
	agentsListCmd.Flags().BoolVar(&agentsListRefresh, "refresh", false, "ignore the cache and rescan")
	agentsListFormatGetter = addOutputFlag(agentsListCmd, OutputText, OutputJSON, OutputYAML)

	// inherit list flags on the parent so `klim agents --type plugin` works.
	agentsCmd.Flags().AddFlagSet(agentsListCmd.Flags())

	// search
	agentsSearchFormatGetter = addOutputFlag(agentsSearchCmd, OutputText, OutputJSON, OutputYAML)

	// launch
	agentsLaunchCmd.Flags().StringVar(&agentsLaunchProvider, "provider", "", "provider id (claude-code|copilot-cli)")
	agentsLaunchCmd.Flags().StringVar(&agentsLaunchSkill, "skill", "", "skill name to focus on after launch")
	agentsLaunchCmd.Flags().StringVar(&agentsLaunchSession, "session", "", "session id to resume")
	agentsLaunchCmd.Flags().StringVar(&agentsLaunchPlugin, "plugin", "", "plugin to keep active in the launched session")
	agentsLaunchCmd.Flags().StringVar(&agentsLaunchCwd, "cwd", "", "working directory to launch from")
	agentsLaunchCmd.Flags().StringVar(&agentsLaunchPrompt, "prompt", "", "non-interactive prompt to send to the agent")
	agentsLaunchCmd.Flags().BoolVar(&agentsLaunchPrintOnly, "print-only", false, "print the exact command without executing")

	// Per-entity subcommands.
	agentsMarketplacesCmd.AddCommand(makeListSub("list marketplaces", agentsListMarketplaces))
	agentsMarketplacesCmd.AddCommand(&cobra.Command{Use: "add <name-or-url>", Short: "Add a marketplace", Args: cobra.ExactArgs(1), RunE: agentsAddMarketplace})
	agentsMarketplacesCmd.AddCommand(&cobra.Command{Use: "remove <name>", Short: "Remove a marketplace", Args: cobra.ExactArgs(1), RunE: agentsRemoveMarketplace})

	agentsPluginsCmd.AddCommand(makeListSub("list plugins", agentsListPlugins))
	agentsPluginsCmd.AddCommand(&cobra.Command{Use: "install <ref>", Short: "Install a plugin (name@marketplace, owner/repo, or path)", Args: cobra.ExactArgs(1), RunE: agentsInstallPlugin})
	agentsPluginsCmd.AddCommand(&cobra.Command{Use: "uninstall <id>", Short: "Uninstall a plugin", Args: cobra.ExactArgs(1), RunE: agentsUninstallPlugin})

	agentsSkillsCmd.AddCommand(makeListSub("list skills", agentsListSkills))

	agentsMCPsCmd.AddCommand(makeListSub("list MCPs", agentsListMCPs))
	agentsMCPsCmd.AddCommand(&cobra.Command{Use: "remove <name>", Short: "Remove an MCP server", Args: cobra.ExactArgs(1), RunE: agentsRemoveMCP})

	agentsSessionsCmd.AddCommand(newAgentsSessionsListCmd())
	agentsSessionsCmd.AddCommand(newAgentsSessionsViewCmd())
	agentsSessionsCmd.AddCommand(newAgentsSessionsTailCmd())
	agentsSessionsCmd.AddCommand(newAgentsSessionsStatsCmd())
	agentsSessionsCmd.AddCommand(newAgentsSessionsFilesCmd())
	agentsSessionsCmd.AddCommand(newAgentsSessionsStarCmd())
	agentsSessionsCmd.AddCommand(newAgentsSessionsUnstarCmd())
	agentsSessionsCmd.AddCommand(newAgentsSessionsGroupCmd())
	resumeCmd := &cobra.Command{
		Use:   "resume [id]",
		Short: "Resume a session (exec the agent CLI)",
		Long: `Resume a session by id, by fuzzy match on title/project, or by passing --last.

Examples:
  klim agents sessions resume claude:foo-bar
  klim agents sessions resume "fix cron"   # fuzzy match on title/project
  klim agents sessions resume --last        # most recently modified session`,
		// usageArgs ensures `accepts at most 1 arg(s)` (cobra's
		// MaximumNArgs message) surfaces as *UsageError → exit 2,
		// matching the >1-args contract in CLI-CONVENTIONS.md.
		Args: usageArgs(cobra.MaximumNArgs(1)),
		RunE: agentsResumeSession,
	}
	resumeCmd.Flags().Bool("last", false, "resume the most recently modified session")
	agentsSessionsCmd.AddCommand(resumeCmd)
	agentsSessionsCmd.AddCommand(&cobra.Command{Use: "delete <id>", Short: "Delete a session", Args: cobra.ExactArgs(1), RunE: agentsDeleteSession})

	// PR #77 review #3: drop the confusingly-named `--provider-filter`
	// persistent flag and replace it with a single `--provider` that
	// every subcommand (including per-entity install/remove ops)
	// inherits. The list-only `--provider` flag is now declared on
	// the parent so all subcommands share one flag identity.
	agentsCmd.PersistentFlags().StringVar(&agentsListProvider, "provider", "", "limit operation to one provider: claude-code|copilot-cli")

	agentsCmd.AddCommand(agentsListCmd)
	agentsCmd.AddCommand(agentsSearchCmd)
	agentsCmd.AddCommand(agentsMarketplacesCmd)
	agentsCmd.AddCommand(agentsPluginsCmd)
	agentsCmd.AddCommand(agentsSkillsCmd)
	agentsCmd.AddCommand(agentsMCPsCmd)
	agentsCmd.AddCommand(agentsSessionsCmd)
	agentsCmd.AddCommand(agentsLaunchCmd)
	agentsCmd.AddCommand(agentsRefreshCmd)
	agentsCmd.AddCommand(agentsDoctorCmd)

	rootCmd.AddCommand(agentsCmd)
}

// ---------------- list runners ----------------

// runAgentsList resolves the snapshot for `klim agents list` and the
// per-entity convenience subcommands. `entityFilter`, when non-empty,
// overrides the `--type` flag — this lets per-entity wrappers narrow
// the result set without mutating the package-level flag variable
// (see PR #77 review #2).
func runAgentsList(cmd *cobra.Command, _ []string, entityFilter agents.EntityType) error {
	ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
	defer cancel()

	svc := newAgentsService()
	snap, err := svc.LoadAll(ctx, agents.LoadOpts{Refresh: agentsListRefresh})
	if err != nil {
		return fmt.Errorf("agents list: %w", err)
	}
	surfaceSnapshotWarnings(snap)

	// CLI-provided --type wins when no per-entity wrapper supplied one.
	if entityFilter == "" {
		entityFilter = agents.EntityType(strings.TrimSpace(agentsListType))
	}

	if agentsListSearch != "" {
		return printSearchResults(svc.Search(agentsListSearch, entityFilter))
	}

	format, err := agentsListFormatGetter()
	if err != nil {
		return err
	}

	switch format {
	case OutputJSON:
		// Pre-existing JSON shape uses Go field names; preserved
		// exactly as before this PR — no schema change.
		return printJSON(filteredSnapshot(snap, entityFilter))
	case OutputYAML:
		// YAML uses agents-native yaml: tags via direct marshaling
		// so snake_case keys / omitempty are honoured without
		// disturbing the JSON contract above.
		return printYAMLDirect(filteredSnapshot(snap, entityFilter))
	}

	return renderSnapshotText(filteredSnapshot(snap, entityFilter))
}

func filteredSnapshot(snap *agents.Snapshot, entityFilter agents.EntityType) *agents.Snapshot {
	provider := agents.ProviderID(strings.TrimSpace(agentsListProvider))

	keepEntity := func(et agents.EntityType) bool {
		return entityFilter == "" || entityFilter == et
	}
	keepProvider := func(p agents.ProviderID) bool {
		return provider == "" || provider == p
	}
	keepInstalledPlugin := func(p agents.Plugin) bool {
		if agentsListInstalled && !p.Installed {
			return false
		}
		if agentsListAvailable && p.Installed {
			return false
		}
		return true
	}

	out := &agents.Snapshot{ProviderStatus: snap.ProviderStatus}
	if keepEntity(agents.EntityMarketplace) {
		for _, m := range snap.Marketplaces {
			if keepProvider(m.Provider) {
				out.Marketplaces = append(out.Marketplaces, m)
			}
		}
	}
	if keepEntity(agents.EntityPlugin) {
		for _, p := range snap.Plugins {
			if keepProvider(p.Provider) && keepInstalledPlugin(p) {
				out.Plugins = append(out.Plugins, p)
			}
		}
	}
	if keepEntity(agents.EntitySkill) {
		for _, s := range snap.Skills {
			if keepProvider(s.Provider) {
				out.Skills = append(out.Skills, s)
			}
		}
	}
	if keepEntity(agents.EntityMCP) {
		for _, m := range snap.MCPs {
			if keepProvider(m.Provider) {
				out.MCPs = append(out.MCPs, m)
			}
		}
	}
	if keepEntity(agents.EntitySession) {
		for _, s := range snap.Sessions {
			if keepProvider(s.Provider) {
				out.Sessions = append(out.Sessions, s)
			}
		}
	}
	return out
}

func renderSnapshotText(snap *agents.Snapshot) error {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	defer func() { _ = w.Flush() }()

	fmt.Fprintf(os.Stderr, "agents: providers=%d marketplaces=%d plugins=%d skills=%d mcps=%d sessions=%d\n",
		len(snap.ProviderStatus), len(snap.Marketplaces), len(snap.Plugins),
		len(snap.Skills), len(snap.MCPs), len(snap.Sessions))

	if len(snap.Marketplaces) > 0 {
		_, _ = fmt.Fprintln(w, "\nMARKETPLACES")
		_, _ = fmt.Fprintln(w, "SOURCE\tNAME\tOWNER\tPLUGINS\tURL\tDESCRIPTION")
		for _, m := range snap.Marketplaces {
			_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
				providerShortCLI(m.Provider), m.Name, dashOr(m.Owner),
				intOrDash(m.PluginCount), truncateAgent(m.URL, 50),
				truncateAgent(m.Description, 50))
		}
	}
	if len(snap.Plugins) > 0 {
		_, _ = fmt.Fprintln(w, "\nPLUGINS")
		_, _ = fmt.Fprintln(w, "SOURCE\tNAME\tVERSION\tMARKETPLACE\tSTATUS\tAUTHOR\tLICENSE\tDESCRIPTION")
		for _, p := range snap.Plugins {
			status := "available"
			switch {
			case p.Installed && p.Enabled:
				status = "installed"
			case p.Installed:
				status = "disabled"
			}
			_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
				providerShortCLI(p.Provider), p.Name, dashOr(p.Version), dashOr(p.Marketplace),
				status, dashOr(p.Author), dashOr(p.License),
				truncateAgent(p.Description, 60))
		}
	}
	if len(snap.Skills) > 0 {
		_, _ = fmt.Fprintln(w, "\nSKILLS")
		_, _ = fmt.Fprintln(w, "SOURCE\tNAME\tSCOPE\tFROM\tMODEL\tALLOWED-TOOLS\tDESCRIPTION")
		for _, s := range snap.Skills {
			from := s.SourcePlugin
			if from == "" {
				from = string(s.Scope)
			}
			_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
				providerShortCLI(s.Provider), s.Name, s.Scope, dashOr(from),
				dashOr(s.Model), truncateAgent(dashOr(s.AllowedTools), 28),
				truncateAgent(s.Description, 60))
		}
	}
	if len(snap.MCPs) > 0 {
		_, _ = fmt.Fprintln(w, "\nMCPS")
		_, _ = fmt.Fprintln(w, "SOURCE\tNAME\tTRANSPORT\tSCOPE\tENABLED\tTOOLS\tENDPOINT")
		for _, m := range snap.MCPs {
			endpoint := m.URL
			if endpoint == "" {
				endpoint = m.Command
			}
			_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%v\t%s\t%s\n",
				providerShortCLI(m.Provider), m.Name, m.Transport, m.Scope, m.Enabled,
				intOrDash(len(m.Tools)), truncateAgent(endpoint, 60))
		}
	}
	if len(snap.Sessions) > 0 {
		_, _ = fmt.Fprintln(w, "\nSESSIONS")
		_, _ = fmt.Fprintln(w, "SOURCE\tID\tTYPE\tSTATUS\tTURNS\tCREATED\tMODIFIED\tPROJECT")
		for _, s := range snap.Sessions {
			typ := s.Type
			if typ == "" {
				typ = "interactive"
			}
			status := string(s.Status)
			if status == "" {
				status = "—"
			}
			turns := "—"
			if s.TurnCount > 0 {
				turns = strconv.Itoa(s.TurnCount)
			}
			created := "—"
			if !s.Created.IsZero() {
				created = s.Created.Format(time.RFC3339)
			}
			modified := "—"
			if !s.LastModified.IsZero() {
				modified = s.LastModified.Format(time.RFC3339)
			}
			_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
				providerShortCLI(s.Provider), s.ID, typ, status, turns, created, modified, truncateAgent(s.ProjectPath, 40))
		}
	}
	return nil
}

// providerShortCLI is the CLI counterpart of providerShort. Mirrors the
// TUI labels so users see the same short tag in both surfaces.
func providerShortCLI(id agents.ProviderID) string {
	switch id {
	case agents.ProviderClaudeCode:
		return "claude"
	case agents.ProviderCopilotCLI:
		return "copilot"
	default:
		return string(id)
	}
}

func truncateAgent(s string, n int) string {
	if len(s) <= n {
		return s
	}
	if n < 4 {
		return s[:n]
	}
	return s[:n-1] + "…"
}

func dashOr(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

func intOrDash(n int) string {
	if n <= 0 {
		return "—"
	}
	return strconv.Itoa(n)
}

// ---------------- search ----------------

func runAgentsSearch(cmd *cobra.Command, query string) error {
	ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
	defer cancel()
	svc := newAgentsService()
	snap, err := svc.LoadAll(ctx, agents.LoadOpts{})
	if err != nil {
		return err
	}
	surfaceSnapshotWarnings(snap)
	results := svc.Search(query, "")
	format, err := agentsSearchFormatGetter()
	if err != nil {
		return err
	}
	switch format {
	case OutputJSON:
		// Pre-existing JSON schema used Go field names — preserved.
		return printJSON(results)
	case OutputYAML:
		// YAML honours SearchResult's yaml: tags directly.
		return printYAMLDirect(results)
	}
	return printSearchResults(results)
}

func printSearchResults(results []agents.SearchResult) error {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	defer func() { _ = w.Flush() }()
	if len(results) == 0 {
		fmt.Fprintln(os.Stderr, "agents: no matches")
		return nil
	}
	_, _ = fmt.Fprintln(w, "SOURCE\tTYPE\tNAME\tSUBTITLE\tSCORE")
	for _, r := range results {
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%d\n",
			providerShortCLI(r.Provider), r.Type, r.Name, truncateAgent(r.Subtitle, 60), r.Score)
	}
	return nil
}

// ---------------- launch ----------------

func runAgentsLaunch(cmd *cobra.Command, _ []string) error {
	svc := newAgentsService()
	providerID := agents.ProviderID(agentsLaunchProvider)

	// Smart provider inference: when --provider is omitted but the
	// caller specified --skill, --session, or --plugin, look at the
	// snapshot to see which provider uniquely owns that entity. If
	// exactly one matches, infer it; if zero or many match, ask the
	// user to disambiguate.
	if providerID == "" {
		inferred, why, err := inferLaunchProvider(cmd.Context(), svc,
			agentsLaunchSkill, agentsLaunchSession, agentsLaunchPlugin)
		if err != nil {
			return &UsageError{Err: err}
		}
		providerID = inferred
		if why != "" {
			fmt.Fprintln(os.Stderr, "agents: inferred provider:", why)
		}
	}

	spec := agents.LaunchSpec{
		Provider:   providerID,
		SkillName:  agentsLaunchSkill,
		SessionID:  agentsLaunchSession,
		PluginName: agentsLaunchPlugin,
		Cwd:        agentsLaunchCwd,
		Prompt:     agentsLaunchPrompt,
	}
	plan, err := svc.Launch(spec)
	if err != nil {
		return fmt.Errorf("agents launch: %w", err)
	}
	if agentsLaunchPrintOnly {
		fmt.Println(plan.CommandLine())
		return nil
	}
	// PR #77 review #5: fmt.Fprintln inserts a space between args,
	// which produced "agents:  <note>" (two spaces). Use Fprintf with
	// an explicit format string so the spacing is exact.
	fmt.Fprintf(os.Stderr, "agents: %s\n", plan.Note)
	fmt.Fprintln(os.Stderr, "agents: $ "+plan.CommandLine())
	return execPlanCLI(plan)
}

// inferLaunchProvider resolves the --provider for a launch when the
// caller omitted it. Returns ("", "", error) when no entity hint is
// given, or when the hint matches zero or many providers.
func inferLaunchProvider(ctx context.Context, svc *agents.Service, skill, session, plugin string) (agents.ProviderID, string, error) {
	if skill == "" && session == "" && plugin == "" {
		return "", "", errors.New("--provider is required (claude-code|copilot-cli) — or pass one of --skill / --session / --plugin to let klim infer it")
	}

	// Session id is provider-prefixed ("claude:…" / "copilot:…"), so we
	// can map it directly without a snapshot scan.
	if session != "" {
		switch {
		case strings.HasPrefix(session, "claude:"):
			return agents.ProviderClaudeCode, "from session id prefix", nil
		case strings.HasPrefix(session, "copilot:"):
			return agents.ProviderCopilotCLI, "from session id prefix", nil
		}
		// PR #77 review #6: a bare session id with no provider prefix
		// would otherwise fall through to the snapshot scan (which
		// doesn't handle sessions) and emit the generic "no provider
		// owns the requested entity" error. Surface a specific hint
		// so users know they need to qualify the id.
		return "", "", fmt.Errorf("session id %q is missing a provider prefix (expected claude:… or copilot:…)", session)
	}

	// Otherwise scan the snapshot and count provider matches.
	scanned, err := svc.LoadAll(ctx, agents.LoadOpts{})
	if err != nil {
		return "", "", fmt.Errorf("scan: %w", err)
	}
	surfaceSnapshotWarnings(scanned)
	snap := svc.Snapshot()
	if snap == nil {
		return "", "", errors.New("no scan data available")
	}

	matches := map[agents.ProviderID]bool{}
	switch {
	case skill != "":
		for _, s := range snap.Skills {
			if s.Name == skill {
				matches[s.Provider] = true
			}
		}
	case plugin != "":
		for _, p := range snap.Plugins {
			if p.Name == plugin {
				matches[p.Provider] = true
			}
		}
	}

	switch len(matches) {
	case 0:
		return "", "", errors.New("no provider owns the requested entity — pass --provider to disambiguate")
	case 1:
		for p := range matches {
			hint := "matched only " + string(p)
			return p, hint, nil
		}
	}
	return "", "", errors.New("multiple providers own this entity — pass --provider to disambiguate")
}

// execPlanCLI replaces the current process (Unix) or runs the child with
// inherited stdio (Windows) so the agent CLI takes over the terminal.
func execPlanCLI(plan agents.ExecPlan) error {
	if plan.Cwd != "" {
		if err := os.Chdir(plan.Cwd); err != nil {
			return fmt.Errorf("chdir: %w", err)
		}
	}
	// On Unix-like systems we exec to fully replace the process so
	// signals (Ctrl-C) work as expected. On Windows that's not
	// available, so we run as a child and forward stdio.
	if syscallExec != nil {
		args := append([]string{plan.Bin}, plan.Args...)
		env := os.Environ()
		env = append(env, plan.Env...)
		return syscallExec(plan.Bin, args, env)
	}
	c := exec.Command(plan.Bin, plan.Args...)
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	c.Env = append(os.Environ(), plan.Env...)
	if err := c.Run(); err != nil {
		var exitErr *exec.ExitError
		if ok := exitErrAs(err, &exitErr); ok {
			os.Exit(exitErr.ExitCode())
		}
		return err
	}
	return nil
}

// syscallExec is platform-specific (set in agents_unix.go on POSIX).
var syscallExec func(argv0 string, argv []string, envv []string) error

// exitErrAs is a small helper for errors.As(err, &exitErr) without an
// import on errors in this file.
func exitErrAs(err error, target **exec.ExitError) bool {
	return errors.As(err, target)
}

// ---------------- doctor ----------------

func runAgentsDoctor(cmd *cobra.Command, _ []string) error {
	ctx, cancel := context.WithTimeout(cmd.Context(), 10*time.Second)
	defer cancel()
	svc := newAgentsService()
	loaded, err := svc.LoadAll(ctx, agents.LoadOpts{})
	if err != nil {
		return err
	}
	surfaceSnapshotWarnings(loaded)
	snap := svc.Snapshot()
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	defer func() { _ = w.Flush() }()
	_, _ = fmt.Fprintln(w, "PROVIDER\tINSTALLED\tVERSION\tBIN\tNOTES")
	for _, p := range svc.Registry().Providers() {
		st := snap.ProviderStatus[p.ID()]
		notes := ""
		if st.Error != "" {
			notes = st.Error
		}
		_, _ = fmt.Fprintf(w, "%s\t%v\t%s\t%s\t%s\n", p.ID(), st.Installed, st.Version, st.BinPath, notes)
	}
	return nil
}

// ---------------- per-entity simple commands ----------------

func makeListSub(short string, run func(cmd *cobra.Command, args []string) error) *cobra.Command {
	return &cobra.Command{Use: "list", Short: short, RunE: run}
}

// PR #77 review #2: pass the per-entity filter explicitly instead of
// mutating the package-level agentsListType global. This makes
// repeated invocations / tests / future REPL embedding safe.

func agentsListMarketplaces(cmd *cobra.Command, _ []string) error {
	return runAgentsList(cmd, nil, agents.EntityMarketplace)
}
func agentsListPlugins(cmd *cobra.Command, _ []string) error {
	return runAgentsList(cmd, nil, agents.EntityPlugin)
}
func agentsListSkills(cmd *cobra.Command, _ []string) error {
	return runAgentsList(cmd, nil, agents.EntitySkill)
}
func agentsListMCPs(cmd *cobra.Command, _ []string) error {
	return runAgentsList(cmd, nil, agents.EntityMCP)
}

// (agentsListSessions removed — the dashboard's runAgentsSessionsList
// in agents_sessions.go fully replaces it, with filter / sort / group
// flags and a TUI fallthrough.)

func agentsAddMarketplace(cmd *cobra.Command, args []string) error {
	return forEachProvider(cmd.Context(), func(p agents.Provider) error {
		return p.AddMarketplace(cmd.Context(), args[0])
	})
}
func agentsRemoveMarketplace(cmd *cobra.Command, args []string) error {
	return forEachProvider(cmd.Context(), func(p agents.Provider) error {
		return p.RemoveMarketplace(cmd.Context(), args[0])
	})
}
func agentsInstallPlugin(cmd *cobra.Command, args []string) error {
	ref := parsePluginRef(args[0])
	return forEachProvider(cmd.Context(), func(p agents.Provider) error {
		return p.InstallPlugin(cmd.Context(), ref)
	})
}
func agentsUninstallPlugin(cmd *cobra.Command, args []string) error {
	return forEachProvider(cmd.Context(), func(p agents.Provider) error {
		return p.UninstallPlugin(cmd.Context(), args[0])
	})
}
func agentsRemoveMCP(cmd *cobra.Command, args []string) error {
	return forEachProvider(cmd.Context(), func(p agents.Provider) error {
		return p.RemoveMCP(cmd.Context(), args[0])
	})
}
func agentsResumeSession(cmd *cobra.Command, args []string) error {
	last, _ := cmd.Flags().GetBool("last")
	if last && len(args) > 0 {
		return usageErrorf("resume: --last cannot be combined with an explicit session id")
	}
	if !last && len(args) == 0 {
		return usageErrorf("resume: pass a session id, a fuzzy match string, or --last")
	}

	svc := newAgentsService()

	var id string
	if !last {
		id = args[0]
	}

	// When the user asked for --last, OR the supplied arg is not
	// already a provider-prefixed id, load the snapshot and resolve.
	if last || (!strings.HasPrefix(id, "claude:") && !strings.HasPrefix(id, "copilot:")) {
		ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
		defer cancel()
		snap, err := svc.LoadAll(ctx, agents.LoadOpts{})
		if err != nil {
			return err
		}
		surfaceSnapshotWarnings(snap)
		switch {
		case last:
			if len(snap.Sessions) == 0 {
				return usageErrorf("resume: no sessions available")
			}
			id = snap.Sessions[0].ID
		default:
			match, ok := findSession(snap.Sessions, id)
			if !ok {
				return usageErrorf("resume: cannot resolve %q to a unique session — pass the full provider-prefixed id (claude:… or copilot:…) or use --last", id)
			}
			id = match.ID
		}
	}

	provID := providerForSessionID(id)
	if provID == "" {
		return usageErrorf("resume: cannot infer provider from session id %q (expected claude:… or copilot:…)", id)
	}
	plan, err := svc.Launch(agents.LaunchSpec{Provider: provID, SessionID: id})
	if err != nil {
		return err
	}
	fmt.Fprintln(os.Stderr, "agents: $ "+plan.CommandLine())
	return execPlanCLI(plan)
}
func agentsDeleteSession(cmd *cobra.Command, args []string) error {
	id := args[0]
	provID := providerForSessionID(id)
	if provID == "" {
		return usageErrorf("delete: cannot infer provider from session id %q", id)
	}
	svc := newAgentsService()
	p := svc.ProviderFor(provID)
	if p == nil {
		return fmt.Errorf("delete: provider %q not registered", provID)
	}
	return p.DeleteSession(cmd.Context(), id)
}

func providerForSessionID(id string) agents.ProviderID {
	switch {
	case strings.HasPrefix(id, "claude:"):
		return agents.ProviderClaudeCode
	case strings.HasPrefix(id, "copilot:"):
		return agents.ProviderCopilotCLI
	}
	return ""
}

func parsePluginRef(arg string) agents.PluginRef {
	if i := strings.IndexByte(arg, '@'); i > 0 {
		return agents.PluginRef{Name: arg[:i], Marketplace: arg[i+1:]}
	}
	if strings.Contains(arg, "/") || strings.HasPrefix(arg, ".") || strings.HasPrefix(arg, "http") {
		return agents.PluginRef{Source: arg}
	}
	return agents.PluginRef{Name: arg}
}

// forEachProvider walks providers (filtered by --provider when set)
// and stops at the first non-NotSupported success. The PR #77 review
// (#4) called this out as ambiguous: previously this walked every
// registered provider in order, so `klim agents plugins install foo`
// would hit whichever provider happened to be registered first.
// We now honor the persistent --provider flag: when set, only that
// provider is considered; otherwise we fall back to the historical
// walk-and-pick behaviour so non-targeted ops still work.
func forEachProvider(ctx context.Context, fn func(agents.Provider) error) error {
	svc := newAgentsService()
	providers := svc.Registry().Providers()
	if len(providers) == 0 {
		return errors.New("no agent providers registered")
	}
	if want := strings.TrimSpace(agentsListProvider); want != "" {
		p := svc.ProviderFor(agents.ProviderID(want))
		if p == nil {
			return fmt.Errorf("--provider %q is not registered (known: claude-code, copilot-cli)", want)
		}
		providers = []agents.Provider{p}
	}
	var lastErr error
	for _, p := range providers {
		err := fn(p)
		if err == nil {
			return nil
		}
		if errors.Is(err, agents.ErrNotSupported) || errors.Is(err, agents.ErrProviderNotInstalled) {
			continue
		}
		lastErr = err
	}
	if lastErr == nil {
		if want := strings.TrimSpace(agentsListProvider); want != "" {
			return fmt.Errorf("provider %q does not support this operation", want)
		}
		return errors.New("no provider could handle the request (try `--provider claude-code` or `--provider copilot-cli`)")
	}
	return lastErr
}

// init quietly references syscall to avoid an "imported and not used"
// when the platform-specific file omits it.
var _ = syscall.Stdin
