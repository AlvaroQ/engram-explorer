package httpapi_test

import (
	"database/sql"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/config"
	"github.com/AlvaroQ/engram-explorer/internal/httpapi"
)

const projectExportPath = "/api/projects/myproject/export"

// seedOtherProject adds a second project (and one whose name is a path).
func seedOtherProject(t *testing.T, path string) {
	t.Helper()
	execOnDB(t, path,
		`INSERT INTO sessions (id, project, directory) VALUES ('sess-o', 'other', '/o'), ('sess-p', '/Users/x/repo/foo', '/Users/x/repo/foo')`,
		`INSERT INTO observations (session_id, type, title, content, project) VALUES
			('sess-o', 'note', 'Other obs', 'x', 'other'),
			('sess-p', 'note', 'Path obs', 'x', '/Users/x/repo/foo')`)
}

// snapshotProjects writes body to a file and returns the distinct observation
// projects in it.
func snapshotProjects(t *testing.T, body []byte) []string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "snap.db")
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	rows, err := db.Query(`SELECT DISTINCT project FROM observations ORDER BY project`)
	if err != nil {
		t.Fatalf("query snapshot: %v", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var p string
		_ = rows.Scan(&p)
		out = append(out, p)
	}
	return out
}

var stampRE = `\d{4}-\d{2}-\d{2}T\d{2}-\d{2}-\d{2}`

func TestProjectExport_DownloadsOnlyTheProject(t *testing.T) {
	c := newWriteContainer(t)
	seedOtherProject(t, c.Config.EngramDbPath)
	handler := httpapi.NewServeMux(c)

	cases := []struct{ path, project, file string }{
		{projectExportPath, "myproject", "myproject"},
		{"/api/projects/%2FUsers%2Fx%2Frepo%2Ffoo/export", "/Users/x/repo/foo", "Users-x-repo-foo"},
	}
	for _, tc := range cases {
		rec := doGet(t, handler, tc.path)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: status %d body %s", tc.path, rec.Code, rec.Body.String())
		}
		if ct := rec.Header().Get("Content-Type"); ct != "application/octet-stream" {
			t.Errorf("%s: Content-Type %q", tc.path, ct)
		}
		cd := rec.Header().Get("Content-Disposition")
		if !regexp.MustCompile(`^attachment; filename="engram-backup-` + regexp.QuoteMeta(tc.file) + `-` + stampRE + `\.db"$`).MatchString(cd) {
			t.Errorf("%s: Content-Disposition %q", tc.path, cd)
		}
		if cl := rec.Header().Get("Content-Length"); cl != strconv.Itoa(rec.Body.Len()) {
			t.Errorf("%s: Content-Length %q, body %d bytes", tc.path, cl, rec.Body.Len())
		}
		if got := snapshotProjects(t, rec.Body.Bytes()); len(got) != 1 || got[0] != tc.project {
			t.Errorf("%s: snapshot projects %v, want only %q", tc.path, got, tc.project)
		}
	}
}

func TestProjectExport_UnknownProject404(t *testing.T) {
	handler := httpapi.NewServeMux(newWriteContainer(t))
	for _, method := range []string{http.MethodGet, http.MethodHead} {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(method, "/api/projects/nope/export", nil))
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s: status %d, want 404", method, rec.Code)
		}
		if rec.Header().Get("Content-Disposition") != "" {
			t.Errorf("%s: 404 must not announce a download", method)
		}
	}
}

// Read-only mode answers exactly like GET /api/db/export: 503 RW_UNAVAILABLE,
// before looking the project up.
func TestProjectExport_ReadOnlyMatchesDBExport(t *testing.T) {
	c, _ := newReadOnlyContainer(t)
	handler := httpapi.NewServeMux(c)
	db := doGet(t, handler, "/api/db/export")
	for _, path := range []string{projectExportPath, "/api/projects/nope/export"} {
		rec := doGet(t, handler, path)
		if rec.Code != http.StatusServiceUnavailable || rec.Code != db.Code || rec.Body.String() != db.Body.String() {
			t.Errorf("%s: %d %s; /api/db/export: %d %s", path, rec.Code, rec.Body.String(), db.Code, db.Body.String())
		}
	}

	// Same with a nil RW pool (read-only startup).
	path := seedEngramDB(t)
	cfg := config.Config{EngramDbPath: path, EngramDataDir: filepath.Dir(path), Env: "development", ExposeDetails: true}
	c2, err := httpapi.NewContainer(cfg, slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})))
	if err != nil {
		t.Fatal(err)
	}
	c2.RWDB = nil
	t.Cleanup(c2.Close)
	if rec := doGet(t, httpapi.NewServeMux(c2), projectExportPath); rec.Code != http.StatusServiceUnavailable {
		t.Errorf("nil RW pool: status %d, want 503", rec.Code)
	}
}

// HEAD describes the download without building a snapshot.
func TestProjectExport_HEADDoesNotExport(t *testing.T) {
	handler := httpapi.NewServeMux(newWriteContainer(t))
	ro := t.TempDir()
	if err := os.Chmod(ro, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(ro, 0o700) })
	t.Setenv("TMPDIR", ro)

	if rec := doGet(t, handler, projectExportPath); rec.Code != http.StatusInternalServerError {
		t.Fatalf("GET with unwritable temp dir: got %d, want 500", rec.Code)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodHead, projectExportPath, nil))
	if rec.Code != http.StatusOK || rec.Body.Len() != 0 {
		t.Fatalf("HEAD: status %d, %d body bytes; want 200 and no body", rec.Code, rec.Body.Len())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/octet-stream" {
		t.Errorf("HEAD Content-Type %q", ct)
	}
	if cd := rec.Header().Get("Content-Disposition"); !regexp.MustCompile(`filename="engram-backup-myproject-`).MatchString(cd) {
		t.Errorf("HEAD Content-Disposition %q", cd)
	}
}

// Like GET /api/db/export, a project backup cannot be triggered by another
// site: cross-site and same-site requests are refused; same-origin requests,
// direct navigations (Sec-Fetch-Site: none) and non-browser clients pass.
func TestProjectExport_SameOriginOnly(t *testing.T) {
	handler := httpapi.NewServeMux(newWriteContainer(t))
	for site, want := range map[string]int{
		"cross-site":  http.StatusForbidden,
		"same-site":   http.StatusForbidden,
		"same-origin": http.StatusOK,
		"none":        http.StatusOK,
		"":            http.StatusOK,
	} {
		req := httptest.NewRequest(http.MethodGet, projectExportPath, nil)
		if site != "" {
			req.Header.Set("Sec-Fetch-Site", site)
		}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != want {
			t.Errorf("Sec-Fetch-Site %q: status %d, want %d", site, rec.Code, want)
		}
	}
	// A foreign Origin without Sec-Fetch-Site is refused too.
	req := httptest.NewRequest(http.MethodGet, projectExportPath, nil)
	req.Header.Set("Origin", "http://attacker.example")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("foreign Origin: status %d, want 403", rec.Code)
	}
}
