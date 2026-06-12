package ui_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/ui"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// makeProfileInfo builds a minimal ui.ProfileInfo for tests.
func makeProfileInfo(name string, active bool) ui.ProfileInfo {
	return ui.ProfileInfo{Name: name, Active: active}
}

// makeAccountDeps builds a ui.Deps with accounts callbacks wired.
func makeAccountDeps(profiles []ui.ProfileInfo, switchErr error) ui.Deps {
	return ui.Deps{
		Profiles:      func() []ui.ProfileInfo { return profiles },
		ActiveProfile: func() string { return "default" },
		SwitchProfile: func(name string) error { return switchErr },
		CreateProfile: func(name string) error { return nil },
		DeleteProfile: func(name string) error { return nil },
	}
}

// ---------------------------------------------------------------------------
// GET /settings/accounts — page renders
// ---------------------------------------------------------------------------

// TestSettingsAccountsPage_Renders verifies that GET /settings/accounts renders
// the accounts page listing profiles with the active one distinguished.
func TestSettingsAccountsPage_Renders(t *testing.T) {
	mux := http.NewServeMux()
	ui.Mount(mux, makeAccountDeps([]ui.ProfileInfo{
		makeProfileInfo("default", true),
		makeProfileInfo("work", false),
	}, nil))

	req := httptest.NewRequest(http.MethodGet, "/settings/accounts", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "default") {
		t.Error("accounts page must list the default profile")
	}
	if !strings.Contains(body, "work") {
		t.Error("accounts page must list the work profile")
	}
}

// TestSettingsAccountsPage_NilCallbacks verifies that GET /settings/accounts
// renders without panicking when account callbacks are nil.
func TestSettingsAccountsPage_NilCallbacks(t *testing.T) {
	db := openTestDB(t)
	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{RoDB: db})

	req := httptest.NewRequest(http.MethodGet, "/settings/accounts", nil)
	w := httptest.NewRecorder()

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("handler panicked with nil account callbacks: %v", r)
		}
	}()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 with nil callbacks, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// POST /settings/profile/switch
// ---------------------------------------------------------------------------

// TestProfileSwitch_Success verifies that POSTing a profile name calls
// SwitchProfile and redirects to /settings/accounts.
func TestProfileSwitch_Success(t *testing.T) {
	called := ""
	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{
		Profiles:      func() []ui.ProfileInfo { return nil },
		ActiveProfile: func() string { return "default" },
		SwitchProfile: func(name string) error {
			called = name
			return nil
		},
		CreateProfile: func(name string) error { return nil },
		DeleteProfile: func(name string) error { return nil },
	})

	form := url.Values{"name": {"work"}}
	req := httptest.NewRequest(http.MethodPost, "/settings/profile/switch", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if called != "work" {
		t.Errorf("SwitchProfile called with %q, want %q", called, "work")
	}
	if w.Code != http.StatusSeeOther {
		t.Errorf("expected 303, got %d", w.Code)
	}
}

// TestProfileSwitch_HTMX verifies that an HTMX profile-switch returns HX-Redirect.
func TestProfileSwitch_HTMX(t *testing.T) {
	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{
		Profiles:      func() []ui.ProfileInfo { return nil },
		ActiveProfile: func() string { return "default" },
		SwitchProfile: func(name string) error { return nil },
		CreateProfile: func(name string) error { return nil },
		DeleteProfile: func(name string) error { return nil },
	})

	form := url.Values{"name": {"work"}}
	req := httptest.NewRequest(http.MethodPost, "/settings/profile/switch", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for HTMX switch, got %d", w.Code)
	}
	if got := w.Header().Get("HX-Redirect"); got != "/settings/accounts" {
		t.Errorf("HX-Redirect: got %q, want /settings/accounts", got)
	}
}

// TestProfileSwitch_Error verifies that a switch failure re-renders the accounts
// page with an inline error (200, error text in body).
func TestProfileSwitch_Error(t *testing.T) {
	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{
		Profiles:      func() []ui.ProfileInfo { return nil },
		ActiveProfile: func() string { return "default" },
		SwitchProfile: func(name string) error {
			return errors.New("provider-open-failed: engram db missing")
		},
		CreateProfile: func(name string) error { return nil },
		DeleteProfile: func(name string) error { return nil },
	})

	form := url.Values{"name": {"work"}}
	req := httptest.NewRequest(http.MethodPost, "/settings/profile/switch", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 (error re-render), got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "provider-open-failed") {
		t.Error("response body must contain the switch error message")
	}
}

// TestProfileSwitch_NilCallback verifies that when SwitchProfile is nil, the
// handler returns 503 without panicking.
func TestProfileSwitch_NilCallback(t *testing.T) {
	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{
		Profiles:      func() []ui.ProfileInfo { return nil },
		ActiveProfile: func() string { return "default" },
		SwitchProfile: nil,
		CreateProfile: func(name string) error { return nil },
		DeleteProfile: func(name string) error { return nil },
	})

	form := url.Values{"name": {"work"}}
	req := httptest.NewRequest(http.MethodPost, "/settings/profile/switch", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("handler panicked with nil SwitchProfile: %v", r)
		}
	}()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503 with nil SwitchProfile, got %d", w.Code)
	}
}

// TestProfileSwitch_EmptyName verifies that posting an empty profile name
// returns a 200 with an error body (no switch attempted).
func TestProfileSwitch_EmptyName(t *testing.T) {
	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{
		Profiles:      func() []ui.ProfileInfo { return nil },
		ActiveProfile: func() string { return "default" },
		SwitchProfile: func(name string) error {
			t.Error("SwitchProfile should not be called for empty name")
			return nil
		},
		CreateProfile: func(name string) error { return nil },
		DeleteProfile: func(name string) error { return nil },
	})

	form := url.Values{"name": {""}}
	req := httptest.NewRequest(http.MethodPost, "/settings/profile/switch", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 (error re-render) for empty name, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// POST /settings/profile/create
// ---------------------------------------------------------------------------

// TestProfileCreate_Success verifies that POSTing a valid name calls
// CreateProfile and redirects to /settings/accounts.
func TestProfileCreate_Success(t *testing.T) {
	created := ""
	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{
		Profiles:      func() []ui.ProfileInfo { return nil },
		ActiveProfile: func() string { return "default" },
		SwitchProfile: func(name string) error { return nil },
		CreateProfile: func(name string) error {
			created = name
			return nil
		},
		DeleteProfile: func(name string) error { return nil },
	})

	form := url.Values{"name": {"work"}}
	req := httptest.NewRequest(http.MethodPost, "/settings/profile/create", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if created != "work" {
		t.Errorf("CreateProfile called with %q, want work", created)
	}
	if w.Code != http.StatusSeeOther {
		t.Errorf("expected 303, got %d", w.Code)
	}
}

// TestProfileCreate_Error verifies that a create failure re-renders with error.
func TestProfileCreate_Error(t *testing.T) {
	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{
		Profiles:      func() []ui.ProfileInfo { return nil },
		ActiveProfile: func() string { return "default" },
		SwitchProfile: func(name string) error { return nil },
		CreateProfile: func(name string) error {
			return errors.New("profile already exists: work")
		},
		DeleteProfile: func(name string) error { return nil },
	})

	form := url.Values{"name": {"work"}}
	req := httptest.NewRequest(http.MethodPost, "/settings/profile/create", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 (error re-render), got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "profile already exists") {
		t.Error("response body must contain the create error message")
	}
}

// ---------------------------------------------------------------------------
// POST /settings/profile/delete
// ---------------------------------------------------------------------------

// TestProfileDelete_Success verifies that POSTing a valid name calls
// DeleteProfile and redirects to /settings/accounts.
func TestProfileDelete_Success(t *testing.T) {
	deleted := ""
	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{
		Profiles:      func() []ui.ProfileInfo { return nil },
		ActiveProfile: func() string { return "default" },
		SwitchProfile: func(name string) error { return nil },
		CreateProfile: func(name string) error { return nil },
		DeleteProfile: func(name string) error {
			deleted = name
			return nil
		},
	})

	form := url.Values{"name": {"work"}}
	req := httptest.NewRequest(http.MethodPost, "/settings/profile/delete", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if deleted != "work" {
		t.Errorf("DeleteProfile called with %q, want work", deleted)
	}
	if w.Code != http.StatusSeeOther {
		t.Errorf("expected 303, got %d", w.Code)
	}
}

// TestProfileDelete_Error verifies that a delete failure re-renders with error.
func TestProfileDelete_Error(t *testing.T) {
	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{
		Profiles:      func() []ui.ProfileInfo { return nil },
		ActiveProfile: func() string { return "default" },
		SwitchProfile: func(name string) error { return nil },
		CreateProfile: func(name string) error { return nil },
		DeleteProfile: func(name string) error {
			return errors.New("cannot delete active profile")
		},
	})

	form := url.Values{"name": {"default"}}
	req := httptest.NewRequest(http.MethodPost, "/settings/profile/delete", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 (error re-render), got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "cannot delete active profile") {
		t.Error("response body must contain the delete error message")
	}
}

// ---------------------------------------------------------------------------
// Concurrent-request safety (race detector gate)
// ---------------------------------------------------------------------------

// TestProfileSwitch_ConcurrentRequests verifies that concurrent requests while
// a profile switch is executing do not trigger a data race or panic.
// This test is the RED gate for the go test -race requirement.
func TestProfileSwitch_ConcurrentRequests(t *testing.T) {
	var mu sync.Mutex
	callCount := 0

	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{
		Profiles:      func() []ui.ProfileInfo { return nil },
		ActiveProfile: func() string { return "default" },
		SwitchProfile: func(name string) error {
			mu.Lock()
			callCount++
			mu.Unlock()
			return nil
		},
		CreateProfile: func(name string) error { return nil },
		DeleteProfile: func(name string) error { return nil },
	})

	const goroutines = 20
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			form := url.Values{"name": {"work"}}
			req := httptest.NewRequest(http.MethodPost, "/settings/profile/switch", strings.NewReader(form.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)
		}()
	}
	wg.Wait()

	if callCount != goroutines {
		t.Errorf("SwitchProfile called %d times, want %d", callCount, goroutines)
	}
}

// TestAccountSwitcher_InSidebar verifies that the sidebar renders an account
// switcher when Profiles callback is set with multiple profiles. We use the
// accounts page (GET /settings/accounts) which uses renderDeps and doesn't
// require any database queries.
func TestAccountSwitcher_InSidebar(t *testing.T) {
	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{
		Profiles: func() []ui.ProfileInfo {
			return []ui.ProfileInfo{
				{Name: "default", Active: true},
				{Name: "work", Active: false},
			}
		},
		ActiveProfile: func() string { return "default" },
		SwitchProfile: func(name string) error { return nil },
		CreateProfile: func(name string) error { return nil },
		DeleteProfile: func(name string) error { return nil },
	})

	req := httptest.NewRequest(http.MethodGet, "/settings/accounts", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	// The account switcher must be present in the sidebar brand area.
	// It renders when there are 2+ profiles — the switch form posts to /settings/profile/switch.
	if !strings.Contains(body, "settings/profile/switch") {
		t.Error("sidebar must contain the profile switch endpoint when 2+ profiles exist")
	}
}
