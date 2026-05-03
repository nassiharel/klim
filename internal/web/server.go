// Package web serves clim's local browser UI. The package is a thin
// frontend over internal/service: every page and JSON endpoint resolves
// data through the same composition root the TUI and other CLI commands
// use, so business logic never duplicates into HTTP handlers.
//
// The server binds to loopback by default and refuses any other
// interface unless InsecureBind is set. All HTML is rendered through
// html/template (XSS-safe) and assets embed into the binary via
// embed.FS so the feature stays single-binary and offline-capable.
package web

import (
	"context"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/nassiharel/clim/internal/service"
)

// Options configure a clim browser server.
type Options struct {
	// Bind is the listen address. Defaults to "127.0.0.1" (loopback).
	Bind string
	// Port is the listen port. 0 lets the kernel pick a free one.
	Port int
	// InsecureBind allows binding non-loopback addresses. Without it,
	// any non-loopback Bind value is refused so users don't accidentally
	// expose an unauthenticated server on a LAN.
	InsecureBind bool
	// Service is the resolved tool service this server reads from.
	// Required.
	Service *service.ToolService
	// Logger receives structured server-side events. Optional; defaults
	// to slog.Default().
	Logger *slog.Logger
}

// Server is a clim browser HTTP server.
type Server struct {
	opts     Options
	mux      *http.ServeMux
	tpls     map[string]*template.Template
	loader   loader
	httpsrv  *http.Server
	listener net.Listener
}

// New constructs a Server. The TCP listener is bound here so the caller
// can read URL() before serving, which is what the CLI prints to stderr
// and passes to the browser-open helper.
func New(opts Options) (*Server, error) {
	if opts.Service == nil {
		return nil, errors.New("web: Options.Service is required")
	}
	if opts.Bind == "" {
		opts.Bind = "127.0.0.1"
	}
	if !opts.InsecureBind && !isLoopback(opts.Bind) {
		return nil, fmt.Errorf("web: bind %q is not a loopback address — pass --insecure-bind to allow it", opts.Bind)
	}
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}

	tpls, err := loadTemplates()
	if err != nil {
		return nil, fmt.Errorf("web: loading templates: %w", err)
	}

	addr := net.JoinHostPort(opts.Bind, fmt.Sprintf("%d", opts.Port))
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("web: listening on %s: %w", addr, err)
	}

	s := &Server{
		opts:     opts,
		mux:      http.NewServeMux(),
		tpls:     tpls,
		loader:   newServiceLoader(opts.Service),
		listener: ln,
	}
	s.routes()
	s.httpsrv = &http.Server{
		Handler:           withRecover(opts.Logger, withAccessLog(opts.Logger, s.mux)),
		ReadHeaderTimeout: 10 * time.Second,
	}
	return s, nil
}

// URL returns the http://host:port URL the server is listening on.
// Safe to call before Serve.
func (s *Server) URL() string {
	addr := s.listener.Addr().(*net.TCPAddr)
	host := s.opts.Bind
	if host == "0.0.0.0" || host == "" {
		host = "127.0.0.1"
	}
	return fmt.Sprintf("http://%s:%d", host, addr.Port)
}

// Serve runs the HTTP server until ctx is cancelled. It performs a
// graceful shutdown on cancellation; in-flight requests are given a
// short grace period to finish.
func (s *Server) Serve(ctx context.Context) error {
	errCh := make(chan error, 1)
	go func() {
		err := s.httpsrv.Serve(s.listener)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.httpsrv.Shutdown(shutdownCtx)
		return nil
	case err := <-errCh:
		return err
	}
}

// Close releases the listener. Useful in tests where Serve is not used.
func (s *Server) Close() error {
	if s.httpsrv != nil {
		return s.httpsrv.Close()
	}
	return s.listener.Close()
}

func (s *Server) routes() {
	// Static assets.
	staticFS, _ := fs.Sub(staticFiles, "static")
	s.mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))

	// Health probe.
	s.mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("ok\n"))
	})

	// HTML pages.
	s.mux.HandleFunc("GET /{$}", s.pageInstalled)
	s.mux.HandleFunc("GET /tools/{name}", s.pageTool)
	s.mux.HandleFunc("GET /updates", s.pageUpdates)
	s.mux.HandleFunc("GET /discover", s.pageDiscover)
	s.mux.HandleFunc("GET /favorites", s.pageFavorites)
	s.mux.HandleFunc("GET /dashboard", s.pageDashboard)
	s.mux.HandleFunc("GET /trail", s.pageTrail)
	s.mux.HandleFunc("GET /trail/{ref...}", s.pageTrailShow)
	// Stubbed tabs (Phase 3 scope).
	s.mux.HandleFunc("GET /backup", s.pageStub("Backup"))
	s.mux.HandleFunc("GET /config", s.pageStub("Config"))

	// JSON API.
	s.mux.HandleFunc("GET /api/tools", s.apiTools)
	s.mux.HandleFunc("GET /api/tools/{name}", s.apiTool)
	s.mux.HandleFunc("GET /api/dashboard", s.apiDashboard)
	s.mux.HandleFunc("GET /api/trail", s.apiTrail)
	s.mux.HandleFunc("GET /api/trail/{ref...}", s.apiTrailShow)
	s.mux.HandleFunc("GET /api/favorites", s.apiFavoritesList)
	// Mutating endpoints. The form-submitting HTML page POSTs here and
	// reloads; the JSON variant is also reachable for scripts.
	s.mux.Handle("POST /api/favorites/{name}/toggle", csrfProtect(s, http.HandlerFunc(s.apiFavoritesToggle)))
	s.mux.Handle("POST /favorites/{name}/toggle", csrfProtect(s, http.HandlerFunc(s.pageFavoritesToggle)))
}

func isLoopback(host string) bool {
	host = strings.TrimSpace(host)
	if host == "" || host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	if ip != nil {
		return ip.IsLoopback()
	}
	return false
}

// withAccessLog logs basic request info at slog.LevelDebug. Errors and
// non-2xx responses are logged at LevelInfo so they show up under
// default verbosity.
func withAccessLog(log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		start := time.Now()
		next.ServeHTTP(rec, r)
		level := slog.LevelDebug
		if rec.status >= 400 {
			level = slog.LevelInfo
		}
		log.LogAttrs(r.Context(), level, "web request",
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.Int("status", rec.status),
			slog.Duration("dur", time.Since(start)),
		)
	})
}

// withRecover converts handler panics into 500s rather than tearing
// down the server. clim is meant to keep running across stray bugs.
func withRecover(log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				log.ErrorContext(r.Context(), "web panic recovered",
					slog.Any("panic", rec),
					slog.String("path", r.URL.Path),
				)
				http.Error(w, "internal server error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}
