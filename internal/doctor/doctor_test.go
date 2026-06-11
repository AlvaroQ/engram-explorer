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

// TestDoctorRootRedirectsMaintenance verifies GET /doctor/ returns a 301
// redirect to /settings/maintenance (PR4 consolidation).
func TestDoctorRootRedirectsMaintenance(t *testing.T) {
	mux := http.NewServeMux()
	doctor.Mount(mux, doctor.Deps{})

	req := httptest.NewRequest(http.MethodGet, "/doctor/", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusMovedPermanently {
		t.Fatalf("GET /doctor/ expected 301, got %d", w.Code)
	}
	loc := w.Header().Get("Location")
	if loc != "/settings/maintenance" {
		t.Errorf("redirect Location = %q; want /settings/maintenance", loc)
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
