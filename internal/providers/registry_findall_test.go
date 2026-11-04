package providers_test

import (
	"context"
	"sync"
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/agentsessions"
	"github.com/AlvaroQ/engram-explorer/internal/providers"
	"github.com/AlvaroQ/engram-explorer/internal/providers/providertest"
)

// sessionInstance decorates a fake instance with the AgentSessionSource
// capability (no sources: only the capability matters here).
type sessionInstance struct{ *providertest.FakeInstance }

func (sessionInstance) AgentSessionSources() []agentsessions.Source { return nil }

func isSessionSource(i providers.Instance) bool {
	_, ok := i.(providers.AgentSessionSource)
	return ok
}

func asSession(i *providertest.FakeInstance) providers.Instance { return sessionInstance{i} }

// findAllFixture boots three providers: "a" and "c" expose agent sessions,
// "b" does not. All three are enabled.
func findAllFixture(t *testing.T) (*providers.Registry, map[string]*providertest.FakeProvider) {
	t.Helper()
	fps := map[string]*providertest.FakeProvider{
		"a": providertest.NewFakeProvider(providertest.FakeProviderOptions{ID: "a", DetectAvailable: true, Wrap: asSession}),
		"b": providertest.NewFakeProvider(providertest.FakeProviderOptions{ID: "b", DetectAvailable: true}),
		"c": providertest.NewFakeProvider(providertest.FakeProviderOptions{ID: "c", DetectAvailable: true, Wrap: asSession}),
	}
	profile := providertest.NewFakeProfile(map[string]providers.ProviderConfig{
		"a": {Enabled: true, Path: "/a"}, "b": {Enabled: true, Path: "/b"}, "c": {Enabled: true, Path: "/c"},
	})
	return providertest.NewRegistry(t, profile, fps["a"], fps["b"], fps["c"]), fps
}

func paths(insts []providers.Instance) []string {
	out := make([]string, 0, len(insts))
	for _, i := range insts {
		out = append(out, i.(sessionInstance).Health(context.Background()).Path)
	}
	return out
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestRegistryFindAll_EveryEnabledMatchInOrder: FindAll returns every Enabled
// instance with the capability, in registry order, and drops disabled ones.
func TestRegistryFindAll_EveryEnabledMatchInOrder(t *testing.T) {
	reg, _ := findAllFixture(t)
	if got := paths(reg.FindAll(isSessionSource)); !equalStrings(got, []string{"/a", "/c"}) {
		t.Fatalf("FindAll = %v, want [/a /c]", got)
	}
	if err := reg.Disable(context.Background(), "a"); err != nil {
		t.Fatal(err)
	}
	if got := paths(reg.FindAll(isSessionSource)); !equalStrings(got, []string{"/c"}) {
		t.Fatalf("FindAll after disabling a = %v, want [/c]", got)
	}
	if got := reg.FindAll(func(providers.Instance) bool { return false }); len(got) != 0 {
		t.Fatalf("FindAll with no match = %v, want empty", got)
	}
}

// TestLeaseFindAll_OnlyPinnedInstances: a lease's FindAll sees exactly the
// instances pinned when it was acquired, even after a toggle re-opens one,
// and a new lease sees the current ones.
func TestLeaseFindAll_OnlyPinnedInstances(t *testing.T) {
	ctx := context.Background()
	reg, fps := findAllFixture(t)
	l := reg.Acquire()
	defer l.Release()

	if err := reg.Disable(ctx, "a"); err != nil {
		t.Fatal(err)
	}
	if err := reg.Enable(ctx, "c", providers.ProviderConfig{Enabled: true, Path: "/c2"}); err != nil {
		t.Fatal(err)
	}
	got := l.FindAll(isSessionSource)
	if len(got) != 2 || got[0].(sessionInstance).FakeInstance != fps["a"].Opened()[0] || got[1].(sessionInstance).FakeInstance != fps["c"].Opened()[0] {
		t.Fatalf("lease FindAll = %v, want the instances of a and c pinned at Acquire", paths(got))
	}

	next := reg.Acquire()
	defer next.Release()
	if got := paths(next.FindAll(isSessionSource)); !equalStrings(got, []string{"/c2"}) {
		t.Fatalf("new lease FindAll = %v, want [/c2]", got)
	}
	var nilLease *providers.Lease
	if got := nilLease.FindAll(isSessionSource); got != nil {
		t.Fatalf("nil lease FindAll = %v, want nil", got)
	}
}

// TestRegistryFindAll_ConcurrentWithToggles exercises FindAll on the registry
// and on leases while providers are toggled (run with -race).
func TestRegistryFindAll_ConcurrentWithToggles(t *testing.T) {
	ctx := context.Background()
	reg, _ := findAllFixture(t)
	var wg sync.WaitGroup
	for g := 0; g < 4; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				if n := len(reg.FindAll(isSessionSource)); n > 2 {
					t.Errorf("FindAll returned %d instances, want at most 2", n)
				}
				l := reg.Acquire()
				_ = l.FindAll(isSessionSource)
				l.Release()
			}
		}()
	}
	for i := 0; i < 50; i++ {
		_ = reg.Disable(ctx, "a")
		_ = reg.Enable(ctx, "a", providers.ProviderConfig{Enabled: true, Path: "/a"})
	}
	wg.Wait()
}
