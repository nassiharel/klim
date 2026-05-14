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
	"github.com/nassiharel/klim/internal/agents/providers/claudecode"
	"github.com/nassiharel/klim/internal/agents/providers/copilotcli"
	"github.com/nassiharel/klim/internal/agents/providers/mcpregistry"
)

// newAgentsService builds the AgentService used by every `klim agents`
// subcommand. Kept as a function so future tests can swap in fakes.
var newAgentsService = func() *agents.Service {
	return agents.NewService(4,
		claudecode.New(),
		copilotcli.New(),
		mcpregistry.New(),
	)
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
	RunE: runAgentsList,
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
	RunE:  runAgentsList,
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
var agentsSessionsCmd = &cobra.Command{Use: "sessions", Aliases: []string{"session"}, Short: "List, resume, and delete agent sessions"}

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
		_, err := svc.LoadAll(ctx, agents.LoadOpts{Refresh: true})
		if err == nil {
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
	agentsListCmd.Flags().StringVar(&agentsListProvider, "provider", "", "filter by provider id: claude-code|copilot-cli|mcp-registry")
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

	agentsSessionsCmd.AddCommand(makeListSub("list sessions", agentsListSessions))
	agentsSessionsCmd.AddCommand(&cobra.Command{Use: "resume <id>", Short: "Resume a session (exec the agent CLI)", Args: cobra.ExactArgs(1), RunE: agentsResumeSession})
	agentsSessionsCmd.AddCommand(&cobra.Command{Use: "delete <id>", Short: "Delete a session", Args: cobra.ExactArgs(1), RunE: agentsDeleteSession})

	// Persistent --provider on agents so subcommands inherit it.
	agentsCmd.PersistentFlags().StringVar(&agentsListProvider, "provider-filter", "", "limit subcommands to one provider (advanced)")

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

func runAgentsList(cmd *cobra.Command, _ []string) error {
	ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
	defer cancel()

	svc := newAgentsService()
	snap, err := svc.LoadAll(ctx, agents.LoadOpts{Refresh: agentsListRefresh})
	if err != nil {
		return fmt.Errorf("agents list: %w", err)
	}

	if agentsListSearch != "" {
		return printSearchResults(svc.Search(agentsListSearch, agents.EntityType(agentsListType)))
	}

	format, err := agentsListFormatGetter()
	if err != nil {
		return err
	}

	switch format {
	case OutputJSON:
		return printJSON(filteredSnapshot(snap))
	case OutputYAML:
		return printYAML(filteredSnapshot(snap))
	}

	return renderSnapshotText(filteredSnapshot(snap))
}

func filteredSnapshot(snap *agents.Snapshot) *agents.Snapshot {
	provider := agents.ProviderID(strings.TrimSpace(agentsListProvider))
	entityFilter := agents.EntityType(strings.TrimSpace(agentsListType))

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
	case agents.ProviderMCPRegistry:
		return "mcp-reg"
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
	if _, err := svc.LoadAll(ctx, agents.LoadOpts{}); err != nil {
		return err
	}
	results := svc.Search(query, "")
	format, err := agentsSearchFormatGetter()
	if err != nil {
		return err
	}
	switch format {
	case OutputJSON:
		return printJSON(results)
	case OutputYAML:
		return printYAML(results)
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
	if agentsLaunchProvider == "" {
		return &UsageError{Err: errors.New("--provider is required (claude-code|copilot-cli)")}
	}
	svc := newAgentsService()
	spec := agents.LaunchSpec{
		Provider:   agents.ProviderID(agentsLaunchProvider),
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
	fmt.Fprintln(os.Stderr, "agents: ", plan.Note)
	fmt.Fprintln(os.Stderr, "agents:  $ "+plan.CommandLine())
	return execPlanCLI(plan)
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
	if _, err := svc.LoadAll(ctx, agents.LoadOpts{}); err != nil {
		return err
	}
	snap := svc.Snapshot()
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	defer func() { _ = w.Flush() }()
	_, _ = fmt.Fprintln(w, "PROVIDER\tINSTALLED\tVERSION\tBIN\tNOTES")
	for _, p := range svc.Registry().Providers() {
		st := snap.ProviderStatus[p.ID()]
		notes := ""
		if st.Error != nil {
			notes = st.Error.Error()
		}
		_, _ = fmt.Fprintf(w, "%s\t%v\t%s\t%s\t%s\n", p.ID(), st.Installed, st.Version, st.BinPath, notes)
	}
	return nil
}

// ---------------- per-entity simple commands ----------------

func makeListSub(short string, run func(cmd *cobra.Command, args []string) error) *cobra.Command {
	return &cobra.Command{Use: "list", Short: short, RunE: run}
}

func agentsListMarketplaces(cmd *cobra.Command, _ []string) error {
	agentsListType = string(agents.EntityMarketplace)
	return runAgentsList(cmd, nil)
}
func agentsListPlugins(cmd *cobra.Command, _ []string) error {
	agentsListType = string(agents.EntityPlugin)
	return runAgentsList(cmd, nil)
}
func agentsListSkills(cmd *cobra.Command, _ []string) error {
	agentsListType = string(agents.EntitySkill)
	return runAgentsList(cmd, nil)
}
func agentsListMCPs(cmd *cobra.Command, _ []string) error {
	agentsListType = string(agents.EntityMCP)
	return runAgentsList(cmd, nil)
}
func agentsListSessions(cmd *cobra.Command, _ []string) error {
	agentsListType = string(agents.EntitySession)
	return runAgentsList(cmd, nil)
}

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
	id := args[0]
	provID := providerForSessionID(id)
	if provID == "" {
		return fmt.Errorf("resume: cannot infer provider from session id %q (expected claude:… or copilot:…)", id)
	}
	svc := newAgentsService()
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
		return fmt.Errorf("delete: cannot infer provider from session id %q", id)
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

// forEachProvider walks all registered providers and stops at the first
// non-NotSupported success. Used by mutations where the provider isn't
// disambiguated by the entity id.
func forEachProvider(ctx context.Context, fn func(agents.Provider) error) error {
	svc := newAgentsService()
	providers := svc.Registry().Providers()
	if len(providers) == 0 {
		return errors.New("no agent providers registered")
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
		return errors.New("no provider could handle the request")
	}
	return lastErr
}

// init quietly references syscall to avoid an "imported and not used"
// when the platform-specific file omits it.
var _ = syscall.Stdin
