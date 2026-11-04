package httpapi_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// S4: a full navigation to a page whose module is disabled gets the whole
// layout (doctype, sidebar, main area) with the localized message and 503;
// an htmx request gets only the message fragment.
func TestModuleLifecycle_DisabledPageFullLayoutUnlessHTMX(t *testing.T) {
	f := newLifecycleFixture(t, false, true)
	for _, route := range []string{"/observations", "/doctor/orphans/list"} {
		w := f.fetch(t, route)
		body := w.Body.String()
		if w.Code != http.StatusServiceUnavailable {
			t.Errorf("GET %s: status %d, want 503", route, w.Code)
		}
		if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(body)), "<!doctype html>") {
			t.Errorf("GET %s: body %.120q, want a full HTML document", route, body)
		}
		for _, want := range []string{`id="main-content"`, `class="sidebar-nav"`, `href="/cc-overview"`, "This module is disabled."} {
			if !strings.Contains(body, want) {
				t.Errorf("GET %s: full page lacks %q", route, want)
			}
		}

		req := httptest.NewRequest(http.MethodGet, route, nil)
		req.Header.Set("HX-Request", "true")
		hx := httptest.NewRecorder()
		f.h.ServeHTTP(hx, req)
		if hx.Code != http.StatusServiceUnavailable {
			t.Errorf("htmx GET %s: status %d, want 503", route, hx.Code)
		}
		if b := hx.Body.String(); strings.Contains(strings.ToLower(b), "<!doctype") || strings.Contains(b, "main-content") ||
			!strings.Contains(b, `class="inline-error"`) || !strings.Contains(b, "This module is disabled.") {
			t.Errorf("htmx GET %s: body %.200q, want only the message fragment", route, b)
		}
	}
}
