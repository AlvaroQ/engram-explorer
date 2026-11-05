// Package codexsessions implements the Provider port for local Codex CLI
// sessions: the rollout files under <home>/sessions (home = CODEX_HOME or
// ~/.codex). It serves them through the vendor-neutral agentsessions model
// (providers.AgentSessionSource) and is READ-ONLY: it never writes to the
// Codex home, skips symlinked entries inside its sessions directory and never
// executes anything.
package codexsessions

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/AlvaroQ/engram-explorer/internal/agentsessions"
	"github.com/AlvaroQ/engram-explorer/internal/agentsessions/codex"
	"github.com/AlvaroQ/engram-explorer/internal/config"
	"github.com/AlvaroQ/engram-explorer/internal/providers"
	"github.com/AlvaroQ/engram-explorer/internal/usererr"
)

// providerID is the stable machine identifier of this provider.
const providerID = "codex-sessions"

// SourceID is the ID of the single agent-session source of an instance. It
// doubles as the first segment of the detail URL the provider mounts.
const SourceID = codex.Agent

type codexProvider struct {
	cfg config.Config
	log *slog.Logger // registry logger (SetLogger); nil logs nothing
}

// NewProvider returns the Codex sessions Provider adapter.
func NewProvider(cfg config.Config) providers.Provider { return &codexProvider{cfg: cfg} }

// SetLogger receives the registry logger (providers.LoggerSetter).
func (p *codexProvider) SetLogger(log *slog.Logger) { p.log = log }

// ID returns the stable machine identifier.
func (p *codexProvider) ID() string { return providerID }

// DisplayName returns the human-readable label.
func (p *codexProvider) DisplayName() string { return "Codex" }

// ProviderTier returns Tier1: Codex stays visible in Settings when not
// auto-detected, offering manual path entry.
func (p *codexProvider) ProviderTier() providers.Tier { return providers.Tier1 }

// Featured returns false: only Engram is featured.
func (p *codexProvider) Featured() bool { return false }

// Detect reports whether <cfg.Path>/sessions is a readable directory. It is a
// pure check: no handle is kept.
func (p *codexProvider) Detect(_ context.Context, cfg providers.ProviderConfig) providers.Detection {
	if validateHome(cfg.Path) != nil {
		return providers.Detection{Available: false, Reason: "dir-missing", Path: cfg.Path}
	}
	return providers.Detection{Available: true, Reason: "ok", Path: cfg.Path}
}

// Validate checks a candidate Codex home (Settings → Modules path entry).
func (p *codexProvider) Validate(_ context.Context, cfg providers.ProviderConfig) error {
	if err := validateHome(cfg.Path); err != nil {
		return fmt.Errorf("codex-sessions: %w", err)
	}
	return nil
}

// validateHome requires <home>/sessions to be a readable directory. Messages
// are user-facing (usererr) and name only the path the caller supplied.
func validateHome(home string) error {
	if home == "" {
		return usererr.New("codex home path is required")
	}
	dir := filepath.Join(home, "sessions")
	info, err := os.Stat(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return usererr.New(fmt.Sprintf("codex sessions directory not found: %s", dir))
	}
	if err != nil {
		return usererr.New(fmt.Sprintf("cannot access codex sessions directory: %s", dir))
	}
	if !info.IsDir() {
		return usererr.New(fmt.Sprintf("not a directory: %s", dir))
	}
	f, err := os.Open(dir)
	if err != nil {
		return usererr.New(fmt.Sprintf("cannot read codex sessions directory: %s", dir))
	}
	defer f.Close()
	if _, err := f.Readdirnames(1); err != nil && !errors.Is(err, io.EOF) {
		return usererr.New(fmt.Sprintf("cannot read codex sessions directory: %s", dir))
	}
	return nil
}

// Open activates the provider: it validates the home and creates the source.
// An error leaves the provider Errored; it is never fatal.
func (p *codexProvider) Open(_ context.Context, cfg providers.ProviderConfig) (providers.Instance, error) {
	if err := validateHome(cfg.Path); err != nil {
		return nil, fmt.Errorf("codex-sessions: %w", err)
	}
	return &codexInstance{
		home:   cfg.Path,
		src:    codex.New(SourceID, cfg.Path),
		expose: p.cfg.ExposeDetails,
		health: providers.NewHealthLog(p.log),
	}, nil
}

// Nav contributes the shared "Agent sessions" sidebar group.
func (p *codexProvider) Nav(providers.Instance) providers.NavGroup {
	return providers.AgentSessionsNav()
}

// errSessionsUnavailable is the production health message.
const errSessionsUnavailable = "codex sessions directory unavailable"

// codexInstance is the live activation: one read-only codex.Source (it
// caches the parsed rollouts by fingerprint, so it is shared by requests).
type codexInstance struct {
	home   string
	src    *codex.Source
	expose bool
	health *providers.HealthLog
}

// AgentSessionSources implements providers.AgentSessionSource.
func (i *codexInstance) AgentSessionSources() []agentsessions.Source {
	return []agentsessions.Source{i.src}
}

// Health reports whether the sessions directory is still there. The error is
// generic in production and logged once per transition.
func (i *codexInstance) Health(context.Context) providers.ProviderHealth {
	h := providers.ProviderHealth{ID: providerID, Path: i.home}
	if err := validateHome(i.home); err != nil {
		i.health.Failing("codex sessions directory unavailable", "path", i.home, "err", err)
		h.Error = errSessionsUnavailable
		if i.expose {
			h.Error = err.Error()
		}
		return h
	}
	h.OK = true
	i.health.Healthy("codex sessions directory available again", "path", i.home)
	return h
}

// Close is a no-op: the source holds no open handle between requests.
func (i *codexInstance) Close(context.Context) error { return nil }
