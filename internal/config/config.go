// Package config loads application configuration.
//
// Configuration sources and precedence (highest first):
//
//  1. Environment variables (ENGRAM_DATA_DIR, CLAUDE_PROJECTS_DIR, CODEX_HOME,
//     ENGRAM_DAEMON_URL): in-memory overrides applied to the active profile at
//     boot. They are NEVER written to disk.
//  2. config.json in ConfigHome (ProfileStore): the ONLY persisted source.
//     Every change made from the UI is saved here, through Store.
//  3. Built-in defaults (~/.engram/engram.db, ~/.claude/projects, ~/.codex,
//     http://127.0.0.1:$ENGRAM_PORT).
//
// The legacy explorer-settings.json is read once, only to seed config.json
// when it does not exist yet (see EnsureConfig); it is never written.
package config

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/AlvaroQ/engram-explorer/internal/agentsessions/codex"
)

// codexProviderID is the provider ID of the Codex sessions provider.
const codexProviderID = "codex-sessions"

// Config holds all runtime configuration for the engram-explorer backend.
type Config struct {
	// HTTP server
	Host string
	Port int
	// AllowRemote, when true (DASHBOARD_ALLOW_REMOTE), permits a non-loopback
	// Host. The dashboard has no authentication, so this is off by default.
	AllowRemote bool

	// Data paths
	EngramDataDir string
	EngramDbPath  string

	// ConfigHome is the directory where config.json is stored (and where the
	// legacy explorer-settings.json is migrated from). It is ENGRAM_DATA_DIR
	// when set, otherwise ~/.engram.
	ConfigHome string

	// EnvOverrides holds the data-source settings supplied through environment
	// variables. They win over config.json at boot and are never persisted.
	EnvOverrides PathSettings

	// Defaults holds the built-in data-source settings, used when neither an
	// environment variable nor config.json provides a value.
	Defaults PathSettings

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
	// mutating route (edits, deletes, project rename/merge, db import) returns
	// 503. Cloud sync is read-only in every mode. Set ENGRAM_DASH_READONLY=true
	// to enable.
	ReadOnly bool

	// ClaudeProjectsDir is the directory where Claude Code stores transcript
	// files (~/.claude/projects by default). Each subdirectory is a project,
	// and each *.jsonl file within is a session transcript.
	// Override with CLAUDE_PROJECTS_DIR env var.
	ClaudeProjectsDir string

	// CodexHome is the Codex CLI home (~/.codex by default); the Codex
	// provider reads the rollouts under <CodexHome>/sessions. Override with
	// the CODEX_HOME env var.
	CodexHome string
}

// PathSettings groups the data-source settings that a profile can also hold.
// Empty fields mean "not set".
type PathSettings struct {
	EngramDbPath      string // ENGRAM_DATA_DIR/engram.db
	ClaudeProjectsDir string // CLAUDE_PROJECTS_DIR
	CodexHome         string // CODEX_HOME
	DaemonBaseURL     string // ENGRAM_DAEMON_URL
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
// It fails when the home directory cannot be resolved or the bind host is not
// loopback without DASHBOARD_ALLOW_REMOTE.
//
// EngramDbPath, ClaudeProjectsDir and DaemonBaseURL hold the environment value
// when set, otherwise the built-in default; EnvOverrides and Defaults keep the
// two apart. Load does not read config.json: the boot code applies the active
// profile with Config.WithProfile (precedence env > config.json > default).
func Load() (Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Config{}, fmt.Errorf("resolve home dir: %w", err)
	}

	engramPort := envInt("ENGRAM_PORT", 7437)
	defaults := PathSettings{
		EngramDbPath:      filepath.Join(home, ".engram", "engram.db"),
		ClaudeProjectsDir: filepath.Join(home, ".claude", "projects"),
		CodexHome:         codex.DefaultHome(nil, home),
		DaemonBaseURL:     fmt.Sprintf("http://127.0.0.1:%d", engramPort),
	}

	// ENGRAM_DATA_DIR doubles as the config home; otherwise ~/.engram.
	var overrides PathSettings
	configHome := filepath.Dir(defaults.EngramDbPath)
	if dir := os.Getenv("ENGRAM_DATA_DIR"); dir != "" {
		configHome = dir
		overrides.EngramDbPath = filepath.Join(dir, "engram.db")
	}
	overrides.ClaudeProjectsDir = os.Getenv("CLAUDE_PROJECTS_DIR")
	overrides.CodexHome = strings.TrimSpace(os.Getenv("CODEX_HOME"))
	overrides.DaemonBaseURL = os.Getenv("ENGRAM_DAEMON_URL")

	dbPath := firstNonEmpty(overrides.EngramDbPath, defaults.EngramDbPath)

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
		EngramDataDir:     filepath.Dir(dbPath),
		EngramDbPath:      dbPath,
		ConfigHome:        configHome,
		EnvOverrides:      overrides,
		Defaults:          defaults,
		DaemonBaseURL:     firstNonEmpty(overrides.DaemonBaseURL, defaults.DaemonBaseURL),
		DaemonTimeoutMs:   envInt("ENGRAM_DAEMON_TIMEOUT_MS", 1500),
		LogLevel:          envString("LOG_LEVEL", defaultLogLevel),
		Env:               env,
		ExposeDetails:     env == "development",
		ReadOnly:          envBool("ENGRAM_DASH_READONLY", false),
		ClaudeProjectsDir: firstNonEmpty(overrides.ClaudeProjectsDir, defaults.ClaudeProjectsDir),
		CodexHome:         firstNonEmpty(overrides.CodexHome, defaults.CodexHome),
		AllowRemote:       envBool("DASHBOARD_ALLOW_REMOTE", false),
	}
	if !cfg.AllowRemote && !IsLoopbackHost(cfg.Host) {
		return Config{}, fmt.Errorf(
			"refusing to start: DASHBOARD_HOST=%q is not a loopback address. "+
				"engram-explorer has no authentication and only listens on 127.0.0.1, ::1 or localhost by default; "+
				"set DASHBOARD_ALLOW_REMOTE=true to bind it anyway at your own risk", cfg.Host)
	}
	return cfg, nil
}

// WithProfile returns c with the data-source settings of the (env-applied)
// active profile p, so the startup snapshot matches what the providers open.
// Settings p leaves empty keep their current value.
func (c Config) WithProfile(p Profile) Config {
	engram := p.ProviderCfg("engram")
	if engram.Path != "" {
		c.EngramDbPath = engram.Path
		c.EngramDataDir = filepath.Dir(engram.Path)
	}
	if engram.DaemonURL != "" {
		c.DaemonBaseURL = engram.DaemonURL
	}
	if dir := p.ProviderCfg("cc-sessions").Path; dir != "" {
		c.ClaudeProjectsDir = dir
	}
	if dir := p.ProviderCfg(codexProviderID).Path; dir != "" {
		c.CodexHome = dir
	}
	return c
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// IsLoopbackHost reports whether host is "localhost" (case-insensitive) or a
// loopback IP literal. The unspecified addresses (0.0.0.0, ::) are not loopback.
func IsLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
