package ui_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/ui"
)

// ---------------------------------------------------------------------------
// Handler tests (session detail)
// ---------------------------------------------------------------------------

func TestSessionDetailFullPage(t *testing.T) {
	db := openTestDB(t)
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS sessions (
			id TEXT PRIMARY KEY, project TEXT, directory TEXT,
			started_at TEXT, ended_at TEXT, summary TEXT
		)`); err != nil {
		t.Fatalf("create sessions table: %v", err)
	}
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS user_prompts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id TEXT, content TEXT, project TEXT,
			created_at TEXT, sync_id TEXT
		)`); err != nil {
		t.Fatalf("create user_prompts table: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO sessions (id, project, directory, started_at, ended_at, summary)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		"sess-abc", "my-project", "/home/user", "2025-01-01 10:00:00", "2025-01-01 11:00:00", "Session summary here",
	); err != nil {
		t.Fatalf("seed session: %v", err)
	}

	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{RoDB: db})

	req := httptest.NewRequest(http.MethodGet, "/sessions/sess-abc", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, "<html") {
		t.Error("full page must contain <html>")
	}
	if !strings.Contains(body, "sess-abc") {
		t.Error("full page must contain the session id")
	}
	if !strings.Contains(body, "Session summary here") {
		t.Error("full page must contain the session summary")
	}
}

func TestSessionDetailHTMXPartial(t *testing.T) {
	db := openTestDB(t)
	_, _ = db.Exec(`CREATE TABLE IF NOT EXISTS sessions (id TEXT PRIMARY KEY, project TEXT, directory TEXT, started_at TEXT, ended_at TEXT, summary TEXT)`)
	_, _ = db.Exec(`CREATE TABLE IF NOT EXISTS user_prompts (id INTEGER PRIMARY KEY AUTOINCREMENT, session_id TEXT, content TEXT, project TEXT, created_at TEXT, sync_id TEXT)`)
	_, _ = db.Exec(`INSERT INTO sessions (id, project) VALUES ('sess-xyz', 'proj')`)

	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{RoDB: db})

	req := httptest.NewRequest(http.MethodGet, "/sessions/sess-xyz", nil)
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
	if !strings.Contains(body, "sess-xyz") {
		t.Error("partial must contain the session id")
	}
}

func TestSessionDetailNotFound(t *testing.T) {
	db := openTestDB(t)
	_, _ = db.Exec(`CREATE TABLE IF NOT EXISTS sessions (id TEXT PRIMARY KEY, project TEXT, directory TEXT, started_at TEXT, ended_at TEXT, summary TEXT)`)
	_, _ = db.Exec(`CREATE TABLE IF NOT EXISTS user_prompts (id INTEGER PRIMARY KEY AUTOINCREMENT, session_id TEXT, content TEXT, project TEXT, created_at TEXT, sync_id TEXT)`)

	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{RoDB: db})

	req := httptest.NewRequest(http.MethodGet, "/sessions/nonexistent-id", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for nonexistent session, got %d", w.Code)
	}
}

func TestSessionDetailRoDBNil(t *testing.T) {
	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{RoDB: nil})

	req := httptest.NewRequest(http.MethodGet, "/sessions/any-id", nil)
	w := httptest.NewRecorder()

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("handler panicked: %v", r)
		}
	}()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 when RoDB is nil, got %d", w.Code)
	}
}

func TestSessionDetailWithEvents(t *testing.T) {
	db := openTestDB(t)
	_, _ = db.Exec(`CREATE TABLE IF NOT EXISTS sessions (id TEXT PRIMARY KEY, project TEXT, directory TEXT, started_at TEXT, ended_at TEXT, summary TEXT)`)
	_, _ = db.Exec(`CREATE TABLE IF NOT EXISTS user_prompts (id INTEGER PRIMARY KEY AUTOINCREMENT, session_id TEXT, content TEXT, project TEXT, created_at TEXT, sync_id TEXT)`)
	_, _ = db.Exec(`INSERT INTO sessions (id, project) VALUES ('sess-events', 'eng')`)

	for i := 0; i < 3; i++ {
		_, _ = db.Exec(
			`INSERT INTO observations (type, title, topic_key, project, created_at, session_id, content, revision_count)
			 VALUES ('decision', ?, 'some/key', 'eng', ?, 'sess-events', 'content here', 1)`,
			fmt.Sprintf("Obs title %d", i),
			fmt.Sprintf("2025-01-01 10:0%d:00", i),
		)
	}
	_, _ = db.Exec(`INSERT INTO user_prompts (session_id, content, project, created_at) VALUES ('sess-events', 'a user prompt', 'eng', '2025-01-01 10:05:00')`)

	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{RoDB: db})

	req := httptest.NewRequest(http.MethodGet, "/sessions/sess-events", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, "Obs title 0") {
		t.Error("page must contain observation titles")
	}
	if !strings.Contains(body, "a user prompt") {
		t.Error("page must contain prompt content")
	}
}
