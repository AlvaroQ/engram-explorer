package ui_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/ui"
)

// TestNavVisibilityPost_SetsCookie verifies the toggle sets the hide cookie and
// redirects back to settings.
func TestNavVisibilityPost_SetsCookie(t *testing.T) {
	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{RoDB: openTestDB(t)})

	form := url.Values{"section": {"claude"}, "value": {"hide"}}
	req := httptest.NewRequest(http.MethodPost, "/settings/nav-visibility", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d", w.Code)
	}
	found := false
	for _, c := range w.Result().Cookies() {
		if c.Name == "nav_claude" && c.Value == "0" {
			found = true
		}
	}
	if !found {
		t.Error("expected nav_claude=0 cookie to be set")
	}
}

// TestSidebar_HidesSectionPerCookie verifies the sidebar omits a section when its
// hide cookie is present, and keeps the other one.
func TestSidebar_HidesSectionPerCookie(t *testing.T) {
	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{RoDB: openTestDB(t)})

	req := httptest.NewRequest(http.MethodGet, "/settings", nil)
	req.AddCookie(&http.Cookie{Name: "nav_claude", Value: "0"})
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	body := w.Body.String()
	if strings.Contains(body, "sidebar-group-claude") {
		t.Error("Claude Code group should be hidden when nav_claude=0")
	}
	if !strings.Contains(body, "sidebar-group-engram") {
		t.Error("Engram group should remain visible")
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
