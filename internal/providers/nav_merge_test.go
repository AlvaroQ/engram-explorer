package providers_test

import (
	"context"
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/providers"
	"github.com/AlvaroQ/engram-explorer/internal/providers/providertest"
)

// TestRegistryNavGroups_MergesSharedGroup: providers that contribute the same
// group ID (both agent-session providers) render one group whose links are
// the union in registry order, without duplicates; disabling one keeps it.
func TestRegistryNavGroups_MergesSharedGroup(t *testing.T) {
	shared := func(providers.Instance) providers.NavGroup { return providers.AgentSessionsNav() }
	own := func(providers.Instance) providers.NavGroup {
		return providers.NavGroup{ID: "notes", LabelKey: "nav.notes", Links: []providers.NavLink{{Href: "/notes", Key: "notes"}}}
	}
	reg := providertest.NewRegistry(t, providertest.NewFakeProfile(map[string]providers.ProviderConfig{
		"x": {Enabled: true}, "notes": {Enabled: true}, "y": {Enabled: true},
	}),
		providertest.NewFakeProvider(providertest.FakeProviderOptions{ID: "x", DetectAvailable: true, Nav: shared}),
		providertest.NewFakeProvider(providertest.FakeProviderOptions{ID: "notes", DetectAvailable: true, Nav: own}),
		providertest.NewFakeProvider(providertest.FakeProviderOptions{ID: "y", DetectAvailable: true, Nav: shared}),
	)

	groups := reg.NavGroups()
	if len(groups) != 2 || groups[0].ID != "agent-sessions" || groups[1].ID != "notes" {
		t.Fatalf("NavGroups = %+v, want [agent-sessions notes]", groups)
	}
	want := providers.AgentSessionsNav().Links
	if got := groups[0].Links; len(got) != len(want) {
		t.Fatalf("merged links = %+v, want %+v (no duplicates)", got, want)
	}
	for i := range want {
		if groups[0].Links[i] != want[i] {
			t.Fatalf("merged link %d = %+v, want %+v", i, groups[0].Links[i], want[i])
		}
	}

	if err := reg.Disable(context.Background(), "x"); err != nil {
		t.Fatal(err)
	}
	groups = reg.NavGroups()
	if len(groups) != 2 || groups[0].ID != "notes" || groups[1].ID != "agent-sessions" || len(groups[1].Links) != len(want) {
		t.Fatalf("NavGroups after disabling x = %+v, want [notes agent-sessions]", groups)
	}
}
