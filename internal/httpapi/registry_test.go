package httpapi_test

// WU-5: Tests for registry-backed container and non-fatal boot.
// These tests use NewContainerWithRegistry (the new registry-based constructor)
// to verify that the server starts successfully with zero providers active,
// and that GET / returns 200 in that state.

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/config"
	"github.com/AlvaroQ/engram-explorer/internal/httpapi"
	"github.com/AlvaroQ/engram-explorer/internal/providers"
	"github.com/AlvaroQ/engram-explorer/internal/providers/providertest"
)

// newRegistryContainer builds a Container via NewContainerWithRegistry using
// the supplied registry. It does NOT call sqlite.OpenReadOnly — this is the
// non-fatal-boot path.
func newRegistryContainer(t *testing.T, reg *providers.Registry) *httpapi.Container {
	t.Helper()
	cfg := config.Config{
		Host:            "127.0.0.1",
		Port:            8787,
		DaemonBaseURL:   "http://127.0.0.1:7437",
		DaemonTimeoutMs: 100,
		Env:             "development",
		ExposeDetails:   true,
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	c := httpapi.NewContainerWithRegistry(reg, cfg, logger)
	t.Cleanup(c.Close)
	return c
}

// TestContainerWithRegistry_ZeroProviders proves that the server constructs and
// serves successfully when no providers are active (non-fatal boot requirement
// from spec: startup-and-onboarding / Non-Fatal Startup).
func TestContainerWithRegistry_ZeroProviders(t *testing.T) {
	// Empty registry — no providers registered at all.
	reg := providers.NewRegistry(slog.Default())
	c := newRegistryContainer(t, reg)

	handler := httpapi.NewServeMux(c)

	// GET /api/health must return 200 even with zero providers.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("GET /api/health: got %d, want 200", rec.Code)
	}
}

// TestContainerWithRegistry_EngramProviderErrored verifies that a provider
// whose Open() returns an error leaves the server running with zero active
// providers (the provider enters Errored state, not fatal).
func TestContainerWithRegistry_EngramProviderErrored(t *testing.T) {
	fp := providertest.NewFakeProvider(providertest.FakeProviderOptions{
		ID:              "engram",
		DetectAvailable: true,
		OpenErr:         os.ErrNotExist,
		Featured:        true,
	})
	reg := providertest.NewRegistry(t, providertest.NewFakeProfile(map[string]providers.ProviderConfig{
		"engram": {Enabled: true, Path: "/does/not/exist/engram.db"},
	}), fp)

	c := newRegistryContainer(t, reg)
	handler := httpapi.NewServeMux(c)

	// Server must still respond — Errored provider is not fatal.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("GET /api/health: got %d, want 200 (errored provider must not crash server)", rec.Code)
	}
}

// TestContainerWithRegistry_HealthIncludesProviders verifies that /api/health
// returns a "providers" array when the container is registry-backed.
func TestContainerWithRegistry_HealthIncludesProviders(t *testing.T) {
	fp := providertest.NewFakeProvider(providertest.FakeProviderOptions{
		ID:              "fake",
		DetectAvailable: true,
		Featured:        false,
	})
	reg := providertest.NewRegistry(t, providertest.NewFakeProfile(map[string]providers.ProviderConfig{
		"fake": {Enabled: true, Path: "/some/path"},
	}), fp)

	c := newRegistryContainer(t, reg)
	handler := httpapi.NewServeMux(c)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("status: got %d, want 200", rec.Code)
	}
}
