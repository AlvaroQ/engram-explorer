// Package logging provides slog-based structured logging factories and middleware.
package logging

import (
	"log/slog"
	"os"
)

// New creates a *slog.Logger with JSON handler (production) or text handler
// (development), at the given level string (debug|info|warn|error).
func New(env, levelStr string) *slog.Logger {
	var level slog.Level
	switch levelStr {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{Level: level}

	var handler slog.Handler
	if env == "development" {
		handler = slog.NewTextHandler(os.Stderr, opts)
	} else {
		handler = slog.NewJSONHandler(os.Stderr, opts)
	}
	return slog.New(handler)
}
