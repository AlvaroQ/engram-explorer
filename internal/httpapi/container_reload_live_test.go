package httpapi_test

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/AlvaroQ/engram-explorer/internal/config"
	"github.com/AlvaroQ/engram-explorer/internal/httpapi"
	"github.com/AlvaroQ/engram-explorer/internal/providers"
	engramprovider "github.com/AlvaroQ/engram-explorer/internal/providers/engram"
	"github.com/AlvaroQ/engram-explorer/internal/sqlite"
)

// swappableStore is the part of the Engram instance the test inspects.
type swappableStore interface {
	ContainerDeps() (roDB sqlite.Querier, rwDB sqlite.Querier, paths *config.RuntimePaths)
}

func roSwapOf(t *testing.T, inst providers.Instance) *sqlite.SwapDB {
	t.Helper()
	s, ok := inst.(swappableStore)
	if !ok {
		t.Fatalf("instance %T has no ContainerDeps", inst)
	}
	ro, _, _ := s.ContainerDeps()
	sw, ok := ro.(*sqlite.SwapDB)
	if !ok {
		t.Fatalf("read pool %T is not a *sqlite.SwapDB", ro)
	}
	return sw
}

func marker(t *testing.T, q sqlite.Querier) string {
	t.Helper()
	var v string
	if err := q.QueryRow("SELECT v FROM marker").Scan(&v); err != nil {
		t.Fatalf("read marker: %v", err)
	}
	return v
}

// TestReloadEngramDB_SwapsLiveInstanceNotLeaseAndClosesAfterGrace pins S11
// (Phase 2 follow-up C1): ReloadEngramDB resolves the Engram instance through
// Registry.Find — the instance live now — not through a request's lease, so a
// request still pinned to a superseded instance keeps reading its database;
// and the live instance's previous pool stays usable for CloseGrace before it
// is closed.
func TestReloadEngramDB_SwapsLiveInstanceNotLeaseAndClosesAfterGrace(t *testing.T) {
	ctx := context.Background()
	pathA := makeEngramDB(t, "a")
	pathB := makeEngramDB(t, "b")
	cfg := config.Config{EngramDbPath: pathA, EngramDataDir: filepath.Dir(pathA), ConfigHome: t.TempDir()}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	reg := providers.NewRegistry(logger)
	reg.Register(engramprovider.NewProvider(cfg))
	engramCfg := providers.ProviderConfig{Enabled: true, Path: pathA}
	reg.Boot(ctx, httpapi.AdaptProfile(config.Profile{Providers: map[string]config.ProviderConfig{
		"engram": {Enabled: true, Path: pathA},
	}}))
	c := httpapi.NewContainerWithRegistry(reg, cfg, logger)
	const grace = 300 * time.Millisecond
	c.CloseGrace = grace
	t.Cleanup(c.Close)

	// A request pins the boot instance, then a re-open supersedes it.
	lease := reg.Acquire()
	defer lease.Release()
	pinned := roSwapOf(t, lease.Instance("engram"))
	if err := reg.Enable(ctx, "engram", engramCfg); err != nil {
		t.Fatalf("re-open: %v", err)
	}
	live := roSwapOf(t, reg.Instance("engram"))
	if live == pinned {
		t.Fatal("re-open did not replace the instance")
	}
	previous := live.Current()

	start := time.Now()
	if err := c.ReloadEngramDB(pathB); err != nil {
		t.Fatalf("ReloadEngramDB: %v", err)
	}

	if got := marker(t, live); got != "b" {
		t.Errorf("live instance reads %q after reload, want b", got)
	}
	if got := marker(t, pinned); got != "a" {
		t.Errorf("the request's pinned instance reads %q, want a (it must not be swapped)", got)
	}
	if err := previous.Ping(); err != nil && time.Since(start) < grace {
		t.Fatalf("previous pool closed before CloseGrace: %v", err)
	}

	deadline := time.Now().Add(grace + 2*time.Second)
	for previous.Ping() == nil {
		if time.Now().After(deadline) {
			t.Fatal("previous pool was not closed after CloseGrace")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if elapsed := time.Since(start); elapsed < grace {
		t.Errorf("previous pool closed after %v, before CloseGrace %v", elapsed, grace)
	}
}
