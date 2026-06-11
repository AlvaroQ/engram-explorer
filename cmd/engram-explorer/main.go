// Command engram-explorer is the single-binary server for the Engram Explorer dashboard.
// It embeds the compiled React frontend and serves both the API and the SPA on
// one port (default 127.0.0.1:8787).
// Build with: make build  (runs pnpm frontend build → embed → go build)
// Version is injected via: -ldflags "-X main.version=<tag>"
package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/AlvaroQ/engram-explorer/internal/config"
	"github.com/AlvaroQ/engram-explorer/internal/httpapi"
	"github.com/AlvaroQ/engram-explorer/internal/logging"
	"github.com/AlvaroQ/engram-explorer/internal/providers"
	ccsessionsprovider "github.com/AlvaroQ/engram-explorer/internal/providers/ccsessions"
	engramprovider "github.com/AlvaroQ/engram-explorer/internal/providers/engram"
	"github.com/AlvaroQ/engram-explorer/internal/web"
)

// version is set at build time via -ldflags "-X main.version=<tag>".
var version = "dev"

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "--version", "-v", "version":
			fmt.Printf("engram-explorer %s\n", version)
			return
		}
	}
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	logger := logging.New(cfg.Env, cfg.LogLevel)

	// Ensure config.json exists (seeds default profile from env + legacy
	// explorer-settings.json on first run). This is non-fatal — if it fails
	// the server still starts with an in-memory profile derived from cfg.
	profileStore, err := config.EnsureConfig(cfg.ConfigHome, cfg)
	if err != nil {
		logger.Warn("could not load or seed config.json; using in-memory defaults",
			"err", err,
		)
		profileStore = &config.ProfileStore{
			Version:       1,
			ActiveProfile: "default",
			Profiles: map[string]config.Profile{
				"default": config.ProfileFromConfig(cfg),
			},
		}
	}

	activeProfile := profileStore.Profiles[profileStore.ActiveProfile]

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
	container.ProfileStore = profileStore
	defer container.Close()

	apiHandler := httpapi.NewServeMux(container)

	// Wrap the API mux with the embedded frontend handler.
	// All /api/* traffic is forwarded to apiHandler; everything else is served
	// from the embedded dist/ FS with SPA deep-link fallback.
	handler := web.Handler(apiHandler)

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
