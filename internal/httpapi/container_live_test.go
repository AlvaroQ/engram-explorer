package httpapi_test

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/config"
	"github.com/AlvaroQ/engram-explorer/internal/httpapi"
	"github.com/AlvaroQ/engram-explorer/internal/providers"
	engramprovider "github.com/AlvaroQ/engram-explorer/internal/providers/engram"
	"github.com/AlvaroQ/engram-explorer/internal/usererr"
)

// newLiveContainer boots a registry with the real Engram provider on pathA
// (enabled or not) and returns the container and the registry.
func newLiveContainer(t *testing.T, enabled bool) (*httpapi.Container, *providers.Registry, string) {
	t.Helper()
	pathA := makeEngramDB(t, "a")
	cfg := config.Config{EngramDbPath: pathA, ConfigHome: t.TempDir()}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	reg := providers.NewRegistry(logger)
	reg.Register(engramprovider.NewProvider(cfg))
	reg.Boot(context.Background(), providerProfile{"engram": {Enabled: enabled, Path: pathA}})
	c := httpapi.NewContainerWithRegistry(reg, cfg, logger)
	c.CloseGrace = 0
	t.Cleanup(c.Close)
	return c, reg, pathA
}

type providerProfile map[string]providers.ProviderConfig

func (p providerProfile) ProviderCfg(id string) providers.ProviderConfig { return p[id] }

func liveMarker(t *testing.T, c *httpapi.Container) string {
	t.Helper()
	var v string
	if err := c.RoDB.QueryRow("SELECT v FROM marker").Scan(&v); err != nil {
		t.Fatalf("read marker through c.RoDB: %v", err)
	}
	return v
}

// TestReloadEngramDB_TargetsLiveInstance covers S4 + S13: after the module is
// disabled and enabled again, the DB switch swaps the pools of the NEW live
// instance (not the closed one), so its data and health path follow.
func TestReloadEngramDB_TargetsLiveInstance(t *testing.T) {
	c, reg, pathA := newLiveContainer(t, true)
	ctx := context.Background()
	if err := reg.Disable(ctx, "engram"); err != nil {
		t.Fatal(err)
	}
	if err := reg.Enable(ctx, "engram", providers.ProviderConfig{Enabled: true, Path: pathA}); err != nil {
		t.Fatal(err)
	}

	pathB := makeEngramDB(t, "b")
	if err := c.ReloadEngramDB(pathB); err != nil {
		t.Fatalf("ReloadEngramDB: %v", err)
	}
	if got := liveMarker(t, c); got != "b" {
		t.Errorf("c.RoDB marker: got %q, want b", got)
	}
	if got := reg.Instance("engram").Health(ctx).Path; got != pathB {
		t.Errorf("live instance health path: got %q, want %q", got, pathB)
	}
}

// TestReloadEngramDB_ModuleDisabled covers S4: with the Engram module disabled
// (at boot or later) the DB switch returns a user error instead of panicking
// or swapping a closed instance, and c.RoDB never reaches a closed pool.
func TestReloadEngramDB_ModuleDisabled(t *testing.T) {
	for _, tc := range []struct {
		name string
		boot bool
	}{{"disabled at boot", false}, {"disabled later", true}} {
		t.Run(tc.name, func(t *testing.T) {
			c, reg, _ := newLiveContainer(t, tc.boot)
			if err := reg.Disable(context.Background(), "engram"); err != nil {
				t.Fatal(err)
			}
			err := c.ReloadEngramDB(makeEngramDB(t, "b"))
			if _, ok := usererr.Message(err); !ok {
				t.Fatalf("ReloadEngramDB: got %v, want a user-facing error", err)
			}
			var v string
			if err := c.RoDB.QueryRow("SELECT v FROM marker").Scan(&v); err == nil {
				t.Errorf("c.RoDB answered %q with the module disabled", v)
			}
		})
	}
}
