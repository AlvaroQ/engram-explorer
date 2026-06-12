package httpapi_test

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/config"
	"github.com/AlvaroQ/engram-explorer/internal/httpapi"
)

// newDemoContainer creates a Container with DemoMode enabled. The DB path
// still points at a seeded temp file (needed so the legacy NewContainer path
// can open pools), but every write route should return 403 regardless of pool
// availability.
func newDemoContainer(t *testing.T) *httpapi.Container {
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
		DemoMode:        true,
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	c, err := httpapi.NewContainer(cfg, logger)
	if err != nil {
		t.Fatalf("NewContainer: %v", err)
	}
	t.Cleanup(c.Close)
	return c
}

// TestDemoMode_WriteRoutes_Return403 verifies that all write endpoints return
// 403 when DemoMode is active, regardless of whether RWDB is available.
func TestDemoMode_WriteRoutes_Return403(t *testing.T) {
	handler := httpapi.NewServeMux(newDemoContainer(t))

	cases := []struct {
		name   string
		method string
		path   string
	}{
		{"PATCH observation", http.MethodPatch, "/api/observations/1"},
		{"PATCH observation project", http.MethodPatch, "/api/observations/1/project"},
		{"PATCH session project", http.MethodPatch, "/api/sessions/sess-1/project"},
		{"PATCH prompt project", http.MethodPatch, "/api/prompts/1/project"},
		{"DELETE observation", http.MethodDelete, "/api/observations/1"},
		{"DELETE session", http.MethodDelete, "/api/sessions/sess-1"},
		{"DELETE prompt", http.MethodDelete, "/api/prompts/1"},
		{"POST rename", http.MethodPost, "/api/projects/myproject/rename"},
		{"GET export", http.MethodGet, "/api/db/export"},
		{"POST import", http.MethodPost, "/api/db/import"},
		{"POST cloud enroll", http.MethodPost, "/api/cloud/enroll"},
		{"POST cloud unenroll", http.MethodPost, "/api/cloud/unenroll"},
		{"POST cloud sync", http.MethodPost, "/api/cloud/sync"},
		{"POST cloud sync-all", http.MethodPost, "/api/cloud/sync-all"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(tc.method, tc.path, nil)
			handler.ServeHTTP(rec, req)
			if rec.Code != http.StatusForbidden {
				t.Errorf("%s %s: got status %d, want 403", tc.method, tc.path, rec.Code)
			}
		})
	}
}

// TestDemoMode_ReadRoutes_Unaffected verifies that read endpoints still return
// 200 in demo mode (demo mode only blocks writes).
func TestDemoMode_ReadRoutes_Unaffected(t *testing.T) {
	handler := httpapi.NewServeMux(newDemoContainer(t))

	readCases := []string{
		"/api/observations",
		"/api/health",
	}
	for _, path := range readCases {
		t.Run(path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, path, nil)
			handler.ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Errorf("GET %s: got status %d, want 200", path, rec.Code)
			}
		})
	}
}
