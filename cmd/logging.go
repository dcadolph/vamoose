package cmd

import (
	"log/slog"
	"os"
	"strings"
)

// newLogger builds the structured logger from the environment: VAMOOSE_LOG_FORMAT (json
// or text, default text) and VAMOOSE_LOG_LEVEL (debug, info, warn, error, default info),
// writing to stderr. A hosted server sets json so logs are machine-parseable.
func newLogger() *slog.Logger {
	level := slog.LevelInfo
	switch strings.ToLower(os.Getenv("VAMOOSE_LOG_LEVEL")) {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}
	opts := &slog.HandlerOptions{Level: level}
	var h slog.Handler
	if strings.EqualFold(os.Getenv("VAMOOSE_LOG_FORMAT"), "json") {
		h = slog.NewJSONHandler(os.Stderr, opts)
	} else {
		h = slog.NewTextHandler(os.Stderr, opts)
	}
	return slog.New(h)
}
