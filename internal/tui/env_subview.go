package tui

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sort"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/nassiharel/klim/internal/config"
	"github.com/nassiharel/klim/internal/custompacks"
	"github.com/nassiharel/klim/internal/envid"
	"github.com/nassiharel/klim/internal/favorites"
	"github.com/nassiharel/klim/internal/registry"
	"github.com/nassiharel/klim/internal/service"
)

// Env sub-view states.
//
// envViewIdle is the landing state — show the locally-built profile's
// token, copy button, and three action prompts (open / diff / apply).
// The other states are entered when the user picks an action.
const (
	envViewIdle        = ""
	envViewInputOpen   = "input-open"
	envViewInputDiff   = "input-diff"
	envViewInputApply  = "input-apply"
	envViewShowResult  = "show"
	envViewDiffResult  = "diff"
	envViewApplyReport = "apply-report"
)

// envBuildResultMsg carries the locally-built profile back from the
// async build command.
type envBuildResultMsg struct {
	profile *envid.Profile
	token   string
	err     error
}

// envDecodedMsg carries the result of decoding a token / file path
// supplied by the user. It distinguishes the three "verb" intents so
// the receiver knows which view to render next.
type envDecodedMsg struct {
	verb    string // "open" | "diff" | "apply"
	profile *envid.Profile
	err     error
}

// envApplyResultMsg arrives after applyTools / applyFavorites /
// applyPacks finish so the report view can summarise outcomes.
type envApplyResultMsg struct {
	favoritesAdded int
	favoritesTotal int
	packsAdded     int
	packsTotal     int
	err            error
}

// resetEnvSubviewState clears every transient env sub-view field back
// to its zero value. Used by both startEnvSubview (which then kicks
// off a build) and the deferred-build path that opens the Profile tab
// during an in-flight scan, so stale "✓ Copied" / old diff / report
// content can't leak across navigations.
func (m *Model) resetEnvSubviewState() {
	m.viewingEnv = true
	m.envState = envViewIdle
	m.envProfile = nil
	m.envToken = ""
	m.envTokenCopied = false
	m.envError = ""
	m.envRemoteProfile = nil
	m.envDiffText = ""
	m.envShowText = ""
	m.envApplyReport = ""
	m.envApplyPending = false
	m.envApplyProfile = nil
	m.envApplyFromProfile = false
}

// startEnvSubview enters the env landing state and kicks off a fresh
// profile build. The build runs asynchronously so the TUI doesn't block
// on PM availability checks (collectTools may probe `where`/`which`).
func (m *Model) startEnvSubview() tea.Cmd {
	m.resetEnvSubviewState()
	return buildEnvProfileCmd(m.svc, m.cfg)
}

// envInputModeFor returns the textinput placeholder used for each
// verb, so the prompt nudges the user toward the right kind of input.
func envInputModeFor(verb string) string {
	switch verb {
	case envViewInputOpen, "open":
		return "paste klim:env:v1:... or path/to/env.yaml to inspect"
	case envViewInputDiff, "diff":
		return "paste klim:env:v1:... or path/to/env.yaml to compare"
	case envViewInputApply, "apply":
		return "paste klim:env:v1:... or path/to/env.yaml to apply"
	}
	return "paste klim:env:v1:... or path/to/env.yaml"
}

// startEnvInput swaps to text-input mode for the chosen verb. Reuses
// the existing tokenInput control rather than introducing a third
// textinput.Model — the lifetime is identical (single-shot, blur-on-
// submit).
func (m *Model) startEnvInput(verb string) tea.Cmd {
	m.envState = verb
	m.envInputVerb = verb
	m.envError = ""
	ti := textinput.New()
	ti.Placeholder = envInputModeFor(verb)
	ti.CharLimit = 4000
	ti.SetWidth(60)
	m.envInput = ti
	return m.envInput.Focus()
}

// handleKeyEnv routes keystrokes while the env sub-view is active. It
// branches on m.envState so each state has the keys that make sense
// there and nothing more — no global "every key works everywhere"
// trap that would let a stray "a" trigger Apply during Show output.
func (m Model) handleKeyEnv(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	inInput := m.envState == envViewInputOpen ||
		m.envState == envViewInputDiff ||
		m.envState == envViewInputApply

	// Quit and parent-tab switching keys take priority over every
	// other env state (including text-input states) so the user is
	// never trapped inside the modal sub-view. In input states we
	// deliberately *exclude* plain Left/Right so the textinput can
	// still move its cursor through pasted tokens — Tab/Shift-Tab
	// remain the input-mode escape hatches.
	switch msg.String() {
	case "ctrl+c":
		m.quitting = true
		return m, tea.Quit
	case "1", "2", "3", "4", "5", "6", "7", "8", "9":
		mp := &m
		// Cancel any in-flight text input so we don't carry stale
		// state to the destination tab.
		if inInput {
			mp.envState = envViewIdle
			mp.envInputVerb = ""
		}
		if mp.activeTab == tabProfile {
			mp.viewingEnv = false
		}
		if handled, cmd := mp.switchToTabByNumber(msg.String()); handled {
			return *mp, cmd
		}
		return m, nil
	case "tab":
		if inInput {
			m.envState = envViewIdle
			m.envInputVerb = ""
		}
		if m.activeTab == tabProfile {
			m.viewingEnv = false
		}
		next := parentTabOrder[(parentIndex(m.activeTab)+1)%len(parentTabOrder)]
		return m.gotoParentTab(next)
	case "shift+tab":
		if inInput {
			m.envState = envViewIdle
			m.envInputVerb = ""
		}
		if m.activeTab == tabProfile {
			m.viewingEnv = false
		}
		prev := parentTabOrder[(parentIndex(m.activeTab)+len(parentTabOrder)-1)%len(parentTabOrder)]
		return m.gotoParentTab(prev)
	case "right":
		if !inInput {
			if m.activeTab == tabProfile {
				m.viewingEnv = false
			}
			next := parentTabOrder[(parentIndex(m.activeTab)+1)%len(parentTabOrder)]
			return m.gotoParentTab(next)
		}
	case "left":
		if !inInput {
			if m.activeTab == tabProfile {
				m.viewingEnv = false
			}
			prev := parentTabOrder[(parentIndex(m.activeTab)+len(parentTabOrder)-1)%len(parentTabOrder)]
			return m.gotoParentTab(prev)
		}
	}

	// Text-input states intercept all remaining keys so the user can
	// paste a token (which may legitimately contain `:`, base64
	// chars, etc.) without one of them being eaten as a hotkey.
	if inInput {
		return m.handleKeyEnvInput(msg)
	}

	switch msg.String() {
	case "q":
		// On the dedicated Profile tab `q` should quit, matching
		// the behaviour on every other tab. (Inside the Backup
		// sub-view, `q` is treated as "back" because Backup hosts
		// the env sub-view as a modal — but Profile *is* the env
		// sub-view, so there's no "back" target.)
		if m.activeTab == tabProfile && m.envState == envViewIdle {
			m.quitting = true
			return m, tea.Quit
		}
		// Fall through to the back-out handler below for the
		// Backup-hosted case.
		fallthrough
	case "esc", "backspace":
		// Layered back-out: result views go back to the landing
		// page; the landing page closes the sub-view entirely
		// (or, on the dedicated Profile tab, stays put — there is
		// no "back" target since Profile IS the env sub-view).
		switch m.envState {
		case envViewIdle:
			if m.activeTab == tabProfile {
				// No-op: Profile tab has no parent menu.
				return m, nil
			}
			m.viewingEnv = false
			m.envState = envViewIdle
			m.statusMsg = ""
		default:
			m.envState = envViewIdle
			m.envError = ""
			m.envShowText = ""
			m.envDiffText = ""
			m.envApplyReport = ""
		}
		return m, nil
	case "c":
		// Copy the local env token. We restrict copy to the idle
		// state — a copy hotkey on result views would be confusing
		// (copy WHAT? the diff text? the show text?) without a
		// clear visual affordance.
		if m.envState == envViewIdle && m.envToken != "" {
			if err := m.clip.WriteAll(m.envToken); err != nil {
				m.statusMsg = "⚠ Clipboard unavailable"
			} else {
				m.envTokenCopied = true
				m.statusMsg = "✓ Copied to clipboard!"
			}
		}
		return m, nil
	case "o":
		if m.envState == envViewIdle {
			cmd := (&m).startEnvInput(envViewInputOpen)
			return m, cmd
		}
	case "d":
		// Diff needs the local profile to compare against — block
		// until phaseDone so we don't have to fail with "local
		// profile not built yet" mid-flow, and so the comparison
		// reflects the *complete* installed set rather than a
		// partial one.
		if m.envState == envViewIdle {
			if m.phase < phaseDone {
				m.statusMsg = "Still scanning — diff is available once scan finishes"
				return m, nil
			}
			cmd := (&m).startEnvInput(envViewInputDiff)
			return m, cmd
		}
	case "a":
		// Apply ultimately runs buildEnvApplyPlanCmd → svc.ScanOnly,
		// which would kick off a second PATH scan in parallel with
		// the in-flight initial scan and produce the same "UI feels
		// stuck" symptom the deferred-build path avoids. Block at
		// idle until phaseDone so users can't accidentally trigger
		// the parallel scan even by typing the verb during the
		// initial load.
		if m.envState == envViewIdle {
			if m.phase < phaseDone {
				m.statusMsg = "Still scanning — apply is available once scan finishes"
				return m, nil
			}
			cmd := (&m).startEnvInput(envViewInputApply)
			return m, cmd
		}
	case "r":
		// Refresh: rebuild the local profile (e.g. after install/
		// upgrade outside the sub-view changed the toolset).
		if m.envState == envViewIdle {
			// Gate on phaseDone — buildEnvProfileCmd calls
			// LoadAndResolveCached, which on a cold cache will
			// kick off a second full PATH+version scan in
			// parallel with the in-flight initial scan and make
			// the UI feel stuck. The deferred-build path in
			// switchToTabByNumber already auto-triggers the
			// build when the initial scan finishes, so there's
			// nothing for the user to do here except wait.
			if m.phase < phaseDone {
				m.statusMsg = "Still scanning — env profile will build when scan finishes"
				return m, nil
			}
			m.envProfile = nil
			m.envToken = ""
			m.envError = ""
			m.statusMsg = "Rebuilding env profile..."
			return m, buildEnvProfileCmd(m.svc, m.cfg)
		}
	case "up", "k":
		// Scroll the body up. Only meaningful on the idle landing
		// page where the My Score + Env Profile content may exceed
		// the viewport on small terminals.
		if m.envState == envViewIdle && m.profileScroll > 0 {
			m.profileScroll--
		}
		return m, nil
	case "down", "j":
		if m.envState == envViewIdle {
			m.profileScroll++
			m.clampScrollOffsets()
		}
		return m, nil
	case "home", "g":
		if m.envState == envViewIdle {
			m.profileScroll = 0
		}
		return m, nil
	case "pgup":
		if m.envState == envViewIdle {
			m.profileScroll -= 5
			if m.profileScroll < 0 {
				m.profileScroll = 0
			}
		}
		return m, nil
	case "pgdown", " ":
		if m.envState == envViewIdle {
			m.profileScroll += 5
			m.clampScrollOffsets()
		}
		return m, nil
	}
	return m, nil
}

// handleKeyEnvInput accepts the textinput while the user is typing /
// pasting a token. Enter submits, Esc cancels.
func (m Model) handleKeyEnvInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.envState = envViewIdle
		m.envInputVerb = ""
		return m, nil
	case "enter":
		input := strings.TrimSpace(m.envInput.Value())
		if input == "" {
			return m, nil
		}
		verb := m.envInputVerb
		m.envState = envViewIdle
		m.envInputVerb = ""
		m.statusMsg = "Decoding env..."
		// We map the verb sentinel back to the lowercase command
		// name so envDecodedMsg.verb stays free of internal state
		// constants.
		mapped := "open"
		switch verb {
		case envViewInputDiff:
			mapped = "diff"
		case envViewInputApply:
			mapped = "apply"
		}
		return m, decodeEnvCmd(input, mapped)
	}
	var cmd tea.Cmd
	m.envInput, cmd = m.envInput.Update(msg)
	return m, cmd
}

// renderEnvSubview is the entry point used by view_backup.go.
func (m Model) renderEnvSubview() string {
	switch m.envState {
	case envViewInputOpen, envViewInputDiff, envViewInputApply:
		return m.renderEnvInputView()
	case envViewShowResult:
		return m.renderEnvShowView()
	case envViewDiffResult:
		return m.renderEnvDiffView()
	case envViewApplyReport:
		return m.renderEnvApplyReportView()
	default:
		return m.renderEnvIdleView()
	}
}

// renderEnvIdleView shows the locally-built profile's token + the four
// action prompts. While the profile is still building we render a
// loading spinner-like hint instead of an empty token.
func (m Model) renderEnvIdleView() string {
	var b strings.Builder
	b.WriteString("\n")

	// My Score lands at the top — it's the answer to "how is this
	// developer environment doing?", which is the question the
	// profile token also addresses (from a portability angle).
	// Showing the breakdown first gives the user the headline grade
	// before the slower-to-scan fingerprint section.
	if m.doctorChecked {
		if section := renderMyScoreSection(m.cachedScore, m.width); section != "" {
			b.WriteString(section + "\n")
		}
	}

	b.WriteString("  " + detailTitleStyle.Render("Env Profile") + "  " +
		dimVersion.Render("portable fingerprint of this environment") + "\n\n")

	if m.envError != "" {
		b.WriteString("  " + complianceErrorStyle.Render("✗ "+m.envError) + "\n\n")
	}

	if m.envProfile == nil {
		b.WriteString("  " + dimVersion.Render("Building profile...") + "\n\n")
	} else {
		hash := envid.ComputeHash(m.envProfile)
		b.WriteString("  " + detailLabelStyle.Render(fixedWidth("Hash", 14)) + nameStyle.Render(hash) + "\n")
		b.WriteString("  " + detailLabelStyle.Render(fixedWidth("klim", 14)) + dimVersion.Render(m.envProfile.Clim.Version) + "\n")
		b.WriteString("  " + detailLabelStyle.Render(fixedWidth("OS", 14)) +
			dimVersion.Render(fmt.Sprintf("%s/%s", m.envProfile.OS.GOOS, m.envProfile.OS.Arch)) + "\n")
		b.WriteString("  " + detailLabelStyle.Render(fixedWidth("Tools", 14)) +
			dimVersion.Render(strconv.Itoa(len(m.envProfile.Tools))) + "\n")
		b.WriteString("  " + detailLabelStyle.Render(fixedWidth("Favorites", 14)) +
			dimVersion.Render(strconv.Itoa(len(m.envProfile.Favorites))) + "\n")
		b.WriteString("  " + detailLabelStyle.Render(fixedWidth("Custom packs", 14)) +
			dimVersion.Render(strconv.Itoa(len(m.envProfile.Packs))) + "\n\n")

		if m.envToken != "" {
			b.WriteString("  " + detailLabelStyle.Render("Token") + "\n")
			maxW := m.width - 6
			if maxW < 40 {
				maxW = 40
			}
			for _, line := range wordWrap(m.envToken, maxW) {
				b.WriteString("  " + dimVersion.Render(line) + "\n")
			}
			b.WriteString("\n")
			if m.envTokenCopied {
				b.WriteString("  " + buttonDoneStyle.Render("✓ Copied to clipboard") + "\n\n")
			} else {
				b.WriteString("  " + buttonStyle.Render("⎘ Copy to clipboard (c)") + "\n\n")
			}
		}
	}

	// Actions key descriptions live in the help footer (renderHelp
	// for tabProfile) so they stay pinned at the bottom of the
	// terminal regardless of how far the user has scrolled. The
	// previous in-body Actions list scrolled off-screen with the
	// rest of the content, which made the footer look like it was
	// "going up" when really the scroll just lifted the in-body
	// shortcuts above the viewport.

	return b.String()
}

// renderEnvInputView prompts for a token / file path. The label is
// driven by m.envInputVerb so the user always sees what they're about
// to do — easy to forget once two prompts in a row use the same
// textinput.
func (m Model) renderEnvInputView() string {
	var b strings.Builder
	b.WriteString("\n")
	verbLabel := "Open"
	switch m.envState {
	case envViewInputDiff:
		verbLabel = "Compare"
	case envViewInputApply:
		verbLabel = "Apply"
	}
	b.WriteString("  " + detailTitleStyle.Render(verbLabel+" Env") + "\n\n")
	b.WriteString("  " + confirmStyle.Render("Paste a token or file path:") + "  " +
		m.envInput.View() + "\n\n")
	b.WriteString("  " + dimVersion.Render("Enter") + "  submit   " +
		dimVersion.Render("Esc") + "  cancel\n")
	return b.String()
}

// renderEnvShowView pretty-prints a remote profile (no side effects).
// Output mirrors `klim env show` so users get parity with the CLI.
//
// We intentionally do NOT emit an in-body "Esc back" hint — the same
// hint is in renderHelp for envViewShowResult, where it stays pinned
// to the actual bottom of the terminal regardless of how far the
// user has scrolled.
func (m Model) renderEnvShowView() string {
	var b strings.Builder
	b.WriteString("\n")
	b.WriteString("  " + detailTitleStyle.Render("Env Show") + "\n\n")
	if m.envShowText == "" {
		b.WriteString("  " + dimVersion.Render("(empty)") + "\n")
	} else {
		for _, line := range strings.Split(m.envShowText, "\n") {
			b.WriteString("  " + line + "\n")
		}
	}
	return b.String()
}

// renderEnvDiffView prints the diff between the local and the supplied
// remote profile. The "Esc back" hint lives in renderHelp for
// envViewDiffResult, same reason as renderEnvShowView.
func (m Model) renderEnvDiffView() string {
	var b strings.Builder
	b.WriteString("\n")
	b.WriteString("  " + detailTitleStyle.Render("Env Diff") + "\n\n")
	if m.envDiffText == "" {
		b.WriteString("  " + dimVersion.Render("(no differences)") + "\n")
	} else {
		for _, line := range strings.Split(m.envDiffText, "\n") {
			b.WriteString("  " + line + "\n")
		}
	}
	return b.String()
}

// renderEnvApplyReportView shows the per-section apply outcome after
// Apply runs. Tool installs already render through the existing
// backupItems progress bar; this view summarises the favorites and
// packs side-effects which don't go through that flow.
//
// As with renderEnvShowView / renderEnvDiffView, the "Esc back" hint
// is in renderHelp (envViewApplyReport state) so it stays pinned
// when the report is long.
func (m Model) renderEnvApplyReportView() string {
	var b strings.Builder
	b.WriteString("\n")
	b.WriteString("  " + detailTitleStyle.Render("Apply Report") + "\n\n")
	if m.envApplyReport == "" {
		b.WriteString("  " + dimVersion.Render("(no changes)") + "\n")
	} else {
		for _, line := range strings.Split(m.envApplyReport, "\n") {
			b.WriteString("  " + line + "\n")
		}
	}
	return b.String()
}

// --- Commands ---

// buildEnvProfileCmd asynchronously builds the local env Profile and
// encodes it as a token. Both happen in the same command because the
// UI only ever uses them together; splitting would force a second
// async round-trip on Refresh.
func buildEnvProfileCmd(svc *service.ToolService, cfg *config.Config) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		p, err := envid.Build(ctx, svc, cfg, envid.BuildOptions{})
		if err != nil {
			return envBuildResultMsg{err: fmt.Errorf("building profile: %w", err)}
		}
		token, err := envid.Encode(p)
		if err != nil {
			return envBuildResultMsg{profile: p, err: fmt.Errorf("encoding token: %w", err)}
		}
		return envBuildResultMsg{profile: p, token: token}
	}
}

// decodeEnvCmd parses either a token or a file path and returns the
// profile back to the model. Mirrors the CLI's loadProfile dispatch.
func decodeEnvCmd(input, verb string) tea.Cmd {
	return func() tea.Msg {
		trimmed := strings.TrimSpace(input)
		if trimmed == "" {
			return envDecodedMsg{verb: verb, err: errors.New("empty input")}
		}
		if strings.HasPrefix(trimmed, "klim:env:") {
			p, err := envid.Decode(trimmed)
			return envDecodedMsg{verb: verb, profile: p, err: err}
		}
		p, err := envid.ReadFile(trimmed)
		return envDecodedMsg{verb: verb, profile: p, err: err}
	}
}

// applyEnvSideEffectsCmd applies favorites + custom packs from a
// remote profile. Tool installs are *not* handled here — they go
// through the existing backupItems install flow so the user sees
// progress per tool. This command is kicked off after the install
// flow finishes and only handles the additive favorites/packs merge.
func applyEnvSideEffectsCmd(p *envid.Profile) tea.Cmd {
	return func() tea.Msg {
		var msg envApplyResultMsg
		msg.favoritesTotal = len(p.Favorites)
		msg.packsTotal = len(p.Packs)

		if len(p.Favorites) > 0 {
			added, err := mergeFavorites(p.Favorites)
			if err != nil {
				msg.err = fmt.Errorf("favorites: %w", err)
				return msg
			}
			msg.favoritesAdded = added
		}

		if len(p.Packs) > 0 {
			added, err := mergeCustomPacks(p.Packs)
			if err != nil {
				msg.err = fmt.Errorf("packs: %w", err)
				return msg
			}
			msg.packsAdded = added
		}
		return msg
	}
}

// mergeFavorites is the additive merge used by applyEnvSideEffectsCmd
// — same semantics as cli.applyFavorites but without stderr writes.
// Returns the count of newly-added names so the caller can summarise.
func mergeFavorites(names []string) (int, error) {
	existing, err := favorites.Load()
	if err != nil {
		return 0, err
	}
	have := make(map[string]struct{}, len(existing))
	merged := make([]string, 0, len(existing))
	for _, n := range existing {
		n = strings.TrimSpace(n)
		if n == "" {
			continue
		}
		if _, dup := have[n]; dup {
			continue
		}
		have[n] = struct{}{}
		merged = append(merged, n)
	}
	added := 0
	for _, n := range names {
		n = strings.TrimSpace(n)
		if n == "" {
			continue
		}
		if _, dup := have[n]; dup {
			continue
		}
		have[n] = struct{}{}
		merged = append(merged, n)
		added++
	}
	if added == 0 {
		return 0, nil
	}
	return added, favorites.Save(merged)
}

// mergeCustomPacks is the additive merge used by applyEnvSideEffectsCmd.
// Mirrors cli.applyPacks: existing names win, env contributes only the
// names that aren't already present.
func mergeCustomPacks(packs []envid.Pack) (int, error) {
	existing, err := custompacks.Load()
	if err != nil {
		return 0, err
	}
	have := make(map[string]struct{}, len(existing))
	for _, p := range existing {
		key := strings.ToLower(strings.TrimSpace(p.Name))
		if key != "" {
			have[key] = struct{}{}
		}
	}
	added := 0
	merged := make([]registry.Pack, 0, len(existing))
	merged = append(merged, existing...)
	for _, p := range packs {
		name := strings.TrimSpace(p.Name)
		if name == "" {
			continue
		}
		key := strings.ToLower(name)
		if _, dup := have[key]; dup {
			continue
		}
		toolNames := make([]string, 0, len(p.Tools))
		for _, tn := range p.Tools {
			tn = strings.TrimSpace(tn)
			if tn != "" {
				toolNames = append(toolNames, tn)
			}
		}
		if len(toolNames) == 0 {
			continue
		}
		have[key] = struct{}{}
		merged = append(merged, registry.Pack{
			Name:        name,
			DisplayName: strings.TrimSpace(p.DisplayName),
			ToolNames:   toolNames,
		})
		added++
	}
	if added == 0 {
		return 0, nil
	}
	return added, custompacks.Save(merged)
}

// buildEnvApplyPlanCmd takes an env Profile and produces backupItems
// for the existing import-installer flow. Mirrors buildImportPlanCmd
// but skips the file-read step — the profile is already in memory.
func buildEnvApplyPlanCmd(svc *service.ToolService, p *envid.Profile) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		regTools, _, err := svc.ScanOnly(ctx)
		if err != nil {
			return backupPlanMsg{err: fmt.Errorf("scanning PATH: %w", err)}
		}
		regMap := registry.ToolMap(regTools)

		var items []backupItem
		for _, t := range p.Tools {
			rt, exists := regMap[t.Name]
			if exists && rt.IsInstalled() {
				items = append(items, backupItem{
					name:    t.Name,
					source:  "—",
					status:  backupSkipped,
					errMsg:  "already installed",
					display: rt.DisplayName,
				})
				continue
			}

			var pkgs registry.PackageIDs
			if exists {
				pkgs = rt.Packages
			} else {
				// No catalog data — env profile carries only
				// name/version/source/category. Without
				// package IDs we can't build an install
				// command, so report the gap instead of
				// silently dropping the tool.
				items = append(items, backupItem{
					name:   t.Name,
					status: backupSkipped,
					errMsg: "not in marketplace catalog",
				})
				continue
			}

			src := pkgs.BestInstallSource()
			if t.Source != "" {
				preferred := registry.InstallSource(t.Source)
				if args := pkgs.InstallArgs(preferred); args != nil {
					src = preferred
				}
			}
			args := pkgs.InstallArgs(src)
			if args == nil {
				reason := "no package for " + runtime.GOOS
				if src == "" && pkgs.HasAnyPackageForOS() {
					reason = "no supported package manager installed"
				}
				items = append(items, backupItem{
					name:   t.Name,
					status: backupSkipped,
					errMsg: reason,
				})
				continue
			}

			items = append(items, backupItem{
				name:     t.Name,
				display:  rt.DisplayName,
				cmdArgs:  args,
				source:   string(src),
				status:   backupPending,
				selected: true,
			})
		}
		return backupPlanMsg{items: items}
	}
}

// formatEnvShow renders an env Profile as the same human-readable
// summary `klim env show` produces. We intentionally don't share the
// CLI helper because that one writes to stderr; here we want a string
// for inline display.
func formatEnvShow(p *envid.Profile) string {
	if p == nil {
		return ""
	}
	var b strings.Builder
	hash := envid.ComputeHash(p)
	fmt.Fprintf(&b, "klim version: %s\n", p.Clim.Version)
	if p.Clim.Commit != "" {
		fmt.Fprintf(&b, "klim commit:  %s\n", p.Clim.Commit)
	}
	fmt.Fprintf(&b, "hash:         %s\n", hash)
	fmt.Fprintf(&b, "OS:           %s/%s", p.OS.GOOS, p.OS.Arch)
	if p.OS.Distro != "" {
		fmt.Fprintf(&b, " (%s)", p.OS.Distro)
	}
	b.WriteString("\n")

	if len(p.PackageManagers) > 0 {
		keys := make([]string, 0, len(p.PackageManagers))
		for k := range p.PackageManagers {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		var avail, miss []string
		for _, k := range keys {
			if p.PackageManagers[k] {
				avail = append(avail, k)
			} else {
				miss = append(miss, k)
			}
		}
		availStr := "—"
		if len(avail) > 0 {
			availStr = strings.Join(avail, ",")
		}
		missStr := "—"
		if len(miss) > 0 {
			missStr = strings.Join(miss, ",")
		}
		fmt.Fprintf(&b, "package mgrs: available=%s   missing=%s\n", availStr, missStr)
	}
	if len(p.Tools) > 0 {
		fmt.Fprintf(&b, "\nTools (%d):\n", len(p.Tools))
		for _, t := range p.Tools {
			ver := t.Version
			if ver == "" {
				ver = "?"
			}
			src := t.Source
			if src == "" {
				src = "?"
			}
			fmt.Fprintf(&b, "  · %-25s %-12s via %s\n", t.Name, ver, src)
		}
	}
	if len(p.Favorites) > 0 {
		fmt.Fprintf(&b, "\nFavorites (%d): %s\n", len(p.Favorites), strings.Join(p.Favorites, ", "))
	}
	if len(p.Packs) > 0 {
		fmt.Fprintf(&b, "\nCustom packs (%d):\n", len(p.Packs))
		for _, pk := range p.Packs {
			fmt.Fprintf(&b, "  · %-20s [%s]\n", pk.Name, strings.Join(pk.Tools, ", "))
		}
	}
	fmt.Fprintf(&b, "\nAudit:    %d warnings, %d infos\n", p.Security.AuditWarnings, p.Security.AuditInfos)
	fmt.Fprintf(&b, "Verdicts: clean=%d watch=%d risk=%d unknown=%d\n",
		p.Security.Verdicts.Clean, p.Security.Verdicts.Watch,
		p.Security.Verdicts.Risk, p.Security.Verdicts.Unknown)
	return b.String()
}

// formatEnvDiff renders the diff between two profiles as the same text
// `klim env diff` writes to stderr. Like formatEnvShow, we duplicate
// the CLI logic to avoid coupling rendering to *os.File.
func formatEnvDiff(local, remote *envid.Profile) string {
	if local == nil || remote == nil {
		return ""
	}
	var b strings.Builder
	localHash := envid.ComputeHash(local)
	remoteHash := envid.ComputeHash(remote)
	fmt.Fprintf(&b, "local:  %s   tools=%d favorites=%d packs=%d\n",
		localHash, len(local.Tools), len(local.Favorites), len(local.Packs))
	fmt.Fprintf(&b, "remote: %s   tools=%d favorites=%d packs=%d\n",
		remoteHash, len(remote.Tools), len(remote.Favorites), len(remote.Packs))

	if localHash == remoteHash {
		b.WriteString("\nSame hash — environments match.\n")
		return b.String()
	}

	localTools := make(map[string]envid.Tool, len(local.Tools))
	for _, t := range local.Tools {
		localTools[t.Name] = t
	}
	remoteTools := make(map[string]envid.Tool, len(remote.Tools))
	for _, t := range remote.Tools {
		remoteTools[t.Name] = t
	}

	var onlyLocal, onlyRemote, drift []string
	for n, lt := range localTools {
		rt, ok := remoteTools[n]
		if !ok {
			onlyLocal = append(onlyLocal, n)
			continue
		}
		if lt.Version != rt.Version && lt.Version != "" && rt.Version != "" {
			drift = append(drift, fmt.Sprintf("%s (local=%s, remote=%s)", n, lt.Version, rt.Version))
		}
	}
	for n := range remoteTools {
		if _, ok := localTools[n]; !ok {
			onlyRemote = append(onlyRemote, n)
		}
	}
	sort.Strings(onlyLocal)
	sort.Strings(onlyRemote)
	sort.Strings(drift)

	if len(onlyRemote) > 0 {
		fmt.Fprintf(&b, "\nTools to install (%d):\n", len(onlyRemote))
		for _, n := range onlyRemote {
			fmt.Fprintf(&b, "  + %s\n", n)
		}
	}
	if len(onlyLocal) > 0 {
		fmt.Fprintf(&b, "\nTools you have that remote doesn't (%d):\n", len(onlyLocal))
		for _, n := range onlyLocal {
			fmt.Fprintf(&b, "  - %s\n", n)
		}
	}
	if len(drift) > 0 {
		fmt.Fprintf(&b, "\nVersion drift (%d):\n", len(drift))
		for _, d := range drift {
			fmt.Fprintf(&b, "  ~ %s\n", d)
		}
	}
	if added := envFavoritesDiff(remote.Favorites, local.Favorites); len(added) > 0 {
		fmt.Fprintf(&b, "\nFavorites to add (%d): %s\n", len(added), strings.Join(added, ", "))
	}
	if added := envPacksDiff(remote.Packs, local.Packs); len(added) > 0 {
		fmt.Fprintf(&b, "\nCustom packs to add (%d): %s\n", len(added), strings.Join(added, ", "))
	}
	return b.String()
}

func envFavoritesDiff(want, have []string) []string {
	hv := make(map[string]struct{}, len(have))
	for _, h := range have {
		hv[h] = struct{}{}
	}
	var out []string
	for _, w := range want {
		if _, ok := hv[w]; !ok {
			out = append(out, w)
		}
	}
	sort.Strings(out)
	return out
}

func envPacksDiff(want, have []envid.Pack) []string {
	hv := make(map[string]struct{}, len(have))
	for _, h := range have {
		hv[h.Name] = struct{}{}
	}
	var out []string
	for _, w := range want {
		if _, ok := hv[w.Name]; !ok {
			out = append(out, w.Name)
		}
	}
	sort.Strings(out)
	return out
}

// fixedWidth is shared from view.go; this file uses it through the
// package-level helper. The local helper below was removed when the
// idle-view layout switched to detailLabelStyle + the package-wide
// fixedWidth — keeping it here would shadow the shared name and
// confuse future readers.
