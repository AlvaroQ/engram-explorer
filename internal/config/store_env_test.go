package config_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/config"
)

// TestOpenStore_EnvAppliedInMemoryOnly covers S6: environment overrides win in
// the effective profile, respect an explicit "enabled": false, enable a
// provider the profile does not configure, and are never persisted — not even
// when the store saves an unrelated change.
func TestOpenStore_EnvAppliedInMemoryOnly(t *testing.T) {
	home := t.TempDir()
	st := config.ProfileStore{
		Version:       1,
		ActiveProfile: "default",
		Profiles: map[string]config.Profile{"default": {Providers: map[string]config.ProviderConfig{
			"engram": {Enabled: false, Path: "/profile/engram.db"},
		}}},
	}
	if err := config.SaveProfileStore(home, &st); err != nil {
		t.Fatalf("SaveProfileStore: %v", err)
	}
	env := config.PathSettings{EngramDbPath: "/env/engram.db", ClaudeProjectsDir: "/env/cc", DaemonBaseURL: "http://127.0.0.1:9"}
	s, err := config.OpenStore(config.Config{ConfigHome: home, EnvOverrides: env, Defaults: config.PathSettings{ClaudeProjectsDir: "/default/cc"}})
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}

	_, persisted, _ := s.ActiveProfile()
	eff := s.Effective(persisted)
	if got := eff.ProviderCfg("engram"); got.Path != env.EngramDbPath || got.Enabled || got.DaemonURL != env.DaemonBaseURL {
		t.Errorf("effective engram: got %+v, want env path/daemon and enabled=false kept", got)
	}
	if got := eff.ProviderCfg("cc-sessions"); got.Path != env.ClaudeProjectsDir || !got.Enabled {
		t.Errorf("effective cc-sessions: got %+v, want env path and enabled (absent from profile)", got)
	}
	if got := (config.Config{}).WithProfile(eff); got.EngramDbPath != env.EngramDbPath || got.ClaudeProjectsDir != env.ClaudeProjectsDir || got.DaemonBaseURL != env.DaemonBaseURL {
		t.Errorf("WithProfile: got %+v", got)
	}

	// Existing installs without accounts get the default one once, at open,
	// from the persisted (non-env) path; reads show the env path.
	if accs := s.CCAccounts(); len(accs) != 1 || accs[0].Path != "/default/cc" {
		t.Errorf("seeded accounts: got %+v, want one default account at /default/cc", accs)
	}
	if accs := s.EffectiveCCAccounts(); len(accs) != 1 || accs[0].Path != env.ClaudeProjectsDir {
		t.Errorf("effective accounts: got %+v, want the default account at the env path", accs)
	}

	if err := s.UpdateActive(func(_ string, p *config.Profile) error {
		p.Preferences.AdvancedView = true
		return nil
	}); err != nil {
		t.Fatalf("UpdateActive: %v", err)
	}
	raw, _ := os.ReadFile(filepath.Join(home, "config.json"))
	for _, v := range []string{env.EngramDbPath, env.ClaudeProjectsDir, env.DaemonBaseURL} {
		if bytes.Contains(raw, []byte(v)) {
			t.Errorf("config.json contains the env value %q:\n%s", v, raw)
		}
	}
}
