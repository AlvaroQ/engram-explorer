package ui_test

// Cloud is read-only: the UI shows enrollment status but never enrolls,
// unenrolls or syncs a project (that is done with the engram CLI directly).
// These tests pin the removed routes and the kept read-only rendering.

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/ui"
)

// TestCloudMutationRoutes_Removed verifies POST /projects/{project}/enroll,
// /projects/{project}/unenroll and /partials/sync-cloud are not registered
// (404/405) even with a writable DB, and leave the enrollment table unchanged.
func TestCloudMutationRoutes_Removed(t *testing.T) {
	t.Setenv("PATH", t.TempDir()) // never spawn a real engram CLI
	db := openProjectsDB(t)
	if _, err := db.Exec(`INSERT INTO observations (type, title, project, created_at, revision_count) VALUES ('manual','t','x','2025-01-01',1)`); err != nil {
		t.Fatalf("seed obs: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO sync_enrolled_projects (project, enrolled_at) VALUES ('x', '2025-01-01')`); err != nil {
		t.Fatalf("seed enrolled: %v", err)
	}

	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{RoDB: db, RWDB: db})

	for _, path := range []string{"/projects/x/enroll", "/projects/x/unenroll", "/partials/sync-cloud"} {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, path, nil)
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)

			if w.Code != http.StatusNotFound && w.Code != http.StatusMethodNotAllowed {
				t.Errorf("POST %s: got %d, want 404 or 405 (body: %.200s)", path, w.Code, w.Body.String())
			}
			var n int
			if err := db.QueryRow(`SELECT COUNT(*) FROM sync_enrolled_projects WHERE project = 'x'`).Scan(&n); err != nil {
				t.Fatalf("count enrolled: %v", err)
			}
			if n != 1 {
				t.Errorf("POST %s: sync_enrolled_projects rows = %d, want 1 (unchanged)", path, n)
			}
		})
	}
}

// TestProjectsTable_ReadOnlyEnrollment verifies GET /projects/list keeps the
// read-only enrollment badges but renders no enroll / unenroll buttons.
func TestProjectsTable_ReadOnlyEnrollment(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	db := openProjectsDB(t)
	for _, p := range []string{"enrolled-proj", "loose-proj"} {
		if _, err := db.Exec(`INSERT INTO observations (type, title, project, created_at, revision_count) VALUES ('manual','t',?,'2025-01-01',1)`, p); err != nil {
			t.Fatalf("seed obs %s: %v", p, err)
		}
	}
	if _, err := db.Exec(`INSERT INTO sync_enrolled_projects (project, enrolled_at) VALUES ('enrolled-proj', '2025-01-01')`); err != nil {
		t.Fatalf("seed enrolled: %v", err)
	}

	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{RoDB: db, RWDB: db})

	req := httptest.NewRequest(http.MethodGet, "/projects/list", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	for _, p := range []string{"enrolled-proj", "loose-proj"} {
		for _, action := range []string{"enroll", "unenroll"} {
			if url := "/projects/" + p + "/" + action; strings.Contains(body, url) {
				t.Errorf("projects table must not offer %s", url)
			}
		}
	}
	if !strings.Contains(body, "badge-neutral") {
		t.Errorf("projects table must keep the not-enrolled badge; body: %.400s", body)
	}
	if strings.Count(body, "<tr") < 3 {
		t.Errorf("projects table must list both projects; body: %.400s", body)
	}
}
