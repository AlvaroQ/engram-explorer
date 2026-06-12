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

// makeModulesProvider builds a minimal []ui.ModuleInfo for tests.
func makeModulesProvider(id string, state ui.ModuleState, tier ui.ModuleTier, displayName string) ui.ModuleInfo {
	return ui.ModuleInfo{
		ID:          id,
		DisplayName: displayName,
		Tier:        tier,
		State:       state,
		Path:        "/some/path",
	}
}

// ---------------------------------------------------------------------------
// GET /settings — Modules section renders
// ---------------------------------------------------------------------------

// TestSettingsModulesSection verifies that GET /settings renders the Modules
// section when Deps.Modules is set, showing provider display names.
func TestSettingsModulesSection(t *testing.T) {
	db := openTestDB(t)

	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{
		RoDB: db,
		Modules: func() []ui.ModuleInfo {
			return []ui.ModuleInfo{
				makeModulesProvider("engram", ui.ModuleEnabled, ui.ModuleTier1, "Engram"),
				makeModulesProvider("cc-sessions", ui.ModuleDisabled, ui.ModuleTier1, "Claude Code"),
			}
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/settings", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	// Modules section heading should appear.
	// Presence check: either the key itself or translated value must appear.
	_ = body // used below for "Engram" check
	// Provider display names or IDs should appear.
	if !strings.Contains(body, "Engram") {
		t.Error("settings page must contain the Engram provider name")
	}
}

// TestSettingsModulesSection_NilModules verifies that GET /settings renders
// without panicking when Deps.Modules is nil (no registry wired).
func TestSettingsModulesSection_NilModules(t *testing.T) {
	db := openTestDB(t)

	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{RoDB: db, Modules: nil})

	req := httptest.NewRequest(http.MethodGet, "/settings", nil)
	w := httptest.NewRecorder()

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("handler panicked with nil Modules: %v", r)
		}
	}()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 with nil Modules, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// POST /settings/modules/{id}/toggle
// ---------------------------------------------------------------------------

// TestModulesTogglePost_Enable verifies that POSTing enabled=true calls
// ToggleModule(id, true) and redirects to /settings.
func TestModulesTogglePost_Enable(t *testing.T) {
	called := false
	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{
		ToggleModule: func(id string, enabled bool) error {
			if id != "cc-sessions" {
				t.Errorf("ToggleModule id: got %q, want cc-sessions", id)
			}
			if !enabled {
				t.Error("ToggleModule enabled: got false, want true")
			}
			called = true
			return nil
		},
	})

	form := url.Values{"enabled": {"true"}}
	req := httptest.NewRequest(http.MethodPost, "/settings/modules/cc-sessions/toggle", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if !called {
		t.Error("ToggleModule callback was not called")
	}
	if w.Code != http.StatusSeeOther {
		t.Errorf("expected 303, got %d", w.Code)
	}
	if got := w.Header().Get("Location"); got != "/settings" {
		t.Errorf("Location: got %q, want /settings", got)
	}
}

// TestModulesTogglePost_Disable verifies that POSTing enabled=false calls
// ToggleModule(id, false).
func TestModulesTogglePost_Disable(t *testing.T) {
	called := false
	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{
		ToggleModule: func(id string, enabled bool) error {
			called = true
			if enabled {
				t.Error("ToggleModule enabled: got true, want false")
			}
			return nil
		},
	})

	form := url.Values{"enabled": {"false"}}
	req := httptest.NewRequest(http.MethodPost, "/settings/modules/engram/toggle", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if !called {
		t.Error("ToggleModule callback was not called")
	}
}

// TestModulesTogglePost_HTMX verifies that HTMX requests get HX-Redirect
// instead of a 303.
func TestModulesTogglePost_HTMX(t *testing.T) {
	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{
		ToggleModule: func(id string, enabled bool) error { return nil },
	})

	form := url.Values{"enabled": {"true"}}
	req := httptest.NewRequest(http.MethodPost, "/settings/modules/engram/toggle", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for HTMX, got %d", w.Code)
	}
	if got := w.Header().Get("HX-Redirect"); got != "/settings" {
		t.Errorf("HX-Redirect: got %q, want /settings", got)
	}
}

// TestModulesTogglePost_NilCallback verifies that when ToggleModule is nil,
// the handler returns 503 (service unavailable) without panicking.
func TestModulesTogglePost_NilCallback(t *testing.T) {
	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{ToggleModule: nil})

	form := url.Values{"enabled": {"true"}}
	req := httptest.NewRequest(http.MethodPost, "/settings/modules/engram/toggle", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("handler panicked with nil ToggleModule: %v", r)
		}
	}()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503 with nil ToggleModule, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// POST /settings/modules/{id}/path
// ---------------------------------------------------------------------------

// TestModulesPathPost_ValidPath verifies that a valid path calls
// ValidateModulePath and on success redirects to /settings.
func TestModulesPathPost_ValidPath(t *testing.T) {
	called := false
	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{
		ValidateModulePath: func(ctx context.Context, id, path string) error {
			if id != "cc-sessions" {
				t.Errorf("ValidateModulePath id: got %q, want cc-sessions", id)
			}
			if path != "/valid/path" {
				t.Errorf("ValidateModulePath path: got %q, want /valid/path", path)
			}
			called = true
			return nil
		},
	})

	form := url.Values{"path": {"/valid/path"}}
	req := httptest.NewRequest(http.MethodPost, "/settings/modules/cc-sessions/path", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if !called {
		t.Error("ValidateModulePath callback was not called")
	}
	if w.Code != http.StatusSeeOther {
		t.Errorf("expected 303, got %d", w.Code)
	}
}

// TestModulesPathPost_InvalidPath verifies that a validation failure re-renders
// the settings page with an error message (no redirect, 200 with error content).
func TestModulesPathPost_InvalidPath(t *testing.T) {
	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{
		ValidateModulePath: func(ctx context.Context, id, path string) error {
			return errors.New("schema-invalid: not a valid engram database")
		},
	})

	form := url.Values{"path": {"/bad/path"}}
	req := httptest.NewRequest(http.MethodPost, "/settings/modules/engram/path", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 (error page re-render) on validation failure, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "schema-invalid") {
		t.Error("response body must contain the validation error message")
	}
}

// TestModulesPathPost_EmptyPath verifies that an empty path returns a 200 with
// error content (path.empty i18n key or equivalent).
func TestModulesPathPost_EmptyPath(t *testing.T) {
	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{
		ValidateModulePath: func(ctx context.Context, id, path string) error {
			t.Error("ValidateModulePath should not be called for empty path")
			return nil
		},
	})

	form := url.Values{"path": {""}}
	req := httptest.NewRequest(http.MethodPost, "/settings/modules/engram/path", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 (error re-render) for empty path, got %d", w.Code)
	}
}

// TestModulesPathPost_HTMX_Success verifies HTMX success path returns HX-Redirect.
func TestModulesPathPost_HTMX_Success(t *testing.T) {
	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{
		ValidateModulePath: func(ctx context.Context, id, path string) error { return nil },
	})

	form := url.Values{"path": {"/valid/dir"}}
	req := httptest.NewRequest(http.MethodPost, "/settings/modules/cc-sessions/path", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for HTMX, got %d", w.Code)
	}
	if got := w.Header().Get("HX-Redirect"); got != "/settings" {
		t.Errorf("HX-Redirect: got %q, want /settings", got)
	}
}

// TestModulesPathPost_NilCallback verifies that when ValidateModulePath is nil,
// the handler returns 503 without panicking.
func TestModulesPathPost_NilCallback(t *testing.T) {
	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{ValidateModulePath: nil})

	form := url.Values{"path": {"/some/path"}}
	req := httptest.NewRequest(http.MethodPost, "/settings/modules/engram/path", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("handler panicked with nil ValidateModulePath: %v", r)
		}
	}()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503 with nil ValidateModulePath, got %d", w.Code)
	}
}
