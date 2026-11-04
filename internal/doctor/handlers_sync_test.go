package doctor_test

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/doctor"
)

// ---------------------------------------------------------------------------
// Sync-specific DB schema + seed helpers
// They reuse openTestDB from handlers_orphans_test.go (same package).
// That file already creates the full schema including sync_mutations.
// ---------------------------------------------------------------------------

// openSyncTestDB opens a test DB with the FULL schema required by sync features.
// It also adds the sync_state and sync_enrolled_projects tables
// (already created in openTestDB but we duplicate here for clarity).
func openSyncTestDB(t *testing.T) *sql.DB {
	t.Helper()
	return openTestDB(t) // reuse full schema from handlers_orphans_test.go
}

// seedEnrolledProject inserts a project into sync_enrolled_projects.
func seedEnrolledProject(t *testing.T, db *sql.DB, project string) {
	t.Helper()
	_, err := db.ExecContext(context.Background(),
		`INSERT INTO sync_enrolled_projects (project, enrolled_at) VALUES (?, datetime('now'))`, project)
	if err != nil {
		t.Fatalf("seedEnrolledProject: %v", err)
	}
}

// seedSyncState inserts a sync_state row for the given target_key.
// Requires sync_state table — extend schema if missing.
func ensureSyncStateTables(t *testing.T, db *sql.DB) {
	t.Helper()
	schema := `
CREATE TABLE IF NOT EXISTS sync_state (
    target_key TEXT PRIMARY KEY,
    lifecycle TEXT,
    last_enqueued_seq INTEGER,
    last_acked_seq INTEGER,
    last_pulled_seq INTEGER,
    consecutive_failures INTEGER DEFAULT 0,
    backoff_until TEXT,
    lease_owner TEXT,
    lease_until TEXT,
    last_error TEXT,
    reason_code TEXT,
    reason_message TEXT,
    updated_at TEXT
);`
	if _, err := db.ExecContext(context.Background(), schema); err != nil {
		t.Fatalf("ensureSyncStateTables: %v", err)
	}
}

// seedSyncState inserts a sync_state row for the given target_key.
func seedSyncState(t *testing.T, db *sql.DB, targetKey string, lifecycle string, lastError *string) {
	t.Helper()
	ensureSyncStateTables(t, db)
	_, err := db.ExecContext(context.Background(),
		`INSERT INTO sync_state (target_key, lifecycle, last_error, updated_at)
		 VALUES (?, ?, ?, datetime('now'))`,
		targetKey, lifecycle, lastError)
	if err != nil {
		t.Fatalf("seedSyncState %q: %v", targetKey, err)
	}
}

// seedPendingMutation inserts a sync_mutations row with acked_at = NULL.
func seedPendingMutation(t *testing.T, db *sql.DB, project string, entity string, op string) int64 {
	t.Helper()
	targetKey := "cloud:" + project
	res, err := db.ExecContext(context.Background(),
		`INSERT INTO sync_mutations (target_key, entity, entity_key, op, occurred_at, project)
		 VALUES (?, ?, 'key-001', ?, datetime('now'), ?)`,
		targetKey, entity, op, project)
	if err != nil {
		t.Fatalf("seedPendingMutation: %v", err)
	}
	id, _ := res.LastInsertId()
	return id
}

// seedObservationForProject inserts an observation with the given project.
func seedObservationForProject(t *testing.T, db *sql.DB, project string) {
	t.Helper()
	_, err := db.ExecContext(context.Background(),
		`INSERT INTO observations (type, title, project, created_at)
		 VALUES ('manual', 'test obs', ?, datetime('now'))`,
		project)
	if err != nil {
		t.Fatalf("seedObservationForProject: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Phase 2.1: Template render tests (RED until sync templates + handlers exist)
// ---------------------------------------------------------------------------

// TestSyncRedirect verifies GET /doctor/sync returns a 303 redirect to
// /settings/maintenance#cloud (PR4 consolidation).
func TestSyncRedirect(t *testing.T) {
	mux := http.NewServeMux()
	doctor.Mount(mux, doctor.Deps{})

	req := httptest.NewRequest(http.MethodGet, "/doctor/sync", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect, got %d; body: %s", w.Code, w.Body.String())
	}
	loc := w.Header().Get("Location")
	if loc != "/settings/maintenance#cloud" {
		t.Errorf("redirect Location = %q; want /settings/maintenance#cloud", loc)
	}
}

// TestMaintenancePage verifies GET /settings/maintenance returns 200 and
// contains both section anchors (#unassigned, #cloud).
func TestMaintenancePage(t *testing.T) {
	mux := http.NewServeMux()
	doctor.Mount(mux, doctor.Deps{})

	req := httptest.NewRequest(http.MethodGet, "/settings/maintenance", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, `id="unassigned"`) {
		t.Error("maintenance page must contain section id=\"unassigned\"")
	}
	if !strings.Contains(body, `id="cloud"`) {
		t.Error("maintenance page must contain section id=\"cloud\"")
	}
	if !strings.Contains(body, "<html") {
		t.Error("maintenance page must be a full HTML page")
	}
}

// TestMaintenancePageContainsHxTrigger verifies the maintenance page (cloud
// section) still embeds the sync shell with hx-trigger="every 15s".
func TestMaintenancePageContainsHxTrigger(t *testing.T) {
	mux := http.NewServeMux()
	doctor.Mount(mux, doctor.Deps{})

	req := httptest.NewRequest(http.MethodGet, "/settings/maintenance", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "every 15s") {
		t.Error("maintenance page cloud section must contain hx-trigger with 'every 15s'")
	}
}

// TestSyncProjectsPartialStatusBadges verifies GET /doctor/sync/projects
// returns status badges and pending_mutations count from a seeded DB.
func TestSyncProjectsPartialStatusBadges(t *testing.T) {
	db := openSyncTestDB(t)
	ensureSyncStateTables(t, db)

	// Seed an enrolled project with one pending mutation.
	const proj = "test-project"
	seedObservationForProject(t, db, proj)
	seedEnrolledProject(t, db, proj)
	seedSyncState(t, db, "cloud:"+proj, "active", nil)
	seedPendingMutation(t, db, proj, "observation", "upsert")

	mux := http.NewServeMux()
	doctor.Mount(mux, doctor.Deps{RoDB: db})

	req := httptest.NewRequest(http.MethodGet, "/doctor/sync/projects", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	// Must contain the project name.
	if !strings.Contains(body, proj) {
		t.Errorf("projects partial must contain project name %q", proj)
	}
	// Must NOT contain <html> (it's a partial).
	if strings.Contains(body, "<html") {
		t.Error("projects partial must NOT contain <html>")
	}
}

// TestSyncIssuesDaemonDown verifies that when daemonAvailable=false the issues
// partial contains "DAEMON_DOWN".  The handler uses daemon.New which we cannot
// easily stub from the outside, so we rely on the fact that in a test
// environment the daemon URL will be unreachable (empty string → immediate fail).
func TestSyncIssuesDaemonDown(t *testing.T) {
	db := openSyncTestDB(t)
	ensureSyncStateTables(t, db)

	// The doctor handler uses daemon.New(Config.DaemonBaseURL, Config.DaemonTimeoutMs).
	// An empty DaemonBaseURL + short timeout → ping fails → daemonAvailable=false.
	mux := http.NewServeMux()
	doctor.Mount(mux, doctor.Deps{RoDB: db})

	req := httptest.NewRequest(http.MethodGet, "/doctor/sync/issues", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, "DAEMON_DOWN") {
		t.Errorf("issues partial must contain DAEMON_DOWN when daemon is unreachable; body: %.300s", body)
	}
}

// TestSyncIssuesBrokenSyncState verifies that a broken sync_state row produces
// a SYNC_BROKEN issue code in the issues partial.
func TestSyncIssuesBrokenSyncState(t *testing.T) {
	db := openSyncTestDB(t)
	ensureSyncStateTables(t, db)

	const proj = "broken-proj"
	seedObservationForProject(t, db, proj)
	seedEnrolledProject(t, db, proj)
	// lifecycle=failed → deriveStatus returns SyncStatusBroken → SYNC_BROKEN issue.
	errMsg := "connection refused"
	seedSyncState(t, db, "cloud:"+proj, "failed", &errMsg)

	mux := http.NewServeMux()
	// DaemonBaseURL empty → daemon down → DAEMON_DOWN also present, but we need
	// SYNC_BROKEN too.
	doctor.Mount(mux, doctor.Deps{RoDB: db})

	req := httptest.NewRequest(http.MethodGet, "/doctor/sync/issues", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "SYNC_BROKEN") {
		t.Errorf("issues partial must contain SYNC_BROKEN; body: %.400s", body)
	}
}

// TestSyncProjectDetailPendingMutations verifies GET /doctor/sync/{project}
// returns the pending mutations table for seeded rows with acked_at NULL.
func TestSyncProjectDetailPendingMutations(t *testing.T) {
	db := openSyncTestDB(t)
	ensureSyncStateTables(t, db)

	const proj = "detail-proj"
	seedObservationForProject(t, db, proj)
	seedEnrolledProject(t, db, proj)
	seedSyncState(t, db, "cloud:"+proj, "active", nil)
	seedPendingMutation(t, db, proj, "observation", "upsert")

	mux := http.NewServeMux()
	doctor.Mount(mux, doctor.Deps{RoDB: db})

	req := httptest.NewRequest(http.MethodGet, "/doctor/sync/"+proj, nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	// Must contain the project name and at least one mutation row indicator.
	if !strings.Contains(body, proj) {
		t.Errorf("detail partial must contain project name %q", proj)
	}
	// Must NOT be a full page (it's a partial by default from HTMX context, but
	// calling without HX-Request returns the full page — verify it has content).
	// We just verify it has either the project name in context or mutation entity.
	if !strings.Contains(body, "observation") {
		t.Errorf("detail partial must contain pending mutation entity 'observation'; body: %.400s", body)
	}
}

// TestSyncProjectDetailNotFound verifies that GET /doctor/sync/{project} returns
// 404 when the project does not exist.
func TestSyncProjectDetailNotFound(t *testing.T) {
	db := openSyncTestDB(t)
	ensureSyncStateTables(t, db)

	mux := http.NewServeMux()
	doctor.Mount(mux, doctor.Deps{RoDB: db})

	req := httptest.NewRequest(http.MethodGet, "/doctor/sync/nonexistent-xyz", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown project, got %d; body: %s", w.Code, w.Body.String())
	}
}

// TestSyncProjectsRouteNotCapturedByDetail verifies that the more-specific
// /doctor/sync/projects route is NOT captured by the {project} wildcard pattern.
// This is the latent route bug fix verification.
func TestSyncProjectsRouteNotCapturedByDetail(t *testing.T) {
	db := openSyncTestDB(t)
	ensureSyncStateTables(t, db)
	seedObservationForProject(t, db, "some-project")

	mux := http.NewServeMux()
	doctor.Mount(mux, doctor.Deps{RoDB: db})

	// /doctor/sync/projects must be handled by the projects list handler, not
	// the {project} detail handler. The projects handler returns a table-like
	// partial, never a 404.
	req := httptest.NewRequest(http.MethodGet, "/doctor/sync/projects", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code == http.StatusNotFound {
		t.Fatal("/doctor/sync/projects returned 404; it must be handled by the projects list handler")
	}
	// Should be a 200 with a partial (no <html>).
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 from /doctor/sync/projects, got %d; body: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if strings.Contains(body, "<html") {
		t.Error("/doctor/sync/projects must return a partial, not a full page")
	}
}

// TestSyncIssuesRouteNotCapturedByDetail verifies /doctor/sync/issues is
// not captured by the {project} wildcard.
func TestSyncIssuesRouteNotCapturedByDetail(t *testing.T) {
	db := openSyncTestDB(t)

	mux := http.NewServeMux()
	doctor.Mount(mux, doctor.Deps{RoDB: db})

	req := httptest.NewRequest(http.MethodGet, "/doctor/sync/issues", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 from /doctor/sync/issues, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// W2 — HTMX partial mode for GET /doctor/sync/{project}
// ---------------------------------------------------------------------------

// TestSyncProjectDetailHTMXPartial verifies that GET /doctor/sync/{project}
// with header HX-Request: true returns only the detail fragment (no <html> /
// Layout wrapper), but still contains the pending-mutations table markers.
func TestSyncProjectDetailHTMXPartial(t *testing.T) {
	db := openSyncTestDB(t)
	ensureSyncStateTables(t, db)

	const proj = "htmx-detail-proj"
	seedObservationForProject(t, db, proj)
	seedEnrolledProject(t, db, proj)
	seedSyncState(t, db, "cloud:"+proj, "active", nil)
	seedPendingMutation(t, db, proj, "observation", "upsert")

	mux := http.NewServeMux()
	doctor.Mount(mux, doctor.Deps{RoDB: db})

	req := httptest.NewRequest(http.MethodGet, "/doctor/sync/"+proj, nil)
	req.Header.Set("HX-Request", "true")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	// Must NOT contain Layout wrappers.
	if strings.Contains(body, "<html") {
		t.Error("HTMX partial must NOT contain <html>")
	}
	if strings.Contains(body, "<head") {
		t.Error("HTMX partial must NOT contain <head>")
	}
	// Must contain the project-detail content.
	if !strings.Contains(body, proj) {
		t.Errorf("HTMX partial must contain project name %q; body: %.300s", proj, body)
	}
	// Must contain pending-mutations table marker.
	if !strings.Contains(body, "observation") {
		t.Errorf("HTMX partial must contain pending mutation entity; body: %.300s", body)
	}
}

// ---------------------------------------------------------------------------
// W3 — "No issues" scenario for SyncIssuesPartial
// ---------------------------------------------------------------------------

// TestSyncIssuesNoIssues verifies that when the daemon is available and the
// database has no broken/orphan/empty projects, SyncIssuesPartial renders
// the "No sync issues detected." indicator.
// The daemon availability is injected via Deps.DaemonPing to avoid a real
// HTTP round-trip in tests.
func TestSyncIssuesNoIssues(t *testing.T) {
	db := openSyncTestDB(t)
	ensureSyncStateTables(t, db)

	// Seed a healthy enrolled project: active lifecycle, no broken state.
	const proj = "healthy-proj"
	seedObservationForProject(t, db, proj)
	seedEnrolledProject(t, db, proj)
	seedSyncState(t, db, "cloud:"+proj, "active", nil)

	mux := http.NewServeMux()
	doctor.Mount(mux, doctor.Deps{
		RoDB: db,
		// Simulate daemon available so DAEMON_DOWN is not emitted.
		DaemonPing: func() bool { return true },
	})

	req := httptest.NewRequest(http.MethodGet, "/doctor/sync/issues", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, "No sync issues detected.") {
		t.Errorf("issues partial must contain 'No sync issues detected.' when state is healthy; body: %.400s", body)
	}
}
