package httpapi_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/config"
	"github.com/AlvaroQ/engram-explorer/internal/httpapi"
	"github.com/AlvaroQ/engram-explorer/internal/providers"
	"github.com/AlvaroQ/engram-explorer/internal/providers/providertest"
)

// liveInstances counts the instances of p that were opened and not closed.
func liveInstances(p *providertest.FakeProvider) int {
	n := 0
	for _, inst := range p.Opened() {
		if !inst.Closed() {
			n++
		}
	}
	return n
}

// TestProfileSwitch_NoInstanceLeaks (S3 regression probe): concurrent
// profile switches over HTTP, switches that fail (a provider that cannot
// open), and a Shutdown racing them leave exactly one live instance for every
// Enabled provider and none for the others; a final Shutdown closes them all.
func TestProfileSwitch_NoInstanceLeaks(t *testing.T) {
	enabled := providers.ProviderConfig{Enabled: true, Path: "/x"}
	both := providertest.NewFakeProvider(providertest.FakeProviderOptions{ID: "both", DetectAvailable: true})
	onlyP1 := providertest.NewFakeProvider(providertest.FakeProviderOptions{ID: "only-p1", DetectAvailable: true})
	broken := providertest.NewFakeProvider(providertest.FakeProviderOptions{
		ID: "broken", DetectAvailable: true, OpenErr: errors.New("open failed"),
	})
	all := []*providertest.FakeProvider{both, onlyP1, broken}

	p1 := config.Profile{Providers: map[string]config.ProviderConfig{
		"both": {Enabled: true, Path: "/x"}, "only-p1": {Enabled: true, Path: "/x"},
	}}
	p2 := config.Profile{Providers: map[string]config.ProviderConfig{
		"both": {Enabled: true, Path: "/x"}, "only-p1": {Enabled: false},
	}}
	bad := config.Profile{Providers: map[string]config.ProviderConfig{
		"both": {Enabled: true, Path: "/x"}, "broken": {Enabled: true, Path: "/x"},
	}}
	store := config.NewEphemeralStore(config.ProfileStore{
		Version: 1, ActiveProfile: "p1",
		Profiles: map[string]config.Profile{"p1": p1, "p2": p2, "bad": bad},
	})

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	reg := providertest.NewRegistry(t, providertest.NewFakeProfile(map[string]providers.ProviderConfig{
		"both": enabled, "only-p1": enabled,
	}), both, onlyP1, broken)
	c := httpapi.NewContainerWithRegistry(reg, config.Config{DaemonTimeoutMs: 1}, logger)
	c.ProfileStore = store
	h := httpapi.NewServeMux(c)

	switchTo := func(name string) {
		form := url.Values{"name": {name}}
		req := httptest.NewRequest(http.MethodPost, "/settings/profile/switch", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		h.ServeHTTP(httptest.NewRecorder(), req)
	}
	run := func(n int, shutdownAt int) {
		var wg sync.WaitGroup
		for i := 0; i < n; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				if i == shutdownAt {
					reg.Shutdown(context.Background())
					return
				}
				switchTo([]string{"p1", "p2", "bad"}[i%3])
			}(i)
		}
		wg.Wait()
	}
	assertMatchesRegistry := func(phase string) {
		t.Helper()
		for _, e := range reg.Entries() {
			want := 0
			if e.State == providers.Enabled {
				want = 1
			}
			for _, p := range all {
				if p.ID() == e.ID {
					if got := liveInstances(p); got != want {
						t.Errorf("%s: provider %s (%s) has %d live instances, want %d", phase, e.ID, e.State, got, want)
					}
				}
			}
		}
	}

	// Phase 1: switches only. The active profile is never "bad" and the
	// registry matches it.
	run(90, -1)
	// Every switch (successful or rolled back) opens a fresh "both": the probe
	// really exercised the lifecycle.
	if n := len(both.Opened()); n < 90 {
		t.Fatalf("only %d instances of both were opened by 90 switches", n)
	}
	active := store.ActiveProfileName()
	if active == "bad" {
		t.Fatal("a failing switch became the active profile")
	}
	wantOnlyP1 := 0
	if active == "p1" {
		wantOnlyP1 = 1
	}
	if got := liveInstances(both); got != 1 {
		t.Errorf("phase 1: both has %d live instances, want 1", got)
	}
	if got := liveInstances(onlyP1); got != wantOnlyP1 {
		t.Errorf("phase 1 (active %s): only-p1 has %d live instances, want %d", active, got, wantOnlyP1)
	}
	if got := liveInstances(broken); got != 0 {
		t.Errorf("phase 1: broken has %d live instances, want 0", got)
	}
	assertMatchesRegistry("phase 1")

	// Phase 2: a Shutdown races the switches; live instances still match
	// the registry state.
	run(90, 45)
	assertMatchesRegistry("phase 2")

	reg.Shutdown(context.Background())
	for _, p := range all {
		if got := liveInstances(p); got != 0 {
			t.Errorf("after Shutdown: provider %s has %d live instances, want 0", p.ID(), got)
		}
	}
}
