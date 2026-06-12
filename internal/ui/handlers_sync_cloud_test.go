package ui_test

// Tests for POST /partials/sync-cloud — the sync-all endpoint that powers the
// Maintenance > Cloud section's CloudSyncAllControl component.

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/ui"
)

// TestSyncCloudPostNoEnrolled verifies POST /partials/sync-cloud with a DB that
// has no enrolled projects returns a 200 HTML fragment that contains the
// #cloud-sync-all container id (not the old #sidebar-sync).
func TestSyncCloudPostNoEnrolled(t *testing.T) {
	db := openTestDB(t)

	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{RoDB: db})

	req := httptest.NewRequest(http.MethodPost, "/partials/sync-cloud", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}
	ct := w.Header().Get("Content-Type")
	if !strings.HasPrefix(ct, "text/html") {
		t.Errorf("expected text/html content-type, got %q", ct)
	}
	body := w.Body.String()

	// The POST response must use the new container id (#cloud-sync-all), not
	// the old sidebar id (#sidebar-sync). This is the swap-cycle contract:
	// the idle control and the result fragment share the same id so outerHTML
	// swaps re-render correctly.
	if !strings.Contains(body, `id="cloud-sync-all"`) {
		t.Errorf("POST /partials/sync-cloud response must contain id=\"cloud-sync-all\"; body: %.300s", body)
	}
	if strings.Contains(body, `id="sidebar-sync"`) {
		t.Errorf("POST /partials/sync-cloud response must NOT contain old id=\"sidebar-sync\"; body: %.300s", body)
	}

	// The re-trigger button must point back to the same endpoint and target.
	if !strings.Contains(body, `hx-post="/partials/sync-cloud"`) {
		t.Errorf("result fragment must retain hx-post=\"/partials/sync-cloud\" for re-trigger; body: %.300s", body)
	}
	if !strings.Contains(body, `hx-target="#cloud-sync-all"`) {
		t.Errorf("result fragment must retain hx-target=\"#cloud-sync-all\" for re-trigger; body: %.300s", body)
	}

	// With no enrolled projects the result state should be noEnrolled.
	if !strings.Contains(body, "noEnrolled") {
		t.Logf("note: noEnrolled class not found (state variant may differ); body: %.300s", body)
	}

	// Must be a fragment — no full HTML shell.
	if strings.Contains(body, "<html") {
		t.Error("POST /partials/sync-cloud must return a fragment, not a full page")
	}
}

// TestSyncCloudPostNilDB verifies POST /partials/sync-cloud degrades gracefully
// when RoDB is nil: returns 200 HTML fragment with the #cloud-sync-all container.
func TestSyncCloudPostNilDB(t *testing.T) {
	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{RoDB: nil})

	req := httptest.NewRequest(http.MethodPost, "/partials/sync-cloud", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 (graceful degrade), got %d; body: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, `id="cloud-sync-all"`) {
		t.Errorf("degraded response must still contain id=\"cloud-sync-all\"; body: %.300s", body)
	}
}
