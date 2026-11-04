package config_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/AlvaroQ/engram-explorer/internal/config"
)

func contentionData() config.ProfileStore {
	return config.ProfileStore{Version: 1, ActiveProfile: "a", Profiles: map[string]config.Profile{
		"a": {Providers: map[string]config.ProviderConfig{"engram": {Enabled: true, Path: "/a.db"}}},
		"b": {Providers: map[string]config.ProviderConfig{"engram": {Enabled: true, Path: "/b.db"}}},
	}}
}

// S2: Store → Registry lock order under contention, with callbacks that take a
// second ("registry") mutex and read-only Store methods outside callbacks, never
// deadlocks; -race clean; the active name always names an existing profile.
func TestStore_NoDeadlockUnderContention(t *testing.T) {
	dir := t.TempDir()
	s := config.NewStore(dir, contentionData())
	var reg sync.RWMutex
	applied := ""
	done := make(chan struct{})
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 150; i++ {
				switch (g + i) % 6 {
				case 0:
					name := []string{"a", "b"}[i%2]
					_ = s.SwitchActive(name, func(next config.Profile) error {
						reg.Lock()
						applied = next.ProviderCfg("engram").Path
						reg.Unlock()
						if i%7 == 0 {
							return errors.New("apply failed")
						}
						return nil
					})
				case 1:
					_ = s.UpdateActive(func(_ string, p *config.Profile) error {
						reg.Lock()
						defer reg.Unlock()
						_ = s.Effective(*p)
						p.Preferences.AdvancedView = !p.Preferences.AdvancedView
						return nil
					})
				case 2:
					n, _, ok := s.ActiveProfile()
					if !ok {
						t.Errorf("active profile %q missing", n)
					}
				case 3:
					snap := s.Snapshot()
					snap.Profiles["x"] = config.Profile{} // mutate copy
					delete(snap.Profiles, "a")
				case 4:
					_, _ = s.AddCCAccount(fmt.Sprintf("acc %d %d", g, i), "/p")
					_ = s.EffectiveCCAccounts()
				case 5:
					reg.RLock()
					_ = applied
					reg.RUnlock()
					_ = s.ProfileNames()
				}
			}
		}(g)
	}
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(60 * time.Second):
		t.Fatal("DEADLOCK: store operations did not finish in 60s")
	}
	if _, ok := s.Profile("a"); !ok {
		t.Error("snapshot mutation leaked into the store")
	}
	disk, err := config.LoadProfileStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if disk.ActiveProfile != s.ActiveProfileName() || len(disk.CCAccounts) != len(s.CCAccounts()) {
		t.Errorf("disk and memory diverged: disk active=%q accounts=%d, mem active=%q accounts=%d",
			disk.ActiveProfile, len(disk.CCAccounts), s.ActiveProfileName(), len(s.CCAccounts()))
	}
}

// S1/S2: ephemeral and memory stores never touch the disk (cwd and TMPDIR).
func TestStore_EphemeralWritesNoFiles(t *testing.T) {
	dir := t.TempDir()
	wd, _ := os.Getwd()
	_ = os.Chdir(dir)
	defer os.Chdir(wd)
	for _, s := range []*config.Store{config.NewEphemeralStore(contentionData()), config.NewMemoryStore(config.Config{EngramDbPath: "/d.db", ConfigHome: dir})} {
		if err := s.Update(func(st *config.ProfileStore) error { st.Profiles["n"] = config.Profile{}; return nil }); err != nil {
			t.Fatal(err)
		}
		_ = s.SwitchActive("n", func(config.Profile) error { return nil })
		_, _ = s.AddCCAccount("x", "/x")
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Errorf("ephemeral store wrote %v", entries)
	}
	if _, err := os.Stat(filepath.Join(dir, "config.json")); err == nil {
		t.Error("config.json written")
	}
}

// S6: env overrides are applied by Effective but never land in config.json,
// even after a write through the store.
func TestOpenStore_EnvNeverPersistedAfterWrites(t *testing.T) {
	home := t.TempDir()
	cfg := config.Config{ConfigHome: home,
		EnvOverrides: config.PathSettings{EngramDbPath: "/env/engram.db", ClaudeProjectsDir: "/env/cc", DaemonBaseURL: "http://env:1"},
		Defaults:     config.PathSettings{EngramDbPath: "/def/engram.db", ClaudeProjectsDir: "/def/cc"},
		EngramDbPath: "/env/engram.db", ClaudeProjectsDir: "/env/cc"}
	s, err := config.OpenStore(cfg)
	if err != nil {
		t.Fatal(err)
	}
	_, p, _ := s.ActiveProfile()
	if got := s.Effective(p).ProviderCfg("engram").Path; got != "/env/engram.db" {
		t.Errorf("effective engram path %q", got)
	}
	_ = s.UpdateActive(func(_ string, p *config.Profile) error { p.Preferences.AdvancedView = true; return nil })
	raw, _ := os.ReadFile(filepath.Join(home, "config.json"))
	for _, bad := range []string{"/env/", "http://env:1"} {
		if strings.Contains(string(raw), bad) {
			t.Errorf("config.json contains env value %q:\n%s", bad, raw)
		}
	}
	if _, err := os.Stat(filepath.Join(home, "explorer-settings.json")); err == nil {
		t.Error("explorer-settings.json written")
	}
}
