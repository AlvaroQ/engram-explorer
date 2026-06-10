package doctor_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/doctor"
)

// TestDoctorPackageCompiles is a compile-only smoke test that verifies the
// package exists, Deps can be instantiated, and Mount can be called.
// RED: this test must fail until doctor.go is created.
func TestDoctorPackageCompiles(t *testing.T) {
	mux := http.NewServeMux()
	deps := doctor.Deps{}
	doctor.Mount(mux, deps)
	// If we got here without panicking, the package compiles and Mount exists.
}

// TestMountSmoke calls doctor.Mount on a fresh mux with zero Deps and confirms
// no panic occurs. RWDB may be nil; Mount must handle that gracefully.
func TestMountSmoke(t *testing.T) {
	mux := http.NewServeMux()
	deps := doctor.Deps{}
	// Must not panic.
	doctor.Mount(mux, deps)
}

// TestStaticServed requests GET /doctor/static/htmx.min.js via httptest and
// asserts a 200 status code. RED until static assets + FileServer are wired.
func TestStaticServed(t *testing.T) {
	mux := http.NewServeMux()
	doctor.Mount(mux, doctor.Deps{})

	req := httptest.NewRequest(http.MethodGet, "/doctor/static/htmx.min.js", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

// TestIsHTMXHelper verifies the isHTMX detection indirectly via the render
// helper: without HX-Request header the response should contain <html>;
// with HX-Request: true it should NOT.
//
// We test this through GET /doctor/ (which redirects to /doctor/orphans is
// slice 1 work, but the root route returns 302 — that also proves Mount ran).
// For a more direct test we rely on the full-page vs partial behaviour that
// slice 1 handler tests will exercise.  Here we just assert the route exists
// and returns something (redirect counts as "no 404").
func TestDoctorRootExists(t *testing.T) {
	mux := http.NewServeMux()
	doctor.Mount(mux, doctor.Deps{})

	req := httptest.NewRequest(http.MethodGet, "/doctor/", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	// Root redirects (302) or returns content (200) — either is fine for Slice 0.
	// It must NOT be 404.
	if w.Code == http.StatusNotFound {
		t.Fatalf("GET /doctor/ returned 404; expected redirect or 200")
	}
}

// TestIsHTMX validates the isHTMX helper is exported as a test helper or
// through the render path.  We test indirectly: hitting a route with
// HX-Request: true should return a fragment (no <html>), hitting without
// should return a full page (contains <html> or a redirect).
//
// This is a table-driven test for the isHTMX detection logic.
func TestIsHTMXHeader(t *testing.T) {
	cases := []struct {
		name       string
		header     string
		wantIsHTMX bool
	}{
		{"no header", "", false},
		{"header true", "true", true},
		{"header false", "false", false},
		{"header random", "1", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tc.header != "" {
				req.Header.Set("HX-Request", tc.header)
			}
			got := doctor.IsHTMX(req)
			if got != tc.wantIsHTMX {
				t.Errorf("IsHTMX() = %v, want %v", got, tc.wantIsHTMX)
			}
		})
	}
}

// TestStaticPicoCSSServed verifies pico.min.css is embedded and served.
func TestStaticPicoCSSServed(t *testing.T) {
	mux := http.NewServeMux()
	doctor.Mount(mux, doctor.Deps{})

	req := httptest.NewRequest(http.MethodGet, "/doctor/static/pico.min.css", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for pico.min.css, got %d", w.Code)
	}
	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "css") {
		t.Errorf("expected CSS content-type, got %q", ct)
	}
}
