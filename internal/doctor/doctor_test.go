package doctor_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/doctor"
	"github.com/AlvaroQ/engram-explorer/internal/ui"
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

// TestDoctorRootRedirectsMaintenance verifies GET /doctor/ returns a 303
// redirect to /settings/maintenance (PR4 consolidation).
func TestDoctorRootRedirectsMaintenance(t *testing.T) {
	mux := http.NewServeMux()
	doctor.Mount(mux, doctor.Deps{})

	req := httptest.NewRequest(http.MethodGet, "/doctor/", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("GET /doctor/ expected 303, got %d", w.Code)
	}
	loc := w.Header().Get("Location")
	if loc != "/settings/maintenance" {
		t.Errorf("redirect Location = %q; want /settings/maintenance", loc)
	}
}

// TestIsHTMXHeader pins the HX-Request detection doctor handlers rely on. The
// helper is shared with the main UI (ui.IsHTMX); doctor has no copy.
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
			got := ui.IsHTMX(req)
			if got != tc.wantIsHTMX {
				t.Errorf("IsHTMX() = %v, want %v", got, tc.wantIsHTMX)
			}
		})
	}
}

// TestStaticPicoCSSNotShipped verifies the unused Pico stylesheet is no longer
// embedded, while the doctor stylesheet that replaced it is still served.
func TestStaticPicoCSSNotShipped(t *testing.T) {
	mux := http.NewServeMux()
	doctor.Mount(mux, doctor.Deps{})

	req := httptest.NewRequest(http.MethodGet, "/doctor/static/pico.min.css", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for pico.min.css, got %d", w.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/doctor/static/styles.css", nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for styles.css, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, "css") {
		t.Errorf("expected CSS content-type, got %q", ct)
	}
}
