package tui

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/nassiharel/klim/internal/agents"
	"github.com/nassiharel/klim/internal/agents/costs"
	"github.com/nassiharel/klim/internal/paths"
)

// agentsCostsState holds state for the Agents → Costs sub-tab. The
// loader runs asynchronously so this struct doubles as the in-flight
// progress carrier.
type agentsCostsState struct {
	loaded   bool
	loading  bool
	loadedAt time.Time
	loadErr  error
	samples  []costs.TokenSample
	rng      costs.Range
	cursor   int
}

// agentsCostsLoadedMsg fires after the background loader completes.
type agentsCostsLoadedMsg struct {
	samples []costs.TokenSample
	err     error
}

// agentsCostsLoadCmd kicks off a scan of every provider's transcripts,
// merging the result with the cache so untouched sessions don't get
// reparsed.
func (m *Model) agentsCostsLoadCmd() tea.Cmd {
	if m.agents != nil {
		m.agents.costs.loading = true
	}
	return func() tea.Msg {
		samples, err := loadCostSamples()
		return agentsCostsLoadedMsg{samples: samples, err: err}
	}
}

// loadCostSamples walks every provider's transcripts (via the
// TokenSamples capability) incrementally: transcripts whose mtime is
// unchanged since the last scan are NOT re-parsed — their token totals
// are reused straight from the on-disk cache. Only new or changed
// transcripts are read and re-aggregated. The cache is refreshed and
// saved synchronously, and entries for sessions that vanished from
// disk are pruned.
//
// This is what keeps the Costs tab fast on a heavy machine: without
// the mtime skip every scan re-read gigabytes of static transcripts.
func loadCostSamples() ([]costs.TokenSample, error) {
	cache, _ := costs.LoadCache()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	svc := agentsService()

	// Prior mtimes feed the providers' incremental skip.
	prior := make(map[string]time.Time, len(cache.Sessions))
	for id, e := range cache.Sessions {
		prior[id] = e.TranscriptMtime
	}

	freshBySession := map[string][]costs.TokenSample{}
	mtimeBySession := map[string]time.Time{}
	present := map[string]bool{}

	for _, p := range svc.Registry().Providers() {
		res, err := p.TokenSamples(ctx, costs.ScanInput{Prior: prior})
		if err != nil {
			continue
		}
		// Every session the provider saw on disk stays in the cache;
		// the rest get pruned below. mtimeBySession is sourced ONLY from
		// res.Seen (the real transcript file mtime) — never from a
		// sample's Day bucket, which is derived from in-transcript
		// timestamps and can be future-dated (clock skew). A future
		// mtime in the cache would make later scans wrongly skip real
		// changes until wall-clock catches up.
		for sessionID, mt := range res.Seen {
			present[sessionID] = true
			if mtimeBySession[sessionID].Before(mt) {
				mtimeBySession[sessionID] = mt
			}
		}
		for _, s := range res.Samples {
			freshBySession[s.SessionID] = append(freshBySession[s.SessionID], s)
			// A provider that didn't populate Seen (older impl) still
			// counts as present so its sessions aren't pruned. Its
			// mtime stays zero, which means "always rescan" — safe, if
			// not incremental.
			present[s.SessionID] = true
		}
	}

	// Re-aggregate only the sessions whose transcripts changed; the
	// untouched ones keep their existing cache entry verbatim.
	for sessionID, samples := range freshBySession {
		cache.Sessions[sessionID] = costs.AggregateSession(samples, mtimeBySession[sessionID])
	}
	cache.PruneMissing(present)
	_ = cache.Save()

	return cache.Samples(), nil
}

// handleAgentsCostsKey routes keys when the Costs sub-tab is active.
// Left/right arrows are deliberately NOT claimed here so they keep
// driving sub-tab navigation; the range selector instead uses `[` / `]`
// (and `,` / `.` as alternates) which don't collide with the global
// navigation chord.
func (m *Model) handleAgentsCostsKey(msg tea.KeyMsg) (bool, tea.Cmd) {
	st := m.agents
	if st == nil {
		return false, nil
	}
	cs := &st.costs
	switch msg.String() {
	case "]", ".":
		cs.rng = (cs.rng + 1) % costs.RangeCount
		cs.cursor = 0
		return true, nil
	case "[", ",":
		cs.rng = (cs.rng + costs.RangeCount - 1) % costs.RangeCount
		cs.cursor = 0
		return true, nil
	case "down", "j":
		rep := costs.Build(cs.samples, cs.rng)
		if cs.cursor < len(rep.TopSessions)-1 {
			cs.cursor++
		}
		return true, nil
	case "up", "k":
		if cs.cursor > 0 {
			cs.cursor--
		}
		return true, nil
	case "enter":
		rep := costs.Build(cs.samples, cs.rng)
		if cs.cursor < 0 || cs.cursor >= len(rep.TopSessions) {
			return true, nil
		}
		sc := rep.TopSessions[cs.cursor]
		st.detailPage = true
		st.detailStack = []agentDetailFrame{{
			subTab:   agentsSubSessions,
			entityID: sc.SessionID,
		}}
		return true, nil
	case "r":
		st.flash = "refreshing token cache…"
		st.flashEnd = time.Now().Add(2 * time.Second)
		if p, err := paths.AgentCostsCache(); err == nil {
			_ = os.Remove(p)
		}
		return true, m.agentsCostsLoadCmd()
	}
	return false, nil
}

// renderAgentsCostsView produces the Costs sub-tab body. Layout:
// range selector → totals → per-provider bars → per-model bars →
// top sessions table.
func (m *Model) renderAgentsCostsView() string {
	st := m.agents
	cs := &st.costs

	var b strings.Builder

	if cs.loading {
		b.WriteString("  scanning transcripts…\n")
		return b.String()
	}
	if cs.loadErr != nil {
		b.WriteString("  costs load error: " + cs.loadErr.Error() + "\n")
		return b.String()
	}
	if !cs.loaded {
		b.WriteString("  press r to scan transcripts and build a token usage report\n")
		return b.String()
	}

	rep := costs.Build(cs.samples, cs.rng)

	// Range tabs.
	var ranges []string
	for i := 0; i < int(costs.RangeCount); i++ {
		r := costs.Range(i)
		label := r.Label()
		if r == cs.rng {
			ranges = append(ranges, lipgloss.NewStyle().Foreground(cyberAccent).Bold(true).Render("["+label+"]"))
		} else {
			ranges = append(ranges, dimVersion.Render(" "+label+" "))
		}
	}
	b.WriteString("  " + strings.Join(ranges, "  ") + "  " + dimVersion.Render("[ / ] or , . switch · r refresh") + "\n\n")

	// Totals + sparkline.
	totalLine := fmt.Sprintf("%s tokens   in %s · out %s",
		lipgloss.NewStyle().Bold(true).Foreground(cyberPrimary).Render(formatTokens(rep.Totals.Total())),
		formatTokens(rep.Totals.Input),
		formatTokens(rep.Totals.Output),
	)
	b.WriteString("  " + totalLine + "\n")
	if rep.Days > 1 {
		b.WriteString("  " + renderSparkline(rep.DailySparkline) + dimVersion.Render(fmt.Sprintf("   (last %d days)", rep.Days)) + "\n")
	}
	b.WriteString("\n")

	if len(rep.ByProvider) > 0 {
		b.WriteString("  " + lipgloss.NewStyle().Bold(true).Foreground(cyberInfo).Render("BY PROVIDER") + "\n")
		b.WriteString(renderTokenBars(providerBarRows(rep), rep.Totals.Total()))
		b.WriteString("\n")
	}

	if len(rep.ByModel) > 0 {
		b.WriteString("  " + lipgloss.NewStyle().Bold(true).Foreground(cyberInfo).Render("BY MODEL") + "\n")
		b.WriteString(renderTokenBars(modelBarRows(rep), rep.Totals.Total()))
		b.WriteString("\n")
	}

	if len(rep.TopSessions) > 0 {
		b.WriteString("  " + lipgloss.NewStyle().Bold(true).Foreground(cyberInfo).Render("TOP SESSIONS") + "  " + dimVersion.Render("↑/↓ move · Enter open") + "\n")
		b.WriteString(renderTopSessions(rep.TopSessions, cs.cursor, m.width))
	} else if rep.Totals.Total() == 0 {
		b.WriteString("  " + dimVersion.Render("no token data in this range — try a wider window") + "\n")
	}

	return b.String()
}

func providerBarRows(rep costs.Report) []tokenBarRow {
	rows := make([]tokenBarRow, 0, len(rep.ByProvider))
	for p, t := range rep.ByProvider {
		rows = append(rows, tokenBarRow{
			label: providerShort(agents.ProviderID(p)),
			value: t.Total(),
		})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].value > rows[j].value })
	return rows
}

func modelBarRows(rep costs.Report) []tokenBarRow {
	rows := make([]tokenBarRow, 0, len(rep.ByModel))
	for key, t := range rep.ByModel {
		var label string
		if slash := strings.Index(key, "/"); slash >= 0 {
			provider := key[:slash]
			model := key[slash+1:]
			if model == "" {
				model = "(unspecified)"
			}
			label = model + " · " + providerShort(agents.ProviderID(provider))
		} else {
			label = key
		}
		rows = append(rows, tokenBarRow{label: label, value: t.Total()})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].value > rows[j].value })
	if len(rows) > 8 {
		rows = rows[:8]
	}
	return rows
}

type tokenBarRow struct {
	label string
	value int
}

func renderTokenBars(rows []tokenBarRow, total int) string {
	if len(rows) == 0 {
		return ""
	}
	peak := rows[0].value
	if total > peak {
		peak = total
	}
	const barWidth = 24
	var b strings.Builder
	for _, r := range rows {
		filled := 0
		if peak > 0 {
			filled = r.value * barWidth / peak
		}
		if filled < 0 {
			filled = 0
		}
		if filled > barWidth {
			filled = barWidth
		}
		bar := lipgloss.NewStyle().Foreground(cyberPrimary).Render(strings.Repeat("█", filled)) +
			lipgloss.NewStyle().Foreground(cyberFGDim).Render(strings.Repeat("·", barWidth-filled))
		pct := 0
		if total > 0 {
			pct = r.value * 100 / total
		}
		labelStyled := lipgloss.NewStyle().Width(30).Render(truncAgentRow(r.label, 30))
		fmt.Fprintf(&b, "  %s %s  %s  %s\n", labelStyled, bar, formatTokens(r.value), dimVersion.Render(fmt.Sprintf("(%d%%)", pct)))
	}
	return b.String()
}

func renderTopSessions(sessions []costs.SessionCost, cursor, totalWidth int) string {
	const maxN = 10
	if len(sessions) > maxN {
		sessions = sessions[:maxN]
	}
	cols := computeColumnWidths([]column{
		{header: "SOURCE", width: 10},
		{header: "TITLE", grow: true},
		{header: "MODEL", width: 22},
		{header: "TOKENS", width: 26},
	}, totalWidth)
	var b strings.Builder
	b.WriteString(renderHeader(cols, -1))
	for i, sc := range sessions {
		title := sc.Title
		if title == "" {
			title = sc.SessionID
		}
		cells := []string{
			agentsProviderChip(agents.ProviderID(sc.Provider)),
			truncAgentRow(title, cols[1].width),
			truncAgentRow(sc.Model, cols[2].width),
			formatTokenCell(sc.Totals),
		}
		b.WriteString(renderRow(cells, cols, rowLead(i, cursor), i == cursor, totalWidth))
	}
	return b.String()
}

// formatTokenCell renders a compact "<total> (<in>↓ <out>↑)" summary
// that fits the TOKENS column without wrapping. The arrows read as
// input (down/received) and output (up/generated); using K/M suffixes
// keeps the worst case ("999.9M (999.9M↓ 999.9M↑)") within the column.
func formatTokenCell(t costs.Totals) string {
	return fmt.Sprintf("%s (%s↓ %s↑)",
		formatTokens(t.Total()),
		formatTokens(t.Input),
		formatTokens(t.Output),
	)
}

func renderSparkline(buckets []int) string {
	if len(buckets) == 0 {
		return ""
	}
	glyphs := []rune("▁▂▃▄▅▆▇█")
	peak := 0
	for _, v := range buckets {
		if v > peak {
			peak = v
		}
	}
	var b strings.Builder
	for _, v := range buckets {
		if peak == 0 || v == 0 {
			b.WriteString(" ")
			continue
		}
		idx := (v * (len(glyphs) - 1)) / peak
		if idx >= len(glyphs) {
			idx = len(glyphs) - 1
		}
		b.WriteRune(glyphs[idx])
	}
	return lipgloss.NewStyle().Foreground(cyberAccent).Render(b.String())
}

func formatTokens(n int) string {
	if n >= 1_000_000 {
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	}
	if n >= 1_000 {
		return fmt.Sprintf("%.1fK", float64(n)/1_000)
	}
	return strconv.Itoa(n)
}
