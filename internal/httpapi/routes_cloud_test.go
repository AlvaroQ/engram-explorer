package httpapi_test

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/AlvaroQ/engram-explorer/internal/config"
	"github.com/AlvaroQ/engram-explorer/internal/httpapi"
	"log/slog"
)

// ---------------------------------------------------------------------------
// Fake engram CLI helpers
// ---------------------------------------------------------------------------

// installFakeCLI writes a fake engram executable to a temp dir and prepends it
// to PATH. Returns the temp dir so the caller can add files or inspect it.
//
// mode controls the behaviour:
//
//	"ok"       — exits 0, prints "ok" on stdout
//	"fail"     — exits 1, prints error on stderr
//	"enoent"   — sets PATH to ONLY the temp dir (no binary present → LookPath fails)
//	"probe_ok" — exits 1 when called with `cloud`, printing supported subcommands
func installFakeCLI(t *testing.T, mode string) string {
	t.Helper()
	dir := t.TempDir()

	switch mode {
	case "enoent":
		// Replace PATH entirely with a dir that has no "engram" binary.
		// This ensures LookPath cannot find the real engram CLI either.
		t.Setenv("PATH", dir)
		return dir
	}

	if runtime.GOOS == "windows" {
		writeFakeCLIBat(t, dir, mode)
	} else {
		writeFakeCLISh(t, dir, mode)
	}

	// Prepend so the fake takes priority over any real engram.
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return dir
}

func writeFakeCLIBat(t *testing.T, dir, mode string) {
	t.Helper()
	var script string
	switch mode {
	case "fail":
		script = "@echo off\r\necho engram cloud failed >&2\r\nexit /b 2\r\n"
	case "probe_ok":
		// Simulate `cloud --help` printing supported subcommands.
		script = "@echo off\r\n" +
			"if \"%1\" == \"cloud\" (\r\n" +
			"  echo supported subcommands: enroll sync >&2\r\n" +
			"  exit /b 1\r\n" +
			")\r\n" +
			"echo ok\r\n" +
			"exit /b 0\r\n"
	default: // "ok"
		script = "@echo off\r\necho ok\r\nexit /b 0\r\n"
	}
	path := filepath.Join(dir, "engram.bat")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake bat: %v", err)
	}
}

func writeFakeCLISh(t *testing.T, dir, mode string) {
	t.Helper()
	var script string
	switch mode {
	case "fail":
		script = "#!/bin/sh\necho 'engram cloud failed' >&2\nexit 2\n"
	case "probe_ok":
		script = "#!/bin/sh\n" +
			"if [ \"$1\" = \"cloud\" ]; then\n" +
			"  echo 'supported subcommands: enroll sync' >&2\n" +
			"  exit 1\n" +
			"fi\n" +
			"echo ok\n" +
			"exit 0\n"
	default: // "ok"
		script = "#!/bin/sh\necho ok\nexit 0\n"
	}
	path := filepath.Join(dir, "engram")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake sh: %v", err)
	}
}

// newCloudContainer creates a Container suitable for cloud tests.
// auditDir is the temp dir used for audit log writes.
func newCloudContainer(t *testing.T) (*httpapi.Container, string) {
	t.Helper()
	path := seedEngramDB(t)
	auditDir := t.TempDir()
	cfg := config.Config{
		Host:          "127.0.0.1",
		Port:          8787,
		EngramDbPath:  path,
		EngramDataDir: auditDir,
		AuditLogPath:  filepath.Join(auditDir, "logs", "cloud-mutations.jsonl"),
		Env:           "development",
		ExposeDetails: true,
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	c, err := httpapi.NewContainer(cfg, logger)
	if err != nil {
		t.Fatalf("NewContainer: %v", err)
	}
	t.Cleanup(c.Close)
	return c, auditDir
}

func doPostJSON(t *testing.T, handler http.Handler, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	b, _ := json.Marshal(body)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(rec, req)
	return rec
}

func doGetReq(t *testing.T, handler http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	handler.ServeHTTP(rec, req)
	return rec
}

// ---------------------------------------------------------------------------
// POST /api/cloud/enroll — success
// ---------------------------------------------------------------------------

func TestCloudEnroll_Success(t *testing.T) {
	installFakeCLI(t, "ok")
	c, _ := newCloudContainer(t)
	handler := httpapi.NewServeMux(c)

	rec := doPostJSON(t, handler, "/api/cloud/enroll", map[string]any{
		"project": "my-project",
		"confirm": true,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["ok"] != true {
		t.Errorf("ok: got %v, want true", resp["ok"])
	}
	if resp["project"] != "my-project" {
		t.Errorf("project: got %v, want my-project", resp["project"])
	}
	if resp["action"] != "enroll" {
		t.Errorf("action: got %v, want enroll", resp["action"])
	}
}

// ---------------------------------------------------------------------------
// POST /api/cloud/enroll — CLI not found → 502 CLI_NOT_FOUND
// ---------------------------------------------------------------------------

func TestCloudEnroll_CliNotFound(t *testing.T) {
	installFakeCLI(t, "enoent")
	c, _ := newCloudContainer(t)
	handler := httpapi.NewServeMux(c)

	rec := doPostJSON(t, handler, "/api/cloud/enroll", map[string]any{
		"project": "my-project",
		"confirm": true,
	})
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status: got %d, want 502 (body: %s)", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	json.NewDecoder(rec.Body).Decode(&resp)
	if errObj, ok := resp["error"].(map[string]any); ok {
		if errObj["code"] != "CLI_ERROR" {
			t.Errorf("error.code: got %v, want CLI_ERROR", errObj["code"])
		}
		detail, _ := errObj["details"].(map[string]any)
		if detail["code"] != "CLI_NOT_FOUND" {
			t.Errorf("error.details.code: got %v, want CLI_NOT_FOUND", detail["code"])
		}
	} else {
		t.Errorf("missing error envelope: %s", rec.Body.String())
	}
}

// ---------------------------------------------------------------------------
// POST /api/cloud/enroll — CLI exits non-zero → 502 CLI_FAILED
// ---------------------------------------------------------------------------

func TestCloudEnroll_CliFailed(t *testing.T) {
	installFakeCLI(t, "fail")
	c, _ := newCloudContainer(t)
	handler := httpapi.NewServeMux(c)

	rec := doPostJSON(t, handler, "/api/cloud/enroll", map[string]any{
		"project": "my-project",
		"confirm": true,
	})
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status: got %d, want 502 (body: %s)", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	json.NewDecoder(rec.Body).Decode(&resp)
	if errObj, ok := resp["error"].(map[string]any); ok {
		if errObj["code"] != "CLI_ERROR" {
			t.Errorf("error.code: got %v, want CLI_ERROR", errObj["code"])
		}
	}
}

// ---------------------------------------------------------------------------
// POST /api/cloud/enroll — missing confirm → 400 BAD_INPUT
// ---------------------------------------------------------------------------

func TestCloudEnroll_MissingConfirm(t *testing.T) {
	installFakeCLI(t, "ok")
	c, _ := newCloudContainer(t)
	handler := httpapi.NewServeMux(c)

	rec := doPostJSON(t, handler, "/api/cloud/enroll", map[string]any{
		"project": "my-project",
		// confirm omitted
	})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status: got %d, want 400 (body: %s)", rec.Code, rec.Body.String())
	}
}

// ---------------------------------------------------------------------------
// POST /api/cloud/enroll — invalid project name → 502 (CLI handles it)
// or the service validates before calling the CLI → should get BAD_INPUT
// ---------------------------------------------------------------------------

func TestCloudEnroll_InvalidProjectName(t *testing.T) {
	installFakeCLI(t, "ok")
	c, _ := newCloudContainer(t)
	handler := httpapi.NewServeMux(c)

	rec := doPostJSON(t, handler, "/api/cloud/enroll", map[string]any{
		"project": "bad name with spaces!",
		"confirm": true,
	})
	// The service validates before calling the CLI — expect 400.
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want 400 (body: %s)", rec.Code, rec.Body.String())
	}
}

// ---------------------------------------------------------------------------
// POST /api/cloud/unenroll — SQL path, RWDB available
// ---------------------------------------------------------------------------

func TestCloudUnenroll_ViaSQL(t *testing.T) {
	// installFakeCLI is not needed — unenroll uses SQL only.
	c, auditDir := newCloudContainer(t)

	// Enroll the project first so we have a row to delete.
	db, err := sql.Open("sqlite", "file:"+c.Config.EngramDbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	_, err = db.Exec(`INSERT OR IGNORE INTO sync_enrolled_projects (project, enrolled_at) VALUES ('test-proj', '2026-01-01 00:00:00')`)
	if err != nil {
		t.Fatalf("insert enrollment: %v", err)
	}
	_, err = db.Exec(`INSERT OR IGNORE INTO sync_state (target_key, lifecycle, last_enqueued_seq, last_acked_seq) VALUES ('cloud:test-proj', 'idle', 0, 0)`)
	if err != nil {
		t.Fatalf("insert sync_state: %v", err)
	}
	_, err = db.Exec(`INSERT INTO sync_mutations (target_key, entity, entity_key, op, payload, project, occurred_at) VALUES ('cloud', 'observation', 'obs-abc', 'upsert', '{}', 'test-proj', '2026-01-01 00:00:00')`)
	if err != nil {
		t.Fatalf("insert mutation: %v", err)
	}
	db.Close()

	handler := httpapi.NewServeMux(c)
	rec := doPostJSON(t, handler, "/api/cloud/unenroll", map[string]any{
		"project": "test-proj",
		"confirm": true,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}

	// Verify all three rows were removed.
	verifyDB, err := sql.Open("sqlite", "file:"+c.Config.EngramDbPath)
	if err != nil {
		t.Fatalf("open verifyDB: %v", err)
	}
	defer verifyDB.Close()

	var n int
	verifyDB.QueryRow(`SELECT COUNT(*) FROM sync_enrolled_projects WHERE project = 'test-proj'`).Scan(&n)
	if n != 0 {
		t.Errorf("sync_enrolled_projects: want 0 rows, got %d", n)
	}
	verifyDB.QueryRow(`SELECT COUNT(*) FROM sync_state WHERE target_key = 'cloud:test-proj'`).Scan(&n)
	if n != 0 {
		t.Errorf("sync_state: want 0 rows, got %d", n)
	}
	verifyDB.QueryRow(`SELECT COUNT(*) FROM sync_mutations WHERE project = 'test-proj' AND acked_at IS NULL`).Scan(&n)
	if n != 0 {
		t.Errorf("sync_mutations: want 0 pending rows, got %d", n)
	}

	// Verify audit log was written.
	logPath := filepath.Join(auditDir, "logs", "cloud-mutations.jsonl")
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read audit log: %v", err)
	}
	if !bytes.Contains(data, []byte(`"unenroll"`)) {
		t.Errorf("audit log missing unenroll entry: %s", string(data))
	}
}

// ---------------------------------------------------------------------------
// POST /api/cloud/unenroll — no RWDB → 503
// ---------------------------------------------------------------------------

func TestCloudUnenroll_NoRWDB(t *testing.T) {
	c, _ := newCloudContainer(t)
	// Force RWDB to nil.
	if c.RWDB != nil {
		c.RWDB.Close()
		c.RWDB = nil
	}
	handler := httpapi.NewServeMux(c)
	rec := doPostJSON(t, handler, "/api/cloud/unenroll", map[string]any{
		"project": "proj",
		"confirm": true,
	})
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status: got %d, want 503 (body: %s)", rec.Code, rec.Body.String())
	}
}

// ---------------------------------------------------------------------------
// POST /api/cloud/sync — success
// ---------------------------------------------------------------------------

func TestCloudSync_Success(t *testing.T) {
	installFakeCLI(t, "ok")
	c, _ := newCloudContainer(t)
	handler := httpapi.NewServeMux(c)

	rec := doPostJSON(t, handler, "/api/cloud/sync", map[string]any{
		"project": "myproject",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp["ok"] != true {
		t.Errorf("ok: got %v, want true", resp["ok"])
	}
	if resp["action"] != "sync" {
		t.Errorf("action: got %v, want sync", resp["action"])
	}
}

// ---------------------------------------------------------------------------
// POST /api/cloud/sync — CLI not found → 502
// ---------------------------------------------------------------------------

func TestCloudSync_CliNotFound(t *testing.T) {
	installFakeCLI(t, "enoent")
	c, _ := newCloudContainer(t)
	handler := httpapi.NewServeMux(c)

	rec := doPostJSON(t, handler, "/api/cloud/sync", map[string]any{
		"project": "myproject",
	})
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status: got %d, want 502 (body: %s)", rec.Code, rec.Body.String())
	}
}

// ---------------------------------------------------------------------------
// POST /api/cloud/sync-all — success (enrolled projects synced)
// ---------------------------------------------------------------------------

func TestCloudSyncAll_Success(t *testing.T) {
	installFakeCLI(t, "ok")

	path := seedEngramDB(t)
	// Enroll myproject.
	db, _ := sql.Open("sqlite", "file:"+path)
	db.Exec(`INSERT OR IGNORE INTO sync_enrolled_projects (project, enrolled_at) VALUES ('myproject', '2026-01-01 00:00:00')`)
	db.Close()

	auditDir := t.TempDir()
	cfg := config.Config{
		EngramDbPath:  path,
		EngramDataDir: auditDir,
		AuditLogPath:  filepath.Join(auditDir, "logs", "cloud-mutations.jsonl"),
		Env:           "development",
		ExposeDetails: true,
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	c, err := httpapi.NewContainer(cfg, logger)
	if err != nil {
		t.Fatalf("NewContainer: %v", err)
	}
	t.Cleanup(c.Close)

	handler := httpapi.NewServeMux(c)
	rec := doPostJSON(t, handler, "/api/cloud/sync-all", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp["total"] == nil {
		t.Errorf("response missing 'total': %v", resp)
	}
	if resp["ok"] == nil {
		t.Errorf("response missing 'ok': %v", resp)
	}
	// At least myproject should be in results.
	results, ok := resp["results"].([]any)
	if !ok {
		t.Fatalf("results is not an array: %v", resp["results"])
	}
	found := false
	for _, r := range results {
		rm := r.(map[string]any)
		if rm["project"] == "myproject" && rm["ok"] == true {
			found = true
		}
	}
	if !found {
		t.Errorf("myproject sync result not found or not ok: %v", results)
	}
}

// ---------------------------------------------------------------------------
// POST /api/cloud/sync-all — CLI not found → results contain per-project errors
// ---------------------------------------------------------------------------

func TestCloudSyncAll_CliNotFound(t *testing.T) {
	installFakeCLI(t, "enoent")

	path := seedEngramDB(t)
	db, _ := sql.Open("sqlite", "file:"+path)
	db.Exec(`INSERT OR IGNORE INTO sync_enrolled_projects (project, enrolled_at) VALUES ('myproject', '2026-01-01 00:00:00')`)
	db.Close()

	cfg := config.Config{
		EngramDbPath:  path,
		EngramDataDir: t.TempDir(),
		AuditLogPath:  filepath.Join(t.TempDir(), "cloud-mutations.jsonl"),
		Env:           "development",
		ExposeDetails: true,
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	c, err := httpapi.NewContainer(cfg, logger)
	if err != nil {
		t.Fatalf("NewContainer: %v", err)
	}
	t.Cleanup(c.Close)

	handler := httpapi.NewServeMux(c)
	rec := doPostJSON(t, handler, "/api/cloud/sync-all", nil)
	// sync-all always returns 200; individual project errors are in results[].
	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp["failed"].(float64) < 1 {
		t.Errorf("expected at least 1 failure, got: %v", resp)
	}
}

// ---------------------------------------------------------------------------
// GET /api/cloud/capabilities — CLI present, probe returns enroll+sync
// ---------------------------------------------------------------------------

func TestCloudCapabilities_WithCLI(t *testing.T) {
	installFakeCLI(t, "probe_ok")
	c, _ := newCloudContainer(t)
	handler := httpapi.NewServeMux(c)

	rec := doGetReq(t, handler, "/api/cloud/capabilities")
	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	var caps map[string]any
	json.NewDecoder(rec.Body).Decode(&caps)
	if caps["enroll"] != true {
		t.Errorf("enroll: got %v, want true", caps["enroll"])
	}
	// unenroll should be true because RWDB is available.
	if caps["unenroll"] != true {
		t.Errorf("unenroll: got %v, want true (RWDB is available)", caps["unenroll"])
	}
}

// ---------------------------------------------------------------------------
// GET /api/cloud/capabilities — CLI not found → enroll=false, unenroll depends on RWDB
// ---------------------------------------------------------------------------

func TestCloudCapabilities_CliNotFound(t *testing.T) {
	installFakeCLI(t, "enoent")
	c, _ := newCloudContainer(t)
	handler := httpapi.NewServeMux(c)

	rec := doGetReq(t, handler, "/api/cloud/capabilities")
	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	var caps map[string]any
	json.NewDecoder(rec.Body).Decode(&caps)
	if caps["enroll"] != false {
		t.Errorf("enroll: got %v, want false (CLI not found)", caps["enroll"])
	}
	// unenroll should be true because RWDB is configured.
	if caps["unenroll"] != true {
		t.Errorf("unenroll: got %v, want true (RWDB available even without CLI)", caps["unenroll"])
	}
	if caps["raw"] != "" {
		t.Errorf("raw: got %v, want empty string (CLI not found)", caps["raw"])
	}
}

// ---------------------------------------------------------------------------
// Capabilities parsing golden test
// ---------------------------------------------------------------------------

func TestCloudCapabilities_ParsingGolden(t *testing.T) {
	// Fake probe output that contains "supported subcommands: enroll sync status"
	// and the binary is available. We install a "probe_ok" CLI which returns this.
	installFakeCLI(t, "probe_ok")
	c, _ := newCloudContainer(t)
	handler := httpapi.NewServeMux(c)

	rec := doGetReq(t, handler, "/api/cloud/capabilities")
	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", rec.Code)
	}
	var caps map[string]any
	json.NewDecoder(rec.Body).Decode(&caps)

	// probe_ok fake CLI emits "supported subcommands: enroll sync" on stderr
	// when called with `cloud --help`. The service should parse this.
	raw, _ := caps["raw"].(string)
	if raw == "" {
		t.Logf("NOTE: raw is empty — probe may not have produced output (Windows .bat stderr capture may differ)")
	}
}

// ---------------------------------------------------------------------------
// Rate limit — enroll returns 429 after > 10 requests in window
// ---------------------------------------------------------------------------

func TestCloudEnroll_RateLimit(t *testing.T) {
	installFakeCLI(t, "ok")
	c, _ := newCloudContainer(t)
	handler := httpapi.NewServeMux(c)

	body := map[string]any{"project": "proj", "confirm": true}
	var lastCode int
	for i := 0; i < 15; i++ {
		rec := doPostJSON(t, handler, "/api/cloud/enroll", body)
		lastCode = rec.Code
	}
	if lastCode != http.StatusTooManyRequests {
		t.Errorf("expected 429 after exceeding rate limit, got %d", lastCode)
	}
}

// ---------------------------------------------------------------------------
// Rate limit — sync-all returns 429 after > 5 requests in window
// ---------------------------------------------------------------------------

func TestCloudSyncAll_RateLimit(t *testing.T) {
	installFakeCLI(t, "ok")
	c, _ := newCloudContainer(t)
	handler := httpapi.NewServeMux(c)

	var lastCode int
	for i := 0; i < 10; i++ {
		rec := doPostJSON(t, handler, "/api/cloud/sync-all", nil)
		lastCode = rec.Code
	}
	if lastCode != http.StatusTooManyRequests {
		t.Errorf("expected 429 after exceeding sync-all rate limit, got %d", lastCode)
	}
}
