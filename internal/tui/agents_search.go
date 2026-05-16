package tui

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/nassiharel/klim/internal/agents"
	"github.com/nassiharel/klim/internal/agents/search"
)

// agentsSearchState holds the search-overlay model. The overlay is
// open whenever Open is true; key dispatch routes through
// handleAgentsSearchKey first when Open. Results are recomputed every
// time the query changes (cheap — pure in-memory substring scan over
// the cached index).
type agentsSearchState struct {
	Open     bool
	Query    string
	Cursor   int
	Hits     []search.Hit
	Index    *search.Index
	Indexing bool
	IndexErr error

	// Viewer overlay (Esc → close viewer, then close search if still open).
	ViewerOpen   bool
	ViewerPath   string
	ViewerLines  []search.SessionLine
	ViewerCursor int
}

// agentsSearchIndexLoadedMsg lands when the background indexer finishes.
type agentsSearchIndexLoadedMsg struct {
	idx *search.Index
	err error
}

// agentsSearchOpenCmd kicks off an index build if we don't have one
// yet, then leaves the overlay open for query input.
func (m *Model) agentsSearchOpenCmd() tea.Cmd {
	if m.agents == nil {
		return nil
	}
	st := m.agents
	st.searchOverlay.Open = true
	st.searchOverlay.Cursor = 0
	if st.searchOverlay.Index != nil {
		return nil
	}
	st.searchOverlay.Indexing = true
	return loadAgentsSearchIndexCmd()
}

func loadAgentsSearchIndexCmd() tea.Cmd {
	return func() tea.Msg {
		idx, err := buildAgentsSearchIndex()
		return agentsSearchIndexLoadedMsg{idx: idx, err: err}
	}
}

// buildAgentsSearchIndex walks every provider's SessionTexts,
// merging fresh extractions with the on-disk cache (mtime-keyed).
// Providers returning ErrNotSupported are skipped silently.
func buildAgentsSearchIndex() (*search.Index, error) {
	idx, _ := search.LoadIndex()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	svc := agentsService()

	present := map[string]bool{}
	for _, p := range svc.Registry().Providers() {
		texts, err := p.SessionTexts(ctx)
		if err != nil {
			continue
		}
		// Mtime-aware refresh: replace cached entries whose source
		// transcripts changed; keep cached entries for sessions we
		// didn't re-read this round.
		for _, t := range texts {
			cached, ok := idx.Sessions[t.SessionID]
			if ok && !t.TranscriptMtime.IsZero() && !t.TranscriptMtime.After(cached.TranscriptMtime) {
				present[t.SessionID] = true
				continue
			}
			idx.Sessions[t.SessionID] = t
			present[t.SessionID] = true
		}
	}
	// Keep cached entries for providers that just returned
	// ErrNotSupported — they'd still be in idx.Sessions but won't
	// have been re-confirmed. Mark them present so PruneMissing
	// doesn't drop them.
	for id := range idx.Sessions {
		present[id] = true
	}
	idx.PruneMissing(present)
	_ = idx.Save()
	return idx, nil
}

// handleAgentsSearchKey routes input while the overlay is open.
func (m *Model) handleAgentsSearchKey(msg tea.KeyMsg) (bool, tea.Cmd) {
	st := m.agents
	if st == nil || !st.searchOverlay.Open {
		return false, nil
	}
	ov := &st.searchOverlay

	// Viewer overlay on top of search — handle its keys first.
	if ov.ViewerOpen {
		switch msg.String() {
		case "esc", "q":
			ov.ViewerOpen = false
			return true, nil
		case "down", "j":
			if ov.ViewerCursor < len(ov.ViewerLines)-1 {
				ov.ViewerCursor++
			}
			return true, nil
		case "up", "k":
			if ov.ViewerCursor > 0 {
				ov.ViewerCursor--
			}
			return true, nil
		case "pgdown":
			ov.ViewerCursor += 10
			if ov.ViewerCursor >= len(ov.ViewerLines) {
				ov.ViewerCursor = len(ov.ViewerLines) - 1
			}
			return true, nil
		case "pgup":
			ov.ViewerCursor -= 10
			if ov.ViewerCursor < 0 {
				ov.ViewerCursor = 0
			}
			return true, nil
		}
		return true, nil
	}

	switch msg.String() {
	case "esc":
		ov.Open = false
		ov.Query = ""
		ov.Hits = nil
		ov.Cursor = 0
		return true, nil
	case "enter":
		if ov.Cursor < 0 || ov.Cursor >= len(ov.Hits) {
			return true, nil
		}
		hit := ov.Hits[ov.Cursor]
		// Locate the source session in the index to grab its full
		// line list for the viewer.
		if sess, ok := ov.Index.Sessions[hit.SessionID]; ok {
			ov.ViewerOpen = true
			ov.ViewerPath = sess.TranscriptPath
			ov.ViewerLines = sess.Lines
			// Place the cursor on the matched line.
			ov.ViewerCursor = 0
			for i, l := range sess.Lines {
				if l.LineNo == hit.LineNo {
					ov.ViewerCursor = i
					break
				}
			}
		}
		return true, nil
	case "down", "ctrl+n":
		if ov.Cursor < len(ov.Hits)-1 {
			ov.Cursor++
		}
		return true, nil
	case "up", "ctrl+p":
		if ov.Cursor > 0 {
			ov.Cursor--
		}
		return true, nil
	case "backspace":
		if len(ov.Query) > 0 {
			ov.Query = ov.Query[:len(ov.Query)-1]
			ov.refreshHits()
		}
		return true, nil
	case "ctrl+u":
		ov.Query = ""
		ov.refreshHits()
		return true, nil
	case "ctrl+r":
		// Force a fresh index build.
		ov.Indexing = true
		ov.Index = nil
		if p, err := os.UserHomeDir(); err == nil {
			_ = p // placeholder; index has its own cache path
		}
		return true, loadAgentsSearchIndexCmd()
	default:
		// Treat any single printable rune as a query character.
		k := msg.String()
		if len(k) == 1 {
			ov.Query += k
			ov.refreshHits()
			return true, nil
		}
	}
	return true, nil
}

// refreshHits recomputes the result set from the current Index.
func (ov *agentsSearchState) refreshHits() {
	ov.Cursor = 0
	if ov.Index == nil {
		ov.Hits = nil
		return
	}
	ov.Hits = ov.Index.Query(ov.Query, 50)
}

// renderAgentsSearchOverlay produces the search overlay. Drawn after
// the rest of the Agents view by the renderer when Open is true.
func (m *Model) renderAgentsSearchOverlay() string {
	st := m.agents
	if st == nil || !st.searchOverlay.Open {
		return ""
	}
	ov := &st.searchOverlay
	width := m.width
	if width <= 0 {
		width = 80
	}

	var b strings.Builder
	b.WriteString("\n")
	box := lipgloss.NewStyle().
		Foreground(cyberFG).
		Background(cyberSelectedBg).
		BorderForeground(cyberPrimary).
		BorderStyle(lipgloss.RoundedBorder()).
		Padding(0, 1).
		Width(width - 4)

	// Header / prompt line.
	queryLine := "🔎  " + ov.Query + lipgloss.NewStyle().Foreground(cyberAccent).Render("▌")
	state := ""
	switch {
	case ov.Indexing:
		state = dimVersion.Render("indexing transcripts…")
	case ov.Index != nil:
		state = dimVersion.Render(fmt.Sprintf("%d sessions indexed · %d results", len(ov.Index.Sessions), len(ov.Hits)))
	}

	// Hits list.
	var hitLines []string
	if ov.Indexing {
		hitLines = append(hitLines, dimVersion.Render("  building search index…"))
	} else if ov.Query == "" {
		hitLines = append(hitLines, dimVersion.Render("  type to search · Enter opens viewer · Esc closes"))
	} else if len(ov.Hits) == 0 {
		hitLines = append(hitLines, dimVersion.Render("  no matches"))
	}
	const maxHits = 12
	visible := ov.Hits
	if len(visible) > maxHits {
		visible = visible[:maxHits]
	}
	for i, hit := range visible {
		lead := "    "
		if i == ov.Cursor {
			lead = "  ▸ "
		}
		title := hit.Title
		if title == "" {
			title = hit.SessionID
		}
		// First line: chip + title.
		header := lead + agentsProviderChip(agents.ProviderID(hit.Provider)) + "  " +
			lipgloss.NewStyle().Bold(true).Render(truncAgentRow(title, 50)) + "  " +
			dimVersion.Render(fmt.Sprintf("line %d · %s", hit.LineNo, hit.Role))
		// Second line: snippet, slightly indented.
		snippet := lipgloss.NewStyle().Foreground(cyberFG).Render("      " + truncAgentRow(hit.Snippet, width-12))
		line := header + "\n" + snippet
		if i == ov.Cursor {
			line = cyberSelectedRowStyle.Render(line)
		}
		hitLines = append(hitLines, line)
	}
	if len(ov.Hits) > maxHits {
		hitLines = append(hitLines, dimVersion.Render(fmt.Sprintf("      … %d more matches", len(ov.Hits)-maxHits)))
	}

	body := queryLine + "  " + state + "\n" + strings.Join(hitLines, "\n")
	b.WriteString(box.Render(body))
	b.WriteString("\n")
	b.WriteString(dimVersion.Render("  ↑/↓ move · Enter open viewer · Ctrl+U clear · Ctrl+R rescan · Esc close") + "\n")

	if ov.ViewerOpen {
		b.WriteString("\n")
		b.WriteString(renderAgentsTranscriptViewer(ov, width))
	}
	return b.String()
}

// renderAgentsTranscriptViewer renders the full transcript viewer
// modal that pops over the search overlay when Enter is pressed.
// Lines are coloured by role; the cursor row is reverse-video so the
// user can scroll without losing their place.
func renderAgentsTranscriptViewer(ov *agentsSearchState, width int) string {
	box := lipgloss.NewStyle().
		Foreground(cyberFG).
		Background(cyberSelectedBg).
		BorderForeground(cyberAccent).
		BorderStyle(lipgloss.RoundedBorder()).
		Padding(0, 1).
		Width(width - 4)

	const window = 18
	start := ov.ViewerCursor - window/2
	if start < 0 {
		start = 0
	}
	end := start + window
	if end > len(ov.ViewerLines) {
		end = len(ov.ViewerLines)
		start = end - window
		if start < 0 {
			start = 0
		}
	}

	var lines []string
	header := lipgloss.NewStyle().Bold(true).Foreground(cyberPrimary).
		Render("📜 transcript  ") + dimVersion.Render(truncAgentRow(ov.ViewerPath, width-12))
	lines = append(lines, header)
	lines = append(lines, "")

	for i := start; i < end; i++ {
		l := ov.ViewerLines[i]
		role := transcriptRoleChip(l.Role)
		ts := ""
		if !l.Timestamp.IsZero() {
			ts = dimVersion.Render(l.Timestamp.Format("15:04:05"))
		}
		text := strings.ReplaceAll(l.Text, "\n", " ")
		text = truncAgentRow(text, width-22)
		line := fmt.Sprintf("  %s %s  %s", role, ts, text)
		if i == ov.ViewerCursor {
			line = cyberSelectedRowStyle.Render(line)
		}
		lines = append(lines, line)
	}

	footer := dimVersion.Render(fmt.Sprintf("  line %d / %d · ↑/↓ scroll · Esc close", ov.ViewerCursor+1, len(ov.ViewerLines)))
	lines = append(lines, "", footer)
	return box.Render(strings.Join(lines, "\n"))
}

func transcriptRoleChip(role string) string {
	fg := cyberFG
	switch role {
	case "user":
		fg = cyberInfo
	case "assistant":
		fg = cyberPrimary
	case "tool":
		fg = cyberAccent
	case "system":
		fg = cyberFGDim
	}
	return lipgloss.NewStyle().
		Foreground(fg).
		Background(cyberChipBg).
		Padding(0, 1).
		Bold(true).
		Render(role)
}
