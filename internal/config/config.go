// Package config loads application configuration from environment variables.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Config holds all runtime configuration for the engram-explorer backend.
type Config struct {
	// HTTP server
	Host string
	Port int

	// Data paths
	EngramDataDir string
	EngramDbPath  string
	AuditLogPath  string

	// ConfigHome is the directory where explorer-settings.json (path overrides)
	// is stored. Defaults to the same directory as EngramDataDir (~/.engram).
	ConfigHome string

	// Daemon
	DaemonBaseURL   string
	DaemonTimeoutMs int

	// Logging / env
	LogLevel string
	Env      string // "development" | "production"

	// Security: when false (production default), internal details are never
	// sent to clients. Set ENGRAM_DASH_ENV=development to enable detail leakage
	// during local development.
	ExposeDetails bool

	// ReadOnly, when true, prevents the backend from opening the read-write
	// pool at all, so the dashboard becomes a pure read-only viewer: every
	// mutating route (edits, deletes, project rename, db import, cloud
	// unenroll) returns 503. Set ENGRAM_DASH_READONLY=true to enable.
	ReadOnly bool

	// DemoMode, when true, means the server is running against the bundled
	// demo database. A dismissible banner is shown in the UI and all write
	// endpoints return 403. Set via --demo CLI flag or ENGRAM_DEMO=1.
	DemoMode bool

	// ClaudeProjectsDir is the directory where Claude Code stores transcript
	// files (~/.claude/projects by default). Each subdirectory is a project,
	// and each *.jsonl file within is a session transcript.
	// Override with CLAUDE_PROJECTS_DIR env var.
	ClaudeProjectsDir string
}

func envString(name, fallback string) string {
	v := os.Getenv(name)
	if v == "" {
		return fallback
	}
	return v
}

func envInt(name string, fallback int) int {
	v := os.Getenv(name)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

// envBool reads a boolean env var. "1", "true", "yes", "on" (case-insensitive)
// are truthy; anything else (including unset) falls back.
func envBool(name string, fallback bool) bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(name)))
	if v == "" {
		return fallback
	}
	switch v {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}

// Load reads all env vars and returns a Config.
// The only source of error is failing to resolve the home directory.
//
// Path resolution precedence (for both EngramDbPath and ClaudeProjectsDir):
//
//  1. Environment variable (ENGRAM_DATA_DIR / CLAUDE_PROJECTS_DIR)
//  2. Saved override in explorer-settings.json
//  3. Cross-platform default (~/.engram / ~/.claude/projects)
func Load() (Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Config{}, fmt.Errorf("resolve home dir: %w", err)
	}

	// configHome is where we store explorer-settings.json.
	// When ENGRAM_DATA_DIR is set it doubles as the config home; otherwise
	// we use the default ~/.engram.
	envDataDir := os.Getenv("ENGRAM_DATA_DIR")
	configHome := filepath.Join(home, ".engram")
	if envDataDir != "" {
		configHome = envDataDir
	}

	// Load saved user overrides (best-effort; missing file → empty struct).
	ov := LoadOverrides(configHome)

	// Resolve the Engram DB path: ENV > override > default.
	var dataDir, dbPath string
	if envDataDir != "" {
		dataDir = envDataDir
		dbPath = filepath.Join(dataDir, "engram.db")
	} else if ov.EngramDbPath != "" {
		dbPath = ov.EngramDbPath
		dataDir = filepath.Dir(dbPath)
	} else {
		dataDir = filepath.Join(home, ".engram")
		dbPath = filepath.Join(dataDir, "engram.db")
	}

	// Resolve the Claude projects dir: ENV > override > default.
	claudeProjectsDir := os.Getenv("CLAUDE_PROJECTS_DIR")
	if claudeProjectsDir == "" {
		if ov.ClaudeProjectsDir != "" {
			claudeProjectsDir = ov.ClaudeProjectsDir
		} else {
			claudeProjectsDir = filepath.Join(home, ".claude", "projects")
		}
	}

	engramPort := envInt("ENGRAM_PORT", 7437)

	rawEnv := envString("ENGRAM_DASH_ENV", "production")
	env := "production"
	if rawEnv == "development" {
		env = "development"
	}

	// Log level: default info in production, debug in development.
	defaultLogLevel := "info"
	if env == "development" {
		defaultLogLevel = "debug"
	}

	cfg := Config{
		Host:              envString("DASHBOARD_HOST", "127.0.0.1"),
		Port:              envInt("DASHBOARD_PORT", 8787),
		EngramDataDir:     dataDir,
		EngramDbPath:      dbPath,
		AuditLogPath:      filepath.Join(dataDir, "logs", "cloud-mutations.jsonl"),
		ConfigHome:        configHome,
		DaemonBaseURL:     envString("ENGRAM_DAEMON_URL", fmt.Sprintf("http://127.0.0.1:%d", engramPort)),
		DaemonTimeoutMs:   envInt("ENGRAM_DAEMON_TIMEOUT_MS", 1500),
		LogLevel:          envString("LOG_LEVEL", defaultLogLevel),
		Env:               env,
		ExposeDetails:     env == "development",
		ReadOnly:          envBool("ENGRAM_DASH_READONLY", false),
		ClaudeProjectsDir: claudeProjectsDir,
	}
	return cfg, nil
}
