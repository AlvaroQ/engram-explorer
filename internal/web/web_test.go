package web

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// makeTestHandler builds a web.Handler backed by a simple stub api mux that
// serves a known response for /doctor/* paths and a JSON stub for /api/*.
func makeTestHandler() http.Handler {
	apiMux := http.NewServeMux()

	// Simulates what doctor.Mount would register; for the guard test we just
	// need something non-index.html to come back from /doctor/*.
	apiMux.HandleFunc("GET /doctor/orphans", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "<html><body>doctor orphans</body></html>")
	})
	apiMux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"ok":true}`)
	})

	return Handler(apiMux)
}

// TestSPAGuard_DoctorRouteNotSPA verifies that GET /doctor/orphans is forwarded
// to the API mux and returns the doctor handler response.
func TestSPAGuard_DoctorRouteNotSPA(t *testing.T) {
	h := makeTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/doctor/orphans", nil)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,*/*")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	body := w.Body.String()

	// Must contain the doctor stub response.
	if !strings.Contains(body, "doctor orphans") {
		t.Errorf("expected doctor response body, got: %q", body)
	}
}

// TestUnknownPath_Returns404 verifies that unknown paths return a 404.
// The React SPA has been removed; there is no SPA fallback anymore.
func TestUnknownPath_Returns404(t *testing.T) {
	h := makeTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/observations", nil)
	req.Header.Set("Accept", "text/html,*/*")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 for unknown path /observations, got %d", w.Code)
	}
}

// TestRootDelegatedToMux verifies that GET "/" is forwarded to the API mux.
// The templ UI now serves the overview at the root (no more redirect to /ui/);
// web.Handler delegates everything except island/asset bundles to the mux.
func TestRootDelegatedToMux(t *testing.T) {
	apiMux := http.NewServeMux()
	apiMux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, "overview stub")
	})
	h := Handler(apiMux)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept", "text/html,*/*")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for GET / delegated to mux, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "overview stub") {
		t.Errorf("expected overview stub body, got %q", w.Body.String())
	}
}

// TestUnknownPaths_Return404 verifies that former SPA deep-link paths return 404.
// The React SPA has been removed; all navigation goes through the templ UI (/ui/*).
func TestUnknownPaths_Return404(t *testing.T) {
	h := makeTestHandler()

	for _, path := range []string{"/observations", "/projects/myproj", "/brain"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Accept", "text/html,*/*")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Errorf("expected 404 for unknown path %q (SPA removed), got %d", path, w.Code)
		}
	}
}

// TestSPAGuard_DoctorRootForwarded verifies that /doctor (no trailing slash)
// is also forwarded to the API mux and does not hit the SPA fallback.
func TestSPAGuard_DoctorRootForwarded(t *testing.T) {
	apiMux := http.NewServeMux()
	apiMux.HandleFunc("GET /doctor", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, "doctor root")
	})
	h := Handler(apiMux)

	req := httptest.NewRequest(http.MethodGet, "/doctor", nil)
	req.Header.Set("Accept", "text/html,*/*")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	body := w.Body.String()
	if !strings.Contains(body, "doctor root") {
		t.Errorf("expected 'doctor root' in body, got: %q", body)
	}
}
