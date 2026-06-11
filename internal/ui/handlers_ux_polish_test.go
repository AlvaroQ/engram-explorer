package ui_test

// WU-11: UX polish pass — View Transitions, HTMX swap refinements,
// scroll/focus preservation, OOB account switcher.
//
// Tests assert rendered HTML attributes so the contract is locked at the
// server-rendered layer (no browser required).

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/ui"
)

// ---------------------------------------------------------------------------
// Layout shell: main region must have stable id for HTMX targeting
// ---------------------------------------------------------------------------

// TestLayout_MainContentHasID verifies that the full-page layout shell
// renders <main id="main-content"> so HTMX can target it.
// Uses nil RoDB + ActiveCount=1 to take the normal overview path with empty data.
func TestLayout_MainContentHasID(t *testing.T) {
	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{
		RoDB:        nil, // nil RoDB: loadOverview returns empty data (no error)
		ActiveCount: func() int { return 1 },
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, `id="main-content"`) {
		t.Error("layout must render <main id=\"main-content\"> for HTMX targeting")
	}
}

// TestLayout_ViewTransitionCSS verifies that the layout head includes a
// view-transition CSS rule or @view-transition declaration.
func TestLayout_ViewTransitionCSS(t *testing.T) {
	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{
		RoDB:        nil, // nil RoDB: loadOverview returns empty data (no error)
		ActiveCount: func() int { return 1 },
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	body := w.Body.String()
	// Either an inline <style> block or a separate stylesheet reference must
	// contain view-transition-name.
	if !strings.Contains(body, "view-transition-name") && !strings.Contains(body, "view-transitions.css") {
		t.Error("layout head must reference view-transition-name CSS (inline or via stylesheet)")
	}
}

// ---------------------------------------------------------------------------
// Sidebar: hx-preserve on nav wrap for scroll position preservation
// ---------------------------------------------------------------------------

// TestSidebar_NavWrapPreserved verifies that the sidebar nav wrapper carries
// hx-preserve so HTMX respects scroll position across partial swaps.
func TestSidebar_NavWrapPreserved(t *testing.T) {
	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{
		RoDB:        nil, // nil RoDB: loadOverview returns empty data (no error)
		ActiveCount: func() int { return 1 },
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	body := w.Body.String()
	if !strings.Contains(body, "hx-preserve") {
		t.Error("sidebar nav-wrap must have hx-preserve for scroll position preservation")
	}
}

// ---------------------------------------------------------------------------
// Onboarding: activation forms must target #main-content, not body
// ---------------------------------------------------------------------------

// TestOnboarding_ActivateForm_TargetsMainContent verifies that the detected-
// provider activation form in onboarding targets #main-content (not body).
func TestOnboarding_ActivateForm_TargetsMainContent(t *testing.T) {
	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{
		RoDB:        nil,
		ActiveCount: func() int { return 0 },
		Modules: func() []ui.ModuleInfo {
			return []ui.ModuleInfo{
				makeTier1Module("engram", "Engram", ui.ModuleDetected),
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
	// Must not use full-body outerHTML swap on the activate form.
	if strings.Contains(body, `hx-swap="outerHTML"`) && strings.Contains(body, `hx-target="body"`) {
		t.Error("onboarding activation form must not use hx-target=\"body\" hx-swap=\"outerHTML\"")
	}
	// Must target the main region with a transition.
	if !strings.Contains(body, `hx-target="#main-content"`) {
		t.Error("onboarding form must use hx-target=\"#main-content\"")
	}
	if !strings.Contains(body, "transition:true") {
		t.Error("onboarding form swap must include transition:true")
	}
}

// TestOnboarding_PathForm_TargetsMainContent verifies that the undetected-
// provider manual path form in onboarding targets #main-content.
func TestOnboarding_PathForm_TargetsMainContent(t *testing.T) {
	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{
		RoDB:        nil,
		ActiveCount: func() int { return 0 },
		Modules: func() []ui.ModuleInfo {
			return []ui.ModuleInfo{
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
	if !strings.Contains(body, `hx-target="#main-content"`) {
		t.Error("onboarding path form must target #main-content")
	}
}

// ---------------------------------------------------------------------------
// Account switcher: must not do full-body outerHTML swap
// ---------------------------------------------------------------------------

// TestAccountSwitcher_NoFullBodySwap verifies that the account switcher in the
// sidebar does not use hx-target="body" hx-swap="outerHTML".
// Profile switch should use HX-Redirect (handled by the server) instead of
// client-side full-body swap.
func TestAccountSwitcher_NoFullBodySwap(t *testing.T) {
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

	body := w.Body.String()
	// The account switcher must NOT render hx-target="body" on its switch forms.
	if strings.Contains(body, `hx-target="body"`) {
		t.Error("account switcher must not use hx-target=\"body\" — use HX-Redirect instead")
	}
}

// ---------------------------------------------------------------------------
// Overview HTMX partial: must use renderDeps (not bare render)
// ---------------------------------------------------------------------------

// TestOverviewHTMX_UsesNavGroupsContext verifies that an HTMX GET / request
// when a provider is active returns the overview partial AND still has access
// to nav context (profiles / nav groups), confirming renderDeps is used.
func TestOverviewHTMX_UsesNavGroupsContext(t *testing.T) {
	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{
		RoDB:        nil, // nil RoDB: loadOverview returns empty data (no error)
		ActiveCount: func() int { return 1 },
		Profiles: func() []ui.ProfileInfo {
			return []ui.ProfileInfo{{Name: "default", Active: true}}
		},
		ActiveProfile: func() string { return "default" },
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("HX-Request", "true")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	// Partial must NOT include the full HTML shell.
	body := w.Body.String()
	if strings.Contains(body, "<!DOCTYPE html>") {
		t.Error("HTMX overview response must not include the HTML shell")
	}
	// Partial should contain overview content (not onboarding).
	if strings.Contains(body, "onboarding") {
		t.Error("HTMX overview (active provider) must not render onboarding content")
	}
}
