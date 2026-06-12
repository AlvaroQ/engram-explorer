package ui_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/ui"
)

// ---------------------------------------------------------------------------
// POST /settings/advanced-view — handler tests
// ---------------------------------------------------------------------------

// TestAdvancedViewPost_Enable verifies that POSTing enabled=true calls
// SetAdvancedView(true) and redirects to /observations.
func TestAdvancedViewPost_Enable(t *testing.T) {
	called := false
	calledWith := false

	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{
		SetAdvancedView: func(enabled bool) error {
			called = true
			calledWith = enabled
			return nil
		},
	})

	form := url.Values{"enabled": {"true"}}
	req := httptest.NewRequest(http.MethodPost, "/settings/advanced-view",
		strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if !called {
		t.Fatal("SetAdvancedView was not called")
	}
	if !calledWith {
		t.Error("SetAdvancedView should have been called with true")
	}
	if w.Code != http.StatusSeeOther {
		t.Errorf("expected 303 SeeOther, got %d", w.Code)
	}
	loc := w.Header().Get("Location")
	if loc != "/observations" {
		t.Errorf("expected redirect to /observations, got %q", loc)
	}
}

// TestAdvancedViewPost_Disable verifies that POSTing enabled=false calls
// SetAdvancedView(false) and redirects to /observations.
func TestAdvancedViewPost_Disable(t *testing.T) {
	calledWith := true // start true so we detect the call changes it

	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{
		SetAdvancedView: func(enabled bool) error {
			calledWith = enabled
			return nil
		},
	})

	form := url.Values{"enabled": {"false"}}
	req := httptest.NewRequest(http.MethodPost, "/settings/advanced-view",
		strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if calledWith {
		t.Error("SetAdvancedView should have been called with false")
	}
	if w.Code != http.StatusSeeOther {
		t.Errorf("expected 303 SeeOther, got %d", w.Code)
	}
}

// TestAdvancedViewPost_HTMXRedirect verifies that an HTMX request gets an
// HX-Redirect header instead of a 303 redirect.
func TestAdvancedViewPost_HTMXRedirect(t *testing.T) {
	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{
		SetAdvancedView: func(_ bool) error { return nil },
	})

	form := url.Values{"enabled": {"true"}}
	req := httptest.NewRequest(http.MethodPost, "/settings/advanced-view",
		strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("HTMX request: expected 200, got %d", w.Code)
	}
	hxRedirect := w.Header().Get("HX-Redirect")
	if hxRedirect != "/observations" {
		t.Errorf("expected HX-Redirect: /observations, got %q", hxRedirect)
	}
}

// TestAdvancedViewPost_NilCallback verifies that the handler returns 503 when
// SetAdvancedView is nil (not wired).
func TestAdvancedViewPost_NilCallback(t *testing.T) {
	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{}) // no SetAdvancedView

	form := url.Values{"enabled": {"true"}}
	req := httptest.NewRequest(http.MethodPost, "/settings/advanced-view",
		strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", w.Code)
	}
}

// TestAdvancedViewPost_CallbackError verifies that a callback error results in 500.
func TestAdvancedViewPost_CallbackError(t *testing.T) {
	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{
		SetAdvancedView: func(_ bool) error {
			return errors.New("disk full")
		},
	})

	form := url.Values{"enabled": {"true"}}
	req := httptest.NewRequest(http.MethodPost, "/settings/advanced-view",
		strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 on callback error, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// Render tests — column visibility gated on advanced bool
// ---------------------------------------------------------------------------

// TestObservationsTableColumns_AdvancedOff verifies that the Rev and Dup columns
// are NOT rendered when advanced view is disabled.
func TestObservationsTableColumns_AdvancedOff(t *testing.T) {
	db := openObservationsTestDB(t)
	_, err := db.Exec(`INSERT INTO observations (type, title, project, created_at, revision_count, duplicate_count)
		VALUES ('decision', 'Test obs', 'proj', '2025-01-01', 3, 2)`)
	if err != nil {
		t.Fatalf("seed observation: %v", err)
	}

	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{
		RoDB:         db,
		AdvancedView: func() bool { return false },
	})

	req := httptest.NewRequest(http.MethodGet, "/observations", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %.300s", w.Code, w.Body.String())
	}
	body := w.Body.String()

	// Rev and Dup column headers must be absent.
	if strings.Contains(body, "observations.table.rev") || strings.Contains(body, ">Rev<") {
		t.Error("Rev column header must not appear when advanced=false")
	}
	if strings.Contains(body, "observations.table.dup") || strings.Contains(body, ">Dup<") {
		t.Error("Dup column header must not appear when advanced=false")
	}
}

// TestObservationsTableColumns_AdvancedOn verifies that the Rev and Dup columns
// ARE rendered when advanced view is enabled.
func TestObservationsTableColumns_AdvancedOn(t *testing.T) {
	db := openObservationsTestDB(t)
	_, err := db.Exec(`INSERT INTO observations (type, title, project, created_at, revision_count, duplicate_count)
		VALUES ('decision', 'Test obs advanced', 'proj', '2025-01-01', 5, 1)`)
	if err != nil {
		t.Fatalf("seed observation: %v", err)
	}

	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{
		RoDB:         db,
		AdvancedView: func() bool { return true },
	})

	req := httptest.NewRequest(http.MethodGet, "/observations", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %.300s", w.Code, w.Body.String())
	}
	body := w.Body.String()

	// Rev and Dup column headers must be present.
	if !strings.Contains(body, "Rev") {
		t.Error("Rev column header must appear when advanced=true")
	}
	if !strings.Contains(body, "Dup") {
		t.Error("Dup column header must appear when advanced=true")
	}
}

// TestObservationsListPartial_AdvancedThreaded verifies that GET /observations/list
// (the load-more/filter partial endpoint) also respects the advanced flag, ensuring
// that paginated rows have matching column counts.
func TestObservationsListPartial_AdvancedThreaded(t *testing.T) {
	db := openObservationsTestDB(t)
	_, err := db.Exec(`INSERT INTO observations (type, title, project, created_at)
		VALUES ('decision', 'Load more obs', 'proj', '2025-01-01')`)
	if err != nil {
		t.Fatalf("seed observation: %v", err)
	}

	for _, tt := range []struct {
		name     string
		advanced bool
		wantRev  bool
	}{
		{"advanced=false", false, false},
		{"advanced=true", true, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			mux := http.NewServeMux()
			adv := tt.advanced
			ui.Mount(mux, ui.Deps{
				RoDB:         db,
				AdvancedView: func() bool { return adv },
			})

			req := httptest.NewRequest(http.MethodGet, "/observations/list", nil)
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d", w.Code)
			}
			body := w.Body.String()
			hasRev := strings.Contains(body, "Rev") || strings.Contains(body, "observations.table.rev")
			if tt.wantRev && !hasRev {
				t.Errorf("advanced=true: Rev column must appear in /observations/list")
			}
			if !tt.wantRev && hasRev {
				t.Errorf("advanced=false: Rev column must NOT appear in /observations/list")
			}
		})
	}
}
