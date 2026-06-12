package httpapi_test

import (
	"context"
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
	"github.com/AlvaroQ/engram-explorer/internal/providers"
	engramprovider "github.com/AlvaroQ/engram-explorer/internal/providers/engram"
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

// ---------------------------------------------------------------------------
// WU-6: Per-provider health aggregation + false-positive fix (registry path)
// ---------------------------------------------------------------------------

// emptyEngramPath creates a temp SQLite file with NO tables and returns its
// path. This simulates the production false-positive: a valid SQLite file that
// is not a real Engram DB.
func emptyEngramPath(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.db")
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatalf("open empty db: %v", err)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		t.Fatalf("ping empty db: %v", err)
	}
	return path
}

// newRegistryContainerOnPath boots a registry-backed Container with the Engram
// provider configured to use the given DB path.
func newRegistryContainerOnPath(t *testing.T, dbPath string) *httpapi.Container {
	t.Helper()
	cfg := config.Config{
		Host:            "127.0.0.1",
		Port:            8787,
		EngramDbPath:    dbPath,
		DaemonBaseURL:   "http://127.0.0.1:7437",
		DaemonTimeoutMs: 100,
		Env:             "development",
		ExposeDetails:   true,
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	reg := providers.NewRegistry(logger)
	reg.Register(engramprovider.NewProvider(cfg))
	reg.Boot(context.Background(), httpapi.AdaptProfile(config.ProfileFromConfig(cfg)))
	c := httpapi.NewContainerWithRegistry(reg, cfg, logger)
	t.Cleanup(c.Close)
	return c
}

// parseHealth decodes the /api/health JSON response body.
func parseHealth(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("parse health JSON: %v\nbody: %s", err, body)
	}
	return result
}

// findProviderEntry returns the providers[] entry for the given id, or fails.
func findProviderEntry(t *testing.T, result map[string]any, id string) map[string]any {
	t.Helper()
	provs, ok := result["providers"].([]any)
	if !ok {
		t.Fatalf("health.providers: not an array; got %T", result["providers"])
	}
	for _, raw := range provs {
		entry, ok := raw.(map[string]any)
		if ok && entry["id"] == id {
			return entry
		}
	}
	t.Fatalf("providers array has no entry with id=%q; entries: %v", id, provs)
	return nil
}

// TestHealth_EmptyEngramDB_ReportsNotOk is the RED test for WU-6.
//
// Spec: health-reporting / Schema-Validity Check — "DB present but schemaless":
//   - GIVEN the Engram DB file exists and is openable but has no tables
//   - WHEN GET /api/health is called
//   - THEN the Engram provider reports ok=false
//   - AND the top-level ok is false
//
// This is the production false-positive: bare "SELECT 1" always returns true
// for an empty SQLite file. The fix uses sqlite_master table-presence check.
func TestHealth_EmptyEngramDB_ReportsNotOk(t *testing.T) {
	// An empty DB file passes the SQLite driver's open check but has no tables.
	// Detect() uses SchemaValid which checks sqlite_master — it returns false
	// (schema-invalid), so the provider is in Detected (not Enabled) state.
	// AggregateHealth() then reports it as active=false with ok=false.
	c := newRegistryContainerOnPath(t, emptyEngramPath(t))
	handler := httpapi.NewServeMux(c)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/health: got %d, want 200 (always 200 per design)", rec.Code)
	}

	result := parseHealth(t, rec.Body.Bytes())

	// Top-level ok: vacuously true when zero providers are Enabled.
	// The false-positive fix is expressed at the individual provider level.
	if _, ok := result["providers"]; !ok {
		t.Fatal("health response missing 'providers' array")
	}

	engramEntry := findProviderEntry(t, result, "engram")

	// The engram entry must report ok=false for the schemaless DB.
	if ok, _ := engramEntry["ok"].(bool); ok {
		t.Errorf("providers[engram].ok: got true for an empty/schemaless DB, want false — this is the false-positive fix")
	}

	// The legacy db{} alias must also reflect ok=false when engram is not ok.
	dbAlias, _ := result["db"].(map[string]any)
	if dbAlias == nil {
		t.Fatal("health response missing legacy 'db' alias")
	}
	if ok, _ := dbAlias["ok"].(bool); ok {
		t.Errorf("db.ok: got true for empty/schemaless DB, want false — db alias must mirror engram provider")
	}
}

// TestHealth_ValidEngramDB_ReportsOk verifies the positive case through the
// registry path: a properly seeded engram DB reports ok=true.
func TestHealth_ValidEngramDB_ReportsOk(t *testing.T) {
	// newTestRegistry boots the registry with a fully seeded DB.
	c := newTestRegistry(t)
	handler := httpapi.NewServeMux(c)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/health: got %d, want 200", rec.Code)
	}

	result := parseHealth(t, rec.Body.Bytes())

	topOk, _ := result["ok"].(bool)
	if !topOk {
		t.Errorf("health.ok: got false for a fully seeded DB, want true")
	}

	engramEntry := findProviderEntry(t, result, "engram")
	if ok, _ := engramEntry["ok"].(bool); !ok {
		t.Errorf("providers[engram].ok: got false for a valid seeded DB, want true")
	}

	// The legacy db{} alias must mirror the engram provider entry.
	dbAlias, _ := result["db"].(map[string]any)
	if dbAlias == nil {
		t.Fatal("health response missing legacy 'db' alias")
	}
	if ok, _ := dbAlias["ok"].(bool); !ok {
		t.Errorf("db.ok: got false for a valid seeded DB, want true (alias must mirror engram)")
	}
}

// TestHealth_MissingEngramDB_ProvidersArrayPresent verifies that a missing
// engram DB still produces a providers[] array with the engram entry showing
// ok=false (not a crash or missing entry).
func TestHealth_MissingEngramDB_ProvidersArrayPresent(t *testing.T) {
	missingPath := filepath.Join(t.TempDir(), "does-not-exist.db")
	c := newRegistryContainerOnPath(t, missingPath)
	handler := httpapi.NewServeMux(c)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/health: got %d, want 200", rec.Code)
	}

	result := parseHealth(t, rec.Body.Bytes())

	if _, ok := result["providers"]; !ok {
		t.Fatal("health response must include 'providers' array even with a missing engram DB")
	}

	engramEntry := findProviderEntry(t, result, "engram")
	if ok, _ := engramEntry["ok"].(bool); ok {
		t.Errorf("providers[engram].ok: got true for missing DB path, want false")
	}
}

// TestHealth_RegistryPath_ShapeComplete verifies the full response shape when
// using the registry-backed path: ok, uptime_s, runtime, daemon, db, providers.
func TestHealth_RegistryPath_ShapeComplete(t *testing.T) {
	reg := providers.NewRegistry(slog.Default())
	c := newRegistryContainer(t, reg)
	handler := httpapi.NewServeMux(c)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/health: got %d, want 200", rec.Code)
	}

	result := parseHealth(t, rec.Body.Bytes())

	for _, field := range []string{"ok", "uptime_s", "runtime", "daemon", "db", "providers"} {
		if _, exists := result[field]; !exists {
			t.Errorf("health response missing required field %q", field)
		}
	}

	// providers must be a (possibly empty) array, not null.
	if provs, ok := result["providers"].([]any); !ok {
		t.Errorf("health.providers: expected array, got %T", result["providers"])
	} else if provs == nil {
		t.Error("health.providers: got nil, want empty array")
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
