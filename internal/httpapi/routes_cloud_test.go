package httpapi_test

// Cloud is read-only: the dashboard never enrolls, unenrolls or syncs projects
// (that is done with the engram CLI directly). These tests pin that contract:
// the former /api/cloud/* mutation and capabilities routes are not registered,
// while the sync diagnostics and the GET-only daemon proxy keep responding.

import (
	"bytes"
	"database/sql"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/AlvaroQ/engram-explorer/internal/config"
	"github.com/AlvaroQ/engram-explorer/internal/httpapi"
)

// newReadOnlyCloudContainer builds a writable Container (RWDB available) whose
// daemon URL is unreachable, and returns it with the seeded DB path.
// PATH is emptied so no real engram CLI can ever be spawned by a test.
func newReadOnlyCloudContainer(t *testing.T) (*httpapi.Container, string) {
	t.Helper()
	t.Setenv("PATH", t.TempDir())
	path := seedEngramDB(t)
	cfg := config.Config{
		Host:            "127.0.0.1",
		Port:            8787,
		EngramDbPath:    path,
		DaemonBaseURL:   "http://127.0.0.1:1", // always refused
		DaemonTimeoutMs: 200,
		Env:             "development",
		ExposeDetails:   true,
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	c, err := httpapi.NewContainer(cfg, logger)
	if err != nil {
		t.Fatalf("NewContainer: %v", err)
	}
	t.Cleanup(c.Close)
	return c, path
}

// enrolledCount returns how many sync_enrolled_projects rows exist for project.
func enrolledCount(t *testing.T, path, project string) int {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sync_enrolled_projects WHERE project = ?`, project).Scan(&n); err != nil {
		t.Fatalf("count enrolled: %v", err)
	}
	return n
}

func seedEnrolled(t *testing.T, path, project string) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO sync_enrolled_projects (project, enrolled_at) VALUES (?, '2026-01-01 10:00:00')`, project); err != nil {
		t.Fatalf("seed enrolled: %v", err)
	}
}

// TestCloudRoutes_Removed verifies every former /api/cloud/* route is gone
// (404 or 405) and that hitting them leaves the enrollment table unchanged.
func TestCloudRoutes_Removed(t *testing.T) {
	c, path := newReadOnlyCloudContainer(t)
	seedEnrolled(t, path, "myproject")
	handler := httpapi.NewServeMux(c)

	cases := []struct {
		method string
		path   string
		body   string
	}{
		{http.MethodPost, "/api/cloud/enroll", `{"project":"myproject","confirm":true}`},
		{http.MethodPost, "/api/cloud/unenroll", `{"project":"myproject","confirm":true}`},
		{http.MethodPost, "/api/cloud/sync", `{"project":"myproject"}`},
		{http.MethodPost, "/api/cloud/sync-all", `{}`},
		{http.MethodGet, "/api/cloud/capabilities", ``},
	}
	for _, tc := range cases {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(tc.method, tc.path, bytes.NewReader([]byte(tc.body)))
			req.Header.Set("Content-Type", "application/json")
			handler.ServeHTTP(rec, req)
			if rec.Code != http.StatusNotFound && rec.Code != http.StatusMethodNotAllowed {
				t.Errorf("%s %s: got status %d, want 404 or 405 (body: %.200s)",
					tc.method, tc.path, rec.Code, rec.Body.String())
			}
			if n := enrolledCount(t, path, "myproject"); n != 1 {
				t.Errorf("%s %s: sync_enrolled_projects rows for myproject = %d, want 1 (unchanged)",
					tc.method, tc.path, n)
			}
		})
	}
}

// TestCloudReadOnly_KeptRoutes verifies the read-only cloud surface is still
// registered: the sync diagnostics answer 200 and the GET daemon proxy answers
// 503 DAEMON_UNAVAILABLE when the daemon is down (registered, not 404/405).
func TestCloudReadOnly_KeptRoutes(t *testing.T) {
	c, _ := newReadOnlyCloudContainer(t)
	handler := httpapi.NewServeMux(c)

	for _, p := range []string{"/api/sync/projects", "/api/sync/issues", "/api/sync/myproject"} {
		rec := doGet(t, handler, p)
		if rec.Code != http.StatusOK {
			t.Errorf("GET %s: got %d, want 200 (body: %.200s)", p, rec.Code, rec.Body.String())
		}
	}
	for _, p := range []string{"/api/daemon/health", "/api/daemon/sync-status", "/api/daemon/stats"} {
		rec := doGet(t, handler, p)
		if rec.Code != http.StatusServiceUnavailable {
			t.Errorf("GET %s: got %d, want 503 (body: %.200s)", p, rec.Code, rec.Body.String())
		}
	}
}
