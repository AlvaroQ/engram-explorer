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

	container, err := httpapi.NewContainer(cfg, logger)
	if err != nil {
		logger.Error("failed to open Engram database",
			"path", cfg.EngramDbPath,
			"err", err,
		)
		return fmt.Errorf("open database: %w", err)
	}
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
		"db", cfg.EngramDbPath,
		"env", cfg.Env,
		"version", version,
	)

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("listen: %w", err)
	}
	return nil
}
