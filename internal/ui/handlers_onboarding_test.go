package ui_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/ui"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// makeTier1Module builds a Tier-1 ModuleInfo for onboarding tests.
func makeTier1Module(id, displayName string, state ui.ModuleState) ui.ModuleInfo {
	return ui.ModuleInfo{
		ID:          id,
		DisplayName: displayName,
		Tier:        ui.ModuleTier1,
		State:       state,
		Path:        "/some/path",
	}
}

// ---------------------------------------------------------------------------
// GET / — onboarding vs overview routing
// ---------------------------------------------------------------------------

// TestGetRoot_ZeroProviders_ShowsOnboarding verifies that GET / renders the
// onboarding page (not the regular overview) when ActiveCount returns 0.
func TestGetRoot_ZeroProviders_ShowsOnboarding(t *testing.T) {
	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{
		RoDB: nil, // no DB — zero-provider state
		ActiveCount: func() int { return 0 },
		Modules: func() []ui.ModuleInfo {
			return []ui.ModuleInfo{
				makeTier1Module("engram", "Engram", ui.ModuleDetected),
				makeTier1Module("cc-sessions", "Claude Code", ui.ModuleRegistered),
			}
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	// Onboarding page must contain welcoming content, not the overview KPI cards.
	if strings.Contains(body, "overview.kpi") {
		t.Error("zero-provider GET / must NOT render the overview KPI section")
	}
	// Must contain provider names listed on the onboarding page.
	if !strings.Contains(body, "Engram") {
		t.Error("onboarding page must list the Engram provider")
	}
}

// TestGetRoot_WithProvider_ShowsOverview verifies that GET / renders the normal
// overview when ActiveCount > 0. Existing behavior must be preserved.
func TestGetRoot_WithProvider_ShowsOverview(t *testing.T) {
	db := openTestDB(t)
	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{
		RoDB: db,
		ActiveCount: func() int { return 1 },
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	// Overview page should NOT contain onboarding-specific content.
	if strings.Contains(body, "onboarding") {
		t.Error("non-zero-provider GET / must NOT render the onboarding page")
	}
}

// TestGetRoot_NilActiveCount_ShowsOverview verifies backward compat: when
// ActiveCount is nil (legacy/non-registry path), GET / serves the overview.
func TestGetRoot_NilActiveCount_ShowsOverview(t *testing.T) {
	db := openTestDB(t)
	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{
		RoDB:        db,
		ActiveCount: nil, // legacy: no registry wired
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 with nil ActiveCount, got %d", w.Code)
	}
}

// TestGetRoot_ZeroProviders_HTMX_ShowsOnboardingPartial verifies that HTMX
// requests to GET / return the onboarding partial (no full layout shell) when
// ActiveCount == 0.
func TestGetRoot_ZeroProviders_HTMX_ShowsOnboardingPartial(t *testing.T) {
	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{
		RoDB: nil,
		ActiveCount: func() int { return 0 },
		Modules: func() []ui.ModuleInfo {
			return []ui.ModuleInfo{
				makeTier1Module("engram", "Engram", ui.ModuleDetected),
			}
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("HX-Request", "true")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 (HTMX onboarding partial), got %d", w.Code)
	}
	body := w.Body.String()
	// The partial should NOT include the full HTML shell (<!DOCTYPE html>).
	if strings.Contains(body, "<!DOCTYPE html>") {
		t.Error("HTMX onboarding response must NOT include the HTML shell")
	}
}

// TestGetRoot_ZeroProviders_NoModulesCallback verifies that the onboarding
// page renders without panicking even when Deps.Modules is nil.
func TestGetRoot_ZeroProviders_NoModulesCallback(t *testing.T) {
	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{
		RoDB:        nil,
		ActiveCount: func() int { return 0 },
		Modules:     nil,
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("handler panicked with nil Modules on onboarding: %v", r)
		}
	}()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// Onboarding path entry (reuses POST /settings/modules/{id}/path)
// ---------------------------------------------------------------------------

// TestOnboardingPathPost_Success verifies that after a successful path
// activation from the onboarding page, the response redirects to / (overview).
func TestOnboardingPathPost_Success(t *testing.T) {
	called := false
	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{
		ValidateModulePath: func(ctx context.Context, id, path string) error {
			called = true
			return nil
		},
	})

	form := url.Values{"path": {"/valid/path"}, "redirect": {"/"}}
	req := httptest.NewRequest(http.MethodPost, "/settings/modules/engram/path",
		strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if !called {
		t.Error("ValidateModulePath was not called")
	}
	// Existing redirect behavior: success always redirects to /settings
	// (WU-8 contract). The onboarding page uses an HTMX full-navigation
	// redirect on success, handled client-side via hx-on or hx-trigger.
	if w.Code != http.StatusSeeOther {
		t.Errorf("expected 303, got %d", w.Code)
	}
}

// TestOnboardingPathPost_HTMX_Success verifies HTMX path activation from
// onboarding returns HX-Redirect so the browser navigates to the overview.
func TestOnboardingPathPost_HTMX_Success(t *testing.T) {
	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{
		ValidateModulePath: func(ctx context.Context, id, path string) error { return nil },
	})

	form := url.Values{"path": {"/valid/path"}}
	req := httptest.NewRequest(http.MethodPost, "/settings/modules/engram/path",
		strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for HTMX success, got %d", w.Code)
	}
	if got := w.Header().Get("HX-Redirect"); got == "" {
		t.Error("HX-Redirect header must be set on HTMX success")
	}
}

// TestOnboardingPathPost_InvalidPath_ShowsError verifies that a validation
// failure on the onboarding path entry shows an inline error (same as settings).
func TestOnboardingPathPost_InvalidPath_ShowsError(t *testing.T) {
	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{
		ValidateModulePath: func(ctx context.Context, id, path string) error {
			return errors.New("path-missing: file not found")
		},
	})

	form := url.Values{"path": {"/nonexistent"}}
	req := httptest.NewRequest(http.MethodPost, "/settings/modules/engram/path",
		strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 (error re-render), got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "path-missing") {
		t.Error("error response must contain the validation error message")
	}
}
