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
// The Home row shows 3 cards: Projects, AI conversations, Memories saved.
// The Prompts card was removed from Home in PR3 (prompts folded into Memory).
func TestOverviewPage_KPIsPresent(t *testing.T) {
	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	body := w.Body.String()

	// KPI labels from i18n en.json (plain-language labels introduced in PR3).
	for _, kpiLabel := range []string{"Projects", "AI conversations", "Memories saved"} {
		if !strings.Contains(body, kpiLabel) {
			t.Errorf("KPI label %q not found in overview page", kpiLabel)
		}
	}

	// Prompts KPI card removed from Home — must NOT appear as a KPI label.
	// (The word "Prompts" may appear elsewhere in the page, so we cannot
	// assert its total absence, but we rely on the three-card grid above.)
}

// TestOverviewPage_IslandMountDivs verifies that the island mount divs are present.
// Brain-preview-card islands were removed from Home in PR3 (visual noise + 3D weight).
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

	// Brain preview card islands removed from Home (PR3) — must NOT appear.
	count := strings.Count(body, `data-island="brain-preview-card"`)
	if count != 0 {
		t.Errorf("overview must NOT include brain-preview-card islands after PR3, found %d", count)
	}
}

// TestOverviewPage_NoAlertWhenNoIssues verifies that the sync alert bar is NOT
// rendered when there are no issues (nil RoDB → empty issues slice).
func TestOverviewPage_NoAlertWhenNoIssues(t *testing.T) {
	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{}) // nil RoDB → empty issues

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	body := w.Body.String()

	// The always-on issues card was replaced by a conditional alert bar (PR3).
	// With zero issues the alert must NOT appear.
	if strings.Contains(body, "overview-sync-alert") {
		t.Error("overview must NOT render the sync alert bar when there are no issues")
	}
	// The old full issues table header must also be absent.
	if strings.Contains(body, "Issues detected") {
		t.Error("old issues section header must not appear on Home after PR3")
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
