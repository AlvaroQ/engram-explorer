package ui_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/ui"
)

// ---------------------------------------------------------------------------
// Handler tests (prompts)
// ---------------------------------------------------------------------------

func TestPromptsFullPage(t *testing.T) {
	db := openTestDB(t)
	_, _ = db.Exec(`
		CREATE TABLE IF NOT EXISTS user_prompts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id TEXT, content TEXT, project TEXT,
			created_at TEXT, sync_id TEXT
		)`)
	_, _ = db.Exec(`
		INSERT INTO user_prompts (session_id, content, project, created_at)
		VALUES ('sess-1', 'Hello world prompt', 'my-project', '2025-06-01 10:00:00')`)

	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{RoDB: db})

	req := httptest.NewRequest(http.MethodGet, "/prompts", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, "<html") {
		t.Error("full page must contain <html>")
	}
	if !strings.Contains(body, "Hello world prompt") {
		t.Error("full page must contain the seeded prompt content")
	}
}

func TestPromptsHTMXPartial(t *testing.T) {
	db := openTestDB(t)
	_, _ = db.Exec(`
		CREATE TABLE IF NOT EXISTS user_prompts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id TEXT, content TEXT, project TEXT,
			created_at TEXT, sync_id TEXT
		)`)
	_, _ = db.Exec(`INSERT INTO user_prompts (content, project) VALUES ('HTMX prompt content', 'proj')`)

	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{RoDB: db})

	req := httptest.NewRequest(http.MethodGet, "/prompts", nil)
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

func TestPromptsListPartial(t *testing.T) {
	db := openTestDB(t)
	_, _ = db.Exec(`
		CREATE TABLE IF NOT EXISTS user_prompts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id TEXT, content TEXT, project TEXT,
			created_at TEXT, sync_id TEXT
		)`)
	_, _ = db.Exec(`INSERT INTO user_prompts (content, project) VALUES ('List partial prompt', 'proj')`)

	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{RoDB: db})

	req := httptest.NewRequest(http.MethodGet, "/prompts/list", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if strings.Contains(body, "<html") {
		t.Error("list partial must NOT contain <html>")
	}
	if !strings.Contains(body, "List partial prompt") {
		t.Error("list partial must contain the seeded prompt content")
	}
}

func TestPromptsRoDBNil(t *testing.T) {
	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{RoDB: nil})

	req := httptest.NewRequest(http.MethodGet, "/prompts", nil)
	w := httptest.NewRecorder()

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("handler panicked: %v", r)
		}
	}()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 with empty state when RoDB is nil, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "empty-state") {
		t.Errorf("expected empty-state in body when RoDB is nil, got: %s", body)
	}
}

func TestPromptsEmptyState(t *testing.T) {
	db := openTestDB(t)
	_, _ = db.Exec(`
		CREATE TABLE IF NOT EXISTS user_prompts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id TEXT, content TEXT, project TEXT,
			created_at TEXT, sync_id TEXT
		)`)
	// No prompts seeded.

	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{RoDB: db})

	req := httptest.NewRequest(http.MethodGet, "/prompts", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "empty-state") {
		t.Errorf("empty-state not found when no prompts exist; body: %s", body)
	}
}

func TestPromptsSearch(t *testing.T) {
	db := openTestDB(t)
	_, _ = db.Exec(`
		CREATE TABLE IF NOT EXISTS user_prompts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id TEXT, content TEXT, project TEXT,
			created_at TEXT, sync_id TEXT
		)`)
	_, _ = db.Exec(`INSERT INTO user_prompts (content, project) VALUES ('unique-needle-content', 'proj')`)
	_, _ = db.Exec(`INSERT INTO user_prompts (content, project) VALUES ('completely different text', 'proj')`)

	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{RoDB: db})

	// With search query but no FTS table — service returns empty slice gracefully.
	req := httptest.NewRequest(http.MethodGet, "/prompts?q=needle", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}
	// The FTS table doesn't exist in test DB so search returns empty — that's the graceful fallback.
	body := w.Body.String()
	if !strings.Contains(body, "<html") {
		t.Error("full-page render expected when not HTMX")
	}
}

func TestPromptsListPartialWithSearch(t *testing.T) {
	db := openTestDB(t)
	_, _ = db.Exec(`
		CREATE TABLE IF NOT EXISTS user_prompts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id TEXT, content TEXT, project TEXT,
			created_at TEXT, sync_id TEXT
		)`)

	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{RoDB: db})

	req := httptest.NewRequest(http.MethodGet, "/prompts/list?q=something", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if strings.Contains(body, "<html") {
		t.Error("list partial must NOT contain <html>")
	}
}
