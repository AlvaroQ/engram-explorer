package ui_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/ui"
)

// mergeSchema is the full DDL required by services.RenameProject and
// services.ProjectsList (used by the merge handler and project-detail page).
const mergeSchema = `
CREATE TABLE IF NOT EXISTS sessions (
    id TEXT PRIMARY KEY, project TEXT NOT NULL,
    directory TEXT NOT NULL DEFAULT '',
    started_at TEXT NOT NULL DEFAULT (datetime('now')),
    ended_at TEXT, summary TEXT
);
CREATE TABLE IF NOT EXISTS user_prompts (
    id INTEGER PRIMARY KEY AUTOINCREMENT, session_id TEXT NOT NULL,
    content TEXT NOT NULL, project TEXT,
    created_at TEXT NOT NULL DEFAULT (datetime('now')), sync_id TEXT
);
CREATE TABLE IF NOT EXISTS sync_enrolled_projects (
    project TEXT PRIMARY KEY, enrolled_at TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE TABLE IF NOT EXISTS sync_state (
    target_key TEXT PRIMARY KEY, lifecycle TEXT NOT NULL DEFAULT 'idle',
    last_enqueued_seq INTEGER NOT NULL DEFAULT 0,
    last_acked_seq INTEGER NOT NULL DEFAULT 0,
    last_pulled_seq INTEGER NOT NULL DEFAULT 0,
    consecutive_failures INTEGER NOT NULL DEFAULT 0,
    backoff_until TEXT, lease_owner TEXT, lease_until TEXT,
    last_error TEXT, updated_at TEXT NOT NULL DEFAULT (datetime('now')),
    reason_code TEXT, reason_message TEXT
);
CREATE TABLE IF NOT EXISTS sync_mutations (
    seq INTEGER PRIMARY KEY AUTOINCREMENT, target_key TEXT NOT NULL,
    entity TEXT NOT NULL, entity_key TEXT NOT NULL, op TEXT NOT NULL,
    payload TEXT NOT NULL, source TEXT NOT NULL DEFAULT 'local',
    occurred_at TEXT NOT NULL DEFAULT (datetime('now')), acked_at TEXT,
    project TEXT NOT NULL DEFAULT ''
);
`

// openMergeDB opens a test DB with the full schema needed for merge tests.
func openMergeDB(t *testing.T) *http.ServeMux {
	t.Helper()
	db := openTestDB(t) // base observations schema
	if _, err := db.ExecContext(context.Background(), mergeSchema); err != nil {
		t.Fatalf("merge schema: %v", err)
	}

	// Seed two projects: "alpha" and "beta".
	seedObs := func(project string) {
		_, err := db.Exec(
			`INSERT INTO observations (type, title, project, created_at, updated_at, scope)
			 VALUES ('decision', 'obs', ?, '2026-01-01 10:00:00', '2026-01-01 10:00:00', 'project')`,
			project,
		)
		if err != nil {
			t.Fatalf("seed obs for %q: %v", project, err)
		}
	}
	seedObs("alpha")
	seedObs("beta")

	mux := http.NewServeMux()
	ui.MountWithCloud(mux, ui.Deps{RoDB: db, RWDB: db}, &fakeCloud{})
	return mux
}

// TestProjectMergePost_Success verifies that POST /projects/{project}/merge
// with a valid target redirects to /projects/{target}.
func TestProjectMergePost_Success(t *testing.T) {
	mux := openMergeDB(t)

	body := url.Values{"target": {"beta"}}.Encode()
	req := httptest.NewRequest(http.MethodPost, "/projects/alpha/merge", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	// Non-HTMX should redirect via 303 to the surviving project.
	if w.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d; body: %s", w.Code, w.Body.String())
	}
	loc := w.Header().Get("Location")
	if loc != "/projects/beta" {
		t.Errorf("expected Location /projects/beta, got %q", loc)
	}
}

// TestProjectMergePost_HTMXSuccess verifies that an HTMX POST returns 200
// with HX-Redirect set to the surviving project.
func TestProjectMergePost_HTMXSuccess(t *testing.T) {
	mux := openMergeDB(t)

	body := url.Values{"target": {"beta"}}.Encode()
	req := httptest.NewRequest(http.MethodPost, "/projects/alpha/merge", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for HTMX merge, got %d; body: %s", w.Code, w.Body.String())
	}
	redirect := w.Header().Get("HX-Redirect")
	if redirect != "/projects/beta" {
		t.Errorf("expected HX-Redirect /projects/beta, got %q", redirect)
	}
}

// TestProjectMergePost_DemoMode verifies that demo mode blocks the merge.
func TestProjectMergePost_DemoMode(t *testing.T) {
	db := openTestDB(t)
	if _, err := db.ExecContext(context.Background(), mergeSchema); err != nil {
		t.Fatalf("merge schema: %v", err)
	}

	mux := http.NewServeMux()
	// DemoMode=true, RWDB still provided — requireRW must block on DemoMode first.
	ui.MountWithCloud(mux, ui.Deps{RoDB: db, RWDB: db, DemoMode: true}, &fakeCloud{})

	body := url.Values{"target": {"beta"}}.Encode()
	req := httptest.NewRequest(http.MethodPost, "/projects/alpha/merge", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	// Must not redirect; must contain an error message.
	if w.Code == http.StatusSeeOther {
		t.Fatal("demo mode must not redirect")
	}
	if !strings.Contains(w.Body.String(), "demo") && !strings.Contains(w.Body.String(), "Write") {
		t.Errorf("demo-mode response must mention demo/write restriction; body: %s", w.Body.String())
	}
}

// TestProjectMergePost_ReadOnlyMode verifies that a nil RWDB blocks the merge.
func TestProjectMergePost_ReadOnlyMode(t *testing.T) {
	db := openTestDB(t)
	if _, err := db.ExecContext(context.Background(), mergeSchema); err != nil {
		t.Fatalf("merge schema: %v", err)
	}

	mux := http.NewServeMux()
	// RWDB=nil → read-only mode.
	ui.MountWithCloud(mux, ui.Deps{RoDB: db, RWDB: nil}, &fakeCloud{})

	body := url.Values{"target": {"beta"}}.Encode()
	req := httptest.NewRequest(http.MethodPost, "/projects/alpha/merge", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code == http.StatusSeeOther {
		t.Fatal("read-only mode must not redirect")
	}
	if !strings.Contains(w.Body.String(), "read-only") && !strings.Contains(w.Body.String(), "Write") {
		t.Errorf("read-only response must mention write restriction; body: %s", w.Body.String())
	}
}

// TestProjectMergePost_TargetNotFound verifies that merging into a nonexistent
// target returns an inline error (not a redirect or 5xx).
func TestProjectMergePost_TargetNotFound(t *testing.T) {
	mux := openMergeDB(t)

	body := url.Values{"target": {"nonexistent-xyz"}}.Encode()
	req := httptest.NewRequest(http.MethodPost, "/projects/alpha/merge", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	// Must not redirect.
	if w.Code == http.StatusSeeOther {
		t.Fatal("TARGET_NOT_FOUND must not redirect")
	}
	if w.Code >= 500 {
		t.Fatalf("expected <500 for TARGET_NOT_FOUND, got %d", w.Code)
	}
	// Must contain an inline error message (not the raw code).
	body2 := w.Body.String()
	if strings.Contains(body2, "TARGET_NOT_FOUND") {
		t.Errorf("response must not expose raw error code; body: %s", body2)
	}
	// Must contain error markup.
	if !strings.Contains(body2, "inline-error") && !strings.Contains(body2, "Error") {
		t.Errorf("expected inline error in body; got: %s", body2)
	}
}

// TestProjectDetailPage_CombineFormPresent verifies that the project-detail
// page includes the combine form when there are other projects, and that it
// excludes the current project from the <select> options.
func TestProjectDetailPage_CombineFormPresent(t *testing.T) {
	db := openTestDB(t)
	extra := mergeSchema
	if _, err := db.ExecContext(context.Background(), extra); err != nil {
		t.Fatalf("extra schema: %v", err)
	}

	// Seed two projects.
	for _, proj := range []string{"alpha", "beta"} {
		if _, err := db.Exec(
			`INSERT INTO observations (type, title, project, created_at, updated_at, scope)
			 VALUES ('decision', 'obs', ?, '2026-01-01 10:00:00', '2026-01-01 10:00:00', 'project')`,
			proj,
		); err != nil {
			t.Fatalf("seed %q: %v", proj, err)
		}
	}

	mux := http.NewServeMux()
	ui.MountWithCloud(mux, ui.Deps{RoDB: db, RWDB: db}, &fakeCloud{})

	req := httptest.NewRequest(http.MethodGet, "/projects/alpha", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()

	// The combine section title must be present.
	if !strings.Contains(body, "Combine") {
		t.Error("project-detail page must contain 'Combine' section")
	}
	// "beta" must appear in the select (it's the other project).
	if !strings.Contains(body, `value="beta"`) {
		t.Error("combine select must list 'beta' as an option")
	}
	// "alpha" (current) must NOT appear as a select option.
	if strings.Contains(body, `value="alpha"`) {
		t.Error("combine select must not include the current project 'alpha'")
	}
}
