package ui_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/ui"
)

// TestOverviewPage_200 verifies that GET / returns 200 text/html with
// the key structural elements: KPIs, island mount divs, and issues section.
func TestOverviewPage_200(t *testing.T) {
	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{}) // nil RoDB → empty structs, no panic

	req := httptest.NewRequest(http.MethodGet, "/", nil)
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

	// Full page shell.
	if !strings.Contains(body, "<html") {
		t.Error("full page must contain <html>")
	}

	// Island loader in head.
	if !strings.Contains(body, "/islands/loader.js") {
		t.Error("layout must include /islands/loader.js script tag")
	}
}

// TestOverviewPage_NilRoDB verifies that GET / does not panic when RoDB is nil.
func TestOverviewPage_NilRoDB(t *testing.T) {
	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{RoDB: nil})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()

	// Must not panic.
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 with nil RoDB, got %d", w.Code)
	}
}

// TestOverviewPage_KPIsPresent verifies that the KPI section is rendered.
func TestOverviewPage_KPIsPresent(t *testing.T) {
	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	body := w.Body.String()

	// KPI labels from i18n en.json.
	for _, kpiLabel := range []string{"Projects", "Sessions", "Observations", "Prompts"} {
		if !strings.Contains(body, kpiLabel) {
			t.Errorf("KPI label %q not found in overview page", kpiLabel)
		}
	}
}

// TestOverviewPage_IslandMountDivs verifies that the island mount divs are present.
func TestOverviewPage_IslandMountDivs(t *testing.T) {
	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	body := w.Body.String()

	// Activity chart island.
	if !strings.Contains(body, `data-island="activity-by-project-chart"`) {
		t.Errorf("overview must include data-island=\"activity-by-project-chart\"")
	}

	// Type breakdown island.
	if !strings.Contains(body, `data-island="type-breakdown"`) {
		t.Errorf("overview must include data-island=\"type-breakdown\"")
	}

	// Brain preview card islands (two: by project + by type).
	count := strings.Count(body, `data-island="brain-preview-card"`)
	if count != 2 {
		t.Errorf("overview must include 2 brain-preview-card islands, found %d", count)
	}
}

// TestOverviewPage_IssuesSectionPresent verifies that the issues section is rendered.
func TestOverviewPage_IssuesSectionPresent(t *testing.T) {
	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	body := w.Body.String()

	// Issues section header (from i18n).
	if !strings.Contains(body, "Issues detected") {
		t.Errorf("overview must include issues section header")
	}
}

// TestOverviewPage_HTMXPartial verifies that an HX-Request returns the partial (no <html>).
func TestOverviewPage_HTMXPartial(t *testing.T) {
	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("HX-Request", "true")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	body := w.Body.String()
	if strings.Contains(body, "<html") {
		t.Error("HTMX partial must NOT contain <html> shell")
	}
}
