package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/nassiharel/klim/internal/agents"
	"github.com/nassiharel/klim/internal/agents/bookmarks"
	"github.com/nassiharel/klim/internal/agents/enrich"
)

// ----- sessions list -----

// agentsSessionsListFlags collects every flag accepted by the
// enriched `klim agents sessions list` command. Kept in one struct
// so the runner doesn't read from a wall of package-level globals
// (the rest of this file follows the same pattern: tightly-scoped
// state, dispatched explicitly into each runner).
var agentsSessionsListFlags struct {
	status  string
	since   string
	until   string
	project string
	starred bool
	sort    string
	reverse bool
	limit   int
	groupBy string
	noGroup bool
	noColor bool
	refresh bool
	watch   bool
}

var agentsSessionsListFormat func() (OutputFormat, error)

// newAgentsSessionsListCmd builds the `sessions list` cobra command.
// Factored into a constructor so it can be wired into both the
// `agents sessions` umbrella and (later) the bare `agents sessions`
// runner.
func newAgentsSessionsListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List agent sessions with live state and filters",
		Long: `list enumerates every agent session known to klim, enriched
with dashboard-friendly fields: live state (working/thinking/waiting/idle),
recent activity, branch, tool counts, and a copy-paste resume command.

Output is grouped by project (with a section header per group); use
--group-by=none to flatten or --group-by=provider to group by provider.

Examples:
  klim agents sessions list
  klim agents sessions list --status waiting
  klim agents sessions list --since 2h --sort turns
  klim agents sessions list --project klim --starred
  klim agents sessions list --output json
  klim agents sessions list --group-by provider`,
		RunE: runAgentsSessionsList,
	}
	cmd.Flags().StringVar(&agentsSessionsListFlags.status, "status", "", "filter by live state: working|thinking|waiting|idle|active|completed|stopped")
	cmd.Flags().StringVar(&agentsSessionsListFlags.since, "since", "", "only show sessions modified after this (e.g. 2h, 7d, 2026-06-01)")
	cmd.Flags().StringVar(&agentsSessionsListFlags.until, "until", "", "only show sessions modified before this")
	cmd.Flags().StringVar(&agentsSessionsListFlags.project, "project", "", "substring match against project path or repo name")
	cmd.Flags().BoolVar(&agentsSessionsListFlags.starred, "starred", false, "only show starred sessions")
	cmd.Flags().StringVar(&agentsSessionsListFlags.sort, "sort", "modified", "sort key: modified|created|turns|state|title")
	cmd.Flags().BoolVar(&agentsSessionsListFlags.reverse, "reverse", false, "reverse the sort order (oldest first / lowest first)")
	cmd.Flags().IntVar(&agentsSessionsListFlags.limit, "limit", 0, "cap output at N rows (0 = all)")
	cmd.Flags().StringVar(&agentsSessionsListFlags.groupBy, "group-by", "project", "group rows by: project|provider|none")
	cmd.Flags().BoolVar(&agentsSessionsListFlags.noColor, "no-color", false, "disable ANSI colours in text output")
	cmd.Flags().BoolVar(&agentsSessionsListFlags.refresh, "refresh", false, "ignore the cache and rescan providers")
	cmd.Flags().BoolVar(&agentsSessionsListFlags.watch, "watch", false, "open the interactive TUI dashboard instead of a one-shot list")
	agentsSessionsListFormat = addOutputFlag(cmd, OutputText, OutputJSON, OutputYAML)
	return cmd
}

// runAgentsSessionsDefault handles the bare `klim agents sessions` form.
// When stdout is a TTY we launch the interactive dashboard; otherwise
// we fall through to the one-shot list so pipes / redirects continue
// working unchanged.
func runAgentsSessionsDefault(cmd *cobra.Command, args []string) error {
	if isStdoutTTY() {
		return launchSessionsTUI(cmd.Context())
	}
	return runAgentsSessionsList(cmd, args)
}

// runAgentsSessionsList implements `klim agents sessions list`.
//
// The runner is intentionally linear: load → filter → sort → render.
// Each stage operates on the slice it receives so the test harness
// can exercise the pure stages without spinning up the cobra layer.
func runAgentsSessionsList(cmd *cobra.Command, _ []string) error {
	if agentsSessionsListFlags.watch {
		return launchSessionsTUI(cmd.Context())
	}
	ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
	defer cancel()

	svc := newAgentsService()
	snap, err := svc.LoadAll(ctx, agents.LoadOpts{Refresh: agentsSessionsListFlags.refresh})
	if err != nil {
		return fmt.Errorf("agents sessions list: %w", err)
	}

	sessions := filterSessions(snap.Sessions, agentsSessionsListFlags.status,
		agentsSessionsListFlags.project, agentsSessionsListFlags.starred,
		agentsSessionsListFlags.since, agentsSessionsListFlags.until,
		string(agentsListProvider), time.Now())
	if errIsTimeParse := lastSessionsFilterErr; errIsTimeParse != nil {
		return &UsageError{Err: errIsTimeParse}
	}
	sortSessions(sessions, agentsSessionsListFlags.sort, agentsSessionsListFlags.reverse)
	if n := agentsSessionsListFlags.limit; n > 0 && len(sessions) > n {
		sessions = sessions[:n]
	}

	format, err := agentsSessionsListFormat()
	if err != nil {
		return err
	}
	switch format {
	case OutputJSON:
		return printJSON(sessions)
	case OutputYAML:
		return printYAMLDirect(sessions)
	}

	noColor := agentsSessionsListFlags.noColor || os.Getenv("NO_COLOR") != ""
	return renderSessionsText(sessions, agentsSessionsListFlags.groupBy, noColor)
}

// lastSessionsFilterErr captures a parse error from filterSessions so
// callers can surface it as a UsageError. Package-level (rather than
// returned) to keep the filter function's signature small.
var lastSessionsFilterErr error

// filterSessions applies the user's `--status`, `--since`, `--until`,
// `--project`, and `--starred` filters in one pass. The function is
// pure — `now` is injected so tests don't depend on the wall clock.
//
// When `--since` or `--until` is malformed, the function returns the
// unfiltered slice and stores the error in lastSessionsFilterErr so
// the caller can convert it to a UsageError (exit 2 per CLI
// conventions).
func filterSessions(in []agents.Session, status, project string, starred bool, sinceStr, untilStr, provider string, now time.Time) []agents.Session {
	lastSessionsFilterErr = nil
	var since, until time.Time
	var err error
	if sinceStr != "" {
		since, err = enrich.ParseSince(sinceStr, now)
		if err != nil {
			lastSessionsFilterErr = fmt.Errorf("--since: %w", err)
			return in
		}
	}
	if untilStr != "" {
		until, err = enrich.ParseSince(untilStr, now)
		if err != nil {
			lastSessionsFilterErr = fmt.Errorf("--until: %w", err)
			return in
		}
	}
	wantStatus := strings.ToLower(strings.TrimSpace(status))
	wantProject := strings.ToLower(strings.TrimSpace(project))
	wantProvider := strings.TrimSpace(provider)

	out := make([]agents.Session, 0, len(in))
	for _, s := range in {
		if wantProvider != "" && string(s.Provider) != wantProvider {
			continue
		}
		if starred && !s.Starred {
			continue
		}
		if wantStatus != "" && !matchesStatus(s, wantStatus) {
			continue
		}
		if wantProject != "" && !matchesProject(s, wantProject) {
			continue
		}
		if !since.IsZero() && s.LastModified.Before(since) {
			continue
		}
		if !until.IsZero() && s.LastModified.After(until) {
			continue
		}
		out = append(out, s)
	}
	return out
}

func matchesStatus(s agents.Session, want string) bool {
	// Accept both LiveState and persisted Status values.
	switch want {
	case "working", "thinking", "waiting", "idle":
		return string(s.LiveState) == want
	case "active", "completed", "stopped":
		return string(s.Status) == want
	case "starred":
		return s.Starred
	}
	return string(s.LiveState) == want || string(s.Status) == want
}

func matchesProject(s agents.Session, needle string) bool {
	if needle == "" {
		return true
	}
	if strings.Contains(strings.ToLower(s.ProjectPath), needle) {
		return true
	}
	if strings.Contains(strings.ToLower(s.Repository), needle) {
		return true
	}
	if strings.Contains(strings.ToLower(s.Group), needle) {
		return true
	}
	return false
}

// sortSessions sorts in place. Unknown sort keys fall back to
// "modified" so the user always gets some ordering.
func sortSessions(s []agents.Session, key string, reverse bool) {
	less := func(i, j int) bool { return s[i].LastModified.After(s[j].LastModified) }
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "created":
		less = func(i, j int) bool { return s[i].Created.After(s[j].Created) }
	case "turns":
		less = func(i, j int) bool { return s[i].TurnCount > s[j].TurnCount }
	case "state":
		less = func(i, j int) bool { return stateOrder(s[i].LiveState) < stateOrder(s[j].LiveState) }
	case "title":
		less = func(i, j int) bool { return s[i].Title < s[j].Title }
	}
	sort.SliceStable(s, less)
	if reverse {
		for i, j := 0, len(s)-1; i < j; i, j = i+1, j-1 {
			s[i], s[j] = s[j], s[i]
		}
	}
}

// stateOrder ranks live states so `--sort state` puts the most-active
// sessions at the top (working > thinking > waiting > idle > unknown).
func stateOrder(st agents.LiveState) int {
	switch st {
	case agents.StateWorking:
		return 0
	case agents.StateThinking:
		return 1
	case agents.StateWaiting:
		return 2
	case agents.StateIdle:
		return 3
	}
	return 4
}

// renderSessionsText writes the grouped session table to stdout and
// a summary line to stderr.
func renderSessionsText(sessions []agents.Session, groupBy string, noColor bool) error {
	if len(sessions) == 0 {
		fmt.Fprintln(os.Stderr, "agents: no sessions match the filters")
		return nil
	}

	fmt.Fprintf(os.Stderr, "agents: %d session(s) shown\n", len(sessions))

	// Tabwriter handles column alignment; group headers are written
	// outside any tab run so they don't get realigned with the rows.
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	defer func() { _ = w.Flush() }()

	now := time.Now()

	render := func(group string, items []agents.Session) {
		if group != "" {
			_ = w.Flush()
			_, _ = fmt.Fprintf(os.Stdout, "\n%s (%d)\n", group, len(items))
		}
		_, _ = fmt.Fprintln(w, "STATE\tSOURCE\tID\tBRANCH\tTITLE\tTURNS\tMODIFIED\tRECENT")
		for _, s := range items {
			star := " "
			if s.Starred {
				star = "★"
			}
			id := truncateAgent(s.ID, 28)
			title := truncateAgent(dashOr(displaySessionTitle(s)), 50)
			branch := truncateAgent(dashOr(s.Branch), 18)
			turns := intOrDash(s.TurnCount)
			modified := enrich.RelativeTime(s.LastModified, now)
			recent := truncateAgent(dashOr(s.RecentActivity), 60)
			_, _ = fmt.Fprintf(w, "%s%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
				stateGlyph(s.LiveState, noColor), star,
				providerShortCLI(s.Provider), id, branch, title, turns, modified, recent)
		}
	}

	switch strings.ToLower(strings.TrimSpace(groupBy)) {
	case "none":
		render("", sessions)
	case "provider":
		for _, g := range groupSessions(sessions, func(s agents.Session) string {
			return providerShortCLI(s.Provider)
		}) {
			render(g.name, g.items)
		}
	default: // project
		for _, g := range groupSessions(sessions, func(s agents.Session) string {
			if s.Group != "" {
				return s.Group
			}
			return "Other"
		}) {
			render(g.name, g.items)
		}
	}
	return nil
}

type sessionGroup struct {
	name  string
	items []agents.Session
}

// groupSessions buckets sessions by the result of keyFn and returns
// the groups sorted by name. Stable ordering within each group is
// preserved from the input slice (so the caller's prior sort wins
// inside each bucket).
func groupSessions(in []agents.Session, keyFn func(agents.Session) string) []sessionGroup {
	idx := map[string]int{}
	var groups []sessionGroup
	for _, s := range in {
		k := keyFn(s)
		if i, ok := idx[k]; ok {
			groups[i].items = append(groups[i].items, s)
			continue
		}
		idx[k] = len(groups)
		groups = append(groups, sessionGroup{name: k, items: []agents.Session{s}})
	}
	sort.SliceStable(groups, func(i, j int) bool { return groups[i].name < groups[j].name })
	return groups
}

// stateGlyph returns a 1-char status indicator. ANSI colour is added
// when colour output is enabled.
func stateGlyph(st agents.LiveState, noColor bool) string {
	glyph := "·"
	switch st {
	case agents.StateWorking:
		glyph = "●"
	case agents.StateThinking:
		glyph = "◐"
	case agents.StateWaiting:
		glyph = "▲"
	case agents.StateIdle:
		glyph = "○"
	}
	if noColor {
		return glyph
	}
	switch st {
	case agents.StateWorking:
		return "\x1b[32m" + glyph + "\x1b[0m" // green
	case agents.StateThinking:
		return "\x1b[36m" + glyph + "\x1b[0m" // cyan
	case agents.StateWaiting:
		return "\x1b[33m" + glyph + "\x1b[0m" // yellow
	case agents.StateIdle:
		return "\x1b[90m" + glyph + "\x1b[0m" // bright black
	}
	return glyph
}

// displaySessionTitle picks the most useful single string for the
// title column: explicit Title, then repository, then project path
// basename.
func displaySessionTitle(s agents.Session) string {
	if s.Title != "" {
		return s.Title
	}
	if s.Repository != "" {
		return s.Repository
	}
	if s.ProjectPath != "" {
		return filepath.Base(filepath.Clean(s.ProjectPath))
	}
	return ""
}

// ----- sessions view -----

var agentsSessionsViewFormat func() (OutputFormat, error)
var agentsSessionsViewTurns int

func newAgentsSessionsViewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "view <id>",
		Short: "Show full details for one session",
		Long: `view loads a single session and prints its enriched detail block:
project, branch, live state, tool histogram, MCP servers, last N
conversation turns from the transcript.

Examples:
  klim agents sessions view claude:3b4dc369-3956-43b0-a52b-cd066984d618
  klim agents sessions view --turns 25 copilot:abcd
  klim agents sessions view --output json claude:3b4dc369-…`,
		Args: cobra.ExactArgs(1),
		RunE: runAgentsSessionsView,
	}
	cmd.Flags().IntVar(&agentsSessionsViewTurns, "turns", 10, "number of recent conversation turns to print")
	agentsSessionsViewFormat = addOutputFlag(cmd, OutputText, OutputJSON, OutputYAML)
	return cmd
}

func runAgentsSessionsView(cmd *cobra.Command, args []string) error {
	id := args[0]
	ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
	defer cancel()
	svc := newAgentsService()
	snap, err := svc.LoadAll(ctx, agents.LoadOpts{})
	if err != nil {
		return err
	}
	session, ok := findSession(snap.Sessions, id)
	if !ok {
		return &UsageError{Err: fmt.Errorf("no session found matching %q", id)}
	}

	format, err := agentsSessionsViewFormat()
	if err != nil {
		return err
	}
	switch format {
	case OutputJSON:
		return printJSON(session)
	case OutputYAML:
		return printYAMLDirect(session)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	defer func() { _ = w.Flush() }()
	now := time.Now()
	fmt.Fprintf(os.Stderr, "agents: session %s\n", session.ID)
	_, _ = fmt.Fprintf(w, "id\t%s\n", session.ID)
	_, _ = fmt.Fprintf(w, "provider\t%s\n", providerShortCLI(session.Provider))
	if session.Title != "" {
		_, _ = fmt.Fprintf(w, "title\t%s\n", session.Title)
	}
	if session.ProjectPath != "" {
		_, _ = fmt.Fprintf(w, "project\t%s\n", session.ProjectPath)
	}
	if session.Repository != "" {
		_, _ = fmt.Fprintf(w, "repository\t%s\n", session.Repository)
	}
	if session.Branch != "" {
		_, _ = fmt.Fprintf(w, "branch\t%s\n", session.Branch)
	}
	if session.Group != "" {
		_, _ = fmt.Fprintf(w, "group\t%s\n", session.Group)
	}
	_, _ = fmt.Fprintf(w, "state\t%s\n", dashOr(string(session.LiveState)))
	_, _ = fmt.Fprintf(w, "status\t%s\n", dashOr(string(session.Status)))
	if session.WaitingContext != "" {
		_, _ = fmt.Fprintf(w, "waiting\t%s\n", session.WaitingContext)
	}
	_, _ = fmt.Fprintf(w, "turns\t%d\n", session.TurnCount)
	if !session.Created.IsZero() {
		_, _ = fmt.Fprintf(w, "created\t%s (%s)\n", session.Created.Format(time.RFC3339), enrich.RelativeTime(session.Created, now))
	}
	if !session.LastModified.IsZero() {
		_, _ = fmt.Fprintf(w, "modified\t%s (%s)\n", session.LastModified.Format(time.RFC3339), enrich.RelativeTime(session.LastModified, now))
	}
	if session.RestartCommand != "" {
		_, _ = fmt.Fprintf(w, "resume\t%s\n", session.RestartCommand)
	}
	if session.RecentActivity != "" {
		_, _ = fmt.Fprintf(w, "recent\t%s\n", session.RecentActivity)
	}
	if len(session.MCPServers) > 0 {
		_, _ = fmt.Fprintf(w, "mcps\t%s\n", strings.Join(session.MCPServers, ", "))
	}
	if session.SubagentRuns > 0 || session.BackgroundTasks > 0 {
		_, _ = fmt.Fprintf(w, "subagents\t%d (running: %d)\n", session.SubagentRuns, session.BackgroundTasks)
	}
	_ = w.Flush()

	// Tool histogram.
	if len(session.ToolCounts) > 0 {
		_, _ = fmt.Fprintln(os.Stdout)
		_, _ = fmt.Fprintln(os.Stdout, "TOOLS")
		printToolHistogram(os.Stdout, session.ToolCounts, 40)
	}

	// Recent turns from the transcript (best-effort).
	if turns := readRecentTurns(session.TranscriptPath, agentsSessionsViewTurns); len(turns) > 0 {
		_, _ = fmt.Fprintln(os.Stdout)
		_, _ = fmt.Fprintln(os.Stdout, "CONVERSATION")
		for _, t := range turns {
			_, _ = fmt.Fprintf(os.Stdout, "  [%s] %s\n", t.role, enrich.TruncateOneLine(t.text, 240))
		}
	}
	return nil
}

// printToolHistogram writes a simple horizontal bar chart of tool
// usage counts. width is the max bar length in chars.
func printToolHistogram(w io.Writer, counts map[string]int, width int) {
	type kv struct {
		name  string
		count int
	}
	var pairs []kv
	peak := 0
	for k, v := range counts {
		pairs = append(pairs, kv{k, v})
		if v > peak {
			peak = v
		}
	}
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].count > pairs[j].count })
	if peak == 0 {
		return
	}
	for _, p := range pairs {
		bar := strings.Repeat("█", (p.count*width+peak-1)/peak)
		_, _ = fmt.Fprintf(w, "  %-18s %4d %s\n", truncateAgent(p.name, 18), p.count, bar)
	}
}

// turnLine is the minimal representation of a transcript turn for
// `view`'s CONVERSATION section.
type turnLine struct {
	role string
	text string
}

// readRecentTurns parses the transcript at `path` and returns the
// last `n` user / assistant text turns. The function is best-effort
// and handles both Claude (per-line JSONL with message.content[].text)
// and Copilot (line JSON with data.message.text) schemas.
func readRecentTurns(path string, n int) []turnLine {
	if path == "" || n <= 0 {
		return nil
	}
	fi, err := os.Stat(path)
	if err != nil {
		return nil
	}
	if fi.IsDir() {
		// Copilot session: pick events.jsonl inside.
		path = filepath.Join(path, "events.jsonl")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer func() { _ = f.Close() }()
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	var out []turnLine
	for scanner.Scan() {
		line := scanner.Bytes()
		// Try Claude-shape first.
		role, text, ok := claudeTurnLine(line)
		if !ok {
			role, text, ok = copilotTurnLine(line)
		}
		if !ok || text == "" {
			continue
		}
		out = append(out, turnLine{role: role, text: text})
		if len(out) > n*2 {
			// Keep the slice bounded; tail-truncate below.
			out = out[len(out)-n*2:]
		}
	}
	if len(out) > n {
		out = out[len(out)-n:]
	}
	return out
}

// claudeTurnLine extracts a (role, text) pair from a Claude transcript
// line. Returns ok=false when the line isn't a user/assistant message.
func claudeTurnLine(line []byte) (role, text string, ok bool) {
	// Minimal partial decode to avoid pulling in the full Claude
	// event schema here.
	type content struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	type msg struct {
		Type    string `json:"type"`
		Message struct {
			Role    string    `json:"role"`
			Content []content `json:"content"`
		} `json:"message"`
	}
	var m msg
	if err := jsonUnmarshal(line, &m); err != nil {
		return "", "", false
	}
	if m.Type != "user" && m.Type != "assistant" {
		return "", "", false
	}
	for _, c := range m.Message.Content {
		if c.Type == "text" && c.Text != "" {
			return m.Type, c.Text, true
		}
	}
	return "", "", false
}

// copilotTurnLine extracts a (role, text) pair from a Copilot CLI
// transcript line.
func copilotTurnLine(line []byte) (role, text string, ok bool) {
	type msg struct {
		Type string `json:"type"`
		Data struct {
			Message struct {
				Role    string `json:"role"`
				Text    string `json:"text"`
				Content string `json:"content"`
			} `json:"message"`
		} `json:"data"`
	}
	var m msg
	if err := jsonUnmarshal(line, &m); err != nil {
		return "", "", false
	}
	switch m.Type {
	case "user.message":
		t := m.Data.Message.Text
		if t == "" {
			t = m.Data.Message.Content
		}
		return "user", t, t != ""
	case "assistant.message":
		t := m.Data.Message.Text
		if t == "" {
			t = m.Data.Message.Content
		}
		return "assistant", t, t != ""
	}
	return "", "", false
}

// jsonUnmarshal is a tiny helper so the json import stays in one place
// even as more readers are added. Behaviour identical to json.Unmarshal.
func jsonUnmarshal(data []byte, v any) error {
	return jsonUnmarshalFn(data, v)
}

// ----- sessions tail -----

var agentsSessionsTailInterval time.Duration

func newAgentsSessionsTailCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tail <id>",
		Short: "Stream the live state of a session until Ctrl-C",
		Long: `tail re-reads the session at the given interval and prints
a single line per change: state transitions, new tool calls, and the
most recent activity. Useful for keeping an eye on a long-running
agent from another terminal.

Examples:
  klim agents sessions tail claude:3b4dc369-…
  klim agents sessions tail --interval 5s copilot:abcd`,
		Args: cobra.ExactArgs(1),
		RunE: runAgentsSessionsTail,
	}
	cmd.Flags().DurationVar(&agentsSessionsTailInterval, "interval", 2*time.Second, "poll interval (e.g. 2s, 500ms)")
	return cmd
}

func runAgentsSessionsTail(cmd *cobra.Command, args []string) error {
	id := args[0]
	interval := agentsSessionsTailInterval
	if interval < 250*time.Millisecond {
		return &UsageError{Err: errors.New("--interval must be >= 250ms")}
	}
	svc := newAgentsService()
	var prevState agents.LiveState
	var prevRecent string

	tick := time.NewTicker(interval)
	defer tick.Stop()

	// First snapshot immediately so the user sees something right away.
	ctx := cmd.Context()
	for {
		snap, err := svc.LoadAll(ctx, agents.LoadOpts{Refresh: true})
		if err != nil {
			return err
		}
		s, ok := findSession(snap.Sessions, id)
		if !ok {
			return &UsageError{Err: fmt.Errorf("no session found matching %q", id)}
		}
		if s.LiveState != prevState || s.RecentActivity != prevRecent {
			_, _ = fmt.Fprintf(os.Stdout, "[%s] %s %s — %s\n",
				time.Now().Format("15:04:05"),
				dashOr(string(s.LiveState)),
				s.ID,
				dashOr(s.RecentActivity))
			prevState = s.LiveState
			prevRecent = s.RecentActivity
		}
		select {
		case <-ctx.Done():
			return nil
		case <-tick.C:
		}
	}
}

// ----- sessions stats -----

var agentsSessionsStatsFormat func() (OutputFormat, error)
var agentsSessionsStatsSince string

// SessionStats is the data model returned by `agents sessions stats`.
// The fields are JSON-tagged so the structured output stays
// deterministic across releases.
type SessionStats struct {
	Total           int            `json:"total"`
	ByLiveState     map[string]int `json:"by_live_state"`
	ByProvider      map[string]int `json:"by_provider"`
	ByStatus        map[string]int `json:"by_status"`
	TotalTurns      int            `json:"total_turns"`
	TotalToolCalls  int            `json:"total_tool_calls"`
	TotalSubagents  int            `json:"total_subagents"`
	BackgroundTasks int            `json:"background_tasks"`
	TopProjects     []countEntry   `json:"top_projects,omitempty"`
	TopMCPs         []countEntry   `json:"top_mcps,omitempty"`
	TopTools        []countEntry   `json:"top_tools,omitempty"`
}

// countEntry is a (name, count) pair used in the top-N lists.
type countEntry struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

func newAgentsSessionsStatsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "stats",
		Short: "Aggregate counters across all sessions",
		Long: `stats produces summary counters (live state distribution,
total turns, top projects, top MCPs, top tools) over the current
session inventory. Use --since to scope the aggregation to a recent
window.

Examples:
  klim agents sessions stats
  klim agents sessions stats --since 7d
  klim agents sessions stats --output json`,
		RunE: runAgentsSessionsStats,
	}
	cmd.Flags().StringVar(&agentsSessionsStatsSince, "since", "", "only count sessions modified after this (e.g. 24h, 7d)")
	agentsSessionsStatsFormat = addOutputFlag(cmd, OutputText, OutputJSON, OutputYAML)
	return cmd
}

func runAgentsSessionsStats(cmd *cobra.Command, _ []string) error {
	ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
	defer cancel()
	svc := newAgentsService()
	snap, err := svc.LoadAll(ctx, agents.LoadOpts{})
	if err != nil {
		return err
	}

	sessions := filterSessions(snap.Sessions, "", "", false, agentsSessionsStatsSince, "", string(agentsListProvider), time.Now())
	if lastSessionsFilterErr != nil {
		return &UsageError{Err: lastSessionsFilterErr}
	}
	stats := computeStats(sessions)

	format, err := agentsSessionsStatsFormat()
	if err != nil {
		return err
	}
	switch format {
	case OutputJSON:
		return printJSON(stats)
	case OutputYAML:
		return printYAMLDirect(stats)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	defer func() { _ = w.Flush() }()
	fmt.Fprintf(os.Stderr, "agents: %d session(s) aggregated\n", stats.Total)
	_, _ = fmt.Fprintf(w, "total\t%d\n", stats.Total)
	_, _ = fmt.Fprintf(w, "total turns\t%d\n", stats.TotalTurns)
	_, _ = fmt.Fprintf(w, "total tool calls\t%d\n", stats.TotalToolCalls)
	_, _ = fmt.Fprintf(w, "subagent runs\t%d\n", stats.TotalSubagents)
	_, _ = fmt.Fprintf(w, "background tasks\t%d\n", stats.BackgroundTasks)
	_ = w.Flush()
	_, _ = fmt.Fprintln(os.Stdout)
	printStateBlock(w, "BY LIVE STATE", stats.ByLiveState)
	_ = w.Flush()
	_, _ = fmt.Fprintln(os.Stdout)
	printStateBlock(w, "BY PROVIDER", stats.ByProvider)
	_ = w.Flush()
	if len(stats.TopProjects) > 0 {
		_, _ = fmt.Fprintln(os.Stdout)
		printTopBlock(w, "TOP PROJECTS", stats.TopProjects)
	}
	if len(stats.TopMCPs) > 0 {
		_ = w.Flush()
		_, _ = fmt.Fprintln(os.Stdout)
		printTopBlock(w, "TOP MCPS", stats.TopMCPs)
	}
	if len(stats.TopTools) > 0 {
		_ = w.Flush()
		_, _ = fmt.Fprintln(os.Stdout)
		printTopBlock(w, "TOP TOOLS", stats.TopTools)
	}
	return nil
}

func printStateBlock(w *tabwriter.Writer, label string, m map[string]int) {
	_, _ = fmt.Fprintf(w, "%s\t\n", label)
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		_, _ = fmt.Fprintf(w, "  %s\t%d\n", k, m[k])
	}
}

func printTopBlock(w *tabwriter.Writer, label string, items []countEntry) {
	_, _ = fmt.Fprintf(w, "%s\t\n", label)
	for _, it := range items {
		_, _ = fmt.Fprintf(w, "  %s\t%d\n", it.Name, it.Count)
	}
}

// computeStats aggregates session counters. The function is pure and
// trivially testable.
func computeStats(sessions []agents.Session) SessionStats {
	stats := SessionStats{
		ByLiveState: map[string]int{},
		ByProvider:  map[string]int{},
		ByStatus:    map[string]int{},
	}
	projects := map[string]int{}
	mcps := map[string]int{}
	tools := map[string]int{}
	for _, s := range sessions {
		stats.Total++
		stats.TotalTurns += s.TurnCount
		stats.TotalSubagents += s.SubagentRuns
		stats.BackgroundTasks += s.BackgroundTasks
		for _, c := range s.ToolCounts {
			stats.TotalToolCalls += c
		}
		stats.ByLiveState[stateKey(s.LiveState)]++
		stats.ByProvider[string(s.Provider)]++
		stats.ByStatus[statusKey(s.Status)]++
		if k := projectKey(s); k != "" {
			projects[k]++
		}
		for _, m := range s.MCPServers {
			mcps[m]++
		}
		for k, c := range s.ToolCounts {
			tools[k] += c
		}
	}
	stats.TopProjects = topNFromMap(projects, 5)
	stats.TopMCPs = topNFromMap(mcps, 5)
	stats.TopTools = topNFromMap(tools, 10)
	return stats
}

func stateKey(s agents.LiveState) string {
	if s == "" {
		return "unknown"
	}
	return string(s)
}

func statusKey(s agents.SessionStatus) string {
	if s == "" {
		return "unknown"
	}
	return string(s)
}

func projectKey(s agents.Session) string {
	if s.Group != "" {
		return s.Group
	}
	if s.Repository != "" {
		return s.Repository
	}
	if s.ProjectPath != "" {
		return filepath.Base(filepath.Clean(s.ProjectPath))
	}
	return ""
}

// topNFromMap returns the top-n keys by descending count, ties broken
// by lexical key order for determinism.
func topNFromMap(m map[string]int, n int) []countEntry {
	if len(m) == 0 || n <= 0 {
		return nil
	}
	out := make([]countEntry, 0, len(m))
	for k, v := range m {
		out = append(out, countEntry{Name: k, Count: v})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Name < out[j].Name
	})
	if len(out) > n {
		out = out[:n]
	}
	return out
}

// ----- sessions files -----

var agentsSessionsFilesFormat func() (OutputFormat, error)
var agentsSessionsFilesTop int
var agentsSessionsFilesSince string

// FileEntry is the per-file aggregate returned by `agents sessions files`.
type FileEntry struct {
	Path         string   `json:"path"`
	SessionCount int      `json:"session_count"`
	SessionIDs   []string `json:"session_ids,omitempty"`
}

func newAgentsSessionsFilesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "files",
		Short: "Show files edited across sessions",
		Long: `files lists the files most frequently edited across the
inventory (by counting per-file Edit/Write tool calls in each
transcript). Use --since to scope to a recent window and --top to
control the row count.

Examples:
  klim agents sessions files
  klim agents sessions files --top 5
  klim agents sessions files --since 24h --output json`,
		RunE: runAgentsSessionsFiles,
	}
	cmd.Flags().IntVar(&agentsSessionsFilesTop, "top", 20, "cap output at N files (0 = all)")
	cmd.Flags().StringVar(&agentsSessionsFilesSince, "since", "", "only count sessions modified after this (e.g. 24h, 7d)")
	agentsSessionsFilesFormat = addOutputFlag(cmd, OutputText, OutputJSON, OutputYAML)
	return cmd
}

func runAgentsSessionsFiles(cmd *cobra.Command, _ []string) error {
	ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
	defer cancel()
	svc := newAgentsService()
	snap, err := svc.LoadAll(ctx, agents.LoadOpts{})
	if err != nil {
		return err
	}
	sessions := filterSessions(snap.Sessions, "", "", false, agentsSessionsFilesSince, "", string(agentsListProvider), time.Now())
	if lastSessionsFilterErr != nil {
		return &UsageError{Err: lastSessionsFilterErr}
	}

	entries := aggregateFiles(sessions, agentsSessionsFilesTop)
	format, err := agentsSessionsFilesFormat()
	if err != nil {
		return err
	}
	switch format {
	case OutputJSON:
		return printJSON(entries)
	case OutputYAML:
		return printYAMLDirect(entries)
	}

	if len(entries) == 0 {
		fmt.Fprintln(os.Stderr, "agents: no file edits detected")
		return nil
	}
	fmt.Fprintf(os.Stderr, "agents: %d file(s) across %d session(s)\n", len(entries), len(sessions))
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	defer func() { _ = w.Flush() }()
	peak := 0
	for _, e := range entries {
		if e.SessionCount > peak {
			peak = e.SessionCount
		}
	}
	_, _ = fmt.Fprintln(w, "SESSIONS\tBAR\tPATH")
	for _, e := range entries {
		bar := strings.Repeat("█", (e.SessionCount*30+peak-1)/peak)
		_, _ = fmt.Fprintf(w, "%d\t%s\t%s\n", e.SessionCount, bar, truncateAgent(e.Path, 80))
	}
	return nil
}

// aggregateFiles walks every transcript referenced by `sessions`,
// counting unique file paths that appear as a `path` / `file_path`
// argument to Read / Edit / Write / NotebookEdit tool calls. The
// returned slice is sorted descending by SessionCount and capped at
// `top` entries (0 = no cap).
//
// The function is best-effort: transcripts that can't be opened or
// don't contain tool-call data simply contribute nothing.
func aggregateFiles(sessions []agents.Session, top int) []FileEntry {
	type fileAgg struct {
		count int
		ids   map[string]bool
	}
	agg := map[string]*fileAgg{}
	for _, s := range sessions {
		if s.TranscriptPath == "" {
			continue
		}
		paths := extractFilePathsFromTranscript(s.TranscriptPath)
		for p := range paths {
			a, ok := agg[p]
			if !ok {
				a = &fileAgg{ids: map[string]bool{}}
				agg[p] = a
			}
			if !a.ids[s.ID] {
				a.ids[s.ID] = true
				a.count++
			}
		}
	}
	out := make([]FileEntry, 0, len(agg))
	for path, a := range agg {
		ids := make([]string, 0, len(a.ids))
		for id := range a.ids {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		out = append(out, FileEntry{Path: path, SessionCount: a.count, SessionIDs: ids})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].SessionCount != out[j].SessionCount {
			return out[i].SessionCount > out[j].SessionCount
		}
		return out[i].Path < out[j].Path
	})
	if top > 0 && len(out) > top {
		out = out[:top]
	}
	return out
}

// extractFilePathsFromTranscript scans a transcript for tool_use
// invocations whose input carries a `path` / `file_path` /
// `notebook_path` value. Returns the unique set.
func extractFilePathsFromTranscript(path string) map[string]bool {
	fi, err := os.Stat(path)
	if err != nil {
		return nil
	}
	if fi.IsDir() {
		path = filepath.Join(path, "events.jsonl")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer func() { _ = f.Close() }()
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	out := map[string]bool{}
	for scanner.Scan() {
		line := scanner.Bytes()
		// Claude shape: message.content[].input.file_path / path /
		// notebook_path.
		type input struct {
			Path         string `json:"path"`
			FilePath     string `json:"file_path"`
			NotebookPath string `json:"notebook_path"`
		}
		type content struct {
			Type  string `json:"type"`
			Name  string `json:"name"`
			Input input  `json:"input"`
		}
		type msg struct {
			Type    string `json:"type"`
			Message struct {
				Content []content `json:"content"`
			} `json:"message"`
		}
		var m msg
		if err := jsonUnmarshal(line, &m); err == nil && m.Type == "assistant" {
			for _, c := range m.Message.Content {
				if c.Type != "tool_use" {
					continue
				}
				// Only count tools that semantically operate on a
				// file by path — the `files` command exists to
				// answer "what did you edit / read recently?", so
				// counting every tool that happens to carry a
				// path-shaped input (e.g. Bash, Glob) would inflate
				// the picture.
				if !isFileEditTool(c.Name) {
					continue
				}
				for _, p := range []string{c.Input.FilePath, c.Input.Path, c.Input.NotebookPath} {
					if p != "" {
						out[p] = true
					}
				}
			}
		}
	}
	return out
}

// isFileEditTool reports whether `name` is one of the per-file Claude
// tools that the `files` aggregate considers a real read/edit. The
// list is conservative: we only count tools whose primary purpose is
// reading or writing one named file (or notebook cell).
//
// Tools deliberately excluded:
//
//   - Bash / Glob / Grep — accept paths but as search inputs, not
//     edit targets.
//   - WebFetch / WebSearch — no file involved.
//   - Skill / Agent / Task* — orchestration, no direct file work.
func isFileEditTool(name string) bool {
	switch name {
	case "Read", "Edit", "Write", "MultiEdit", "NotebookEdit", "NotebookRead":
		return true
	}
	return false
}

// ----- sessions star / unstar -----

func newAgentsSessionsStarCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "star <id>",
		Short: "Pin a session so it sorts above unstarred in list / TUI",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := args[0]
			st, err := bookmarks.Load()
			if err != nil {
				return err
			}
			st.Add(id, "")
			if err := st.Save(); err != nil {
				return err
			}
			fmt.Fprintln(os.Stderr, "agents: starred", id)
			return nil
		},
	}
}

func newAgentsSessionsUnstarCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "unstar <id>",
		Short: "Remove a session's star",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := args[0]
			st, err := bookmarks.Load()
			if err != nil {
				return err
			}
			if !st.Remove(id) {
				fmt.Fprintf(os.Stderr, "agents: %s was not starred\n", id)
				return nil
			}
			if err := st.Save(); err != nil {
				return err
			}
			fmt.Fprintln(os.Stderr, "agents: unstarred", id)
			return nil
		},
	}
}

// ----- sessions group -----

var agentsSessionsGroupListFormat func() (OutputFormat, error)

func newAgentsSessionsGroupCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "group",
		Short: "Manage smart-group mappings for sessions list / TUI",
	}
	cmd.AddCommand(newAgentsSessionsGroupSetCmd())
	cmd.AddCommand(newAgentsSessionsGroupDeleteCmd())
	cmd.AddCommand(newAgentsSessionsGroupListCmd())
	return cmd
}

func newAgentsSessionsGroupSetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "set <pattern>=<group>",
		Short: "Map a cwd substring to a group label (e.g. klim=Klim)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			pattern, group, ok := splitMappingArg(args[0])
			if !ok {
				return &UsageError{Err: fmt.Errorf("expected pattern=group, got %q", args[0])}
			}
			gm, err := enrich.LoadGroupingMappings()
			if err != nil {
				return err
			}
			gm.Set(pattern, group)
			if err := gm.Save(); err != nil {
				return err
			}
			fmt.Fprintf(os.Stderr, "agents: mapped %q → %q\n", pattern, group)
			return nil
		},
	}
}

func newAgentsSessionsGroupDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <pattern>",
		Short: "Remove a cwd→group mapping",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			gm, err := enrich.LoadGroupingMappings()
			if err != nil {
				return err
			}
			if !gm.Delete(args[0]) {
				fmt.Fprintf(os.Stderr, "agents: no mapping for %q\n", args[0])
				return nil
			}
			if err := gm.Save(); err != nil {
				return err
			}
			fmt.Fprintln(os.Stderr, "agents: removed mapping", args[0])
			return nil
		},
	}
}

func newAgentsSessionsGroupListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "Show all cwd→group mappings",
		RunE: func(cmd *cobra.Command, args []string) error {
			gm, err := enrich.LoadGroupingMappings()
			if err != nil {
				return err
			}
			entries := gm.Entries()
			format, err := agentsSessionsGroupListFormat()
			if err != nil {
				return err
			}
			switch format {
			case OutputJSON:
				return printJSON(entries)
			case OutputYAML:
				return printYAMLDirect(entries)
			}
			if len(entries) == 0 {
				fmt.Fprintln(os.Stderr, "agents: no mappings configured")
				return nil
			}
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			defer func() { _ = w.Flush() }()
			_, _ = fmt.Fprintln(w, "PATTERN\tGROUP")
			for _, e := range entries {
				_, _ = fmt.Fprintf(w, "%s\t%s\n", e.Pattern, e.Group)
			}
			return nil
		},
	}
	agentsSessionsGroupListFormat = addOutputFlag(cmd, OutputText, OutputJSON, OutputYAML)
	return cmd
}

// splitMappingArg parses a "pattern=group" CLI argument. Returns
// ok=false when the `=` separator is missing.
func splitMappingArg(arg string) (pattern, group string, ok bool) {
	i := strings.IndexByte(arg, '=')
	if i <= 0 || i == len(arg)-1 {
		return "", "", false
	}
	return strings.TrimSpace(arg[:i]), strings.TrimSpace(arg[i+1:]), true
}

// ----- shared helpers -----

// findSession resolves a session by full ID, by trimmed UUID
// (without the provider prefix), or by a unique substring match
// against the ID, project path, or title.
//
// Returns the first match. When the input is ambiguous (multiple
// non-exact matches) the function falls through to ok=false so the
// caller can surface a UsageError; users should supply more of the
// ID to disambiguate.
func findSession(sessions []agents.Session, q string) (agents.Session, bool) {
	q = strings.TrimSpace(q)
	if q == "" {
		return agents.Session{}, false
	}
	// 1. exact ID match.
	for _, s := range sessions {
		if s.ID == q {
			return s, true
		}
	}
	// 2. trimmed-prefix match: user passed bare UUID.
	for _, s := range sessions {
		if strings.TrimPrefix(s.ID, "claude:") == q || strings.TrimPrefix(s.ID, "copilot:") == q {
			return s, true
		}
	}
	// 3. substring against ID / project / title.
	lower := strings.ToLower(q)
	var matches []agents.Session
	for _, s := range sessions {
		if strings.Contains(strings.ToLower(s.ID), lower) ||
			strings.Contains(strings.ToLower(s.ProjectPath), lower) ||
			strings.Contains(strings.ToLower(s.Title), lower) {
			matches = append(matches, s)
		}
	}
	if len(matches) == 1 {
		return matches[0], true
	}
	return agents.Session{}, false
}

// ----- jsonUnmarshalFn indirection -----
//
// CLI files in this repo tend to import json directly. Pulled out into
// a var so the rest of this file doesn't acquire an encoding/json
// import — keeps the file size grep-friendly.

var jsonUnmarshalFn = jsonStdUnmarshal

// (Implemented in agents_sessions_json.go to keep encoding/json out of
// this file's import block — it would otherwise read like a generic
// JSON helper rather than a session-specific runner.)

// (Unused-import safe-guard for strconv, retained for any int formatting
// the rendering helpers may use during future iterations.)
var _ = strconv.Itoa

// launchSessionsTUI starts the interactive Bubbletea dashboard. Pulled
// into a separate function so the bare-sessions and --watch entry
// points share the same setup (service construction, program options).
//
// The variable indirection (isStdoutTTY, launchSessionsTUIImpl) makes
// the function testable: a test can stub them to bypass TTY detection
// and bubbletea startup without rewriting the dispatch logic.
func launchSessionsTUI(ctx context.Context) error {
	if launchSessionsTUIImpl != nil {
		return launchSessionsTUIImpl(ctx)
	}
	return errors.New("sessions TUI not available in this build")
}

// isStdoutTTY reports whether os.Stdout is a terminal. Replaced in
// agents_sessions_tui.go (the only non-test consumer) so this file
// stays free of x/term and os import bloat.
var isStdoutTTY = func() bool { return false }

// launchSessionsTUIImpl is set in agents_sessions_tui.go to the real
// Bubbletea entry point. Nil means the TUI wasn't compiled in (e.g.
// a stripped-down build), in which case launchSessionsTUI returns an
// error.
var launchSessionsTUIImpl func(ctx context.Context) error
