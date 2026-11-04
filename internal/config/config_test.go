package config_test

import (
	"path/filepath"
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/config"
)

func TestLoad_Defaults(t *testing.T) {
	// Hermetic home so a real ~/.engram/explorer-settings.json on the dev machine
	// cannot override the defaults under test.
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	// Ensure the vars we test are not set in the test environment.
	unset := []string{
		"ENGRAM_DATA_DIR", "DASHBOARD_HOST", "DASHBOARD_PORT",
		"ENGRAM_PORT", "ENGRAM_DAEMON_URL", "ENGRAM_DAEMON_TIMEOUT_MS",
		"LOG_LEVEL", "ENGRAM_DASH_ENV", "CLAUDE_PROJECTS_DIR",
	}
	for _, k := range unset {
		t.Setenv(k, "")
	}

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}

	if cfg.Host != "127.0.0.1" {
		t.Errorf("Host: got %q, want 127.0.0.1", cfg.Host)
	}
	if cfg.Port != 8787 {
		t.Errorf("Port: got %d, want 8787", cfg.Port)
	}
	if cfg.DaemonTimeoutMs != 1500 {
		t.Errorf("DaemonTimeoutMs: got %d, want 1500", cfg.DaemonTimeoutMs)
	}
	if cfg.DaemonBaseURL != "http://127.0.0.1:7437" {
		t.Errorf("DaemonBaseURL: got %q, want http://127.0.0.1:7437", cfg.DaemonBaseURL)
	}

	wantDataDir := filepath.Join(home, ".engram")
	if cfg.EngramDataDir != wantDataDir {
		t.Errorf("EngramDataDir: got %q, want %q", cfg.EngramDataDir, wantDataDir)
	}
	if cfg.EngramDbPath != filepath.Join(wantDataDir, "engram.db") {
		t.Errorf("EngramDbPath: got %q", cfg.EngramDbPath)
	}
}

func TestLoad_ExposeDetailsDefaultFalse(t *testing.T) {
	t.Setenv("ENGRAM_DASH_ENV", "")
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}
	if cfg.ExposeDetails {
		t.Error("ExposeDetails should default to false (production)")
	}
	if cfg.Env != "production" {
		t.Errorf("Env: got %q, want production", cfg.Env)
	}
}

func TestLoad_ExposeDetailsInDevelopment(t *testing.T) {
	t.Setenv("ENGRAM_DASH_ENV", "development")
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}
	if !cfg.ExposeDetails {
		t.Error("ExposeDetails should be true when ENGRAM_DASH_ENV=development")
	}
}

func TestLoad_EnvOverrides(t *testing.T) {
	t.Setenv("DASHBOARD_HOST", "0.0.0.0")
	t.Setenv("DASHBOARD_ALLOW_REMOTE", "true")
	t.Setenv("DASHBOARD_PORT", "9000")
	t.Setenv("ENGRAM_DATA_DIR", "/tmp/test-engram")
	t.Setenv("LOG_LEVEL", "warn")
	t.Setenv("ENGRAM_DAEMON_TIMEOUT_MS", "3000")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}

	if cfg.Host != "0.0.0.0" {
		t.Errorf("Host: got %q, want 0.0.0.0", cfg.Host)
	}
	if cfg.Port != 9000 {
		t.Errorf("Port: got %d, want 9000", cfg.Port)
	}
	if cfg.EngramDataDir != "/tmp/test-engram" {
		t.Errorf("EngramDataDir: got %q", cfg.EngramDataDir)
	}
	if cfg.LogLevel != "warn" {
		t.Errorf("LogLevel: got %q", cfg.LogLevel)
	}
	if cfg.DaemonTimeoutMs != 3000 {
		t.Errorf("DaemonTimeoutMs: got %d", cfg.DaemonTimeoutMs)
	}
}

func TestLoad_LogLevelDefaults(t *testing.T) {
	t.Run("production defaults to info", func(t *testing.T) {
		t.Setenv("ENGRAM_DASH_ENV", "production")
		t.Setenv("LOG_LEVEL", "")
		cfg, _ := config.Load()
		if cfg.LogLevel != "info" {
			t.Errorf("got %q, want info", cfg.LogLevel)
		}
	})
	t.Run("development defaults to debug", func(t *testing.T) {
		t.Setenv("ENGRAM_DASH_ENV", "development")
		t.Setenv("LOG_LEVEL", "")
		cfg, _ := config.Load()
		if cfg.LogLevel != "debug" {
			t.Errorf("got %q, want debug", cfg.LogLevel)
		}
	})
}

func TestSaveLoadOverrides_Roundtrip(t *testing.T) {
	dir := t.TempDir()
	want := config.Overrides{
		EngramDbPath:      filepath.Join(dir, "db", "engram.db"),
		ClaudeProjectsDir: filepath.Join(dir, "claude", "projects"),
	}
	if err := config.SaveOverrides(dir, want); err != nil {
		t.Fatalf("SaveOverrides: %v", err)
	}
	got := config.LoadOverrides(dir)
	if got != want {
		t.Fatalf("roundtrip mismatch: got %+v, want %+v", got, want)
	}
}

func TestLoadOverrides_MissingFileIsEmpty(t *testing.T) {
	got := config.LoadOverrides(t.TempDir())
	if got != (config.Overrides{}) {
		t.Fatalf("missing file should yield empty Overrides, got %+v", got)
	}
}

// TestLoad_IgnoresLegacySettingsFile: config.Load resolves env > built-in
// default only. The legacy explorer-settings.json is no longer a live source
// (it only seeds a new config.json, see EnsureConfig); env values are recorded
// in EnvOverrides so they can be applied without being persisted.
func TestLoad_IgnoresLegacySettingsFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("ENGRAM_DATA_DIR", "")
	t.Setenv("CLAUDE_PROJECTS_DIR", "")

	configHome := filepath.Join(home, ".engram")
	customDB := filepath.Join(home, "custom", "engram.db")
	customClaude := filepath.Join(home, "custom-claude")
	if err := config.SaveOverrides(configHome, config.Overrides{
		EngramDbPath:      customDB,
		ClaudeProjectsDir: customClaude,
	}); err != nil {
		t.Fatalf("SaveOverrides: %v", err)
	}

	// With no env vars set, Load returns the built-in defaults.
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	defDB := filepath.Join(configHome, "engram.db")
	if cfg.EngramDbPath != defDB || cfg.Defaults.EngramDbPath != defDB {
		t.Errorf("EngramDbPath: got %q (default %q), want %q", cfg.EngramDbPath, cfg.Defaults.EngramDbPath, defDB)
	}
	if want := filepath.Join(home, ".claude", "projects"); cfg.ClaudeProjectsDir != want {
		t.Errorf("ClaudeProjectsDir: got %q, want default %q", cfg.ClaudeProjectsDir, want)
	}
	if cfg.EnvOverrides != (config.PathSettings{}) {
		t.Errorf("EnvOverrides: got %+v, want none", cfg.EnvOverrides)
	}

	// An env var wins over the default and is recorded as an override.
	envDir := filepath.Join(home, "envdir")
	t.Setenv("ENGRAM_DATA_DIR", envDir)
	cfg2, err := config.Load()
	if err != nil {
		t.Fatalf("Load (env): %v", err)
	}
	want := filepath.Join(envDir, "engram.db")
	if cfg2.EngramDbPath != want || cfg2.EnvOverrides.EngramDbPath != want {
		t.Errorf("env should win: got %q (override %q), want %q", cfg2.EngramDbPath, cfg2.EnvOverrides.EngramDbPath, want)
	}
	if cfg2.Defaults.EngramDbPath != defDB {
		t.Errorf("Defaults.EngramDbPath: got %q, want %q", cfg2.Defaults.EngramDbPath, defDB)
	}
	if cfg2.ConfigHome != envDir {
		t.Errorf("ConfigHome: got %q, want ENGRAM_DATA_DIR %q", cfg2.ConfigHome, envDir)
	}
}
