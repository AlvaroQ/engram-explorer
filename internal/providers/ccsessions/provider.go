// Package ccsessions implements the Provider port for the Claude Code sessions
// data source. It wraps the internal/services/cc_sessions*.go reader behind
// the providers.Provider interface and serves every enabled Claude account as
// a vendor-neutral agent-session source (providers.AgentSessionSource).
//
// This provider is READ-ONLY: it does NOT implement the providers.Writable
// capability. The CC sessions store is owned by Claude Code — this provider
// only reads .jsonl transcript files.
package ccsessions

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"sync"

	"github.com/AlvaroQ/engram-explorer/internal/config"
	"github.com/AlvaroQ/engram-explorer/internal/providers"
	"github.com/AlvaroQ/engram-explorer/internal/usererr"
)

// ---------------------------------------------------------------------------
// ccProvider — implements providers.Provider
// ---------------------------------------------------------------------------

// ccProvider is the singleton descriptor for the Claude Code sessions data source.
type ccProvider struct {
	cfg      config.Config
	log      *slog.Logger              // registry logger (SetLogger); nil logs nothing
	accounts func() []config.CCAccount // WithAccounts; nil serves the provider path
	stats    statsCache                // deprecated /api/cc-sessions/stats
}

// NewProvider returns the Claude Code sessions Provider adapter.
func NewProvider(cfg config.Config, opts ...Option) providers.Provider {
	p := &ccProvider{cfg: cfg}
	for _, o := range opts {
		o(p)
	}
	return p
}

// SetLogger receives the registry logger (providers.LoggerSetter); instances
// opened afterwards log their health transitions through it.
func (p *ccProvider) SetLogger(log *slog.Logger) { p.log = log }

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
		health:       providers.NewHealthLog(p.log),
		accounts:     p.accounts,
	}, nil
}

// Nav contributes the shared "Agent sessions" sidebar group (the registry
// merges it with the other agent-session providers').
func (p *ccProvider) Nav(providers.Instance) providers.NavGroup {
	return providers.AgentSessionsNav()
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
	if err := ValidateProjectsDir(cfg.Path); err != nil {
		return fmt.Errorf("cc-sessions: %w", err)
	}
	return nil
}

// ValidateProjectsDir is the single Claude Code projects-directory check shared
// by the provider's Validate (Settings Modules path) and the Settings Claude
// dir switcher: the path must be an existing directory holding at least one
// project subfolder.
//
// Error messages are user-facing (usererr): they name only the path the caller
// supplied and never wrap the underlying OS error text.
func ValidateProjectsDir(path string) error {
	if path == "" {
		return usererr.New("projects directory path is required")
	}

	info, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return usererr.New(fmt.Sprintf("projects directory not found: %s", path))
	}
	if err != nil {
		return usererr.New(fmt.Sprintf("cannot access projects directory: %s", path))
	}
	if !info.IsDir() {
		return usererr.New(fmt.Sprintf("not a directory: %s", path))
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		return usererr.New(fmt.Sprintf("cannot read projects directory: %s", path))
	}
	for _, e := range entries {
		if e.IsDir() {
			return nil // at least one project folder found
		}
	}

	return usererr.New(fmt.Sprintf("no project folders found in %s — directory appears empty", path))
}

// ---------------------------------------------------------------------------
// ccInstance — implements providers.Instance (NO Writable — read-only)
// ---------------------------------------------------------------------------

// errProjectsDirUnavailable is the production health message for a missing or
// unreadable projects directory.
const errProjectsDirUnavailable = "projects directory unavailable"

// ccInstance is the per-profile live activation of the CC sessions provider.
// Because the DiskProjectsReader has no persistent handle (reads per request),
// this struct only holds the resolved path and config.
type ccInstance struct {
	cfg          config.Config
	runtimePaths *config.RuntimePaths
	path         string
	health       *providers.HealthLog
	accounts     func() []config.CCAccount

	mu      sync.Mutex
	sources map[string]*claudeSource // by account id + path (AgentSessionSources)
}

// Health returns per-provider health for /api/health aggregation. For CC
// sessions the health check is whether the projects directory still exists.
// The error is generic in production (the OS error text names the path and
// goes to the log, once per transition — see providers.HealthLog);
// development (ExposeDetails) shows the detail.
func (i *ccInstance) Health(ctx context.Context) providers.ProviderHealth {
	h := providers.ProviderHealth{
		ID:   "cc-sessions",
		Path: i.path,
	}

	info, err := os.Stat(i.path)
	if err != nil || !info.IsDir() {
		detail := "path is not a directory"
		if err != nil {
			detail = err.Error()
		}
		i.health.Failing("claude code projects directory unavailable", "path", i.path, "err", detail)
		h.Error = errProjectsDirUnavailable
		if i.cfg.ExposeDetails {
			h.Error = detail
		}
		return h
	}

	h.OK = true
	i.health.Healthy("claude code projects directory available again", "path", i.path)
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
