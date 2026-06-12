package ui_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/ui"
)

// TestObsDateStr_Formatted verifies that the date column renders as dd-mm-yyyy.
func TestObsDateStr_Formatted(t *testing.T) {
	db := openObservationsTestDB(t)
	_, err := db.Exec(`INSERT INTO observations (type, title, project, created_at, updated_at)
		VALUES ('decision', 'Date format test', 'proj', '2026-06-12 06:07:19', '2026-06-12 06:07:19')`)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{
		RoDB:         db,
		AdvancedView: func() bool { return false },
	})

	req := httptest.NewRequest(http.MethodGet, "/observations/list", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "12-06-2026") {
		t.Errorf("date must render as dd-mm-yyyy (12-06-2026); body excerpt: %.500s", body)
	}
	if strings.Contains(body, "2026-06-12 06:07:19") {
		t.Errorf("raw SQLite timestamp must not appear in rendered output")
	}
}

// TestObsToolColumnRemoved verifies that the Tool column header and data are absent.
func TestObsToolColumnRemoved(t *testing.T) {
	db := openObservationsTestDB(t)
	_, err := db.Exec(`INSERT INTO observations (type, title, project, tool_name, created_at)
		VALUES ('decision', 'Tool removal test', 'proj', 'SomeTool', '2025-01-01')`)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	for _, advanced := range []bool{false, true} {
		adv := advanced
		mux := http.NewServeMux()
		ui.Mount(mux, ui.Deps{
			RoDB:         db,
			AdvancedView: func() bool { return adv },
		})

		req := httptest.NewRequest(http.MethodGet, "/observations/list", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("advanced=%v: expected 200, got %d", adv, w.Code)
		}
		body := w.Body.String()
		// The word "Tool" as a column header must be absent.
		if strings.Contains(body, ">Tool<") {
			t.Errorf("advanced=%v: Tool column header must not appear in table", adv)
		}
	}
}
