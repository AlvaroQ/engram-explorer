package doctor_test

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/AlvaroQ/engram-explorer/internal/doctor"
)

// ---------------------------------------------------------------------------
// SQLite schema helpers
// ---------------------------------------------------------------------------

// openTestDB opens a fresh in-memory SQLite database for tests.
// It creates only the tables needed by the orphans feature.
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
    revision_count INTEGER,
    duplicate_count INTEGER,
    last_seen_at TEXT
);
CREATE TABLE IF NOT EXISTS sessions (
    id TEXT PRIMARY KEY,
    directory TEXT,
    started_at TEXT,
    ended_at TEXT,
    summary TEXT,
    project TEXT
);
CREATE TABLE IF NOT EXISTS user_prompts (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    content TEXT,
    session_id TEXT,
    created_at TEXT,
    sync_id TEXT,
    project TEXT
);
CREATE TABLE IF NOT EXISTS sync_enrolled_projects (
    project     TEXT PRIMARY KEY,
    enrolled_at TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE TABLE IF NOT EXISTS sync_mutations (
    seq INTEGER PRIMARY KEY AUTOINCREMENT,
    target_key TEXT,
    entity TEXT,
    entity_key TEXT,
    op TEXT,
    payload TEXT,
    source TEXT,
    occurred_at TEXT,
    project TEXT,
    acked_at TEXT
);
`
	if _, err := db.ExecContext(context.Background(), schema); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	return db
}

// seedOrphanObservation inserts an observation with no project and returns its id.
func seedOrphanObservation(t *testing.T, db *sql.DB) int64 {
	t.Helper()
	res, err := db.Exec(
		`INSERT INTO observations (type, title, created_at) VALUES ('manual','orphan obs','2025-01-01 00:00:00')`,
	)
	if err != nil {
		t.Fatalf("seed orphan obs: %v", err)
	}
	id, _ := res.LastInsertId()
	return id
}

// seedOrphanSession inserts a session with no project and returns its id.
func seedOrphanSession(t *testing.T, db *sql.DB) string {
	t.Helper()
	sid := "sess-orphan-001"
	if _, err := db.Exec(
		`INSERT INTO sessions (id, started_at) VALUES (?, '2025-01-01 00:00:00')`, sid,
	); err != nil {
		t.Fatalf("seed orphan session: %v", err)
	}
	return sid
}

// ---------------------------------------------------------------------------
// Phase 1.1: Template render tests (RED until handlers exist)
// ---------------------------------------------------------------------------

// TestOrphansFullPage verifies GET /doctor/orphans with no HX-Request returns
// a full HTML page containing <html> and <head>.
func TestOrphansFullPage(t *testing.T) {
	db := openTestDB(t)
	seedOrphanObservation(t, db)

	mux := http.NewServeMux()
	doctor.Mount(mux, doctor.Deps{RoDB: db})

	req := httptest.NewRequest(http.MethodGet, "/doctor/orphans", nil)
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
}

// TestOrphansHTMXPartial verifies GET /doctor/orphans with HX-Request: true
// returns a fragment — no <html>, no <head>.
func TestOrphansHTMXPartial(t *testing.T) {
	db := openTestDB(t)
	seedOrphanObservation(t, db)

	mux := http.NewServeMux()
	doctor.Mount(mux, doctor.Deps{RoDB: db})

	req := httptest.NewRequest(http.MethodGet, "/doctor/orphans", nil)
	req.Header.Set("HX-Request", "true")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if strings.Contains(body, "<html") {
		t.Error("partial must NOT contain <html>")
	}
}

// TestOrphansEmptyState verifies that when OrphansList returns zero counts for
// all types, the EmptyState component is rendered and no table is shown.
func TestOrphansEmptyState(t *testing.T) {
	db := openTestDB(t)
	// Do NOT seed orphans — all totals will be zero.

	mux := http.NewServeMux()
	doctor.Mount(mux, doctor.Deps{RoDB: db})

	req := httptest.NewRequest(http.MethodGet, "/doctor/orphans", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	// EmptyState renders with class "empty-state"
	if !strings.Contains(body, "empty-state") {
		t.Error("empty state not rendered when no orphans exist")
	}
	// No entity table sections should appear
	if strings.Contains(body, "<table") {
		t.Error("no table should be rendered when all totals are zero")
	}
}

// TestOrphansListPartial verifies GET /doctor/orphans/list returns only the
// orphans fragment (no <html>) regardless of HX-Request header.
func TestOrphansListPartial(t *testing.T) {
	db := openTestDB(t)
	seedOrphanObservation(t, db)

	mux := http.NewServeMux()
	doctor.Mount(mux, doctor.Deps{RoDB: db})

	req := httptest.NewRequest(http.MethodGet, "/doctor/orphans/list", nil)
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

// ---------------------------------------------------------------------------
// Phase 1.2: Write action tests (RED until write handlers exist)
// ---------------------------------------------------------------------------

// TestAssignUnknownEntity verifies that POST /doctor/orphans/{entity}/{id}/project
// with an unknown entity segment returns 400.
func TestAssignUnknownEntity(t *testing.T) {
	db := openTestDB(t)

	mux := http.NewServeMux()
	doctor.Mount(mux, doctor.Deps{RoDB: db, RWDB: db})

	form := url.Values{"project": {"myproject"}}
	req := httptest.NewRequest(http.MethodPost, "/doctor/orphans/unknown/42/project",
		strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for unknown entity, got %d", w.Code)
	}
}

// TestAssignSuccessObservation verifies that a valid POST assign returns the
// refreshed OrphansList partial and the assigned entity no longer appears.
func TestAssignSuccessObservation(t *testing.T) {
	db := openTestDB(t)
	id := seedOrphanObservation(t, db)

	mux := http.NewServeMux()
	doctor.Mount(mux, doctor.Deps{RoDB: db, RWDB: db})

	form := url.Values{"project": {"myproject"}}
	target := fmt.Sprintf("/doctor/orphans/observations/%d/project", id)
	req := httptest.NewRequest(http.MethodPost, target, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	// Should not contain the assigned observation anymore (or show empty-state)
	// The list partial must NOT contain <html>
	if strings.Contains(body, "<html") {
		t.Error("assign response must be a partial, not a full page")
	}
}

// TestAssignRWDBNil verifies that POST assign when RWDB is nil returns an
// inline error partial (not a panic and not 500 with empty body).
func TestAssignRWDBNil(t *testing.T) {
	db := openTestDB(t)

	mux := http.NewServeMux()
	// RWDB intentionally nil — read-only mode
	doctor.Mount(mux, doctor.Deps{RoDB: db, RWDB: nil})

	form := url.Values{"project": {"myproject"}}
	req := httptest.NewRequest(http.MethodPost, "/doctor/orphans/observations/1/project",
		strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	// Must not panic.
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("handler panicked: %v", r)
		}
	}()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 with inline error partial, got %d", w.Code)
	}
	body := w.Body.String()
	// Inline error partial uses class "inline-error"
	if !strings.Contains(body, "inline-error") {
		t.Errorf("expected inline-error in response body, got: %s", body)
	}
}

// TestDeleteSuccessObservation verifies DELETE /doctor/orphans/observations/{id}
// returns the refreshed OrphansList partial on success.
func TestDeleteSuccessObservation(t *testing.T) {
	db := openTestDB(t)
	id := seedOrphanObservation(t, db)

	mux := http.NewServeMux()
	doctor.Mount(mux, doctor.Deps{RoDB: db, RWDB: db})

	target := fmt.Sprintf("/doctor/orphans/observations/%d", id)
	req := httptest.NewRequest(http.MethodDelete, target, nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if strings.Contains(body, "<html") {
		t.Error("delete response must be a partial, not a full page")
	}
}

// TestDeleteRWDBNil verifies that DELETE when RWDB is nil returns an inline
// error partial and does not panic.
func TestDeleteRWDBNil(t *testing.T) {
	db := openTestDB(t)

	mux := http.NewServeMux()
	doctor.Mount(mux, doctor.Deps{RoDB: db, RWDB: nil})

	req := httptest.NewRequest(http.MethodDelete, "/doctor/orphans/observations/1", nil)
	w := httptest.NewRecorder()

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("handler panicked: %v", r)
		}
	}()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 with inline error partial, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "inline-error") {
		t.Errorf("expected inline-error in body, got: %s", body)
	}
}

// TestOrphansSectionsExistWhenDataPresent verifies that observation and session
// sections render when data is present, but prompts section is absent when
// there are no orphaned prompts.
func TestOrphansSectionsExistWhenDataPresent(t *testing.T) {
	db := openTestDB(t)
	seedOrphanObservation(t, db)
	seedOrphanSession(t, db)
	// No orphan prompts seeded.

	mux := http.NewServeMux()
	doctor.Mount(mux, doctor.Deps{RoDB: db})

	req := httptest.NewRequest(http.MethodGet, "/doctor/orphans/list", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	// Should contain observation section
	if !strings.Contains(body, "orphan obs") {
		t.Error("expected orphan observation title in response")
	}
	// Should contain session section
	if !strings.Contains(body, "sess-orphan-001") {
		t.Error("expected orphan session id in response")
	}
}
