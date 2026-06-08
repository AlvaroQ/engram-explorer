package ui_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/ui"
)

// TestProjectDetailPage_NilRoDB verifies that GET /projects/{project}
// returns 404 when RoDB is nil (no DB configured).
func TestProjectDetailPage_NilRoDB(t *testing.T) {
	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{RoDB: nil})

	req := httptest.NewRequest(http.MethodGet, "/projects/my-project", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 when RoDB is nil, got %d; body: %s", w.Code, w.Body.String())
	}
}

// TestProjectDetailPage_NotFound verifies that GET /projects/{project}
// returns 404 for a project that does not exist.
// Uses nil RoDB which always returns 404 (no project can exist).
func TestProjectDetailPage_NotFound(t *testing.T) {
	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{RoDB: nil})

	req := httptest.NewRequest(http.MethodGet, "/projects/nonexistent-project-xyz", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for nonexistent project, got %d", w.Code)
	}
}

// TestProjectDetailPage_RouteRegistered verifies that the /projects/{project}
// route is registered and does not conflict with the /projects list route.
func TestProjectDetailPage_RouteRegistered(t *testing.T) {
	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{})

	// The list route must still return 200.
	req := httptest.NewRequest(http.MethodGet, "/projects", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("/projects list route broken after adding detail route, got %d", w.Code)
	}
}

// TestProjectDetailPage_200_WithIslands verifies that GET /projects/{project}
// returns 200 with text/html, KPI content, and data-island divs for both charts.
func TestProjectDetailPage_200_WithIslands(t *testing.T) {
	db := openTestDB(t)

	// Extend the minimal schema from openTestDB with project-detail dependencies.
	extra := []string{
		`CREATE TABLE IF NOT EXISTS sessions (
			id TEXT PRIMARY KEY, project TEXT NOT NULL,
			directory TEXT NOT NULL DEFAULT '', started_at TEXT NOT NULL DEFAULT (datetime('now')),
			ended_at TEXT, summary TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS user_prompts (
			id INTEGER PRIMARY KEY AUTOINCREMENT, session_id TEXT NOT NULL,
			content TEXT NOT NULL, project TEXT, created_at TEXT NOT NULL DEFAULT (datetime('now')), sync_id TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS sync_enrolled_projects (project TEXT PRIMARY KEY, enrolled_at TEXT NOT NULL DEFAULT (datetime('now')))`,
		`CREATE TABLE IF NOT EXISTS sync_state (
			target_key TEXT PRIMARY KEY, lifecycle TEXT NOT NULL DEFAULT 'idle',
			last_enqueued_seq INTEGER NOT NULL DEFAULT 0, last_acked_seq INTEGER NOT NULL DEFAULT 0,
			last_pulled_seq INTEGER NOT NULL DEFAULT 0, consecutive_failures INTEGER NOT NULL DEFAULT 0,
			backoff_until TEXT, lease_owner TEXT, lease_until TEXT, last_error TEXT,
			updated_at TEXT NOT NULL DEFAULT (datetime('now')), reason_code TEXT, reason_message TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS sync_mutations (
			seq INTEGER PRIMARY KEY AUTOINCREMENT, target_key TEXT NOT NULL, entity TEXT NOT NULL,
			entity_key TEXT NOT NULL, op TEXT NOT NULL, payload TEXT NOT NULL,
			source TEXT NOT NULL DEFAULT 'local', occurred_at TEXT NOT NULL DEFAULT (datetime('now')),
			acked_at TEXT, project TEXT NOT NULL DEFAULT ''
		)`,
	}
	for _, stmt := range extra {
		if _, err := db.ExecContext(context.Background(), stmt); err != nil {
			t.Fatalf("extra schema: %v", err)
		}
	}

	// Seed session + observation for "testproject".
	if _, err := db.Exec(`INSERT INTO sessions (id, project, directory, started_at) VALUES ('s1', 'testproject', '/tmp', '2026-01-01 10:00:00')`); err != nil {
		t.Fatalf("seed session: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO observations (id, session_id, type, title, content, project, scope, topic_key, created_at, updated_at) VALUES (10, 's1', 'decision', 'My Decision', 'body', 'testproject', 'project', 'arch/test', '2026-01-01 10:00:00', '2026-01-01 10:00:00')`); err != nil {
		t.Fatalf("seed observation: %v", err)
	}

	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{RoDB: db})

	req := httptest.NewRequest(http.MethodGet, "/projects/testproject", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}

	body := w.Body.String()

	// Verify Content-Type.
	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/html") {
		t.Errorf("expected text/html content-type, got %q", ct)
	}

	// Verify both island mount divs are present.
	if !strings.Contains(body, `data-island="project-activity-chart"`) {
		t.Error("response must contain data-island=\"project-activity-chart\"")
	}
	if !strings.Contains(body, `data-island="type-breakdown"`) {
		t.Error("response must contain data-island=\"type-breakdown\"")
	}

	// Verify KPI content is rendered.
	if !strings.Contains(body, "testproject") {
		t.Error("response must contain the project name")
	}

	// Verify island loader script is included.
	if !strings.Contains(body, "/islands/loader.js") {
		t.Error("layout must include /islands/loader.js script tag")
	}
}
