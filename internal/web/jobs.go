package web

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"time"
)

// JobStatus is a job's lifecycle state.
type JobStatus string

const (
	JobStatusRunning JobStatus = "running"
	JobStatusSuccess JobStatus = "success"
	JobStatusFailed  JobStatus = "failed"
)

// JobAction enumerates the package-manager actions a Job can run.
type JobAction string

const (
	ActionInstall JobAction = "install"
	ActionUpgrade JobAction = "upgrade"
	ActionRemove  JobAction = "remove"
)

// Job tracks one package-manager invocation. Output is captured
// line-by-line so SSE subscribers can replay history and stream new
// lines uniformly. The struct is mutated under jobManager's lock.
type Job struct {
	ID        string    `json:"id"`
	Action    JobAction `json:"action"`
	Tool      string    `json:"tool"`
	Source    string    `json:"source"`
	Cmd       []string  `json:"cmd"`
	Status    JobStatus `json:"status"`
	StartedAt time.Time `json:"started_at"`
	EndedAt   time.Time `json:"ended_at,omitempty"`
	ExitCode  int       `json:"exit_code"`
	Output    []string  `json:"output"`
	Err       string    `json:"error,omitempty"`
}

// jobEvent is one SSE message. Type is "line", "done", or "error".
// Line is set only when Type == "line".
type jobEvent struct {
	Type string
	Line string
}

// Executor abstracts running a command and streaming its combined
// stdout/stderr line-by-line. The production implementation wraps
// exec.Command; tests substitute a canned implementation so handler
// tests don't shell out for real.
type Executor interface {
	// Run starts cmd, returning a read-only channel of output lines and
	// a result channel that fires exactly once with the exit error
	// (nil on success). Both channels close when the process exits.
	Run(ctx context.Context, args []string) (<-chan string, <-chan error)
}

// execExecutor is the production Executor. It runs a real subprocess
// with stdout and stderr both routed into a single line-buffered
// stream so the user sees the same interleaving they'd see in a
// terminal.
type execExecutor struct{}

func newExecExecutor() Executor { return execExecutor{} }

func (execExecutor) Run(ctx context.Context, args []string) (<-chan string, <-chan error) {
	out := make(chan string, 32)
	done := make(chan error, 1)
	if len(args) == 0 {
		out <- "[no command]"
		close(out)
		done <- errors.New("empty command")
		close(done)
		return out, done
	}
	cmd := exec.CommandContext(ctx, args[0], args[1:]...) //nolint:gosec // args come from registry.PackageIDs templates which we control.
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		out <- fmt.Sprintf("[stdout pipe: %v]", err)
		close(out)
		done <- err
		close(done)
		return out, done
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		out <- fmt.Sprintf("[stderr pipe: %v]", err)
		close(out)
		done <- err
		close(done)
		return out, done
	}
	if err := cmd.Start(); err != nil {
		out <- fmt.Sprintf("[start: %v]", err)
		close(out)
		done <- err
		close(done)
		return out, done
	}
	var wg sync.WaitGroup
	scan := func(r io.Reader) {
		defer wg.Done()
		s := bufio.NewScanner(r)
		s.Buffer(make([]byte, 64*1024), 1024*1024)
		for s.Scan() {
			out <- s.Text()
		}
	}
	wg.Add(2)
	go scan(stdout)
	go scan(stderr)
	go func() {
		wg.Wait()
		err := cmd.Wait()
		close(out)
		done <- err
		close(done)
	}()
	return out, done
}

// jobManager owns the live Job catalog and SSE subscriber registry.
// Methods are safe for concurrent use.
type jobManager struct {
	mu      sync.Mutex
	jobs    map[string]*Job
	subs    map[string][]chan jobEvent
	running map[string]string // tool name → running job ID
	exec    Executor
}

func newJobManager(exec Executor) *jobManager {
	return &jobManager{
		jobs:    make(map[string]*Job),
		subs:    make(map[string][]chan jobEvent),
		running: make(map[string]string),
		exec:    exec,
	}
}

// ErrJobInProgress means a job is already running for the same tool.
// Callers should surface the existing job ID so the user can watch it
// rather than starting a duplicate.
type ErrJobInProgress struct {
	Tool        string
	ExistingID  string
	ExistingFor JobAction
}

func (e *ErrJobInProgress) Error() string {
	return fmt.Sprintf("a %s job for %q is already running (id %s)", e.ExistingFor, e.Tool, e.ExistingID)
}

// Start spawns a new job. The returned Job is a snapshot safe for
// callers to read; further updates land in the Manager-owned object.
//
// Returns *ErrJobInProgress if the same tool already has a running
// job — running two package-manager actions on the same tool at once
// fights with most managers' internal locks, so we surface the
// existing job ID and let the caller redirect the user to it.
func (m *jobManager) Start(ctx context.Context, action JobAction, tool, source string, args []string) (*Job, error) {
	if len(args) == 0 {
		return nil, errors.New("jobManager.Start: empty command args")
	}
	id, err := newJobID()
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	if existingID, busy := m.running[tool]; busy {
		existing := m.jobs[existingID]
		m.mu.Unlock()
		var existingFor JobAction
		if existing != nil {
			existingFor = existing.Action
		}
		return nil, &ErrJobInProgress{Tool: tool, ExistingID: existingID, ExistingFor: existingFor}
	}
	job := &Job{
		ID:        id,
		Action:    action,
		Tool:      tool,
		Source:    source,
		Cmd:       append([]string(nil), args...),
		Status:    JobStatusRunning,
		StartedAt: time.Now().UTC(),
		Output:    []string{},
	}
	m.jobs[id] = job
	m.running[tool] = id
	m.mu.Unlock()
	go m.run(ctx, job)
	return job.snapshot(), nil
}

// run consumes the executor's output channel into the job's Output
// slice and notifies subscribers as lines arrive.
func (m *jobManager) run(ctx context.Context, job *Job) {
	lines, done := m.exec.Run(ctx, job.Cmd)
	for line := range lines {
		m.appendOutput(job.ID, line)
	}
	err := <-done
	m.mu.Lock()
	job.EndedAt = time.Now().UTC()
	if err != nil {
		job.Status = JobStatusFailed
		job.Err = err.Error()
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			job.ExitCode = exit.ExitCode()
		} else {
			job.ExitCode = -1
		}
	} else {
		job.Status = JobStatusSuccess
		job.ExitCode = 0
	}
	// Release the per-tool slot so a follow-up action can run.
	if cur, ok := m.running[job.Tool]; ok && cur == job.ID {
		delete(m.running, job.Tool)
	}
	m.mu.Unlock()
	m.broadcast(job.ID, jobEvent{Type: "done"})
	m.closeSubs(job.ID)
}

func (m *jobManager) appendOutput(id, line string) {
	m.mu.Lock()
	if job, ok := m.jobs[id]; ok {
		job.Output = append(job.Output, line)
	}
	m.mu.Unlock()
	m.broadcast(id, jobEvent{Type: "line", Line: line})
}

func (m *jobManager) broadcast(id string, ev jobEvent) {
	m.mu.Lock()
	subs := append([]chan jobEvent(nil), m.subs[id]...)
	m.mu.Unlock()
	for _, ch := range subs {
		// Non-blocking send so a slow client can't stall the runner.
		// The client will reconnect and replay via the snapshot path.
		select {
		case ch <- ev:
		default:
		}
	}
}

func (m *jobManager) closeSubs(id string) {
	m.mu.Lock()
	subs := m.subs[id]
	delete(m.subs, id)
	m.mu.Unlock()
	for _, ch := range subs {
		close(ch)
	}
}

// Snapshot returns a copy of the job's current state, or nil if id is
// unknown. The copy is safe to read without holding the manager lock.
func (m *jobManager) Snapshot(id string) *Job {
	m.mu.Lock()
	defer m.mu.Unlock()
	job, ok := m.jobs[id]
	if !ok {
		return nil
	}
	return job.snapshot()
}

// Subscribe registers a channel that receives events for id. The
// returned snapshot reflects the job state at subscribe time so the
// caller can replay history before processing live events. The
// returned channel is closed when the job ends (or immediately, if
// it's already done).
func (m *jobManager) Subscribe(id string) (*Job, <-chan jobEvent) {
	m.mu.Lock()
	job, ok := m.jobs[id]
	if !ok {
		m.mu.Unlock()
		return nil, nil
	}
	ch := make(chan jobEvent, 16)
	if job.Status == JobStatusRunning {
		m.subs[id] = append(m.subs[id], ch)
	} else {
		// Job is already finished — close the channel right away so
		// the consumer's range loop exits after replaying history.
		close(ch)
	}
	snap := job.snapshot()
	m.mu.Unlock()
	return snap, ch
}

func (j *Job) snapshot() *Job {
	out := *j
	out.Cmd = append([]string(nil), j.Cmd...)
	out.Output = append([]string(nil), j.Output...)
	return &out
}

func newJobID() (string, error) {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}
