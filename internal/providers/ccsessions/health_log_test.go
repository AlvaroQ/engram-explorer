package ccsessions_test

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/config"
	"github.com/AlvaroQ/engram-explorer/internal/providers"
	"github.com/AlvaroQ/engram-explorer/internal/providers/ccsessions"
)

type captureHandler struct {
	mu   sync.Mutex
	recs []slog.Record
}

func (h *captureHandler) Enabled(context.Context, slog.Level) bool { return true }
func (h *captureHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.recs = append(h.recs, r)
	return nil
}
func (h *captureHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *captureHandler) WithGroup(string) slog.Handler      { return h }

func (h *captureHandler) count(level slog.Level) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	n := 0
	for _, r := range h.recs {
		if r.Level == level {
			n++
		}
	}
	return n
}

// TestHealth_FailuresLogOncePerTransition covers S13 (Phase 2 follow-up C3)
// for the Claude Code provider: a projects directory that disappears logs one
// warning through the registry's logger however often /api/health polls, and
// one info line when it comes back. Nothing goes to the global slog default.
func TestHealth_FailuresLogOncePerTransition(t *testing.T) {
	global := &captureHandler{}
	prev := slog.Default()
	slog.SetDefault(slog.New(global))
	t.Cleanup(func() { slog.SetDefault(prev) })

	injected := &captureHandler{}
	dir := filepath.Join(t.TempDir(), "projects")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	reg := providers.NewRegistry(slog.New(injected))
	reg.Register(ccsessions.NewProvider(config.Config{}))
	if err := reg.Enable(context.Background(), "cc-sessions", providers.ProviderConfig{Enabled: true, Path: dir}); err != nil {
		t.Fatalf("Enable: %v", err)
	}
	t.Cleanup(func() { reg.Shutdown(context.Background()) })
	inst := reg.Instance("cc-sessions")

	if err := os.Remove(dir); err != nil {
		t.Fatal(err)
	}
	const n = 5
	for i := 0; i < n; i++ {
		if inst.Health(context.Background()).OK {
			t.Fatalf("call %d: health OK without the projects directory", i)
		}
	}
	if got := injected.count(slog.LevelWarn); got != 1 {
		t.Errorf("%d failing health checks logged %d warnings, want 1", n, got)
	}

	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < n; i++ {
		if !inst.Health(context.Background()).OK {
			t.Fatalf("recovery call %d failed", i)
		}
	}
	if got := injected.count(slog.LevelInfo); got != 1 {
		t.Errorf("recovery logged %d info lines, want 1", got)
	}
	if got := global.count(slog.LevelWarn) + global.count(slog.LevelInfo); got != 0 {
		t.Errorf("health logged %d records through the global slog default, want 0", got)
	}
}
