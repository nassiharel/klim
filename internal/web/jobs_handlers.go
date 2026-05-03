package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/nassiharel/clim/internal/registry"
)

// resolveAction picks the package-manager command for action+tool. It
// returns the source it picked so the user sees which manager will run
// (winget, brew, etc.). Returns an error if no package is configured
// for any source on the host OS.
//
// Choice rules:
//   - install: best source for current OS (highest priority among
//     configured + available managers).
//   - upgrade / remove: prefer the source the tool is currently
//     installed from; fall back to the best source if not installed.
func resolveAction(action JobAction, tool registry.Tool) (source string, args []string, err error) {
	pkgs := tool.Packages
	pickInstalled := func() registry.InstallSource {
		if pi := tool.PrimaryInstance(); pi != nil && pi.Source != "" {
			return pi.Source
		}
		return ""
	}
	switch action {
	case ActionInstall:
		src := pkgs.BestInstallSource()
		if src == "" {
			return "", nil, fmt.Errorf("no install source available for %q on this OS", tool.Name)
		}
		args = pkgs.InstallArgs(src)
		source = string(src)
	case ActionUpgrade:
		src := pickInstalled()
		if src == "" {
			src = pkgs.BestInstallSource()
		}
		if src == "" {
			return "", nil, fmt.Errorf("no upgrade source available for %q", tool.Name)
		}
		args = pkgs.UpgradeArgs(src)
		source = string(src)
	case ActionRemove:
		src := pickInstalled()
		if src == "" {
			return "", nil, fmt.Errorf("%q is not installed", tool.Name)
		}
		args = pkgs.RemoveArgs(src)
		source = string(src)
	default:
		return "", nil, fmt.Errorf("unknown action %q", action)
	}
	if len(args) == 0 {
		return "", nil, fmt.Errorf("no %s command available for %q on source %s", action, tool.Name, source)
	}
	return source, args, nil
}

// startActionJob is the shared entry point used by both the JSON API
// and the HTML form handlers. It validates the action + tool and
// schedules a Job on the manager.
func (s *Server) startActionJob(ctx context.Context, action JobAction, name string) (*Job, error) {
	if name == "" {
		return nil, fmt.Errorf("missing tool name")
	}
	switch action {
	case ActionInstall, ActionUpgrade, ActionRemove:
	default:
		return nil, fmt.Errorf("unknown action %q (expected install, upgrade, or remove)", action)
	}
	tool, err := s.loader.LoadTool(ctx, name)
	if err != nil {
		return nil, err
	}
	source, args, err := resolveAction(action, tool)
	if err != nil {
		return nil, err
	}
	// We deliberately use a context.Background() for the long-running
	// job rather than ctx — the HTTP request that triggered the job
	// usually finishes (303 redirect) long before the package manager
	// does, and we don't want the redirect's request cancellation to
	// kill the install.
	return s.jobs.Start(context.Background(), action, tool.Name, source, args)
}

// apiJobsCreate creates a new job from a JSON body. Returns the job
// snapshot. Returns 409 Conflict with the existing job ID when a job
// for the same tool is already running.
func (s *Server) apiJobsCreate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Action JobAction `json:"action"`
		Tool   string    `json:"tool"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		s.jsonError(w, http.StatusBadRequest, fmt.Sprintf("decode body: %v", err))
		return
	}
	job, err := s.startActionJob(r.Context(), body.Action, body.Tool)
	if err != nil {
		var inProg *ErrJobInProgress
		if errors.As(err, &inProg) {
			w.Header().Set("Location", "/jobs/"+inProg.ExistingID)
			s.writeJSON(w, http.StatusConflict, map[string]any{
				"error":           err.Error(),
				"existing_job_id": inProg.ExistingID,
				"existing_action": inProg.ExistingFor,
				"redirect_to":     "/jobs/" + inProg.ExistingID,
			})
			return
		}
		status := http.StatusBadRequest
		if isNotFound(err) {
			status = http.StatusNotFound
		}
		s.jsonError(w, status, err.Error())
		return
	}
	w.Header().Set("Location", "/jobs/"+job.ID)
	s.writeJSON(w, http.StatusAccepted, job)
}

// pageStartJob handles the HTML form POST from the tool detail page.
// On success it 303s to the job page where the user watches progress.
// When a job for the same tool is already running, redirects to that
// existing job rather than starting a duplicate.
func (s *Server) pageStartJob(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	action := JobAction(strings.ToLower(r.PathValue("action")))
	job, err := s.startActionJob(r.Context(), action, name)
	if err != nil {
		var inProg *ErrJobInProgress
		if errors.As(err, &inProg) {
			http.Redirect(w, r, "/jobs/"+inProg.ExistingID, http.StatusSeeOther)
			return
		}
		status := http.StatusBadRequest
		if isNotFound(err) {
			status = http.StatusNotFound
		}
		s.serveError(w, r, err, status)
		return
	}
	http.Redirect(w, r, "/jobs/"+job.ID, http.StatusSeeOther)
}

// apiJobsGet returns the job snapshot in JSON. 404 if unknown.
func (s *Server) apiJobsGet(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	job := s.jobs.Snapshot(id)
	if job == nil {
		s.jsonError(w, http.StatusNotFound, fmt.Sprintf("job %q not found", id))
		return
	}
	s.writeJSON(w, http.StatusOK, job)
}

// apiJobsStream returns a Server-Sent Events stream of the job's
// output as it arrives. Subscribers also receive the snapshot history
// as a sequence of "line" events on connect, so the client doesn't
// have to fetch the snapshot separately to backfill late attaches.
func (s *Server) apiJobsStream(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	snap, ch := s.jobs.Subscribe(id)
	if snap == nil {
		s.jsonError(w, http.StatusNotFound, fmt.Sprintf("job %q not found", id))
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		s.jsonError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	// Replay history, then stream live events. We replay even if the
	// job is already finished so a client that hits /stream after
	// completion still gets the full transcript.
	for _, line := range snap.Output {
		writeSSE(w, "line", line)
	}
	if snap.Status != JobStatusRunning {
		writeSSE(w, "done", string(snap.Status))
		flusher.Flush()
		return
	}
	flusher.Flush()

	// Heartbeat to keep proxies / browsers from closing idle streams
	// during long package-manager pauses.
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case ev, open := <-ch:
			if !open {
				snap = s.jobs.Snapshot(id)
				if snap != nil {
					writeSSE(w, "done", string(snap.Status))
				}
				flusher.Flush()
				return
			}
			switch ev.Type {
			case "line":
				writeSSE(w, "line", ev.Line)
			case "done":
				snap = s.jobs.Snapshot(id)
				if snap != nil {
					writeSSE(w, "done", string(snap.Status))
				}
			}
			flusher.Flush()
		case <-heartbeat.C:
			// SSE comments (lines starting with ":") are heartbeats by
			// convention — clients ignore them.
			_, _ = w.Write([]byte(": heartbeat\n\n"))
			flusher.Flush()
		}
	}
}

// writeSSE writes one named SSE event. Escapes embedded newlines so a
// multi-line payload doesn't terminate the event prematurely (each
// physical newline in data: is treated as a record terminator by the
// browser EventSource API after the next blank line).
func writeSSE(w http.ResponseWriter, event, data string) {
	if event != "" {
		_, _ = fmt.Fprintf(w, "event: %s\n", event)
	}
	for _, line := range strings.Split(data, "\n") {
		_, _ = fmt.Fprintf(w, "data: %s\n", line)
	}
	_, _ = fmt.Fprint(w, "\n")
}

// pageJob renders the live job-progress HTML page.
func (s *Server) pageJob(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	job := s.jobs.Snapshot(id)
	if job == nil {
		s.serveError(w, r, fmt.Errorf("job %q not found", id), http.StatusNotFound)
		return
	}
	s.renderPage(w, r, "job.html", pageData{
		Title:     fmt.Sprintf("%s %s", job.Action, job.Tool),
		ActiveTab: "installed",
		Data:      job,
	})
}
