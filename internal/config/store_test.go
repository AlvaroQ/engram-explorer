package config_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/config"
)

func twoProfiles() config.ProfileStore {
	return config.ProfileStore{
		Version:       1,
		ActiveProfile: "default",
		Profiles: map[string]config.Profile{
			"default": {Providers: map[string]config.ProviderConfig{"engram": {Enabled: true, Path: "/a.db"}}},
			"work":    {Providers: map[string]config.ProviderConfig{"engram": {Enabled: true, Path: "/w.db"}}},
		},
		CCAccounts: []config.CCAccount{{ID: "default", Label: "Personal", Path: "/cc", Enabled: true}},
	}
}

// TestStore_UpdateFailureLeavesStateUnchanged: a failing callback leaves memory
// and disk untouched; a successful one is saved and visible.
func TestStore_UpdateFailureLeavesStateUnchanged(t *testing.T) {
	home := t.TempDir()
	s := config.NewStore(home, twoProfiles())

	boom := errors.New("boom")
	err := s.UpdateActive(func(_ string, p *config.Profile) error {
		p.Preferences.AdvancedView = true
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("UpdateActive: got %v, want boom", err)
	}
	if _, p, _ := s.ActiveProfile(); p.IsAdvancedView() {
		t.Error("failed update leaked into memory")
	}
	if _, err := os.Stat(filepath.Join(home, "config.json")); !os.IsNotExist(err) {
		t.Errorf("failed update wrote config.json (stat err %v)", err)
	}

	if err := s.UpdateActive(func(_ string, p *config.Profile) error {
		p.Preferences.AdvancedView = true
		return nil
	}); err != nil {
		t.Fatalf("UpdateActive: %v", err)
	}
	saved, err := config.LoadProfileStore(home)
	if err != nil {
		t.Fatalf("LoadProfileStore: %v", err)
	}
	if !saved.Profiles["default"].IsAdvancedView() {
		t.Error("successful update was not persisted")
	}
}

// TestStore_SnapshotsAreCopies: mutating a returned snapshot never changes the store.
func TestStore_SnapshotsAreCopies(t *testing.T) {
	s := config.NewStore(t.TempDir(), twoProfiles())
	snap := s.Snapshot()
	snap.Profiles["default"].Providers["engram"] = config.ProviderConfig{Path: "/mutated"}
	snap.CCAccounts[0].Path = "/mutated"
	if p, _ := s.Profile("default"); p.ProviderCfg("engram").Path != "/a.db" {
		t.Error("snapshot shares the providers map with the store")
	}
	if s.CCAccounts()[0].Path != "/cc" {
		t.Error("snapshot shares the accounts slice with the store")
	}
}

// TestStore_SwitchActiveSaveFailureRestores: when persisting the switch fails,
// the live providers are restored to the previous profile and nothing changes.
func TestStore_SwitchActiveSaveFailureRestores(t *testing.T) {
	file := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := config.NewStore(file, twoProfiles()) // config home is a file: every save fails

	var applied []string
	err := s.SwitchActive("work", func(next config.Profile) error {
		applied = append(applied, next.ProviderCfg("engram").Path)
		return nil
	})
	if err == nil {
		t.Fatal("SwitchActive: expected save error")
	}
	if want := []string{"/w.db", "/a.db"}; fmt.Sprint(applied) != fmt.Sprint(want) {
		t.Errorf("apply calls: got %v, want %v (switch, then restore)", applied, want)
	}
	if got := s.ActiveProfileName(); got != "default" {
		t.Errorf("active profile: got %q, want default", got)
	}

	if err := s.SwitchActive("missing", func(config.Profile) error { return nil }); err == nil {
		t.Error("SwitchActive to an unknown profile: expected error")
	}
}

// TestStore_EphemeralNeverWrites: an ephemeral store applies changes in memory
// and never creates files.
func TestStore_EphemeralNeverWrites(t *testing.T) {
	s := config.NewEphemeralStore(twoProfiles())
	if err := s.SwitchActive("work", func(config.Profile) error { return nil }); err != nil {
		t.Fatalf("SwitchActive: %v", err)
	}
	if _, err := s.AddCCAccount("Work", "/work/cc"); err != nil {
		t.Fatalf("AddCCAccount: %v", err)
	}
	if s.ActiveProfileName() != "work" || len(s.CCAccounts()) != 2 {
		t.Errorf("ephemeral changes not applied in memory: active %q, accounts %d", s.ActiveProfileName(), len(s.CCAccounts()))
	}
	if !s.Ephemeral() {
		t.Error("Ephemeral: want true")
	}
}

// TestStore_ConcurrentAccess hammers every Store method; run with -race.
func TestStore_ConcurrentAccess(t *testing.T) {
	s := config.NewStore(t.TempDir(), twoProfiles())
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 20; i++ {
				switch (g + i) % 5 {
				case 0:
					_ = s.SwitchActive([]string{"default", "work"}[i%2], func(config.Profile) error { return nil })
				case 1:
					_ = s.UpdateActive(func(_ string, p *config.Profile) error {
						p.Preferences.AdvancedView = i%2 == 0
						return nil
					})
				case 2:
					if acc, err := s.AddCCAccount(fmt.Sprintf("a%d-%d", g, i), "/p"); err == nil {
						_ = s.RemoveCCAccount(acc.ID)
					}
				case 3:
					_ = s.Snapshot()
					_, _, _ = s.ActiveProfile()
				default:
					_ = s.ProfileNames()
					_ = s.CCAccounts()
				}
			}
		}(g)
	}
	wg.Wait()
	if n := len(s.CCAccounts()); n != 1 {
		t.Errorf("accounts after add/remove pairs: got %d, want 1", n)
	}
}
