package tui

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// batchItemStatus tracks the state of a single item in a batch operation.
type batchItemStatus int

const (
	batchPending batchItemStatus = iota
	batchRunning
	batchDone
	batchFailed
	batchSkipped
)

// batchItem represents a single tool operation in a batch.
type batchItem struct {
	name    string          // tool name (for display)
	display string          // display name
	cmdArgs []string        // command to execute
	source  string          // package manager source
	status  batchItemStatus // current status
	errMsg  string          // error message (failed/skipped reason)
}

// batchOp manages a sequential batch of tool operations (install/upgrade/remove).
// Currently used by the batch upgrade flow on the Updates tab.
type batchOp struct {
	label     string      // operation label: "Installing", "Upgrading", "Importing"
	items     []batchItem // all items in the batch
	running   bool        // true while the batch is in progress
	done      int         // count of completed/skipped/failed items
	cancelled bool        // true if user cancelled
}

// batchOpDoneMsg is sent when a single item in the batch completes.
type batchOpDoneMsg struct {
	idx int   // index into batchOp.items
	err error // nil = success
}

// newBatchOp creates a new batch operation with the given label and items.
// Items already in a terminal state (skipped or failed) are counted as done.
func newBatchOp(label string, items []batchItem) *batchOp {
	done := 0
	for _, item := range items {
		if item.status == batchSkipped || item.status == batchFailed {
			done++
		}
	}
	return &batchOp{
		label:   label,
		items:   items,
		running: true,
		done:    done,
	}
}

// next finds the next pending item, marks it as running, and returns a
// command to execute it. Returns nil when no pending items remain.
func (b *batchOp) next() tea.Cmd {
	for i := range b.items {
		if b.items[i].status == batchPending {
			b.items[i].status = batchRunning
			return execBatchItemCmd(i, b.items[i].cmdArgs)
		}
	}
	return nil
}

// complete marks an item as done or failed. Only transitions from
// non-terminal states (pending/running) to avoid double-counting
// when an item was already skipped.
func (b *batchOp) complete(idx int, err error) {
	if idx < 0 || idx >= len(b.items) {
		return
	}
	prev := b.items[idx].status
	if prev != batchPending && prev != batchRunning {
		return // already terminal (skipped/done/failed)
	}
	if err != nil {
		b.items[idx].status = batchFailed
		b.items[idx].errMsg = err.Error()
	} else {
		b.items[idx].status = batchDone
	}
	b.done++
}

// cancel marks all remaining pending items as skipped.
func (b *batchOp) cancel() {
	b.cancelled = true
	for i := range b.items {
		if b.items[i].status == batchPending {
			b.items[i].status = batchSkipped
			b.items[i].errMsg = "cancelled"
			b.done++
		}
	}
}

// skip marks the currently running item as skipped. The process may
// still be running — complete() will be a no-op when it arrives since
// the item is already in a terminal state.
func (b *batchOp) skip() {
	for i := range b.items {
		if b.items[i].status == batchRunning {
			b.items[i].status = batchSkipped
			b.items[i].errMsg = "skipped"
			b.done++
			return
		}
	}
}

// finish marks the batch as no longer running.
func (b *batchOp) finish() {
	b.running = false
}

// isRunning returns true if the batch is actively executing items.
func (b *batchOp) isRunning() bool {
	return b.running
}

// progress returns "3/8" style progress string.
func (b *batchOp) progress() string {
	return fmt.Sprintf("%d/%d", b.done, len(b.items))
}

// currentName returns the display name of the currently running item.
func (b *batchOp) currentName() string {
	for _, item := range b.items {
		if item.status == batchRunning {
			if item.display != "" {
				return item.display
			}
			return item.name
		}
	}
	return ""
}

// statusLine returns a status message like "Installing tool-name (3/8)..."
func (b *batchOp) statusLine() string {
	name := b.currentName()
	if name == "" {
		return fmt.Sprintf("%s... (%s)", b.label, b.progress())
	}
	return fmt.Sprintf("%s %s (%s)...", b.label, name, b.progress())
}

// summary returns a completion summary like "✓ 7 installed, 1 failed, 2 skipped".
func (b *batchOp) summary() string {
	var succeeded, failed, skipped int
	for _, item := range b.items {
		switch item.status {
		case batchDone:
			succeeded++
		case batchFailed:
			failed++
		case batchSkipped:
			skipped++
		}
	}

	var parts []string
	if succeeded > 0 {
		parts = append(parts, fmt.Sprintf("%d succeeded", succeeded))
	}
	if failed > 0 {
		parts = append(parts, fmt.Sprintf("%d failed", failed))
	}
	if skipped > 0 {
		parts = append(parts, fmt.Sprintf("%d skipped", skipped))
	}

	prefix := "✓"
	if b.cancelled {
		prefix = "⚠ Cancelled —"
	} else if failed > 0 {
		prefix = "⚠"
	}

	if len(parts) == 0 {
		return prefix + " Nothing to do"
	}
	return prefix + " " + strings.Join(parts, ", ")
}

// execBatchItemCmd runs a single batch item command.
func execBatchItemCmd(idx int, args []string) tea.Cmd {
	if len(args) == 0 {
		return func() tea.Msg {
			return batchOpDoneMsg{idx: idx, err: errors.New("no command")}
		}
	}
	cmd := exec.Command(args[0], args[1:]...)
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		return batchOpDoneMsg{idx: idx, err: err}
	})
}
