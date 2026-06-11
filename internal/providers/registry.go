package providers

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
)

// State is the lifecycle state of a provider entry in the registry.
type State int

const (
	// Registered — provider type is known but not yet detected/opened.
	Registered State = iota
	// Detected — source is available (Detect returned true) but not opened yet.
	Detected
	// Enabled — provider is open and serving requests.
	Enabled
	// Disabled — user toggled the provider off.
	Disabled
	// Errored — detect or open returned an error; provider is not serving.
	Errored
)

func (s State) String() string {
	switch s {
	case Registered:
		return "registered"
	case Detected:
		return "detected"
	case Enabled:
		return "enabled"
	case Disabled:
		return "disabled"
	case Errored:
		return "errored"
	default:
		return "unknown"
	}
}

// activeEntry holds one provider's runtime state.
type activeEntry struct {
	p     Provider
	inst  Instance // nil unless state == Enabled
	state State
	err   error
}

// Registry holds an ordered list of providers and their per-profile runtime
// state. Engram (featured=true) is always iterated first. All mutations are
// guarded by a read-write mutex so the registry is safe for concurrent access.
type Registry struct {
	mu      sync.RWMutex
	order   []string // stable render order; featured providers sorted first
	entries map[string]*activeEntry
	log     *slog.Logger
}

// NewRegistry creates an empty registry. Providers are added via Register.
func NewRegistry(log *slog.Logger) *Registry {
	return &Registry{
		entries: make(map[string]*activeEntry),
		log:     log,
	}
}

// Register adds a provider to the registry. The order of registration determines
// render order within the same Featured tier (featured providers are always
// sorted before non-featured ones regardless of registration order).
// Register panics if a provider with the same ID is registered twice.
func (r *Registry) Register(p Provider) {
	r.mu.Lock()
	defer r.mu.Unlock()

	id := p.ID()
	if _, exists := r.entries[id]; exists {
		panic(fmt.Sprintf("providers: duplicate provider ID %q", id))
	}
	r.entries[id] = &activeEntry{p: p, state: Registered}

	// Insert into order: featured providers go first.
	if p.Featured() {
		r.order = append([]string{id}, r.order...)
	} else {
		r.order = append(r.order, id)
	}
}

// Boot runs Detect then conditionally Open for every registered provider in
// order. Boot is NEVER fatal — a per-provider detect/open error sets that
// provider's state to Errored and continues to the next one.
func (r *Registry) Boot(ctx context.Context, profile Profile) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, id := range r.order {
		e := r.entries[id]
		cfg := profile.ProviderCfg(id)

		det := e.p.Detect(ctx, cfg)
		if !det.Available {
			// Tier-1 providers stay visible in Settings for manual path entry even
			// when not auto-detected. Tier-2 providers are hidden entirely.
			e.state = Detected // "detected but unavailable" — keeps Tier-1 visible
			if r.log != nil {
				r.log.Info("provider not detected", "id", id, "reason", det.Reason)
			}
			continue
		}

		if !cfg.Enabled {
			e.state = Disabled
			continue
		}

		inst, err := e.p.Open(ctx, cfg)
		if err != nil {
			e.state = Errored
			e.err = err
			if r.log != nil {
				r.log.Warn("provider open failed", "id", id, "err", err)
			}
			continue
		}
		e.inst, e.state = inst, Enabled
	}
}

// SwitchProfile hot-swaps all providers to the next profile.
// It opens the next profile's instances BEFORE closing the old ones (validate-
// first, no downtime window). On ANY open failure it aborts, keeps the old
// profile fully active, and returns the error — no half-switched state.
func (r *Registry) SwitchProfile(ctx context.Context, next Profile) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Phase 1: open every currently-enabled provider for the next profile.
	type opened struct {
		id   string
		inst Instance
	}
	var newInsts []opened
	for _, id := range r.order {
		e := r.entries[id]
		if e.state != Enabled {
			continue
		}
		cfg := next.ProviderCfg(id)
		if !cfg.Enabled {
			continue
		}
		inst, err := e.p.Open(ctx, cfg)
		if err != nil {
			// Abort: close everything we already opened in this loop.
			for _, o := range newInsts {
				_ = o.inst.Close(ctx)
			}
			return fmt.Errorf("switch profile: provider %q open failed: %w", id, err)
		}
		newInsts = append(newInsts, opened{id, inst})
	}

	// Phase 2: all opens succeeded — swap in the new instances and close old ones.
	for _, o := range newInsts {
		e := r.entries[o.id]
		old := e.inst
		e.inst = o.inst
		if old != nil {
			// Best-effort close of old instance; errors are logged, not fatal.
			if err := old.Close(ctx); err != nil && r.log != nil {
				r.log.Warn("close old provider instance", "id", o.id, "err", err)
			}
		}
	}
	return nil
}

// NavGroups returns the sidebar nav groups for all enabled providers in
// registry order.
func (r *Registry) NavGroups() []NavGroup {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var groups []NavGroup
	for _, id := range r.order {
		e := r.entries[id]
		if e.state != Enabled {
			continue
		}
		groups = append(groups, e.p.Nav(e.inst))
	}
	return groups
}

// AggregateHealth returns per-provider health for all registered providers.
// The top-level ok value is the AND of all enabled providers only; inactive /
// not-detected providers do not fail the aggregate.
func (r *Registry) AggregateHealth(ctx context.Context) (ok bool, healths []ProviderHealth) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ok = true // vacuously true when no provider is enabled
	for _, id := range r.order {
		e := r.entries[id]
		switch e.state {
		case Enabled:
			h := e.inst.Health(ctx)
			h.Active = true
			healths = append(healths, h)
			if !h.OK {
				ok = false
			}
		default:
			// Include inactive/errored/not-detected providers with active=false.
			healths = append(healths, ProviderHealth{
				ID:     id,
				OK:     false,
				Active: false,
				Error:  e.state.String(),
			})
		}
	}
	return ok, healths
}

// ActiveCount returns the number of providers in Enabled state.
func (r *Registry) ActiveCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	n := 0
	for _, e := range r.entries {
		if e.state == Enabled {
			n++
		}
	}
	return n
}

// Instance returns the live Instance for the named provider, or nil if the
// provider is not in Enabled state (not found, not open, errored, disabled).
// The returned Instance belongs to the registry — callers must not close it.
func (r *Registry) Instance(providerID string) Instance {
	r.mu.RLock()
	defer r.mu.RUnlock()

	e, ok := r.entries[providerID]
	if !ok || e.state != Enabled || e.inst == nil {
		return nil
	}
	return e.inst
}

// Shutdown closes all active (Enabled) provider instances. It is called on
// server shutdown or test cleanup to release DB handles, file locks, etc.
// Errors are logged but do not prevent other providers from being closed.
func (r *Registry) Shutdown(ctx context.Context) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, id := range r.order {
		e := r.entries[id]
		if e.state == Enabled && e.inst != nil {
			if err := e.inst.Close(ctx); err != nil && r.log != nil {
				r.log.Warn("provider close on shutdown", "id", id, "err", err)
			}
			e.inst = nil
			e.state = Registered
		}
	}
}

// Entries returns a snapshot of all provider IDs and their states, in order.
// Primarily for diagnostics and tests.
func (r *Registry) Entries() []RegistryEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]RegistryEntry, 0, len(r.order))
	for _, id := range r.order {
		e := r.entries[id]
		out = append(out, RegistryEntry{
			ID:    id,
			State: e.state,
			Err:   e.err,
		})
	}
	return out
}

// RegistryEntry is a snapshot of one provider's identity and state.
type RegistryEntry struct {
	ID    string
	State State
	Err   error
}

// ProviderMeta is a snapshot of a provider's static identity and tier
// alongside its current runtime state. Used by the Settings Modules page.
type ProviderMeta struct {
	ID          string
	DisplayName string
	Tier        Tier
	Featured    bool
	State       State
	Err         error
}

// ProviderMeta returns a snapshot of the named provider's static identity and
// current runtime state. Returns (meta, true) when the provider is registered,
// or (zero, false) when the ID is unknown.
func (r *Registry) ProviderMeta(id string) (ProviderMeta, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	e, ok := r.entries[id]
	if !ok {
		return ProviderMeta{}, false
	}
	return ProviderMeta{
		ID:          id,
		DisplayName: e.p.DisplayName(),
		Tier:        e.p.ProviderTier(),
		Featured:    e.p.Featured(),
		State:       e.state,
		Err:         e.err,
	}, true
}

// AllProviderMetas returns ProviderMeta for every registered provider in order.
// Used by the Settings Modules page to list all providers regardless of state.
func (r *Registry) AllProviderMetas() []ProviderMeta {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]ProviderMeta, 0, len(r.order))
	for _, id := range r.order {
		e := r.entries[id]
		out = append(out, ProviderMeta{
			ID:          id,
			DisplayName: e.p.DisplayName(),
			Tier:        e.p.ProviderTier(),
			Featured:    e.p.Featured(),
			State:       e.state,
			Err:         e.err,
		})
	}
	return out
}

// Validate runs the named provider's Validate method with the given config.
// Returns an error if the provider is not registered or if Validate fails.
func (r *Registry) Validate(ctx context.Context, id string, cfg ProviderConfig) error {
	r.mu.RLock()
	e, ok := r.entries[id]
	r.mu.RUnlock()

	if !ok {
		return fmt.Errorf("provider %q is not registered", id)
	}
	return e.p.Validate(ctx, cfg)
}

// Enable opens a provider with the given config and transitions it to Enabled.
// If the provider is already Enabled it is re-opened with the new config (useful
// when the user changes the path for a Tier-1 provider). Returns an error if the
// provider is not registered or if Open fails.
func (r *Registry) Enable(ctx context.Context, id string, cfg ProviderConfig) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	e, ok := r.entries[id]
	if !ok {
		return fmt.Errorf("provider %q is not registered", id)
	}

	inst, err := e.p.Open(ctx, cfg)
	if err != nil {
		e.state = Errored
		e.err = err
		if r.log != nil {
			r.log.Warn("provider enable (open) failed", "id", id, "err", err)
		}
		return err
	}

	// Close the old instance if one exists (idempotent re-enable with new path).
	if e.inst != nil {
		if cerr := e.inst.Close(ctx); cerr != nil && r.log != nil {
			r.log.Warn("close old instance on re-enable", "id", id, "err", cerr)
		}
	}

	e.inst = inst
	e.state = Enabled
	e.err = nil
	return nil
}

// Disable closes the named provider's active instance and transitions it to
// Disabled state. Returns an error if the provider is not registered. If the
// provider is not currently Enabled, Disable is a no-op (returns nil).
func (r *Registry) Disable(ctx context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	e, ok := r.entries[id]
	if !ok {
		return fmt.Errorf("provider %q is not registered", id)
	}
	if e.state != Enabled {
		e.state = Disabled
		return nil
	}

	if e.inst != nil {
		if err := e.inst.Close(ctx); err != nil && r.log != nil {
			r.log.Warn("close instance on disable", "id", id, "err", err)
		}
		e.inst = nil
	}
	e.state = Disabled
	return nil
}

// Profile provides per-provider config for a given profile. The registry reads
// from this interface so it is not coupled to the config package directly.
type Profile interface {
	// ProviderCfg returns the config for the named provider. Unknown IDs return
	// a zero ProviderConfig (Enabled=false, empty paths).
	ProviderCfg(providerID string) ProviderConfig
}
