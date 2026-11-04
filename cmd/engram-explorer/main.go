// Command engram-explorer is the single-binary server for the Engram Explorer dashboard.
// It embeds the compiled React island bundles and serves the templ UI, the API
// and the islands on one port (default 127.0.0.1:8787).
// Build with: make build  (runs pnpm frontend build → embed → go build)
// Version is injected via: -ldflags "-X main.version=<tag>"
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/AlvaroQ/engram-explorer/internal/config"
	"github.com/AlvaroQ/engram-explorer/internal/httpapi"
	"github.com/AlvaroQ/engram-explorer/internal/logging"
	"github.com/AlvaroQ/engram-explorer/internal/providers"
	ccsessionsprovider "github.com/AlvaroQ/engram-explorer/internal/providers/ccsessions"
	engramprovider "github.com/AlvaroQ/engram-explorer/internal/providers/engram"
)

// version is set at build time via -ldflags "-X main.version=<tag>".
var version = "dev"

// removedDemoOptions lists the command-line flags and environment variables of
// the retired demo mode. They are rejected instead of ignored: a user who still
// passes one asked for the sample dataset and must not be served their real data.
var (
	removedDemoFlags = []string{"--demo", "--demo-db"}
	removedDemoEnv   = []string{"ENGRAM_DEMO", "ENGRAM_DEMO_DB"}
)

// checkRemovedDemoOptions returns an error naming the replacement when args
// contain a removed demo flag (bare or with =value) or getenv reports a
// removed demo variable with a non-empty value.
func checkRemovedDemoOptions(args []string, getenv func(string) string) error {
	var found []string
	for _, a := range args {
		for _, f := range removedDemoFlags {
			if a == f || strings.HasPrefix(a, f+"=") {
				found = append(found, f)
			}
		}
	}
	for _, k := range removedDemoEnv {
		if strings.TrimSpace(getenv(k)) != "" {
			found = append(found, k)
		}
	}
	if len(found) == 0 {
		return nil
	}
	return fmt.Errorf("demo mode was removed; %s is no longer supported. "+
		"Run `make demo` to browse the sample dataset, or point a normal run at any dataset with "+
		"ENGRAM_DATA_DIR=<dir> CLAUDE_PROJECTS_DIR=<dir> ENGRAM_DASH_READONLY=true",
		strings.Join(found, ", "))
}

func main() {
	// Scan every arg (not just os.Args[1]) so the version flag still works when
	// combined with other flags, e.g. `engram-explorer --foo --version`.
	for _, a := range os.Args[1:] {
		switch a {
		case "--version", "-v", "version":
			fmt.Printf("engram-explorer %s\n", version)
			return
		}
	}
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	// Fail before loading configuration: nothing may be read, created or served.
	if err := checkRemovedDemoOptions(args, os.Getenv); err != nil {
		return err
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	logger := logging.New(cfg.Env, cfg.LogLevel)
	// UI and doctor handlers log internal error details through slog.Default()
	// (the user only sees a generic message), so route it to the configured logger.
	slog.SetDefault(logger)

	container := buildContainer(cfg, logger)
	defer container.Close()
	n := container.Registry.ActiveCount()

	// NewHandler composes the request guard (Host allow-list, cross-origin
	// protection, security headers) in front of the embedded island bundles
	// (/islands/*, /assets/*) and the API/UI mux.
	handler := httpapi.NewHandler(container)

	addr := net.JoinHostPort(cfg.Host, fmt.Sprintf("%d", cfg.Port))
	srv := &http.Server{
		Addr:         addr,
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Graceful shutdown on SIGINT / SIGTERM.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-stop
		logger.Info("shutting down", "signal", sig.String())

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			logger.Error("server shutdown error", "err", err)
		}
	}()

	logger.Info("engram-explorer listening",
		"host", cfg.Host,
		"port", cfg.Port,
		"active_providers", n,
		"env", cfg.Env,
		"version", version,
	)

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("listen: %w", err)
	}
	return nil
}

// buildContainer loads the profile configuration, boots the provider registry
// and returns the HTTP container. Boot is never fatal.
func buildContainer(cfg config.Config, logger *slog.Logger) *httpapi.Container {
	// Configuration precedence: env > config.json (active profile) > default.
	// config.json is the only persisted source; env overrides are applied in
	// memory by the store and never saved. Demo mode never reads or writes
	// config.json: it serves the demo database from an ephemeral store.
	var store *config.Store
	if cfg.DemoMode {
		store = config.NewMemoryStore(cfg)
	} else {
		var err error
		if store, err = config.OpenStore(cfg); err != nil {
			// Non-fatal, and never overwrite an unreadable config.json: run on an
			// ephemeral profile derived from env + defaults.
			logger.Warn("could not load or seed config.json; using in-memory defaults (changes will not be saved)",
				"err", err,
			)
			store = config.NewMemoryStore(cfg)
		}
	}
	_, persisted, _ := store.ActiveProfile()
	activeProfile := store.Effective(persisted)
	cfg = cfg.WithProfile(activeProfile)

	// Build the provider registry and boot all providers. Boot is NEVER fatal:
	// a provider that fails to open enters Errored state and the server
	// continues. With zero active providers, the server serves the UI shell and
	// /api/health, ready for the onboarding flow (WU-10).
	reg := providers.NewRegistry(logger)
	reg.Register(engramprovider.NewProvider(cfg))
	reg.Register(ccsessionsprovider.NewProvider(cfg))
	reg.Boot(context.Background(), httpapi.AdaptProfile(activeProfile))

	n := reg.ActiveCount()
	if n == 0 {
		logger.Info("no providers active at startup — server running in zero-provider mode",
			"hint", "visit /settings to configure a data source",
		)
	} else {
		logger.Info("providers booted", "active", n)
	}

	// NewContainerWithRegistry never calls sqlite.OpenReadOnly — it gets DB
	// handles from the registry's Engram instance (if any). This is the
	// non-fatal-boot path: the server starts whether Engram is present or not.
	container := httpapi.NewContainerWithRegistry(reg, cfg, logger)
	container.ProfileStore = store

	return container
}
