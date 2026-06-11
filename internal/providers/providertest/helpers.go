// Package providertest provides fake providers and registry builders for
// framework and handler tests. It avoids importing real data sources.
package providertest

import (
	"context"
	"log/slog"
	"net/http"
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/providers"
)

// FakeProviderOptions controls the behaviour of a FakeProvider.
type FakeProviderOptions struct {
	// ID is the provider's stable machine identifier.
	ID string
	// DisplayName is the human label.
	DisplayName string
	// Tier is the detection tier (default Tier1).
	Tier providers.Tier
	// Featured marks this provider as featured (rendered first).
	Featured bool
	// DetectAvailable controls what Detect returns.
	DetectAvailable bool
	// DetectReason is the reason code returned by Detect (default "ok" / "path-missing").
	DetectReason string
	// OpenErr, if non-nil, is returned by Open.
	OpenErr error
	// ValidateErr, if non-nil, is returned by Validate.
	ValidateErr error
}

// FakeProvider is a test-double Provider.
type FakeProvider struct {
	opts FakeProviderOptions
}

// NewFakeProvider returns a FakeProvider configured by opts.
func NewFakeProvider(opts FakeProviderOptions) *FakeProvider {
	if opts.ID == "" {
		opts.ID = "fake"
	}
	if opts.DisplayName == "" {
		opts.DisplayName = "Fake Provider"
	}
	if opts.DetectReason == "" {
		if opts.DetectAvailable {
			opts.DetectReason = "ok"
		} else {
			opts.DetectReason = "path-missing"
		}
	}
	return &FakeProvider{opts: opts}
}

func (f *FakeProvider) ID() string                   { return f.opts.ID }
func (f *FakeProvider) DisplayName() string          { return f.opts.DisplayName }
func (f *FakeProvider) ProviderTier() providers.Tier { return f.opts.Tier }
func (f *FakeProvider) Featured() bool               { return f.opts.Featured }

func (f *FakeProvider) Detect(_ context.Context, cfg providers.ProviderConfig) providers.Detection {
	return providers.Detection{
		Available: f.opts.DetectAvailable,
		Reason:    f.opts.DetectReason,
		Path:      cfg.Path,
	}
}

func (f *FakeProvider) Open(_ context.Context, cfg providers.ProviderConfig) (providers.Instance, error) {
	if f.opts.OpenErr != nil {
		return nil, f.opts.OpenErr
	}
	return &FakeInstance{providerID: f.opts.ID, path: cfg.Path}, nil
}

func (f *FakeProvider) Routes(_ *http.ServeMux, _ providers.Instance, _ *slog.Logger) {}

func (f *FakeProvider) Nav(_ providers.Instance) providers.NavGroup {
	return providers.NavGroup{
		ID:       f.opts.ID,
		LabelKey: "nav." + f.opts.ID,
		Featured: f.opts.Featured,
	}
}

func (f *FakeProvider) Validate(_ context.Context, _ providers.ProviderConfig) error {
	return f.opts.ValidateErr
}

// SetOpenErr replaces the error returned by Open. Useful for mid-test error injection.
func (f *FakeProvider) SetOpenErr(err error) {
	f.opts.OpenErr = err
}

// FakeInstance is a test-double Instance.
type FakeInstance struct {
	providerID string
	path       string
	closed     bool
}

func (i *FakeInstance) Health(_ context.Context) providers.ProviderHealth {
	return providers.ProviderHealth{
		ID:     i.providerID,
		OK:     true,
		Path:   i.path,
		Active: true,
	}
}

func (i *FakeInstance) Close(_ context.Context) error {
	i.closed = true
	return nil
}

// Closed reports whether Close was called.
func (i *FakeInstance) Closed() bool { return i.closed }

// FakeProfile implements providers.Profile for tests.
type FakeProfile struct {
	cfgs map[string]providers.ProviderConfig
}

// NewFakeProfile returns a FakeProfile from a map of providerID → ProviderConfig.
func NewFakeProfile(cfgs map[string]providers.ProviderConfig) *FakeProfile {
	if cfgs == nil {
		cfgs = make(map[string]providers.ProviderConfig)
	}
	return &FakeProfile{cfgs: cfgs}
}

func (p *FakeProfile) ProviderCfg(id string) providers.ProviderConfig {
	return p.cfgs[id]
}

// NewRegistry creates a Registry populated with the given providers and
// calls Boot with the supplied profile. It is the preferred setup helper for
// tests that need a working registry without real data sources.
func NewRegistry(t *testing.T, profile providers.Profile, provs ...providers.Provider) *providers.Registry {
	t.Helper()
	log := slog.Default()
	reg := providers.NewRegistry(log)
	for _, p := range provs {
		reg.Register(p)
	}
	reg.Boot(context.Background(), profile)
	return reg
}
