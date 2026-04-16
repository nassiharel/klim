// Package progress provides a simple CLI spinner for non-interactive commands.
// The spinner writes to stderr and auto-detects TTY mode: animated frames when
// stderr is a terminal, static lines otherwise.
package progress

import (
	"fmt"
	"os"
	"sync"
	"time"

	"golang.org/x/term"
)

// Braille spinner frames (same as Bubbletea's spinner.Dot).
var frames = [...]string{"⣾", "⣽", "⣻", "⢿", "⡿", "⣟", "⣯", "⣷"}

// Spinner displays an animated progress indicator on stderr.
type Spinner struct {
	mu      sync.Mutex
	wg      sync.WaitGroup
	msg     string
	frame   int
	done    chan struct{}
	stopped bool
	isTTY   bool
}

// New creates and starts a spinner with the given message.
func New(msg string) *Spinner {
	s := &Spinner{
		msg:   msg,
		done:  make(chan struct{}),
		isTTY: term.IsTerminal(int(os.Stderr.Fd())), //nolint:gosec // fd fits int on all supported platforms
	}
	if s.isTTY {
		s.wg.Add(1)
		go s.animate()
	} else {
		fmt.Fprintf(os.Stderr, "  %s\n", msg)
	}
	return s
}

func (s *Spinner) animate() {
	defer s.wg.Done()
	ticker := time.NewTicker(80 * time.Millisecond)
	defer ticker.Stop()

	s.render()
	for {
		select {
		case <-s.done:
			return
		case <-ticker.C:
			s.mu.Lock()
			s.frame = (s.frame + 1) % len(frames)
			s.mu.Unlock()
			s.render()
		}
	}
}

func (s *Spinner) render() {
	s.mu.Lock()
	msg := s.msg
	frame := frames[s.frame]
	s.mu.Unlock()

	// \r returns to start of line; \033[K clears to end of line.
	fmt.Fprintf(os.Stderr, "\r\033[K  %s %s", frame, msg)
}

// Update changes the spinner message without stopping it.
func (s *Spinner) Update(msg string) {
	s.mu.Lock()
	s.msg = msg
	s.mu.Unlock()

	if !s.isTTY {
		fmt.Fprintf(os.Stderr, "  %s\n", msg)
	}
}

// Done stops the spinner and prints a success message with a ✓ prefix.
func (s *Spinner) Done(msg string) {
	s.stop()
	if s.isTTY {
		fmt.Fprintf(os.Stderr, "\r\033[K  ✓ %s\n", msg)
	} else {
		fmt.Fprintf(os.Stderr, "  ✓ %s\n", msg)
	}
}

// Fail stops the spinner and prints an error message with a ✗ prefix.
func (s *Spinner) Fail(msg string) {
	s.stop()
	if s.isTTY {
		fmt.Fprintf(os.Stderr, "\r\033[K  ✗ %s\n", msg)
	} else {
		fmt.Fprintf(os.Stderr, "  ✗ %s\n", msg)
	}
}

func (s *Spinner) stop() {
	s.mu.Lock()
	if !s.stopped {
		s.stopped = true
		close(s.done)
	}
	s.mu.Unlock()
	s.wg.Wait() // wait for animate goroutine to exit before printing final line
}
