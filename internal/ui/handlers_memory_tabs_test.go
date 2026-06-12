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

// openObservationsTestDB returns a test DB with the full observations schema
// (including review_after) so ObservationsList queries succeed.
func openObservationsTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dir := t.TempDir()
	db, err := sql.Open("sqlite", fmt.Sprintf("file:%s/obs_test.db?_pragma=journal_mode(WAL)", dir))
	if err != nil {
		t.Fatalf("open observations test DB: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	_, err = db.ExecContext(context.Background(), `
CREATE TABLE IF NOT EXISTS observations (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    type            TEXT,
    title           TEXT,
    tool_name       TEXT,
    topic_key       TEXT,
    created_at      TEXT,
    updated_at      TEXT,
    session_id      TEXT,
    sync_id         TEXT,
    project         TEXT,
    deleted_at      TEXT,
    content         TEXT,
    scope           TEXT,
    normalized_hash TEXT,
    revision_count  INTEGER DEFAULT 0,
    duplicate_count INTEGER DEFAULT 0,
    last_seen_at    TEXT,
    review_after    TEXT
);
`)
	if err != nil {
		t.Fatalf("create observations schema: %v", err)
	}
	return db
}

// ---------------------------------------------------------------------------
// Memory page — default Memories tab
// ---------------------------------------------------------------------------

// TestMemoryDefaultTabFullPage verifies GET /observations (no view param) returns 200
// with a full HTML page and the active tab marker.
func TestMemoryDefaultTabFullPage(t *testing.T) {
	db := openObservationsTestDB(t)

	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{RoDB: db})

	req := httptest.NewRequest(http.MethodGet, "/observations", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, "<html") {
		t.Errorf("full page must contain <html>; got: %.300s", body)
	}
	// The active Memories tab must have aria-current="page"
	if !strings.Contains(body, `aria-current="page"`) {
		t.Error("active tab must carry aria-current=page")
	}
}

// TestMemoryDefaultTabHTMX verifies the HTMX partial does not wrap in <html>.
func TestMemoryDefaultTabHTMX(t *testing.T) {
	db := openObservationsTestDB(t)

	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{RoDB: db})

	req := httptest.NewRequest(http.MethodGet, "/observations", nil)
	req.Header.Set("HX-Request", "true")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %.300s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "<html") {
		t.Error("HTMX partial must NOT contain <html>")
	}
}

// ---------------------------------------------------------------------------
// Memory page — Threads tab (?view=threads)
// ---------------------------------------------------------------------------

// TestMemoryThreadsTabFullPage verifies GET /observations?view=threads returns 200
// with the Memory shell and topics content.
func TestMemoryThreadsTabFullPage(t *testing.T) {
	db := openTestDB(t)
	seedTopic(t, db, "architecture/core", "engram-explorer")

	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{RoDB: db})

	req := httptest.NewRequest(http.MethodGet, "/observations?view=threads", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, "<html") {
		t.Error("full page must contain <html>")
	}
	if !strings.Contains(body, "architecture/core") {
		t.Error("Threads view must contain seeded topic key")
	}
}

// TestMemoryThreadsTabHTMX verifies HTMX partial for the threads view.
func TestMemoryThreadsTabHTMX(t *testing.T) {
	db := openTestDB(t)
	seedTopic(t, db, "some/thread", "proj")

	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{RoDB: db})

	req := httptest.NewRequest(http.MethodGet, "/observations?view=threads", nil)
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
	if !strings.Contains(body, "some/thread") {
		t.Error("threads partial must contain seeded topic")
	}
}

// ---------------------------------------------------------------------------
// Memory page — Conversations tab (?view=conversations)
// ---------------------------------------------------------------------------

// TestMemoryConversationsTabFullPage verifies GET /observations?view=conversations
// returns 200 with the Memory shell and prompts content.
func TestMemoryConversationsTabFullPage(t *testing.T) {
	db := openTestDB(t)
	_, _ = db.Exec(`
		CREATE TABLE IF NOT EXISTS user_prompts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id TEXT, content TEXT, project TEXT,
			created_at TEXT, sync_id TEXT
		)`)
	_, _ = db.Exec(`INSERT INTO user_prompts (content, project, created_at) VALUES ('Convo prompt text', 'proj', '2025-01-01')`)

	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{RoDB: db})

	req := httptest.NewRequest(http.MethodGet, "/observations?view=conversations", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, "<html") {
		t.Error("full page must contain <html>")
	}
	if !strings.Contains(body, "Convo prompt text") {
		t.Error("Conversations view must contain seeded prompt")
	}
}

// TestMemoryConversationsTabHTMX verifies HTMX partial for the conversations view.
func TestMemoryConversationsTabHTMX(t *testing.T) {
	db := openTestDB(t)
	_, _ = db.Exec(`
		CREATE TABLE IF NOT EXISTS user_prompts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id TEXT, content TEXT, project TEXT,
			created_at TEXT, sync_id TEXT
		)`)
	_, _ = db.Exec(`INSERT INTO user_prompts (content, project) VALUES ('HTMX convo content', 'proj')`)

	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{RoDB: db})

	req := httptest.NewRequest(http.MethodGet, "/observations?view=conversations", nil)
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

// ---------------------------------------------------------------------------
// Redirects
// ---------------------------------------------------------------------------

// TestTopicsRedirect verifies GET /topics redirects 301 to /observations?view=threads.
func TestTopicsRedirect(t *testing.T) {
	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{})

	req := httptest.NewRequest(http.MethodGet, "/topics", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusMovedPermanently {
		t.Fatalf("expected 301, got %d", w.Code)
	}
	loc := w.Header().Get("Location")
	if loc != "/observations?view=threads" {
		t.Errorf("expected redirect to /observations?view=threads, got %q", loc)
	}
}

// TestPromptsRedirect verifies GET /prompts redirects 301 to /observations?view=conversations.
func TestPromptsRedirect(t *testing.T) {
	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{})

	req := httptest.NewRequest(http.MethodGet, "/prompts", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusMovedPermanently {
		t.Fatalf("expected 301, got %d", w.Code)
	}
	loc := w.Header().Get("Location")
	if loc != "/observations?view=conversations" {
		t.Errorf("expected redirect to /observations?view=conversations, got %q", loc)
	}
}

// TestTopicsListPartialNotRedirected verifies GET /topics/list returns 200 (not redirected).
func TestTopicsListPartialNotRedirected(t *testing.T) {
	db := openTestDB(t)
	seedTopic(t, db, "test/topic", "proj")

	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{RoDB: db})

	req := httptest.NewRequest(http.MethodGet, "/topics/list", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}
}

// TestPromptsListPartialNotRedirected verifies GET /prompts/list returns 200 (not redirected).
func TestPromptsListPartialNotRedirected(t *testing.T) {
	db := openTestDB(t)
	_, _ = db.Exec(`
		CREATE TABLE IF NOT EXISTS user_prompts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id TEXT, content TEXT, project TEXT,
			created_at TEXT, sync_id TEXT
		)`)

	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{RoDB: db})

	req := httptest.NewRequest(http.MethodGet, "/prompts/list", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}
}
