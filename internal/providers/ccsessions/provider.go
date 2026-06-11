// Package ccsessions implements the Provider port for the Claude Code sessions
// data source. It wraps the existing internal/services/cc_sessions.go reader
// and the internal/httpapi/routes_cc_sessions.go + internal/ui cc-sessions routes
// behind the providers.Provider interface.
//
// This provider is READ-ONLY: it does NOT implement the providers.Writable
// capability. The CC sessions store is owned by Claude Code — this provider
// only reads .jsonl transcript files.
package ccsessions

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"github.com/AlvaroQ/engram-explorer/internal/config"
	"github.com/AlvaroQ/engram-explorer/internal/providers"
)

// ---------------------------------------------------------------------------
// ccProvider — implements providers.Provider
// ---------------------------------------------------------------------------

// ccProvider is the singleton descriptor for the Claude Code sessions data source.
type ccProvider struct {
	cfg config.Config
}

// NewProvider returns the Claude Code sessions Provider adapter.
func NewProvider(cfg config.Config) providers.Provider {
	return &ccProvider{cfg: cfg}
}

// ID returns the stable machine identifier.
func (p *ccProvider) ID() string { return "cc-sessions" }

// DisplayName returns the human-readable label.
func (p *ccProvider) DisplayName() string { return "Claude Code Sessions" }

// ProviderTier returns Tier1: CC sessions stays visible in Settings when not
// auto-detected, offering manual path entry.
func (p *ccProvider) ProviderTier() providers.Tier { return providers.Tier1 }

// Featured returns false: CC sessions is not the primary featured provider.
func (p *ccProvider) Featured() bool { return false }

// Detect reports whether a valid Claude Code projects directory exists at cfg.Path.
// Contract (design verdict 4):
//  1. Directory must exist and be a directory.
//  2. Directory must contain at least one subdirectory (project folder).
//
// It is a pure check — no long-lived handles opened.
func (p *ccProvider) Detect(ctx context.Context, cfg providers.ProviderConfig) providers.Detection {
	path := cfg.Path
	if path == "" {
		return providers.Detection{Available: false, Reason: "dir-missing", Path: path}
	}

	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return providers.Detection{Available: false, Reason: "dir-missing", Path: path}
	}

	// Check for at least one project subfolder.
	entries, err := os.ReadDir(path)
	if err != nil {
		return providers.Detection{Available: false, Reason: "no-projects", Path: path}
	}
	for _, e := range entries {
		if e.IsDir() {
			return providers.Detection{Available: true, Reason: "ok", Path: path}
		}
	}

	return providers.Detection{Available: false, Reason: "no-projects", Path: path}
}

// Open activates the CC sessions provider. For a filesystem reader there are
// no long-lived handles to acquire — the DiskProjectsReader is constructed per
// request from RuntimePaths. Open validates the path and records it.
//
// Open returns an error if the directory is absent. The registry treats an
// Open error as Errored state and continues — it is NEVER fatal.
func (p *ccProvider) Open(ctx context.Context, cfg providers.ProviderConfig) (providers.Instance, error) {
	path := cfg.Path
	if path == "" {
		return nil, fmt.Errorf("cc-sessions: path is required")
	}

	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("cc-sessions: projects directory not found at %q: %w", path, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("cc-sessions: path %q is not a directory", path)
	}

	appCfg := p.cfg
	appCfg.ClaudeProjectsDir = path

	runtimePaths := config.NewRuntimePaths(appCfg.EngramDbPath, path)

	return &ccInstance{
		cfg:          appCfg,
		runtimePaths: runtimePaths,
		path:         path,
	}, nil
}

// Routes registers the Claude Code sessions HTTP routes on mux. It calls the
// same route-group functions that server.go and ui.Mount call today, moved
// verbatim (incremental adapter strategy).
//
// Like the Engram provider, the actual route wiring is a stub in WU-4 to avoid
// the import cycle (providers/ccsessions cannot import internal/httpapi or
// internal/ui). WU-5 (container wiring) will perform the real registration by
// calling the existing ccSessionsRoutes and ui.Mount CC routes through the
// container which already imports both packages.
func (p *ccProvider) Routes(mux *http.ServeMux, inst providers.Instance, logger *slog.Logger) {
	// Stub: route wiring deferred to WU-5 (container) to avoid import cycle.
}

// Nav returns the sidebar NavGroup for the CC sessions provider.
func (p *ccProvider) Nav(inst providers.Instance) providers.NavGroup {
	return providers.NavGroup{
		ID:       "cc-sessions",
		LabelKey: "nav.ccsessions",
		Featured: false,
		Links: []providers.NavLink{
			{Href: "/cc-sessions", Key: "cc-sessions", LabelKey: "nav.ccsessions.list"},
			{Href: "/cc-overview", Key: "cc-overview", LabelKey: "nav.ccsessions.overview"},
		},
	}
}

// Validate checks a candidate config before activation (Tier-1 manual path
// entry). For CC sessions (design verdict 4):
//  1. Path must not be empty.
//  2. Path must exist and be a directory.
//  3. Directory must contain at least one project subfolder (ReadDir check).
//
// Path-component safety: ValidateCCSessionPath is the per-request guard for
// individual session file access; at provider-level validation we check the
// base directory itself.
func (p *ccProvider) Validate(ctx context.Context, cfg providers.ProviderConfig) error {
	path := cfg.Path
	if path == "" {
		return fmt.Errorf("cc-sessions: path is required")
	}

	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("cc-sessions: projects directory not found at %q: %w", path, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("cc-sessions: path %q is not a directory", path)
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		return fmt.Errorf("cc-sessions: cannot read projects directory %q: %w", path, err)
	}
	for _, e := range entries {
		if e.IsDir() {
			return nil // at least one project folder found
		}
	}

	return fmt.Errorf("cc-sessions: no project folders found in %q — directory appears empty", path)
}

// ---------------------------------------------------------------------------
// ccInstance — implements providers.Instance (NO Writable — read-only)
// ---------------------------------------------------------------------------

// ccInstance is the per-profile live activation of the CC sessions provider.
// Because the DiskProjectsReader has no persistent handle (reads per request),
// this struct only holds the resolved path and config.
type ccInstance struct {
	cfg          config.Config
	runtimePaths *config.RuntimePaths
	path         string
}

// Health returns per-provider health for /api/health aggregation. For CC
// sessions the health check is whether the projects directory still exists.
func (i *ccInstance) Health(ctx context.Context) providers.ProviderHealth {
	h := providers.ProviderHealth{
		ID:   "cc-sessions",
		Path: i.path,
	}

	info, err := os.Stat(i.path)
	if err != nil || !info.IsDir() {
		if err != nil {
			h.Error = err.Error()
		} else {
			h.Error = "path is not a directory"
		}
		return h
	}

	h.OK = true
	return h
}

// Close is a no-op for the CC sessions provider — there are no persistent
// handles to release. It satisfies the providers.Instance interface.
func (i *ccInstance) Close(ctx context.Context) error {
	return nil
}

// RuntimePaths returns the RuntimePaths held by this instance. Used by WU-5
// (container wiring) to construct the DiskProjectsReader for route handlers.
func (i *ccInstance) RuntimePaths() *config.RuntimePaths { return i.runtimePaths }

// Config returns the app config held by this instance. Used by WU-5.
func (i *ccInstance) Config() config.Config { return i.cfg }
