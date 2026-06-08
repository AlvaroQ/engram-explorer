package ui_test

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/ui"
)

// openSessionsDB creates a test DB with the sessions schema appended to the
// base observations schema from openTestDB.
func openSessionsDB(t *testing.T) *sql.DB {
	t.Helper()
	db := openTestDB(t)
	schema := `
CREATE TABLE IF NOT EXISTS sessions (
    id TEXT PRIMARY KEY, project TEXT, directory TEXT,
    started_at TEXT, ended_at TEXT, summary TEXT
);
CREATE TABLE IF NOT EXISTS user_prompts (
    id INTEGER PRIMARY KEY AUTOINCREMENT, session_id TEXT,
    project TEXT, content TEXT, created_at TEXT
);
`
	if _, err := db.ExecContext(context.Background(), schema); err != nil {
		t.Fatalf("create sessions schema: %v", err)
	}
	return db
}

// TestSessionsListFullPage verifies GET /sessions returns a full HTML page.
func TestSessionsListFullPage(t *testing.T) {
	db := openSessionsDB(t)
	_, err := db.Exec(
		`INSERT INTO sessions (id, project, started_at) VALUES (?, ?, ?)`,
		"sess-001-abc", "myproject", "2025-01-01 10:00:00",
	)
	if err != nil {
		t.Fatalf("seed session: %v", err)
	}

	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{RoDB: db})

	req := httptest.NewRequest(http.MethodGet, "/sessions", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, "<html") {
		t.Error("full page must contain <html>")
	}
	if !strings.Contains(body, "sess-001") {
		t.Errorf("page must contain the seeded session id; got:\n%s", body)
	}
}

// TestSessionsListHTMXPartial verifies HX-Request returns a fragment without <html>.
func TestSessionsListHTMXPartial(t *testing.T) {
	db := openSessionsDB(t)

	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{RoDB: db})

	req := httptest.NewRequest(http.MethodGet, "/sessions", nil)
	req.Header.Set("HX-Request", "true")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if strings.Contains(w.Body.String(), "<html") {
		t.Error("HTMX partial must NOT contain <html>")
	}
}

// TestSessionsListPartialEndpoint verifies GET /sessions/list returns a fragment.
func TestSessionsListPartialEndpoint(t *testing.T) {
	db := openSessionsDB(t)

	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{RoDB: db})

	req := httptest.NewRequest(http.MethodGet, "/sessions/list", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "<html") {
		t.Error("list partial must NOT contain <html>")
	}
}

// TestSessionsListRoDBNil verifies nil RoDB does not panic and returns 200.
func TestSessionsListRoDBNil(t *testing.T) {
	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{RoDB: nil})

	req := httptest.NewRequest(http.MethodGet, "/sessions", nil)
	w := httptest.NewRecorder()

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("handler panicked: %v", r)
		}
	}()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}
}

// TestSessionsListHasSummaryFilter verifies ?has_summary=true filters results.
func TestSessionsListHasSummaryFilter(t *testing.T) {
	db := openSessionsDB(t)

	// One session with summary, one without.
	if _, err := db.Exec(
		`INSERT INTO sessions (id, started_at, summary) VALUES ('sess-with-summary', '2025-01-01', 'This is a summary')`,
	); err != nil {
		t.Fatalf("seed with-summary: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO sessions (id, started_at) VALUES ('sess-no-summary', '2025-01-01')`,
	); err != nil {
		t.Fatalf("seed no-summary: %v", err)
	}

	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{RoDB: db})

	req := httptest.NewRequest(http.MethodGet, "/sessions?has_summary=true", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "sess-with-summary") {
		t.Error("filtered results must include session with summary")
	}
	if strings.Contains(body, "sess-no-summary") {
		t.Error("filtered results must not include session without summary")
	}
}

// TestSessionsListEmptyState verifies empty DB renders empty state.
func TestSessionsListEmptyState(t *testing.T) {
	db := openSessionsDB(t)

	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{RoDB: db})

	req := httptest.NewRequest(http.MethodGet, "/sessions", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	// Should render empty-state, not a table.
	body := w.Body.String()
	_ = fmt.Sprintf("body len=%d", len(body)) // suppress unused import
	if strings.Contains(body, "<table") {
		t.Error("no table should be rendered when there are no sessions")
	}
}
