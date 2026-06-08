package httpapi_test

import (
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/config"
	"github.com/AlvaroQ/engram-explorer/internal/httpapi"
	_ "modernc.org/sqlite"
)

// seedDB creates a minimal SQLite database file and returns its path.
func seedDB(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "engram.db")
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatalf("seed open: %v", err)
	}
	if _, err := db.Exec("CREATE TABLE t (id INTEGER PRIMARY KEY)"); err != nil {
		db.Close()
		t.Fatalf("seed table: %v", err)
	}
	db.Close()
	return path
}

func newTestContainer(t *testing.T) *httpapi.Container {
	t.Helper()
	path := seedDB(t)
	cfg := config.Config{
		Host:          "127.0.0.1",
		Port:          8787,
		EngramDbPath:  path,
		DaemonBaseURL: "http://127.0.0.1:7437",
		Env:           "development",
		ExposeDetails: true,
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	c, err := httpapi.NewContainer(cfg, logger)
	if err != nil {
		t.Fatalf("NewContainer: %v", err)
	}
	t.Cleanup(c.Close)
	return c
}

func TestHealthEndpoint_Status200(t *testing.T) {
	c := newTestContainer(t)
	handler := httpapi.NewServeMux(c)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status: got %d, want 200", rec.Code)
	}
}

func TestHealthEndpoint_ResponseShape(t *testing.T) {
	c := newTestContainer(t)
	handler := httpapi.NewServeMux(c)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	handler.ServeHTTP(rec, req)

	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}

	// Top-level "ok" key
	if _, ok := body["ok"]; !ok {
		t.Error("missing top-level 'ok'")
	}

	// "db" object with "ok" and "path"
	dbRaw, ok := body["db"]
	if !ok {
		t.Fatal("missing 'db' key")
	}
	dbObj, ok := dbRaw.(map[string]any)
	if !ok {
		t.Fatalf("'db' is not an object: %T", dbRaw)
	}
	if _, ok := dbObj["ok"]; !ok {
		t.Error("db missing 'ok'")
	}
	if _, ok := dbObj["path"]; !ok {
		t.Error("db missing 'path'")
	}

	// "daemon" object with "ok" and "url"
	daemonRaw, ok := body["daemon"]
	if !ok {
		t.Fatal("missing 'daemon' key")
	}
	daemonObj, ok := daemonRaw.(map[string]any)
	if !ok {
		t.Fatalf("'daemon' is not an object: %T", daemonRaw)
	}
	if _, ok := daemonObj["ok"]; !ok {
		t.Error("daemon missing 'ok'")
	}
	if _, ok := daemonObj["url"]; !ok {
		t.Error("daemon missing 'url'")
	}

	// "uptime_s" key
	if _, ok := body["uptime_s"]; !ok {
		t.Error("missing 'uptime_s'")
	}
}

func TestHealthEndpoint_DbOkTrue(t *testing.T) {
	c := newTestContainer(t)
	handler := httpapi.NewServeMux(c)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	handler.ServeHTTP(rec, req)

	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}

	// With a valid DB, dbOk should be true.
	dbObj := body["db"].(map[string]any)
	if dbOk, _ := dbObj["ok"].(bool); !dbOk {
		t.Error("db.ok should be true for a valid database")
	}
	if topOk, _ := body["ok"].(bool); !topOk {
		t.Error("top-level ok should be true when db is reachable")
	}
}

func TestHealthEndpoint_ContentTypeJSON(t *testing.T) {
	c := newTestContainer(t)
	handler := httpapi.NewServeMux(c)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	handler.ServeHTTP(rec, req)

	ct := rec.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type: got %q, want application/json", ct)
	}
}

func TestSecurityHeaders(t *testing.T) {
	c := newTestContainer(t)
	handler := httpapi.NewServeMux(c)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	handler.ServeHTTP(rec, req)

	cases := []struct{ header, want string }{
		{"X-Frame-Options", "DENY"},
		{"X-Content-Type-Options", "nosniff"},
		{"Referrer-Policy", "no-referrer"},
		{"Cross-Origin-Opener-Policy", "same-origin"},
		{"Cross-Origin-Resource-Policy", "same-origin"},
	}
	for _, tc := range cases {
		got := rec.Header().Get(tc.header)
		if got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.header, got, tc.want)
		}
	}
}
