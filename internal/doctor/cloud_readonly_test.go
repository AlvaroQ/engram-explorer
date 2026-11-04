package doctor_test

// Cloud is read-only: the doctor module shows sync diagnostics but never
// enrolls, unenrolls or syncs a project (that is done with the engram CLI
// directly). These tests pin the removed routes and the kept diagnostics.

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/doctor"
)

func countRows(t *testing.T, db *sql.DB, query string, args ...any) int {
	t.Helper()
	var n int
	if err := db.QueryRowContext(context.Background(), query, args...).Scan(&n); err != nil {
		t.Fatalf("count %q: %v", query, err)
	}
	return n
}

// TestSyncCloudMutationRoutes_Removed verifies POST /doctor/sync/{project}/
// enroll|unenroll|sync are not registered (404/405) even with a writable DB,
// and that hitting them leaves enrollment and sync state unchanged.
func TestSyncCloudMutationRoutes_Removed(t *testing.T) {
	t.Setenv("PATH", t.TempDir()) // never spawn a real engram CLI
	db := openSyncTestDB(t)
	ensureSyncStateTables(t, db)

	const proj = "ro-proj"
	seedObservationForProject(t, db, proj)
	seedEnrolledProject(t, db, proj)
	seedSyncState(t, db, "cloud:"+proj, "active", nil)
	seedPendingMutation(t, db, proj, "observation", "upsert")

	mux := http.NewServeMux()
	doctor.Mount(mux, doctor.Deps{RoDB: db, RWDB: db})

	for _, action := range []string{"enroll", "unenroll", "sync"} {
		t.Run(action, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/doctor/sync/"+proj+"/"+action, nil)
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)

			if w.Code != http.StatusNotFound && w.Code != http.StatusMethodNotAllowed {
				t.Errorf("POST /doctor/sync/%s/%s: got %d, want 404 or 405 (body: %.200s)",
					proj, action, w.Code, w.Body.String())
			}
			if n := countRows(t, db, `SELECT COUNT(*) FROM sync_enrolled_projects WHERE project = ?`, proj); n != 1 {
				t.Errorf("sync_enrolled_projects rows = %d, want 1 (unchanged)", n)
			}
			if n := countRows(t, db, `SELECT COUNT(*) FROM sync_state WHERE target_key = ?`, "cloud:"+proj); n != 1 {
				t.Errorf("sync_state rows = %d, want 1 (unchanged)", n)
			}
			if n := countRows(t, db, `SELECT COUNT(*) FROM sync_mutations WHERE project = ? AND acked_at IS NULL`, proj); n != 1 {
				t.Errorf("pending sync_mutations = %d, want 1 (unchanged)", n)
			}
		})
	}
}

// TestSyncDiagnostics_NoMutationAffordances verifies the kept diagnostics
// render (200) without any enroll / unenroll / sync buttons, while still
// showing the read-only enrollment status.
func TestSyncDiagnostics_NoMutationAffordances(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	db := openSyncTestDB(t)
	ensureSyncStateTables(t, db)

	const enrolled = "enrolled-proj"
	const notEnrolled = "loose-proj"
	seedObservationForProject(t, db, enrolled)
	seedObservationForProject(t, db, notEnrolled)
	seedEnrolledProject(t, db, enrolled)
	seedSyncState(t, db, "cloud:"+enrolled, "active", nil)

	mux := http.NewServeMux()
	doctor.Mount(mux, doctor.Deps{RoDB: db, RWDB: db, DaemonPing: func() bool { return true }})

	get := func(path string) string {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("HX-Request", "true")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("GET %s: got %d, want 200 (body: %.200s)", path, w.Code, w.Body.String())
		}
		return w.Body.String()
	}

	list := get("/doctor/sync/projects")
	for _, p := range []string{enrolled, notEnrolled} {
		for _, action := range []string{"enroll", "unenroll", "sync"} {
			if url := "/doctor/sync/" + p + "/" + action; strings.Contains(list, url) {
				t.Errorf("projects partial must not offer %s", url)
			}
		}
	}
	if !strings.Contains(list, "badge-enrolled") || !strings.Contains(list, "badge-not-enrolled") {
		t.Errorf("projects partial must keep the read-only enrollment badges; body: %.400s", list)
	}

	detail := get("/doctor/sync/" + enrolled)
	if strings.Contains(detail, "/doctor/sync/"+enrolled+"/sync") || strings.Contains(detail, "Sync Now") {
		t.Errorf("project detail must not offer a Sync Now action; body: %.400s", detail)
	}
	if !strings.Contains(detail, enrolled) {
		t.Errorf("project detail must still render the project; body: %.200s", detail)
	}

	_ = get("/doctor/sync/issues")
}

// TestMaintenancePage_NoSyncAllControl verifies the Maintenance > Cloud
// section no longer embeds the "Sync all" control, but still loads the sync
// diagnostics shell.
func TestMaintenancePage_NoSyncAllControl(t *testing.T) {
	mux := http.NewServeMux()
	doctor.Mount(mux, doctor.Deps{})

	req := httptest.NewRequest(http.MethodGet, "/settings/maintenance", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if strings.Contains(body, "/partials/sync-cloud") || strings.Contains(body, `id="cloud-sync-all"`) {
		t.Error("maintenance page must not embed the sync-all control")
	}
	if !strings.Contains(body, `hx-get="/doctor/sync/projects"`) {
		t.Error("maintenance page must keep the sync projects diagnostics")
	}
}

// TestSyncDeleteProject_StillWorks verifies the kept POST
// /doctor/sync/{project}/delete route removes an unenrolled, observation-free
// project and re-renders the projects table.
func TestSyncDeleteProject_StillWorks(t *testing.T) {
	db := openSyncTestDB(t)
	ensureSyncStateTables(t, db)

	const proj = "stray-repo"
	if _, err := db.Exec(`INSERT INTO sessions (id, project, directory, started_at) VALUES ('s-stray', ?, '/tmp', datetime('now'))`, proj); err != nil {
		t.Fatalf("seed session: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO user_prompts (session_id, content, project, created_at) VALUES ('s-stray', 'hi', ?, datetime('now'))`, proj); err != nil {
		t.Fatalf("seed prompt: %v", err)
	}

	mux := http.NewServeMux()
	doctor.Mount(mux, doctor.Deps{RoDB: db, RWDB: db})

	req := httptest.NewRequest(http.MethodPost, "/doctor/sync/"+proj+"/delete", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if strings.Contains(body, "inline-error") {
		t.Fatalf("delete must succeed; body: %.300s", body)
	}
	if !strings.Contains(body, `id="sync-projects-content"`) {
		t.Errorf("delete must re-render the projects table; body: %.300s", body)
	}
	if n := countRows(t, db, `SELECT COUNT(*) FROM user_prompts WHERE project = ?`, proj); n != 0 {
		t.Errorf("prompts for %s = %d, want 0", proj, n)
	}
	if n := countRows(t, db, `SELECT COUNT(*) FROM sessions WHERE project = ?`, proj); n != 0 {
		t.Errorf("sessions for %s = %d, want 0", proj, n)
	}
}
