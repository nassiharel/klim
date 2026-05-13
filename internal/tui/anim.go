package tui

import (
	"time"

	tea "charm.land/bubbletea/v2"
)

// frameMsg is the universal animation tick. Sent on a steady ~100ms
// cadence so every animated element (HUD pulse, cyber spinner, boot
// splash scanline) can derive its frame purely from m.animFrame
// without needing its own ticker.
//
// At 100ms cadence the renderer runs at 10fps for animated elements —
// well below the 60fps cost of typical bubbletea apps, but visually
// indistinguishable from "smooth" for the kind of slow pulses /
// scanlines used here.
type frameMsg time.Time

// frameInterval is the inter-tick delay. 100ms gives 10fps, which is
// enough for the slow ambient motion we want without taxing
// terminals (every frame is a full re-render).
const frameInterval = 100 * time.Millisecond

// tickFrame returns a tea.Cmd that schedules the next frameMsg.
// Designed to be self-replicating: every frameMsg handler calls
// tickFrame again at the end of its work as long as
// Model.animationActive returns true, forming a stable loop that
// pauses cleanly when the screen has nothing left to animate.
func tickFrame() tea.Cmd {
	return tea.Tick(frameInterval, func(t time.Time) tea.Msg {
		return frameMsg(t)
	})
}

// animationActive reports whether anything currently rendered on
// screen needs the 10 fps tick to look right. When false, the
// frameMsg handler stops rescheduling — so idle terminals don't
// pay any per-second render cost.
//
// Animated states:
//   - phaseLoading: boot splash logo reveal + spinner + progress bar
//   - phaseResolving: HUD shows live "// RESOLVE (N)" label
//   - pending > 0: still resolving version data; HUD dot pulses
//   - activeBatch != nil && isRunning(): batch op in flight
//   - fixModal in the running state: spinner ticks while command runs
func (m Model) animationActive() bool {
	if m.phase != phaseDone {
		return true
	}
	if m.pending > 0 {
		return true
	}
	if m.activeBatch != nil && m.activeBatch.isRunning() {
		return true
	}
	if m.fixModal.Open && m.fixModal.State == fixModalRunning {
		return true
	}
	return false
}

// ensureAnimating returns a tickFrame command iff the tick chain
// has stopped (animTicking == false). The caller is responsible for
// including the returned command in its returned tea.Batch.
// No-op when a tick is already in flight, so it's safe to call from
// any state-transition path without worrying about doubling the
// effective frame rate.
func (m *Model) ensureAnimating() tea.Cmd {
	if m.animTicking {
		return nil
	}
	m.animTicking = true
	return tickFrame()
}
