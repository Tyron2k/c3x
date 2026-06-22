package observability_test

// Tests for the logger and tracer wrappers. These are thin — most of
// the value is asserting we never accidentally leak log lines to
// stdout (which would break `--format json` pipelines) and that the
// verbosity → level mapping stays stable.

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/c3xdev/c3x/internal/observability"
)

func TestParseLevelKnownValues(t *testing.T) {
	t.Parallel()
	cases := map[string]slog.Level{
		"debug":   slog.LevelDebug,
		"DEBUG":   slog.LevelDebug,
		"info":    slog.LevelInfo,
		"warn":    slog.LevelWarn,
		"warning": slog.LevelWarn,
		"error":   slog.LevelError,
	}
	for in, want := range cases {
		got, ok := observability.ParseLevel(in)
		if !ok {
			t.Errorf("ParseLevel(%q): ok=false, want true", in)
			continue
		}
		if got != want {
			t.Errorf("ParseLevel(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestParseLevelRejectsUnknown(t *testing.T) {
	t.Parallel()
	for _, in := range []string{"", "trace", "verbose", "nonsense"} {
		if _, ok := observability.ParseLevel(in); ok {
			t.Errorf("ParseLevel(%q) returned ok=true for unknown level", in)
		}
	}
}

// TestConfigureForTestingEmitsAtChosenLevel exercises the test
// helper used by every package under test. Confirms the verbosity
// dial actually gates output.
func TestConfigureForTestingEmitsAtChosenLevel(t *testing.T) {
	t.Parallel()
	cases := []struct {
		verbosity int
		levelCall slog.Level
		wantEmit  bool
	}{
		{verbosity: 0, levelCall: slog.LevelInfo, wantEmit: false}, // warn-or-above
		{verbosity: 0, levelCall: slog.LevelWarn, wantEmit: true},
		{verbosity: 1, levelCall: slog.LevelInfo, wantEmit: true},
		{verbosity: 2, levelCall: slog.LevelDebug, wantEmit: true},
	}
	for _, tc := range cases {
		buf := &bytes.Buffer{}
		log := observability.ConfigureForTesting(buf, tc.verbosity)
		log.Log(t.Context(), tc.levelCall, "test-msg")
		emitted := strings.Contains(buf.String(), "test-msg")
		if emitted != tc.wantEmit {
			t.Errorf("verbosity=%d level=%v: emit=%v want=%v\nbuf:%s",
				tc.verbosity, tc.levelCall, emitted, tc.wantEmit, buf.String())
		}
	}
}

// TestTracerIsNoopByDefault confirms the otel default provider
// returns a tracer that emits non-recording spans. This matters
// because c3x doesn't pull in the OTel SDK — production users who
// haven't installed a provider get zero-cost spans.
func TestTracerIsNoopByDefault(t *testing.T) {
	t.Parallel()
	tr := observability.Tracer()
	if tr == nil {
		t.Fatal("Tracer() returned nil")
	}
	_, span := tr.Start(t.Context(), "test-span")
	defer span.End()
	if span.IsRecording() {
		t.Error("default tracer should produce non-recording spans (no SDK installed)")
	}
}
