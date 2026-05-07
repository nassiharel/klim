// Package logging sets up structured logging for klim using log/slog.
// Logs are written to a file (~/.klim/klim.log) by default.
// When verbose mode is enabled, logs are also written to stderr.
package logging

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"gopkg.in/natefinch/lumberjack.v2"

	"github.com/nassiharel/klim/internal/paths"
)

var (
	logPath   string
	logPathMu sync.Mutex
)

// Init configures the default slog logger.
// Call once at startup, before any other package uses slog.
// The verbose parameter (from --verbose flag) is OR'd with the config setting.
func Init(level string, fileEnabled bool, verbose bool) {
	logPathMu.Lock()
	logPath = "" // reset in case of repeated calls
	var resolvedPath string

	var writers []io.Writer

	if fileEnabled {
		resolvedPath = resolveLogPath()
		if resolvedPath != "" {
			logPath = resolvedPath
			writers = append(writers, &lumberjack.Logger{
				Filename:   resolvedPath,
				MaxSize:    2, // MB
				MaxBackups: 3,
				MaxAge:     14, // days
				Compress:   false,
			})
		}
	}
	logPathMu.Unlock()

	if verbose {
		writers = append(writers, os.Stderr)
	}

	if len(writers) == 0 {
		slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
		return
	}

	w := io.MultiWriter(writers...)
	handler := slog.NewTextHandler(w, &slog.HandlerOptions{Level: parseLevel(level)})
	slog.SetDefault(slog.New(handler))

	slog.Debug("logging initialized", "path", resolvedPath, "level", level, "verbose", verbose)
}

// Path returns the resolved log file path, or "" if file logging is disabled.
func Path() string {
	logPathMu.Lock()
	defer logPathMu.Unlock()
	return logPath
}

func parseLevel(s string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "info":
		return slog.LevelInfo
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelDebug
	}
}

func resolveLogPath() string {
	p, err := paths.LogFile()
	if err != nil {
		return ""
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return ""
	}
	return p
}
