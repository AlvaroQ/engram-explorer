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
func Load() (Config, error) {
	dataDir := os.Getenv("ENGRAM_DATA_DIR")
	if dataDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return Config{}, fmt.Errorf("resolve home dir: %w", err)
		}
		dataDir = filepath.Join(home, ".engram")
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

	claudeProjectsDir := os.Getenv("CLAUDE_PROJECTS_DIR")
	if claudeProjectsDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return Config{}, fmt.Errorf("resolve home dir for claude projects: %w", err)
		}
		claudeProjectsDir = filepath.Join(home, ".claude", "projects")
	}

	cfg := Config{
		Host:              envString("DASHBOARD_HOST", "127.0.0.1"),
		Port:              envInt("DASHBOARD_PORT", 8787),
		EngramDataDir:     dataDir,
		EngramDbPath:      filepath.Join(dataDir, "engram.db"),
		AuditLogPath:      filepath.Join(dataDir, "logs", "cloud-mutations.jsonl"),
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
