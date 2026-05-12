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
// tickFrame again at the end of its work, forming a stable loop
// that ends only when Quit is dispatched.
func tickFrame() tea.Cmd {
	return tea.Tick(frameInterval, func(t time.Time) tea.Msg {
		return frameMsg(t)
	})
}
