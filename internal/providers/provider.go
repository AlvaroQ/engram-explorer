// Package providers defines the port (interface) every data-source adapter must
// implement, plus shared view-model types used by the UI and health layers.
package providers

import (
	"context"
	"net/http"
)

// Tier classifies how a provider is detected and presented in the UI.
type Tier int

const (
	// Tier1 providers are first-class: the app tries to load them at startup,
	// and if absent the UI offers a manual path-entry field + validation before
	// activation. Engram and Claude Code sessions are Tier-1.
	Tier1 Tier = iota

	// Tier2 providers are silent auto-detect only: hidden entirely when not
	// loadable, no manual path affordance. No Tier-2 provider ships in this
	// change — the seam is defined here to satisfy the framework tests and to
	// allow future extension without modifying this port.
	Tier2
)

// Provider is the core port every data source adapter must implement.
// It is a singleton describing a source TYPE; Instance (below) represents
// the per-profile live activation.
type Provider interface {
	// ID returns the stable machine identifier, e.g. "engram" or "cc-sessions".
	ID() string

	// DisplayName returns the human-readable label. The UI resolves i18n from
	// this value; the provider itself returns the English base string.
	DisplayName() string

	// ProviderTier classifies detection/onboarding behaviour.
	ProviderTier() Tier

	// Featured returns true when this provider should be rendered first / visually
	// featured. Only the Engram provider returns true.
	Featured() bool

	// Detect reports whether the source is available for the given profile config.
	// It is a pure check (path exists + shape valid); it MUST NOT open long-lived
	// handles and MUST NOT be fatal.
	Detect(ctx context.Context, cfg ProviderConfig) Detection

	// Open activates the provider for a profile: acquires handles (DB pools,
	// readers). Returns an Instance the registry holds until Close. Idempotent per
	// (provider, profile).
	Open(ctx context.Context, cfg ProviderConfig) (Instance, error)

	// Routes registers the provider's own HTTP routes on r. The host calls it
	// exactly once at boot for every registered provider, whatever its state;
	// r serves each route only while the provider is Enabled and hands the
	// live instance to handlers through r.Live(req) per request. Existing paths
	// are kept verbatim (no forced /api/{id}/ prefix in v1).
	Routes(r Router)

	// Nav returns the sidebar group + links for this provider (data-driven menu).
	Nav(inst Instance) NavGroup

	// Validate checks a candidate config before activation (Tier-1 manual path
	// entry). Engram: file exists AND schema valid. CC: dir exists AND contains
	// project subdirectories. Returns nil on success, a descriptive error on failure.
	Validate(ctx context.Context, cfg ProviderConfig) error
}

// Router is the route registrar the host hands to Provider.Routes. It is
// defined here, not in the HTTP layer, so provider packages can register
// routes without importing it. The host implementation gates every route by
// the provider's state: while the provider is not Enabled the route answers
// 503 with code MODULE_DISABLED (JSON under /api/, an HTML message
// elsewhere), so enabling or disabling a provider never remounts routes.
type Router interface {
	Handle(pattern string, h http.Handler)
	HandleFunc(pattern string, h func(http.ResponseWriter, *http.Request))

	// Live returns the provider instance serving req, or nil when the
	// provider was not Enabled when req started. The instance stays open
	// until req's handler returns, even if a toggle or a profile switch
	// supersedes it meanwhile; never keep it beyond the request.
	Live(req *http.Request) Instance
}

// Instance is the per-profile live activation of a provider — handles, pools,
// and readers acquired by Provider.Open.
type Instance interface {
	// Health returns per-provider health for /api/health aggregation.
	Health(ctx context.Context) ProviderHealth

	// Close releases handles; called on profile switch or server shutdown.
	Close(ctx context.Context) error
}

// Optional capabilities discovered by type-assertion on the live Instance —
// NOT in the core port. Providers that do not implement a capability simply
// omit it. The host resolves them per request on the first Enabled instance
// (registry order) that implements them (Registry.Find).

// Searcher is an optional capability for providers that support full-text
// search. It backs GET /api/observations/search; the result is encoded as the
// "items" array of the response.
type Searcher interface {
	Search(ctx context.Context, q string, limit int) (any, error)
}

// StatsSource is an optional capability for providers that expose aggregate
// stats. It backs GET /api/overview; the result is encoded as the response body.
type StatsSource interface {
	Stats(ctx context.Context) (any, error)
}

// Writable marks an instance that can accept write operations right now.
// The host serves write routes and hands out read-write handles only when the
// live instance implements Writable and reports true; absence or false means
// read-only (write routes answer RW_UNAVAILABLE).
type Writable interface {
	Writable() bool
}

// ProviderConfig holds per-profile configuration for a single provider.
// It is a generic bag-of-settings; each provider interprets the fields it needs.
type ProviderConfig struct {
	// Enabled is the user-set activation flag for this provider in this profile.
	Enabled bool

	// Path is the filesystem path to the data source (DB file for Engram,
	// directory for Claude Code sessions).
	Path string

	// DaemonURL is the Engram daemon HTTP base URL (used only by the Engram
	// provider; ignored by others).
	DaemonURL string
}

// Detection is the result of Provider.Detect.
type Detection struct {
	Available bool
	// Reason is a machine-readable code explaining the detection result:
	// "ok" | "path-missing" | "schema-invalid" | "empty" | "dir-missing" | "no-projects"
	Reason string
	Path   string
}

// ProviderHealth is the per-provider entry in the /api/health response.
type ProviderHealth struct {
	ID     string `json:"id"`
	OK     bool   `json:"ok"`
	Path   string `json:"path,omitempty"`
	Error  string `json:"error,omitempty"`
	Active bool   `json:"active"`
}

// NavGroup is the data-driven sidebar menu group contributed by a provider.
type NavGroup struct {
	ID       string
	LabelKey string // i18n key, resolved by the templ template via T(lang, key)
	Featured bool
	Links    []NavLink
}

// NavLink is a single sidebar entry within a NavGroup.
type NavLink struct {
	Href     string
	Key      string // active-nav match key
	LabelKey string
	External bool   // true for cross-module links like orphans/doctor
	Icon     string // icon name resolved by navIcon() in layout.templ; empty = no icon
}
