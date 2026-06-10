package cli

import (
	"context"
	"os"

	tea "charm.land/bubbletea/v2"
	"golang.org/x/term"

	"github.com/nassiharel/klim/internal/agents"
	"github.com/nassiharel/klim/internal/sessionstui"
)

// init wires the sessionstui package into the CLI dispatch. Done in
// init so the bare `klim agents sessions` command and `--watch` flag
// can hand off to bubbletea without agents_sessions.go importing the
// TUI package directly (which would couple the CLI test surface to
// terminal-only types).
func init() {
	isStdoutTTY = func() bool {
		return term.IsTerminal(int(os.Stdout.Fd()))
	}
	launchSessionsTUIImpl = func(ctx context.Context) error {
		svc := newAgentsService()
		model := sessionstui.New(svc)
		// AltScreen is requested by the model's View() so we omit
		// the program-level option here.
		prog := tea.NewProgram(
			model,
			tea.WithContext(ctx),
		)
		_, err := prog.Run()
		_ = agents.SessionStatusActive // sentinel to keep agents import non-dead
		return err
	}
}