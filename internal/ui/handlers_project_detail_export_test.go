package ui_test

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/ui"
)

func openProjectBackupDB(t *testing.T) *sql.DB {
	t.Helper()
	db := openTestDB(t)
	if _, err := db.ExecContext(context.Background(), mergeSchema); err != nil {
		t.Fatalf("schema: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO observations (type, title, project, created_at, updated_at, scope) VALUES
		('note', 'obs', 'alpha', '2026-01-01 10:00:00', '2026-01-01 10:00:00', 'project'),
		('note', 'obs', '/Users/x/repo/foo', '2026-01-01 10:00:00', '2026-01-01 10:00:00', 'project')`); err != nil {
		t.Fatalf("seed: %v", err)
	}
	return db
}

func getProjectDetail(t *testing.T, d ui.Deps, path, lang string) string {
	t.Helper()
	mux := http.NewServeMux()
	ui.Mount(mux, d)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if lang != "" {
		req.AddCookie(&http.Cookie{Name: "lang", Value: lang})
	}
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GET %s: status %d", path, w.Code)
	}
	return w.Body.String()
}

// The project page offers a download of that project's backup when writes
// are available (the export route needs the read-write pool).
func TestProjectDetail_DownloadProjectBackup(t *testing.T) {
	db := openProjectBackupDB(t)
	rw := ui.Deps{RoDB: db, RWDB: db}

	body := getProjectDetail(t, rw, "/projects/alpha", "")
	if !strings.Contains(body, `href="/api/projects/alpha/export"`) || !strings.Contains(body, "Download project backup") {
		t.Errorf("backup link missing; body: %.3000s", body)
	}
	if !strings.Contains(getProjectDetail(t, rw, "/projects/alpha", "es"), "Descargar copia del proyecto") {
		t.Error("Spanish label missing")
	}
	// Project names that are paths are escaped into one path segment.
	if body := getProjectDetail(t, rw, "/projects/%2FUsers%2Fx%2Frepo%2Ffoo", ""); !strings.Contains(body, `href="/api/projects/%2FUsers%2Fx%2Frepo%2Ffoo/export"`) {
		t.Errorf("path project link not escaped; body: %.3000s", body)
	}
	if body := getProjectDetail(t, ui.Deps{RoDB: db}, "/projects/alpha", ""); strings.Contains(body, "/api/projects/alpha/export") {
		t.Error("read-only mode must not offer the backup download")
	}
}
