package config_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/config"
)

// TestEnsureConfig_LegacySettingsSeedNewConfigOnce covers S6: the legacy
// explorer-settings.json is read only to seed a config.json that does not
// exist yet; once config.json exists, later edits to the legacy file are
// ignored and the legacy file itself is never rewritten.
func TestEnsureConfig_LegacySettingsSeedNewConfigOnce(t *testing.T) {
	home := t.TempDir()
	legacyDB := filepath.Join(home, "legacy", "engram.db")
	legacyClaude := filepath.Join(home, "legacy-claude")
	if err := config.SaveOverrides(home, config.Overrides{EngramDbPath: legacyDB, ClaudeProjectsDir: legacyClaude}); err != nil {
		t.Fatalf("SaveOverrides: %v", err)
	}
	legacyBytes, _ := os.ReadFile(config.OverridesPath(home))

	// Built-in defaults, as config.Load resolves them with no env vars set.
	resolved := config.Config{
		ConfigHome:        home,
		EngramDbPath:      filepath.Join(home, ".engram", "engram.db"),
		ClaudeProjectsDir: filepath.Join(home, ".claude", "projects"),
	}
	store, err := config.EnsureConfig(home, resolved)
	if err != nil {
		t.Fatalf("EnsureConfig: %v", err)
	}
	def := store.Profiles["default"]
	if got := def.ProviderCfg("engram").Path; got != legacyDB {
		t.Errorf("seeded engram path: got %q, want legacy %q", got, legacyDB)
	}
	if got := def.ProviderCfg("cc-sessions").Path; got != legacyClaude {
		t.Errorf("seeded cc-sessions path: got %q, want legacy %q", got, legacyClaude)
	}
	seeded, _ := os.ReadFile(filepath.Join(home, "config.json"))

	// A later edit to the legacy file must not change the existing config.json.
	if err := config.SaveOverrides(home, config.Overrides{EngramDbPath: filepath.Join(home, "x.db")}); err != nil {
		t.Fatalf("SaveOverrides: %v", err)
	}
	legacyBytes, _ = os.ReadFile(config.OverridesPath(home))
	if _, err := config.EnsureConfig(home, resolved); err != nil {
		t.Fatalf("EnsureConfig (second): %v", err)
	}
	if again, _ := os.ReadFile(filepath.Join(home, "config.json")); !bytes.Equal(seeded, again) {
		t.Errorf("config.json changed after the legacy file was edited:\n%s", again)
	}
	if after, _ := os.ReadFile(config.OverridesPath(home)); !bytes.Equal(legacyBytes, after) {
		t.Error("EnsureConfig rewrote explorer-settings.json")
	}
}
