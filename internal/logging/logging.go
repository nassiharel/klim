// Package logging sets up structured logging for clim using log/slog.
// Logs are written to a file (~/.config/clim/clim.log) by default.
// When verbose mode is enabled, logs are also written to stderr.
package logging

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/natefinch/lumberjack.v2"
)

var logPath string

// Init configures the default slog logger.
// Call once at startup, before any other package uses slog.
// The verbose parameter (from --verbose flag) is OR'd with the config setting.
func Init(level string, fileEnabled bool, verbose bool) {
	var writers []io.Writer

	if fileEnabled {
		logPath = resolveLogPath()
		if logPath != "" {
			writers = append(writers, &lumberjack.Logger{
				Filename:   logPath,
				MaxSize:    2, // MB
				MaxBackups: 3,
				MaxAge:     14, // days
				Compress:   false,
			})
		}
	}

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

	slog.Debug("logging initialized", "path", logPath, "level", level, "verbose", verbose)
}

// Path returns the resolved log file path, or "" if file logging is disabled.
func Path() string {
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
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	logDir := filepath.Join(dir, "clim")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return ""
	}
	return filepath.Join(logDir, "clim.log")
}
