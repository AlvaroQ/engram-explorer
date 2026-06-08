package ui_test

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/AlvaroQ/engram-explorer/internal/ui"
)

// ---------------------------------------------------------------------------
// SQLite schema helpers
// ---------------------------------------------------------------------------

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dir := t.TempDir()
	db, err := sql.Open("sqlite", fmt.Sprintf("file:%s/test.db?_pragma=journal_mode(WAL)", dir))
	if err != nil {
		t.Fatalf("open test DB: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	schema := `
CREATE TABLE IF NOT EXISTS observations (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    type TEXT,
    title TEXT,
    tool_name TEXT,
    topic_key TEXT,
    created_at TEXT,
    updated_at TEXT,
    session_id TEXT,
    sync_id TEXT,
    project TEXT,
    deleted_at TEXT,
    content TEXT,
    scope TEXT,
    normalized_hash TEXT,
    revision_count INTEGER DEFAULT 0,
    duplicate_count INTEGER DEFAULT 0,
    last_seen_at TEXT
);
`
	if _, err := db.ExecContext(context.Background(), schema); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	return db
}

func seedTopic(t *testing.T, db *sql.DB, topicKey, project string) {
	t.Helper()
	_, err := db.Exec(
		`INSERT INTO observations (type, title, topic_key, project, created_at, revision_count)
         VALUES ('manual', 'test obs', ?, ?, '2025-01-01 00:00:00', 1)`,
		topicKey, project,
	)
	if err != nil {
		t.Fatalf("seed topic %q: %v", topicKey, err)
	}
}

// ---------------------------------------------------------------------------
// Handler tests
// ---------------------------------------------------------------------------

// TestTopicsFullPage verifies that GET /topics without HX-Request returns
// a full HTML page containing <html> and <head>.
func TestTopicsFullPage(t *testing.T) {
	db := openTestDB(t)
	seedTopic(t, db, "architecture/auth-model", "engram-explorer")

	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{RoDB: db})

	req := httptest.NewRequest(http.MethodGet, "/topics", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, "<html") {
		t.Error("full page must contain <html>")
	}
	if !strings.Contains(body, "<head") {
		t.Error("full page must contain <head>")
	}
	if !strings.Contains(body, "architecture/auth-model") {
		t.Error("full page must contain the seeded topic key")
	}
}

// TestTopicsHTMXPartial verifies GET /topics with HX-Request: true returns
// a fragment — no <html> wrapper.
func TestTopicsHTMXPartial(t *testing.T) {
	db := openTestDB(t)
	seedTopic(t, db, "some/topic", "myproject")

	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{RoDB: db})

	req := httptest.NewRequest(http.MethodGet, "/topics", nil)
	req.Header.Set("HX-Request", "true")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if strings.Contains(body, "<html") {
		t.Error("HTMX partial must NOT contain <html>")
	}
}

// TestTopicsQueryFilter verifies that ?q= filters the results server-side.
func TestTopicsQueryFilter(t *testing.T) {
	db := openTestDB(t)
	seedTopic(t, db, "architecture/auth-model", "engram-explorer")
	seedTopic(t, db, "bugfix/null-pointer", "other-project")

	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{RoDB: db})

	req := httptest.NewRequest(http.MethodGet, "/topics?q=architecture", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "architecture/auth-model") {
		t.Error("filtered results must include matching topic")
	}
	if strings.Contains(body, "bugfix/null-pointer") {
		t.Error("filtered results must not include non-matching topic")
	}
}

// TestTopicsListPartial verifies GET /topics/list returns a fragment without <html>.
func TestTopicsListPartial(t *testing.T) {
	db := openTestDB(t)
	seedTopic(t, db, "some/topic", "myproject")

	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{RoDB: db})

	req := httptest.NewRequest(http.MethodGet, "/topics/list", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if strings.Contains(body, "<html") {
		t.Error("list partial must NOT contain <html>")
	}
}

// TestTopicsListPartialFilter verifies GET /topics/list?q= filters correctly.
func TestTopicsListPartialFilter(t *testing.T) {
	db := openTestDB(t)
	seedTopic(t, db, "architecture/auth-model", "eng")
	seedTopic(t, db, "bugfix/crash", "eng")

	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{RoDB: db})

	req := httptest.NewRequest(http.MethodGet, "/topics/list?q=arch", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "architecture/auth-model") {
		t.Error("partial should include matching topic")
	}
	if strings.Contains(body, "bugfix/crash") {
		t.Error("partial should exclude non-matching topic")
	}
}

// TestTopicsRoDBNil verifies that when RoDB is nil the handler returns an empty
// state without panicking.
func TestTopicsRoDBNil(t *testing.T) {
	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{RoDB: nil})

	req := httptest.NewRequest(http.MethodGet, "/topics", nil)
	w := httptest.NewRecorder()

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("handler panicked: %v", r)
		}
	}()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 with empty state, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "empty-state") {
		t.Errorf("expected empty-state in body when RoDB is nil, got: %s", body)
	}
}

// TestTopicsEmptyState verifies that an empty DB renders the empty-state element.
func TestTopicsEmptyState(t *testing.T) {
	db := openTestDB(t)
	// No topics seeded.

	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{RoDB: db})

	req := httptest.NewRequest(http.MethodGet, "/topics", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "empty-state") {
		t.Errorf("empty-state not found when no topics exist; body: %s", body)
	}
	if strings.Contains(body, "<table") {
		t.Error("no table should be rendered when there are no topics")
	}
}
