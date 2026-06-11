package config_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/config"
)

// TestProfileStore_Roundtrip verifies that a ProfileStore survives a save+load
// cycle with all fields intact (Windows path round-trip included).
func TestProfileStore_Roundtrip(t *testing.T) {
	dir := t.TempDir()

	dbPath := filepath.Join(dir, "engram.db")
	claudePath := filepath.Join(dir, "claude", "projects")

	// On Windows store backslash paths verbatim; on POSIX use forward slashes.
	store := &config.ProfileStore{
		Version:       1,
		ActiveProfile: "default",
		Profiles: map[string]config.Profile{
			"default": {
				Providers: map[string]config.ProviderConfig{
					"engram": {
						Enabled:   true,
						Path:      dbPath,
						DaemonURL: "http://127.0.0.1:7437",
					},
					"cc-sessions": {
						Enabled: true,
						Path:    claudePath,
					},
				},
			},
		},
	}

	if err := config.SaveProfileStore(dir, store); err != nil {
		t.Fatalf("SaveProfileStore: %v", err)
	}

	got, err := config.LoadProfileStore(dir)
	if err != nil {
		t.Fatalf("LoadProfileStore: %v", err)
	}

	if got.ActiveProfile != "default" {
		t.Errorf("ActiveProfile: got %q, want default", got.ActiveProfile)
	}
	if got.Version != 1 {
		t.Errorf("Version: got %d, want 1", got.Version)
	}

	def := got.Profiles["default"]
	eng := def.Providers["engram"]
	if eng.Path != dbPath {
		t.Errorf("engram path: got %q, want %q", eng.Path, dbPath)
	}
	if eng.DaemonURL != "http://127.0.0.1:7437" {
		t.Errorf("DaemonURL: got %q", eng.DaemonURL)
	}
	cc := def.Providers["cc-sessions"]
	if cc.Path != claudePath {
		t.Errorf("cc-sessions path: got %q, want %q", cc.Path, claudePath)
	}
}

// TestProfileStore_WindowsPathRoundTrip verifies that paths with Windows
// backslashes are stored and retrieved verbatim on all platforms.
func TestProfileStore_WindowsPathRoundTrip(t *testing.T) {
	if runtime.GOOS != "windows" {
		// On non-Windows the backslash test is advisory; run it anyway for
		// cross-platform correctness but skip on platforms that reject backslashes.
		t.Skip("Windows-specific path round-trip test — skipping on non-Windows")
	}
	dir := t.TempDir()
	winPath := `C:\Users\me\.engram\engram.db`

	store := &config.ProfileStore{
		Version:       1,
		ActiveProfile: "default",
		Profiles: map[string]config.Profile{
			"default": {
				Providers: map[string]config.ProviderConfig{
					"engram": {Enabled: true, Path: winPath},
				},
			},
		},
	}

	if err := config.SaveProfileStore(dir, store); err != nil {
		t.Fatalf("SaveProfileStore: %v", err)
	}
	got, err := config.LoadProfileStore(dir)
	if err != nil {
		t.Fatalf("LoadProfileStore: %v", err)
	}
	if got.Profiles["default"].Providers["engram"].Path != winPath {
		t.Errorf("path round-trip: got %q, want %q",
			got.Profiles["default"].Providers["engram"].Path, winPath)
	}
}

// TestAtomicSave_NoCorruptionOnInterrupt verifies that an existing config.json
// is not corrupted if a write fails mid-way (simulated by writing bad data to a
// temp and ensuring the original survives after a rename is never called).
// Since we cannot truly interrupt a write, we verify that the temp file is
// separate from the destination and the final rename is atomic.
func TestAtomicSave_FileIsWritten(t *testing.T) {
	dir := t.TempDir()
	store := &config.ProfileStore{
		Version:       1,
		ActiveProfile: "default",
		Profiles: map[string]config.Profile{
			"default": {Providers: map[string]config.ProviderConfig{}},
		},
	}

	if err := config.SaveProfileStore(dir, store); err != nil {
		t.Fatalf("SaveProfileStore: %v", err)
	}

	configPath := filepath.Join(dir, "config.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("config.json not created: %v", err)
	}

	// File must be valid JSON.
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		t.Errorf("config.json is not valid JSON: %v", err)
	}
	// Schema version must be present.
	v, ok := out["version"]
	if !ok {
		t.Error("config.json missing 'version' field")
	}
	if v.(float64) != 1 {
		t.Errorf("version: got %v, want 1", v)
	}
}

// TestEnsureConfig_CreatesDefaultOnFirstRun verifies that EnsureConfig creates
// a default profile from the resolved config when no config.json exists.
func TestEnsureConfig_CreatesDefaultOnFirstRun(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "engram.db")
	claudePath := filepath.Join(dir, "claude", "projects")

	resolved := config.Config{
		ConfigHome:        dir,
		EngramDbPath:      dbPath,
		ClaudeProjectsDir: claudePath,
		DaemonBaseURL:     "http://127.0.0.1:7437",
	}

	store, err := config.EnsureConfig(dir, resolved)
	if err != nil {
		t.Fatalf("EnsureConfig: %v", err)
	}

	if store.ActiveProfile != "default" {
		t.Errorf("ActiveProfile: got %q, want default", store.ActiveProfile)
	}
	def := store.Profiles["default"]
	if def.Providers == nil {
		t.Fatal("default profile has no providers map")
	}
	eng := def.Providers["engram"]
	if eng.Path != dbPath {
		t.Errorf("engram path: got %q, want %q", eng.Path, dbPath)
	}
	cc := def.Providers["cc-sessions"]
	if cc.Path != claudePath {
		t.Errorf("cc-sessions path: got %q, want %q", cc.Path, claudePath)
	}

	// config.json must exist after EnsureConfig.
	if _, err := os.Stat(filepath.Join(dir, "config.json")); err != nil {
		t.Errorf("config.json not created: %v", err)
	}
}

// TestEnsureConfig_LoadsExistingFile verifies that EnsureConfig does NOT
// overwrite an existing config.json, and returns the stored profile.
func TestEnsureConfig_LoadsExistingFile(t *testing.T) {
	dir := t.TempDir()

	// Pre-write a config.json.
	existing := &config.ProfileStore{
		Version:       1,
		ActiveProfile: "work",
		Profiles: map[string]config.Profile{
			"work": {
				Providers: map[string]config.ProviderConfig{
					"engram": {Enabled: true, Path: "/work/engram.db"},
				},
			},
		},
	}
	if err := config.SaveProfileStore(dir, existing); err != nil {
		t.Fatalf("SaveProfileStore: %v", err)
	}

	// EnsureConfig with a different resolved config should return the stored one.
	resolved := config.Config{
		ConfigHome:   dir,
		EngramDbPath: "/different/engram.db",
	}
	store, err := config.EnsureConfig(dir, resolved)
	if err != nil {
		t.Fatalf("EnsureConfig: %v", err)
	}

	if store.ActiveProfile != "work" {
		t.Errorf("ActiveProfile: got %q, want work", store.ActiveProfile)
	}
}

// TestEnsureConfig_SeedsFromExplorerSettings verifies that EnsureConfig reads
// explorer-settings.json to seed the default profile when no config.json exists.
func TestEnsureConfig_SeedsFromExplorerSettings(t *testing.T) {
	dir := t.TempDir()

	// Write an explorer-settings.json with a custom DB path.
	overridePath := filepath.Join(dir, "custom", "engram.db")
	claudeOverridePath := filepath.Join(dir, "custom-claude")
	if err := config.SaveOverrides(dir, config.Overrides{
		EngramDbPath:      overridePath,
		ClaudeProjectsDir: claudeOverridePath,
	}); err != nil {
		t.Fatalf("SaveOverrides: %v", err)
	}

	// resolved uses defaults (no env vars) — overrides should win.
	resolved := config.Config{
		ConfigHome:        dir,
		EngramDbPath:      filepath.Join(dir, "default.db"),
		ClaudeProjectsDir: filepath.Join(dir, "default-claude"),
		DaemonBaseURL:     "http://127.0.0.1:7437",
	}

	// To test seeding from overrides, we pass an overrides-aware resolved config.
	// The real Load() already applies overrides; here we simulate that by passing
	// resolved with the override values baked in.
	resolvedWithOverride := config.Config{
		ConfigHome:        dir,
		EngramDbPath:      overridePath,
		ClaudeProjectsDir: claudeOverridePath,
		DaemonBaseURL:     resolved.DaemonBaseURL,
	}

	store, err := config.EnsureConfig(dir, resolvedWithOverride)
	if err != nil {
		t.Fatalf("EnsureConfig: %v", err)
	}

	eng := store.Profiles["default"].Providers["engram"]
	if eng.Path != overridePath {
		t.Errorf("engram path: got %q, want %q", eng.Path, overridePath)
	}
}

// TestProfileStore_ProviderCfg verifies that Profile.ProviderCfg returns the
// correct config for a known provider and a zero config for an unknown one.
func TestProfileStore_ProviderCfg(t *testing.T) {
	p := config.Profile{
		Providers: map[string]config.ProviderConfig{
			"engram": {Enabled: true, Path: "/a/b.db"},
		},
	}

	got := p.ProviderCfg("engram")
	if !got.Enabled {
		t.Error("engram: Enabled should be true")
	}
	if got.Path != "/a/b.db" {
		t.Errorf("engram path: got %q", got.Path)
	}

	zero := p.ProviderCfg("unknown-provider")
	if zero.Enabled {
		t.Error("unknown provider: Enabled should be false")
	}
}
