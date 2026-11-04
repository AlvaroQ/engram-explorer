package providers

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
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
	h     *handle // the open instance; nil unless state == Enabled
	state State
	err   error
}

// Registry holds an ordered list of providers and their per-profile runtime
// state. Engram (featured=true) is always iterated first. All mutations are
// guarded by a read-write mutex so the registry is safe for concurrent access.
//
// Requests pin the instances they use with Acquire (see Lease). An instance
// superseded by Enable, Disable, SwitchProfile or Shutdown is closed outside
// the lock once its leases are released, bounded by the drain timeout.
type Registry struct {
	mu      sync.RWMutex
	order   []string // stable render order; featured providers sorted first
	entries map[string]*activeEntry
	log     *slog.Logger
	drain   time.Duration  // drain timeout; DefaultDrainTimeout when not positive
	pending sync.WaitGroup // background closes waiting for leases to drain
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
// sorted before non-featured ones regardless of registration order). A
// provider implementing LoggerSetter receives the registry logger.
// Register panics if a provider with the same ID is registered twice.
func (r *Registry) Register(p Provider) {
	r.mu.Lock()
	defer r.mu.Unlock()

	id := p.ID()
	if _, exists := r.entries[id]; exists {
		panic(fmt.Sprintf("providers: duplicate provider ID %q", id))
	}
	r.entries[id] = &activeEntry{p: p, state: Registered}
	if ls, ok := p.(LoggerSetter); ok && r.log != nil {
		ls.SetLogger(r.log)
	}

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
		e.h, e.state = newHandle(inst), Enabled
	}
}

// SwitchProfile moves every registered provider to the state the next profile
// asks for, using the same lifecycle rules as Boot:
//
//   - source not detected        → Detected (degraded, not fatal), no instance
//   - detected but not enabled   → Disabled, no instance
//   - detected and enabled       → Enabled with a freshly opened instance
//
// It is atomic: every new instance is opened BEFORE any old one is closed
// (no downtime window). If any open fails, the instances opened during the
// attempt are closed, every entry keeps its previous instance, state and
// error, and the error is returned — no half-switched state. Only after all
// opens succeed are the new states committed and the old instances retired;
// each is closed once the requests holding it finish (see Lease).
func (r *Registry) SwitchProfile(ctx context.Context, next Profile) error {
	r.mu.Lock()
	var retired []retiredHandle
	defer func() {
		r.mu.Unlock()
		r.closeRetired(ctx, retired, "close old provider instance")
	}()

	type target struct {
		e     *activeEntry
		inst  Instance
		state State
	}
	targets := make([]target, 0, len(r.order))

	// Phase 1: compute every target and open the new instances.
	for _, id := range r.order {
		e := r.entries[id]
		cfg := next.ProviderCfg(id)
		t := target{e: e}
		switch {
		case !e.p.Detect(ctx, cfg).Available:
			t.state = Detected
		case !cfg.Enabled:
			t.state = Disabled
		default:
			inst, err := e.p.Open(ctx, cfg)
			if err != nil {
				for _, o := range targets {
					if o.inst != nil {
						_ = o.inst.Close(ctx)
					}
				}
				return fmt.Errorf("switch profile: provider %q open failed: %w", id, err)
			}
			t.inst, t.state = inst, Enabled
		}
		targets = append(targets, t)
	}

	// Phase 2: every open succeeded — commit and retire the superseded instances.
	for _, t := range targets {
		retired = t.e.retire(retired)
		if t.inst != nil {
			t.e.h = newHandle(t.inst)
		}
		t.e.state, t.e.err = t.state, nil
	}
	return nil
}

// NavGroups returns the sidebar nav groups for all enabled providers in
// registry order. Groups sharing an ID (e.g. AgentSessionsNavID) are merged
// into the first one: its links are followed by the links of the later ones
// whose Key it does not already have.
func (r *Registry) NavGroups() []NavGroup {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var groups []NavGroup
	at := map[string]int{}
	for _, id := range r.order {
		e := r.entries[id]
		if e.state != Enabled {
			continue
		}
		g := e.p.Nav(e.h.inst)
		i, seen := at[g.ID]
		if !seen {
			at[g.ID] = len(groups)
			g.Links = append([]NavLink(nil), g.Links...)
			groups = append(groups, g)
			continue
		}
		for _, l := range g.Links {
			if !hasLinkKey(groups[i].Links, l.Key) {
				groups[i].Links = append(groups[i].Links, l)
			}
		}
	}
	return groups
}

func hasLinkKey(links []NavLink, key string) bool {
	for _, l := range links {
		if l.Key == key {
			return true
		}
	}
	return false
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
			h := e.h.inst.Health(ctx)
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
	if !ok || e.state != Enabled || e.h == nil {
		return nil
	}
	return e.h.inst
}

// Find returns the first Enabled instance, in registry order, for which match
// returns true, or nil when there is none. The host uses it to resolve a
// capability (an optional interface) per request; match runs under the
// registry read lock and must only inspect the instance (a type assertion).
// The returned Instance belongs to the registry — callers must not close it.
func (r *Registry) Find(match func(Instance) bool) Instance {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, id := range r.order {
		e := r.entries[id]
		if e.state == Enabled && e.h != nil && match(e.h.inst) {
			return e.h.inst
		}
	}
	return nil
}

// FindAll returns every Enabled instance, in registry order, for which match
// returns true (nil when none). It is Find for capabilities that several
// providers serve at once (e.g. AgentSessionSource); match runs under the
// registry read lock. The instances belong to the registry — callers must
// not close them.
func (r *Registry) FindAll(match func(Instance) bool) []Instance {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var out []Instance
	for _, id := range r.order {
		e := r.entries[id]
		if e.state == Enabled && e.h != nil && match(e.h.inst) {
			out = append(out, e.h.inst)
		}
	}
	return out
}

// Providers returns every registered provider in registry order, whatever its
// state. The host calls Provider.Routes once for each at boot.
func (r *Registry) Providers() []Provider {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]Provider, 0, len(r.order))
	for _, id := range r.order {
		out = append(out, r.entries[id].p)
	}
	return out
}

// Shutdown closes every open provider instance. It is called on server
// shutdown or test cleanup to release DB handles, file locks, etc. An instance
// still leased by a request is closed when that request finishes; Shutdown
// waits for it, and for closes queued earlier by toggles or switches, at most
// the drain timeout, then closes what is left. Errors are logged but do not
// prevent other providers from being closed.
func (r *Registry) Shutdown(ctx context.Context) {
	r.mu.Lock()
	var retired []retiredHandle
	for _, id := range r.order {
		e := r.entries[id]
		if e.h != nil {
			retired = e.retire(retired)
			e.state = Registered
		}
	}
	r.mu.Unlock()

	r.closeRetired(ctx, retired, "provider close on shutdown")
	// Each background close is bounded by the drain timeout; the extra second
	// only covers the Close calls themselves.
	r.waitPending(r.drainTimeout() + time.Second)
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
//
// A replaced instance is retired and closed once the requests holding it
// finish (see Lease); new requests get the re-opened instance. When Open fails
// the entry becomes Errored and stops serving, so its previous instance is
// retired the same way instead of staying open until Shutdown.
func (r *Registry) Enable(ctx context.Context, id string, cfg ProviderConfig) error {
	r.mu.Lock()
	var retired []retiredHandle
	defer func() {
		r.mu.Unlock()
		r.closeRetired(ctx, retired, "close old instance on re-enable")
	}()

	e, ok := r.entries[id]
	if !ok {
		return fmt.Errorf("provider %q is not registered", id)
	}

	inst, err := e.p.Open(ctx, cfg)
	if err != nil {
		retired = e.retire(retired)
		e.state = Errored
		e.err = err
		if r.log != nil {
			r.log.Warn("provider enable (open) failed", "id", id, "err", err)
		}
		return err
	}

	// Retire the old instance if one exists (idempotent re-enable with new path).
	retired = e.retire(retired)

	e.h = newHandle(inst)
	e.state = Enabled
	e.err = nil
	return nil
}

// Disable transitions the named provider to Disabled state and retires its
// active instance: new requests no longer get it, and it is closed once the
// requests holding it finish (see Lease). Returns an error if the provider is
// not registered. Whatever the current state, the entry ends Disabled and any
// instance it still holds is retired.
func (r *Registry) Disable(ctx context.Context, id string) error {
	r.mu.Lock()
	var retired []retiredHandle
	defer func() {
		r.mu.Unlock()
		r.closeRetired(ctx, retired, "close instance on disable")
	}()

	e, ok := r.entries[id]
	if !ok {
		return fmt.Errorf("provider %q is not registered", id)
	}
	retired = e.retire(retired)
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
