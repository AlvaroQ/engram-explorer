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
	if cfg.AuditLogPath != filepath.Join(wantDataDir, "logs", "cloud-mutations.jsonl") {
		t.Errorf("AuditLogPath: got %q", cfg.AuditLogPath)
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

func TestLoad_FileOverridePrecedence(t *testing.T) {
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

	// With no env vars set, the saved override takes precedence over defaults.
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.EngramDbPath != customDB {
		t.Errorf("EngramDbPath: got %q, want override %q", cfg.EngramDbPath, customDB)
	}
	if cfg.EngramDataDir != filepath.Dir(customDB) {
		t.Errorf("EngramDataDir: got %q, want %q", cfg.EngramDataDir, filepath.Dir(customDB))
	}
	if cfg.ClaudeProjectsDir != customClaude {
		t.Errorf("ClaudeProjectsDir: got %q, want override %q", cfg.ClaudeProjectsDir, customClaude)
	}

	// The env var wins over the saved override.
	envDir := filepath.Join(home, "envdir")
	t.Setenv("ENGRAM_DATA_DIR", envDir)
	cfg2, err := config.Load()
	if err != nil {
		t.Fatalf("Load (env): %v", err)
	}
	if want := filepath.Join(envDir, "engram.db"); cfg2.EngramDbPath != want {
		t.Errorf("env should win: got %q, want %q", cfg2.EngramDbPath, want)
	}
}
