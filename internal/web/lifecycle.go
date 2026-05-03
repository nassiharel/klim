package web

import (
	"context"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// lifecycle tracks how many browser tabs are currently connected to
// the server's /api/lifecycle SSE endpoint. When the count drops to
// zero, a grace timer starts; if no new connection arrives before it
// fires, the server's shutdown is triggered.
//
// This gives `clim browser` the same UX as `npm start --open`: as
// soon as the user closes the last tab, the local server stops on
// its own. The grace period covers in-page navigation, where the
// outgoing tab closes its EventSource a few ms before the new page
// opens its own.
type lifecycle struct {
	mu      sync.Mutex
	conns   int
	timer   *time.Timer
	grace   time.Duration
	cancel  context.CancelFunc
	stopped atomic.Bool
}

// newLifecycle returns a lifecycle with the given grace period and
// a cancel function that triggers when the last connection is
// considered gone. cancel typically wraps signal.NotifyContext's
// stop func so the main Serve loop unblocks the same way it does on
// Ctrl-C.
func newLifecycle(grace time.Duration, cancel context.CancelFunc) *lifecycle {
	return &lifecycle{grace: grace, cancel: cancel}
}

// register adds a connection. Cancels any pending shutdown timer so
// the server doesn't stop while an in-page navigation is in flight.
func (l *lifecycle) register() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.conns++
	if l.timer != nil {
		l.timer.Stop()
		l.timer = nil
	}
}

// unregister removes a connection. When the count hits zero, schedule
// a shutdown trigger after the grace period.
func (l *lifecycle) unregister() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.conns > 0 {
		l.conns--
	}
	if l.conns == 0 && l.cancel != nil && !l.stopped.Load() {
		l.timer = time.AfterFunc(l.grace, func() {
			if l.stopped.CompareAndSwap(false, true) {
				l.cancel()
			}
		})
	}
}

// apiLifecycleStream is the SSE endpoint each layout opens on page
// load. It never sends real events — the connection itself is the
// signal. When r.Context() fires (browser closes the tab or
// navigates away), unregister runs and may schedule shutdown.
//
// The endpoint goes through authMiddleware like every other route;
// when --insecure-bind enables a token, the browser's auth cookie
// (set on the first ?token= visit) carries through to this stream
// automatically. We send a 15-second SSE comment heartbeat so
// intermediary proxies don't idle-close the long-lived connection.
func (s *Server) apiLifecycleStream(w http.ResponseWriter, r *http.Request) {
	if s.lifecycle == nil {
		http.Error(w, "lifecycle disabled", http.StatusNotFound)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	s.lifecycle.register()
	defer s.lifecycle.unregister()

	// Initial event so the client EventSource resolves "connected".
	_, _ = w.Write([]byte("event: ready\ndata: ok\n\n"))
	flusher.Flush()

	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-heartbeat.C:
			if _, err := w.Write([]byte(": heartbeat\n\n")); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}
