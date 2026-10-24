package logger

import (
	"io"
	"log/slog"
	"os"
	"strings"
)

// ParseLevel converts a string representation of log level into slog.Level.
func ParseLevel(lvl string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(lvl)) {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// New creates a new *slog.Logger configured with the given level, format and writer.
func New(levelStr, formatStr string, out io.Writer) *slog.Logger {
	if out == nil {
		out = os.Stdout
	}

	level := ParseLevel(levelStr)
	opts := &slog.HandlerOptions{
		Level: level,
	}

	var handler slog.Handler
	if strings.ToLower(strings.TrimSpace(formatStr)) == "json" {
		handler = slog.NewJSONHandler(out, opts)
	} else {
		handler = slog.NewTextHandler(out, opts)
	}

	return slog.New(handler)
}

// Init initializes the default global logger.
func Init(levelStr, formatStr string) *slog.Logger {
	l := New(levelStr, formatStr, os.Stdout)
	slog.SetDefault(l)
	return l
}
