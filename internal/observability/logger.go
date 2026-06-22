// Package observability owns the slog logger and (in later phases) the
// OpenTelemetry tracer. Centralizing them here means every other module
// imports `observability` instead of stdlib `log/slog` directly, which
// gives us a single seam to swap implementations or add wrappers later.
package observability

import (
	"io"
	"log/slog"
	"os"
	"strings"
)

// Logger is the canonical type passed through the engine. We deliberately
// re-export *slog.Logger instead of wrapping it so call sites still get
// the full structured-logging API surface.
type Logger = slog.Logger

// Configure installs a global slog handler writing to stderr with the
// given verbosity. Verbosity is the count of -v flags: 0=warn, 1=info,
// 2=debug, 3+=debug with source locations.
//
// We write to stderr so `c3x estimate --format json` can be piped into
// jq without log lines polluting the JSON payload.
func Configure(verbosity int) *Logger {
	level := levelFor(verbosity)
	addSource := verbosity >= 3
	handler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level:     level,
		AddSource: addSource,
	})
	logger := slog.New(handler)
	slog.SetDefault(logger)
	return logger
}

// ConfigureForTesting returns a logger that writes to the given Writer.
// Tests use this to capture log output without touching globals.
func ConfigureForTesting(w io.Writer, verbosity int) *Logger {
	return slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{
		Level: levelFor(verbosity),
	}))
}

func levelFor(verbosity int) slog.Level {
	switch verbosity {
	case 0:
		return slog.LevelWarn
	case 1:
		return slog.LevelInfo
	default:
		return slog.LevelDebug
	}
}

// ParseLevel converts a CLI string ("debug", "info", "warn", "error")
// into a slog.Level. Used by viper-bound CLI flags.
func ParseLevel(s string) (slog.Level, bool) {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug, true
	case "info":
		return slog.LevelInfo, true
	case "warn", "warning":
		return slog.LevelWarn, true
	case "error":
		return slog.LevelError, true
	default:
		return slog.LevelWarn, false
	}
}
