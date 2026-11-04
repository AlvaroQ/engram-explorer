package engram_test

import (
	"context"
	"database/sql"
	"log/slog"
	"path/filepath"
	"sync"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/AlvaroQ/engram-explorer/internal/config"
	"github.com/AlvaroQ/engram-explorer/internal/providers"
	"github.com/AlvaroQ/engram-explorer/internal/providers/engram"
	"github.com/AlvaroQ/engram-explorer/internal/sqlite"
)

// captureHandler records every slog record it handles.
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

// captureDefault routes the global slog default to a capture handler for the
// test, so it can assert nothing is logged through it.
func captureDefault(t *testing.T) *captureHandler {
	t.Helper()
	h := &captureHandler{}
	prev := slog.Default()
	slog.SetDefault(slog.New(h))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return h
}

func openDB(t *testing.T, withSchema bool) *sql.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "engram.db")
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if withSchema {
		for _, s := range []string{"CREATE TABLE observations (id INTEGER)", "CREATE TABLE sessions (id INTEGER)"} {
			if _, err := db.Exec(s); err != nil {
				t.Fatal(err)
			}
		}
	} else if _, err := db.Exec("CREATE TABLE other (id INTEGER)"); err != nil {
		t.Fatal(err)
	}
	return db
}

// TestHealth_FailuresLogOncePerTransition covers S13 (Phase 2 follow-up C3):
// consecutive failing health checks log one warning through the registry's
// logger (never the global default), recovery logs once, and a new failure
// warns again.
func TestHealth_FailuresLogOncePerTransition(t *testing.T) {
	global := captureDefault(t)
	injected := &captureHandler{}
	good := openDB(t, true)
	path := filepath.Join(t.TempDir(), "engram.db")
	if err := copySchemaDB(path); err != nil {
		t.Fatal(err)
	}

	reg := providers.NewRegistry(slog.New(injected))
	reg.Register(engram.NewProvider(config.Config{ReadOnly: true}))
	if err := reg.Enable(context.Background(), "engram", providers.ProviderConfig{Enabled: true, Path: path}); err != nil {
		t.Fatalf("Enable: %v", err)
	}
	t.Cleanup(func() { reg.Shutdown(context.Background()) })
	inst := reg.Instance("engram")
	ro, _, _ := inst.(interface {
		ContainerDeps() (sqlite.Querier, sqlite.Querier, *config.RuntimePaths)
	}).ContainerDeps()
	swap := ro.(*sqlite.SwapDB)
	original := swap.Swap(openDB(t, false)) // schemaless: health fails
	defer original.Close()

	const n = 5
	for i := 0; i < n; i++ {
		if h := inst.Health(context.Background()); h.OK {
			t.Fatalf("call %d: health OK on a schemaless database", i)
		}
	}
	if got := injected.count(slog.LevelWarn); got != 1 {
		t.Errorf("%d failing health checks logged %d warnings, want 1", n, got)
	}

	swap.Swap(good)
	for i := 0; i < n; i++ {
		if h := inst.Health(context.Background()); !h.OK {
			t.Fatalf("recovery call %d: health %+v", i, h)
		}
	}
	if got := injected.count(slog.LevelInfo); got != 1 {
		t.Errorf("recovery logged %d info lines, want 1", got)
	}

	swap.Swap(openDB(t, false))
	inst.Health(context.Background())
	inst.Health(context.Background())
	if got := injected.count(slog.LevelWarn); got != 2 {
		t.Errorf("a new failure after recovery: %d warnings in total, want 2", got)
	}
	if got := global.count(slog.LevelWarn) + global.count(slog.LevelInfo); got != 0 {
		t.Errorf("health logged %d records through the global slog default, want 0", got)
	}
}

// copySchemaDB writes an Engram-schema database at path (Open validates it).
func copySchemaDB(path string) error {
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		return err
	}
	defer db.Close()
	for _, s := range []string{"CREATE TABLE observations (id INTEGER)", "CREATE TABLE sessions (id INTEGER)"} {
		if _, err := db.Exec(s); err != nil {
			return err
		}
	}
	return nil
}
