package httpapi_test

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/AlvaroQ/engram-explorer/internal/config"
	"github.com/AlvaroQ/engram-explorer/internal/httpapi"
	"github.com/AlvaroQ/engram-explorer/internal/providers"
	engramprovider "github.com/AlvaroQ/engram-explorer/internal/providers/engram"
)

// newReadOnlyContainer boots the registry path (as production does) with
// ENGRAM_DASH_READONLY semantics: Config.ReadOnly makes the Engram provider
// open no read-write pool. It returns the container and the database path.
func newReadOnlyContainer(t *testing.T) (*httpapi.Container, string) {
	t.Helper()
	path := seedEngramDB(t)
	cfg := config.Config{
		Host:            "127.0.0.1",
		Port:            8787,
		EngramDbPath:    path,
		DaemonBaseURL:   "http://127.0.0.1:7437",
		DaemonTimeoutMs: 100,
		Env:             "development",
		ExposeDetails:   true,
		ReadOnly:        true,
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	reg := providers.NewRegistry(logger)
	reg.Register(engramprovider.NewProvider(cfg))
	reg.Boot(context.Background(), httpapi.AdaptProfile(config.ProfileFromConfig(cfg)))
	c := httpapi.NewContainerWithRegistry(reg, cfg, logger)
	c.CloseGrace = 0
	t.Cleanup(c.Close)
	return c, path
}

// dbDigest renders every row a write route could change, read through a
// separate connection, so a test can assert the database is unchanged.
func dbDigest(t *testing.T, path string) string {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		t.Fatalf("open digest db: %v", err)
	}
	defer db.Close()
	var b strings.Builder
	for _, q := range []string{
		`SELECT id, session_id, type, title, content, IFNULL(project,''), IFNULL(topic_key,''), IFNULL(deleted_at,'') FROM observations ORDER BY id`,
		`SELECT id, project, directory FROM sessions ORDER BY id`,
		`SELECT id, session_id, content, IFNULL(project,'') FROM user_prompts ORDER BY id`,
	} {
		rows, err := db.Query(q)
		if err != nil {
			t.Fatalf("digest %q: %v", q, err)
		}
		cols, _ := rows.Columns()
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		for rows.Next() {
			if err := rows.Scan(ptrs...); err != nil {
				t.Fatalf("scan: %v", err)
			}
			fmt.Fprintln(&b, vals...)
		}
		rows.Close()
	}
	return b.String()
}

// TestReadOnly_WriteRoutes_Return503 covers the read-only replacement of the
// removed demo mode (`make demo` sets ENGRAM_DASH_READONLY=true): every JSON
// write route answers 503 RW_UNAVAILABLE and leaves the database unchanged,
// even with a well-formed body that would otherwise succeed.
func TestReadOnly_WriteRoutes_Return503(t *testing.T) {
	c, path := newReadOnlyContainer(t)
	handler := httpapi.NewServeMux(c)
	before := dbDigest(t, path)

	cases := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{"PATCH observation", http.MethodPatch, "/api/observations/1", `{"title":"changed"}`},
		{"PATCH observation project", http.MethodPatch, "/api/observations/1/project", `{"project":"other"}`},
		{"PATCH session project", http.MethodPatch, "/api/sessions/sess-1/project", `{"project":"other"}`},
		{"PATCH prompt project", http.MethodPatch, "/api/prompts/1/project", `{"project":"other"}`},
		{"DELETE observation", http.MethodDelete, "/api/observations/1", ""},
		{"DELETE session", http.MethodDelete, "/api/sessions/sess-1", ""},
		{"DELETE prompt", http.MethodDelete, "/api/prompts/1", ""},
		{"POST rename", http.MethodPost, "/api/projects/myproject/rename", `{"target":"renamed"}`},
		{"GET export", http.MethodGet, "/api/db/export", ""},
		{"POST import", http.MethodPost, "/api/db/import", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			if tc.body != "" {
				req.Header.Set("Content-Type", "application/json")
			}
			handler.ServeHTTP(rec, req)
			if rec.Code != http.StatusServiceUnavailable || errorCode(rec) != "RW_UNAVAILABLE" {
				t.Errorf("%s %s: status %d body %q, want 503 RW_UNAVAILABLE",
					tc.method, tc.path, rec.Code, rec.Body.String())
			}
			if after := dbDigest(t, path); after != before {
				t.Errorf("%s %s changed the database:\nbefore %s\nafter  %s", tc.method, tc.path, before, after)
			}
		})
	}
}

// TestReadOnly_ReadRoutes_Unaffected verifies that read endpoints still answer
// 200 in read-only mode.
func TestReadOnly_ReadRoutes_Unaffected(t *testing.T) {
	c, _ := newReadOnlyContainer(t)
	handler := httpapi.NewServeMux(c)

	for _, path := range []string{"/api/observations", "/api/health"} {
		t.Run(path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
			if rec.Code != http.StatusOK {
				t.Errorf("GET %s: got status %d, want 200", path, rec.Code)
			}
		})
	}
}
