// Package config loads application configuration from environment variables.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
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
	DaemonBaseURL    string
	DaemonTimeoutMs  int

	// Logging / env
	LogLevel string
	Env      string // "development" | "production"

	// Security: when false (production default), internal details are never
	// sent to clients. Set ENGRAM_DASH_ENV=development to enable detail leakage
	// during local development.
	ExposeDetails bool
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

	cfg := Config{
		Host:            envString("DASHBOARD_HOST", "127.0.0.1"),
		Port:            envInt("DASHBOARD_PORT", 8787),
		EngramDataDir:   dataDir,
		EngramDbPath:    filepath.Join(dataDir, "engram.db"),
		AuditLogPath:    filepath.Join(dataDir, "logs", "cloud-mutations.jsonl"),
		DaemonBaseURL:   envString("ENGRAM_DAEMON_URL", fmt.Sprintf("http://127.0.0.1:%d", engramPort)),
		DaemonTimeoutMs: envInt("ENGRAM_DAEMON_TIMEOUT_MS", 1500),
		LogLevel:        envString("LOG_LEVEL", defaultLogLevel),
		Env:             env,
		ExposeDetails:   env == "development",
	}
	return cfg, nil
}
