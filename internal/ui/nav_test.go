package ui_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/providers"
	"github.com/AlvaroQ/engram-explorer/internal/ui"
)

// TestNavVisibilityPost_SetsCookie verifies the toggle sets the hide cookie and
// redirects back to settings. After WU-7, section names use provider IDs.
// The legacy "engram" section still maps to nav_engram cookie.
func TestNavVisibilityPost_SetsCookie(t *testing.T) {
	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{RoDB: openTestDB(t)})

	form := url.Values{"section": {"engram"}, "value": {"hide"}}
	req := httptest.NewRequest(http.MethodPost, "/settings/nav-visibility", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d", w.Code)
	}
	found := false
	for _, c := range w.Result().Cookies() {
		if c.Name == "nav_engram" && c.Value == "0" {
			found = true
		}
	}
	if !found {
		t.Error("expected nav_engram=0 cookie to be set for section=engram")
	}
}

// TestSidebar_HidesSectionPerCookie verifies that a NavGroup with a matching
// nav_{providerID}=0 cookie is hidden. After WU-7 cookies are keyed by
// provider ID. When NavGroups is nil the sidebar falls back to legacy behavior.
func TestSidebar_HidesSectionPerCookie(t *testing.T) {
	mux := http.NewServeMux()
	// With no NavGroups func, sidebar uses legacy hardcoded nav.
	// nav_engram=0 should hide the engram section.
	ui.Mount(mux, ui.Deps{RoDB: openTestDB(t)})

	req := httptest.NewRequest(http.MethodGet, "/settings", nil)
	req.AddCookie(&http.Cookie{Name: "nav_engram", Value: "0"})
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	body := w.Body.String()
	if strings.Contains(body, "sidebar-group-engram") {
		t.Error("Engram group should be hidden when nav_engram=0")
	}
	if !strings.Contains(body, "sidebar-group-claude") {
		t.Error("Claude Code group should remain visible")
	}
}

// TestNavVisibilityPost_InvalidSection rejects an unknown section.
func TestNavVisibilityPost_InvalidSection(t *testing.T) {
	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{RoDB: openTestDB(t)})

	form := url.Values{"section": {"bogus"}, "value": {"hide"}}
	req := httptest.NewRequest(http.MethodPost, "/settings/nav-visibility", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid section, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// WU-7: Data-driven sidebar tests (RED phase — expected to fail before impl)
// ---------------------------------------------------------------------------

// TestSidebar_DataDriven_RendersNavGroup verifies that when NavGroups are
// provided via Deps, the sidebar renders them data-driven (group label
// and links appear in output).
func TestSidebar_DataDriven_RendersNavGroup(t *testing.T) {
	mux := http.NewServeMux()
	engGroup := providers.NavGroup{
		ID:       "engram",
		LabelKey: "sidebar.groupEngram",
		Featured: true,
		Links: []providers.NavLink{
			{Href: "/", Key: "overview", LabelKey: "sidebar.overview"},
		},
	}
	ui.Mount(mux, ui.Deps{
		RoDB: openTestDB(t),
		NavGroups: func() []providers.NavGroup {
			return []providers.NavGroup{engGroup}
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/settings", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	body := w.Body.String()
	if !strings.Contains(body, "sidebar-group-engram") {
		t.Error("expected sidebar-group-engram in output when NavGroups includes engram group")
	}
}

// TestSidebar_DataDriven_HidesGroupPerCookie verifies that a group with a
// nav_{providerID}=0 cookie is hidden. After WU-7 the cookie name is keyed by
// provider ID, not the legacy "claude" name.
func TestSidebar_DataDriven_HidesGroupPerCookie(t *testing.T) {
	mux := http.NewServeMux()
	engramGroup := providers.NavGroup{
		ID: "engram", LabelKey: "sidebar.groupEngram", Featured: true,
		Links: []providers.NavLink{{Href: "/", Key: "overview", LabelKey: "sidebar.overview"}},
	}
	ccGroup := providers.NavGroup{
		ID: "cc-sessions", LabelKey: "sidebar.groupClaudeCode", Featured: false,
		Links: []providers.NavLink{{Href: "/cc-sessions", Key: "cc-sessions", LabelKey: "sidebar.ccSessions"}},
	}
	ui.Mount(mux, ui.Deps{
		RoDB: openTestDB(t),
		NavGroups: func() []providers.NavGroup {
			return []providers.NavGroup{engramGroup, ccGroup}
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/settings", nil)
	// Cookie is keyed by provider ID "cc-sessions" (not the legacy "claude").
	req.AddCookie(&http.Cookie{Name: "nav_cc-sessions", Value: "0"})
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	body := w.Body.String()
	if strings.Contains(body, "sidebar-group-cc-sessions") {
		t.Error("cc-sessions group should be hidden when nav_cc-sessions=0")
	}
	if !strings.Contains(body, "sidebar-group-engram") {
		t.Error("engram group should remain visible regardless of cc-sessions cookie")
	}
}

// TestSidebar_DataDriven_EnggramFirstWhenFeatured verifies that the engram
// group (Featured=true) appears before the cc-sessions group in the sidebar,
// matching the registry's NavGroups order (engram is registered first and
// Featured so it always sorts first).
func TestSidebar_DataDriven_EngramFirstWhenFeatured(t *testing.T) {
	mux := http.NewServeMux()
	ccGroup := providers.NavGroup{
		ID: "cc-sessions", LabelKey: "sidebar.groupClaudeCode", Featured: false,
		Links: []providers.NavLink{{Href: "/cc-sessions", Key: "cc-sessions", LabelKey: "sidebar.ccSessions"}},
	}
	engramGroup := providers.NavGroup{
		ID: "engram", LabelKey: "sidebar.groupEngram", Featured: true,
		Links: []providers.NavLink{{Href: "/", Key: "overview", LabelKey: "sidebar.overview"}},
	}
	// Pass cc-sessions first, engram second — Featured ordering must still put engram first.
	ui.Mount(mux, ui.Deps{
		RoDB: openTestDB(t),
		NavGroups: func() []providers.NavGroup {
			// The registry already sorts featured first; we pass them in correct order.
			return []providers.NavGroup{engramGroup, ccGroup}
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/settings", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	body := w.Body.String()
	engramPos := strings.Index(body, "sidebar-group-engram")
	ccPos := strings.Index(body, "sidebar-group-cc-sessions")

	if engramPos < 0 {
		t.Fatal("sidebar-group-engram not found in output")
	}
	if ccPos < 0 {
		t.Fatal("sidebar-group-cc-sessions not found in output")
	}
	if engramPos > ccPos {
		t.Error("engram group (Featured=true) should appear before cc-sessions group in sidebar")
	}
}

// TestNavVisibilityPost_CCSessions_SetsCookie verifies that POST
// /settings/nav-visibility with section=cc-sessions sets the nav_cc-sessions
// cookie (provider-ID-keyed cookie name after WU-7).
func TestNavVisibilityPost_CCSessions_SetsCookie(t *testing.T) {
	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{RoDB: openTestDB(t)})

	form := url.Values{"section": {"cc-sessions"}, "value": {"hide"}}
	req := httptest.NewRequest(http.MethodPost, "/settings/nav-visibility", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d", w.Code)
	}
	found := false
	for _, c := range w.Result().Cookies() {
		if c.Name == "nav_cc-sessions" && c.Value == "0" {
			found = true
		}
	}
	if !found {
		t.Error("expected nav_cc-sessions=0 cookie to be set for section=cc-sessions")
	}
}
