package config_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/config"
)

// TestLoad_CodexHome: CODEX_HOME is an in-memory override of the Codex home;
// without it the default is ~/.codex.
func TestLoad_CodexHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("CODEX_HOME", "")

	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	def := filepath.Join(home, ".codex")
	if cfg.CodexHome != def || cfg.Defaults.CodexHome != def || cfg.EnvOverrides.CodexHome != "" {
		t.Errorf("default: CodexHome %q Defaults %q Env %q, want %q / %q / empty", cfg.CodexHome, cfg.Defaults.CodexHome, cfg.EnvOverrides.CodexHome, def, def)
	}

	t.Setenv("CODEX_HOME", "/env/codex")
	cfg, err = config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.CodexHome != "/env/codex" || cfg.EnvOverrides.CodexHome != "/env/codex" || cfg.Defaults.CodexHome != def {
		t.Errorf("env: CodexHome %q Env %q Defaults %q", cfg.CodexHome, cfg.EnvOverrides.CodexHome, cfg.Defaults.CodexHome)
	}
}

// TestOpenStore_CodexProviderDefaultsAndEnvNeverSaved: a new config.json
// seeds an enabled codex-sessions entry at the default home; an existing one
// without the entry is not rewritten but runs on the default home in the
// effective profile; CODEX_HOME wins there and is never written to disk; an
// explicit disabled entry stays disabled.
func TestOpenStore_CodexProviderDefaultsAndEnvNeverSaved(t *testing.T) {
	defaults := config.PathSettings{CodexHome: "/default/codex"}

	fresh := t.TempDir()
	s, err := config.OpenStore(config.Config{ConfigHome: fresh, CodexHome: "/default/codex", Defaults: defaults})
	if err != nil {
		t.Fatal(err)
	}
	if _, p, _ := s.ActiveProfile(); p.ProviderCfg("codex-sessions") != (config.ProviderConfig{Enabled: true, Path: "/default/codex"}) {
		t.Errorf("new config.json codex-sessions = %+v, want enabled at the default home", p.ProviderCfg("codex-sessions"))
	}

	existing := t.TempDir()
	st := config.ProfileStore{Version: 1, ActiveProfile: "default", Profiles: map[string]config.Profile{
		"default": {Providers: map[string]config.ProviderConfig{"engram": {Enabled: true, Path: "/e.db"}}},
	}, CCAccounts: []config.CCAccount{{ID: "default", Label: "Personal", Path: "/cc", Enabled: true}}}
	if err := config.SaveProfileStore(existing, &st); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(filepath.Join(existing, "config.json"))
	s, err = config.OpenStore(config.Config{ConfigHome: existing, Defaults: defaults})
	if err != nil {
		t.Fatal(err)
	}
	_, persisted, _ := s.ActiveProfile()
	if got := s.Effective(persisted).ProviderCfg("codex-sessions"); got != (config.ProviderConfig{Enabled: true, Path: "/default/codex"}) {
		t.Errorf("effective codex-sessions without an entry = %+v, want enabled at the default home", got)
	}
	if after, _ := os.ReadFile(filepath.Join(existing, "config.json")); !bytes.Equal(before, after) {
		t.Errorf("OpenStore rewrote an existing config.json:\n%s", after)
	}

	s, err = config.OpenStore(config.Config{ConfigHome: existing, EnvOverrides: config.PathSettings{CodexHome: "/env/codex"}, Defaults: defaults})
	if err != nil {
		t.Fatal(err)
	}
	_, persisted, _ = s.ActiveProfile()
	if got := s.Effective(persisted).ProviderCfg("codex-sessions"); got != (config.ProviderConfig{Enabled: true, Path: "/env/codex"}) {
		t.Errorf("effective codex-sessions with CODEX_HOME = %+v", got)
	}
	disabled := config.Profile{Providers: map[string]config.ProviderConfig{"codex-sessions": {Enabled: false}}}
	if got := s.Effective(disabled).ProviderCfg("codex-sessions"); got.Enabled || got.Path != "/env/codex" {
		t.Errorf("explicitly disabled entry = %+v, want disabled at the CODEX_HOME path", got)
	}
	if err := s.UpdateActive(func(_ string, p *config.Profile) error { p.Preferences.AdvancedView = true; return nil }); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(filepath.Join(existing, "config.json"))
	if bytes.Contains(raw, []byte("/env/codex")) || bytes.Contains(raw, []byte("/default/codex")) {
		t.Errorf("config.json contains an in-memory Codex home:\n%s", raw)
	}
}
