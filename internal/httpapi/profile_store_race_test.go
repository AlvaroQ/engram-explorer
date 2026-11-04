package httpapi_test

import (
	"fmt"
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

// profileFixture is a registry-backed container with a fake "cc" provider and
// two profiles ("default" and "work") persisted in a temp config home.
type profileFixture struct {
	c    *httpapi.Container
	h    http.Handler
	home string
}

func newProfileFixture(t *testing.T) profileFixture {
	t.Helper()
	home := t.TempDir()
	data := config.ProfileStore{
		Version:       1,
		ActiveProfile: "default",
		Profiles: map[string]config.Profile{
			"default": {Providers: map[string]config.ProviderConfig{"cc": {Enabled: true, Path: "/default/cc"}}},
			"work":    {Providers: map[string]config.ProviderConfig{"cc": {Enabled: true, Path: "/work/cc"}}},
		},
		CCAccounts: []config.CCAccount{{ID: "default", Label: "Personal", Path: "/default/cc", Enabled: true}},
	}
	if err := config.SaveProfileStore(home, &data); err != nil {
		t.Fatalf("SaveProfileStore: %v", err)
	}
	fp := providertest.NewFakeProvider(providertest.FakeProviderOptions{ID: "cc", DetectAvailable: true})
	reg := providertest.NewRegistry(t, providertest.NewFakeProfile(map[string]providers.ProviderConfig{
		"cc": {Enabled: true, Path: "/default/cc"},
	}), fp)
	c := newRegistryContainer(t, reg)
	c.Config.ConfigHome = home
	c.ProfileStore = config.NewStore(home, data)
	return profileFixture{c: c, h: httpapi.NewServeMux(c), home: home}
}

func (f profileFixture) post(t *testing.T, route string, form url.Values) int {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, route, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	f.h.ServeHTTP(w, req)
	return w.Code
}

func (f profileFixture) get(route string) int {
	w := httptest.NewRecorder()
	f.h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, route, nil))
	return w.Code
}

// TestProfileSwitch_ToggleWritesNewActiveProfile covers S2: after switching
// profiles, a module toggle must be persisted into the NEW active profile, not
// into the profile that was active when the routes were mounted.
func TestProfileSwitch_ToggleWritesNewActiveProfile(t *testing.T) {
	f := newProfileFixture(t)

	if code := f.post(t, "/settings/profile/switch", url.Values{"name": {"work"}}); code != http.StatusSeeOther {
		t.Fatalf("switch: status %d, want 303", code)
	}
	if code := f.post(t, "/settings/modules/cc/toggle", url.Values{"enabled": {"false"}}); code != http.StatusSeeOther {
		t.Fatalf("toggle: status %d, want 303", code)
	}

	saved, err := config.LoadProfileStore(f.home)
	if err != nil {
		t.Fatalf("LoadProfileStore: %v", err)
	}
	if saved.ActiveProfile != "work" {
		t.Errorf("persisted active profile: got %q, want work", saved.ActiveProfile)
	}
	if saved.Profiles["work"].ProviderCfg("cc").Enabled {
		t.Error("toggle was not written into the new active profile (work.cc still enabled)")
	}
	if !saved.Profiles["default"].ProviderCfg("cc").Enabled {
		t.Error("toggle leaked into the previous profile (default.cc disabled)")
	}
}

// TestProfileStore_ConcurrentHandlers covers S2: profile switches, module
// toggles, preference writes, profile CRUD and the read callbacks run
// concurrently from net/http handlers. Run with -race: any unguarded access to
// the profile store or the active profile name is reported as a data race.
func TestProfileStore_ConcurrentHandlers(t *testing.T) {
	f := newProfileFixture(t)

	const workers, iterations = 6, 15
	var wg sync.WaitGroup
	for g := 0; g < workers; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				switch (g + i) % 6 {
				case 0:
					name := "default"
					if i%2 == 0 {
						name = "work"
					}
					f.post(t, "/settings/profile/switch", url.Values{"name": {name}})
				case 1:
					f.post(t, "/settings/modules/cc/toggle", url.Values{"enabled": {fmt.Sprint(i%2 == 0)}})
				case 2:
					f.post(t, "/settings/advanced-view", url.Values{"enabled": {fmt.Sprint(i%2 == 0)}})
				case 3:
					name := fmt.Sprintf("p-%d-%d", g, i)
					f.post(t, "/settings/profile/create", url.Values{"name": {name}})
					f.post(t, "/settings/profile/delete", url.Values{"name": {name}})
				case 4:
					f.get("/settings/accounts")
				default:
					f.get("/settings")
				}
			}
		}(g)
	}
	wg.Wait()

	saved, err := config.LoadProfileStore(f.home)
	if err != nil {
		t.Fatalf("LoadProfileStore after concurrent writes: %v", err)
	}
	if _, ok := saved.Profiles[saved.ActiveProfile]; !ok {
		t.Errorf("persisted active profile %q does not exist", saved.ActiveProfile)
	}
}
