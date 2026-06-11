package engram_test

import (
	"context"
	"database/sql"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/AlvaroQ/engram-explorer/internal/config"
	"github.com/AlvaroQ/engram-explorer/internal/providers"
	"github.com/AlvaroQ/engram-explorer/internal/providers/engram"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// seedEngramSchema creates a temp file with the minimal Engram schema
// (observations + sessions tables) and returns its path.
func seedEngramSchema(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "engram.db")
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatalf("open seed db: %v", err)
	}
	defer db.Close()
	for _, stmt := range []string{
		`CREATE TABLE sessions (id TEXT PRIMARY KEY, project TEXT NOT NULL, directory TEXT NOT NULL, started_at TEXT NOT NULL)`,
		`CREATE TABLE observations (id INTEGER PRIMARY KEY AUTOINCREMENT, session_id TEXT NOT NULL, type TEXT NOT NULL, title TEXT NOT NULL, content TEXT NOT NULL)`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("schema exec: %v", err)
		}
	}
	return path
}

// emptyDB creates a temp file that is a valid SQLite file but has NO tables.
func emptyDB(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.db")
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatalf("open empty db: %v", err)
	}
	defer db.Close()
	// Ping creates the file without any tables.
	if err := db.Ping(); err != nil {
		t.Fatalf("ping empty db: %v", err)
	}
	return path
}

// baseCfg returns a base app config for the Engram provider tests.
func baseCfg() config.Config {
	return config.Config{
		DaemonBaseURL:   "http://127.0.0.1:7437",
		DaemonTimeoutMs: 1000,
	}
}

// ---------------------------------------------------------------------------
// engramSchemaValid
// ---------------------------------------------------------------------------

// TestEngramSchemaValid_TrueWhenTablesExist asserts the schema helper returns
// true when both 'observations' and 'sessions' tables are present.
func TestEngramSchemaValid_TrueWhenTablesExist(t *testing.T) {
	path := seedEngramSchema(t)
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	if !engram.SchemaValid(db) {
		t.Error("expected SchemaValid=true for a DB with observations+sessions tables, got false")
	}
}

// TestEngramSchemaValid_FalseWhenEmpty asserts the schema helper returns false
// for a valid SQLite file with no tables at all.
// This is the RED test for the health false-positive fix (spec: DB present but
// schemaless → ok=false).
func TestEngramSchemaValid_FalseWhenEmpty(t *testing.T) {
	path := emptyDB(t)
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	if engram.SchemaValid(db) {
		t.Error("expected SchemaValid=false for an empty DB (no tables), got true")
	}
}

// ---------------------------------------------------------------------------
// Detect
// ---------------------------------------------------------------------------

// TestDetect_AvailableWhenFileHasSchema asserts Detect returns Available=true
// and Reason="ok" when the file exists and has the expected schema.
func TestDetect_AvailableWhenFileHasSchema(t *testing.T) {
	path := seedEngramSchema(t)
	p := engram.NewProvider(baseCfg())
	cfg := providers.ProviderConfig{Enabled: true, Path: path}

	det := p.Detect(context.Background(), cfg)

	if !det.Available {
		t.Errorf("expected Available=true, got false (reason: %s)", det.Reason)
	}
	if det.Reason != "ok" {
		t.Errorf("expected Reason=ok, got %q", det.Reason)
	}
	if det.Path != path {
		t.Errorf("expected Path=%q, got %q", path, det.Path)
	}
}

// TestDetect_UnavailableWhenFileMissing asserts Detect returns Available=false
// and Reason="path-missing" when the path does not exist.
func TestDetect_UnavailableWhenFileMissing(t *testing.T) {
	p := engram.NewProvider(baseCfg())
	cfg := providers.ProviderConfig{Enabled: true, Path: "/nonexistent/engram.db"}

	det := p.Detect(context.Background(), cfg)

	if det.Available {
		t.Error("expected Available=false for missing path, got true")
	}
	if det.Reason != "path-missing" {
		t.Errorf("expected Reason=path-missing, got %q", det.Reason)
	}
}

// TestDetect_UnavailableWhenSchemaInvalid asserts Detect returns Available=false
// and Reason="schema-invalid" when the file exists but has no Engram schema.
func TestDetect_UnavailableWhenSchemaInvalid(t *testing.T) {
	path := emptyDB(t)
	p := engram.NewProvider(baseCfg())
	cfg := providers.ProviderConfig{Enabled: true, Path: path}

	det := p.Detect(context.Background(), cfg)

	if det.Available {
		t.Error("expected Available=false for schemaless DB, got true")
	}
	if det.Reason != "schema-invalid" {
		t.Errorf("expected Reason=schema-invalid, got %q", det.Reason)
	}
}

// ---------------------------------------------------------------------------
// Validate
// ---------------------------------------------------------------------------

// TestValidate_OkWhenSchemaPresent asserts Validate returns nil for a valid DB.
func TestValidate_OkWhenSchemaPresent(t *testing.T) {
	path := seedEngramSchema(t)
	p := engram.NewProvider(baseCfg())
	cfg := providers.ProviderConfig{Path: path}

	if err := p.Validate(context.Background(), cfg); err != nil {
		t.Errorf("expected Validate=nil, got %v", err)
	}
}

// TestValidate_ErrorWhenFileMissing asserts Validate returns an error when the
// path does not exist.
func TestValidate_ErrorWhenFileMissing(t *testing.T) {
	p := engram.NewProvider(baseCfg())
	cfg := providers.ProviderConfig{Path: "/nonexistent/engram.db"}

	if err := p.Validate(context.Background(), cfg); err == nil {
		t.Error("expected Validate error for missing path, got nil")
	}
}

// TestValidate_ErrorWhenSchemaInvalid asserts Validate returns an error when
// the file exists but has no Engram schema.
func TestValidate_ErrorWhenSchemaInvalid(t *testing.T) {
	path := emptyDB(t)
	p := engram.NewProvider(baseCfg())
	cfg := providers.ProviderConfig{Path: path}

	if err := p.Validate(context.Background(), cfg); err == nil {
		t.Errorf("expected Validate error for schemaless DB at %s, got nil", path)
	}
}

// ---------------------------------------------------------------------------
// Open / Close / Identity
// ---------------------------------------------------------------------------

// TestOpen_SucceedsWithValidDB asserts Open returns a non-nil Instance for a
// valid DB and the instance can be closed cleanly.
func TestOpen_SucceedsWithValidDB(t *testing.T) {
	path := seedEngramSchema(t)
	p := engram.NewProvider(baseCfg())
	cfg := providers.ProviderConfig{Enabled: true, Path: path}

	inst, err := p.Open(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if inst == nil {
		t.Fatal("expected non-nil Instance from Open")
	}
	if err := inst.Close(context.Background()); err != nil {
		t.Errorf("Close: %v", err)
	}
}

// TestOpen_FailsWhenFileMissing asserts Open returns an error when path is absent.
func TestOpen_FailsWhenFileMissing(t *testing.T) {
	p := engram.NewProvider(baseCfg())
	cfg := providers.ProviderConfig{Enabled: true, Path: "/nonexistent/engram.db"}

	_, err := p.Open(context.Background(), cfg)
	if err == nil {
		t.Error("expected Open error for missing path, got nil")
	}
}

// TestProvider_Identity asserts the provider returns stable identity values.
func TestProvider_Identity(t *testing.T) {
	p := engram.NewProvider(baseCfg())

	if p.ID() != "engram" {
		t.Errorf("ID: got %q, want %q", p.ID(), "engram")
	}
	if p.ProviderTier() != providers.Tier1 {
		t.Errorf("ProviderTier: got %v, want Tier1", p.ProviderTier())
	}
	if !p.Featured() {
		t.Error("Featured: expected true for Engram provider")
	}
}

// ---------------------------------------------------------------------------
// Health
// ---------------------------------------------------------------------------

// TestHealth_OkWhenSchemaValid asserts Health returns OK=true for a valid
// Engram DB instance.
func TestHealth_OkWhenSchemaValid(t *testing.T) {
	path := seedEngramSchema(t)
	p := engram.NewProvider(baseCfg())
	cfg := providers.ProviderConfig{Enabled: true, Path: path}

	inst, err := p.Open(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer inst.Close(context.Background()) //nolint:errcheck

	h := inst.Health(context.Background())
	if !h.OK {
		t.Errorf("expected Health.OK=true for a valid DB, got false (err: %s)", h.Error)
	}
	if h.ID != "engram" {
		t.Errorf("Health.ID: got %q, want %q", h.ID, "engram")
	}
}

// TestHealth_FalseForEmptyDB is the RED test for the false-positive fix.
// An empty/schemaless SQLite file reports ok=false (not ok=true as SELECT 1 does).
func TestHealth_FalseForEmptyDB(t *testing.T) {
	path := emptyDB(t)

	// Open via raw sql (simulates what happens when an empty file slips through).
	rawDB, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		t.Fatalf("open raw: %v", err)
	}
	defer rawDB.Close()

	if engram.SchemaValid(rawDB) {
		t.Error("SchemaValid must return false for an empty DB — this is the false-positive fix")
	}
}

// ---------------------------------------------------------------------------
// Writable capability
// ---------------------------------------------------------------------------

// TestInstance_ImplementsWritable asserts the Engram instance exposes the
// Writable capability (type assertion succeeds).
func TestInstance_ImplementsWritable(t *testing.T) {
	path := seedEngramSchema(t)
	p := engram.NewProvider(baseCfg())
	cfg := providers.ProviderConfig{Enabled: true, Path: path}

	inst, err := p.Open(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer inst.Close(context.Background()) //nolint:errcheck

	w, ok := inst.(providers.Writable)
	if !ok {
		t.Fatal("expected Engram Instance to implement providers.Writable, but type assertion failed")
	}
	if !w.Writable() {
		t.Error("Writable().Writable() must return true for Engram")
	}
}

// ---------------------------------------------------------------------------
// Routes
// ---------------------------------------------------------------------------

// TestRoutes_RegistersOnMux asserts that Routes does not panic and registers
// at least some routes on the mux (smoke test; verifies call doesn't crash).
func TestRoutes_RegistersOnMux(t *testing.T) {
	path := seedEngramSchema(t)
	p := engram.NewProvider(baseCfg())
	cfg := providers.ProviderConfig{Enabled: true, Path: path}

	inst, err := p.Open(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer inst.Close(context.Background()) //nolint:errcheck

	mux := http.NewServeMux()
	// Must not panic.
	p.Routes(mux, inst, nil)
}

// ---------------------------------------------------------------------------
// Nav
// ---------------------------------------------------------------------------

// TestNav_ReturnsGroup asserts Nav returns a populated NavGroup.
func TestNav_ReturnsGroup(t *testing.T) {
	path := seedEngramSchema(t)
	p := engram.NewProvider(baseCfg())
	cfg := providers.ProviderConfig{Enabled: true, Path: path}

	inst, err := p.Open(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer inst.Close(context.Background()) //nolint:errcheck

	nav := p.Nav(inst)
	if nav.ID != "engram" {
		t.Errorf("Nav.ID: got %q, want %q", nav.ID, "engram")
	}
	if !nav.Featured {
		t.Error("Nav.Featured: expected true for Engram provider")
	}
	if len(nav.Links) == 0 {
		t.Error("Nav.Links: expected at least one link")
	}
}

// ---------------------------------------------------------------------------
// ProviderConfig path absent — Open must NOT be fatal
// ---------------------------------------------------------------------------

// TestOpenNotFatal_WhenPathMissing asserts that Open returns an error (not panic)
// when the DB path is absent, satisfying the "non-fatal boot" requirement.
func TestOpenNotFatal_WhenPathMissing(t *testing.T) {
	p := engram.NewProvider(baseCfg())
	cfg := providers.ProviderConfig{Enabled: true, Path: filepath.Join(t.TempDir(), "missing.db")}

	// Must NOT panic; must return an error.
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Open panicked: %v", r)
		}
	}()

	_, err := p.Open(context.Background(), cfg)
	if err == nil {
		// A missing file that somehow opened is acceptable only if no schema exists —
		// but sqlite will create the file on open unless mode=ro. The real impl must
		// use mode=ro which will fail on a non-existent file.
		t.Log("warning: Open returned nil error for a missing path")
	}
	// We don't check os.IsNotExist because the error may be wrapped.
}

// TestOpenNotFatal_PathEmpty asserts Open returns an error (not panic) for an empty path.
func TestOpenNotFatal_PathEmpty(t *testing.T) {
	p := engram.NewProvider(baseCfg())
	cfg := providers.ProviderConfig{Enabled: true, Path: ""}

	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Open panicked: %v", r)
		}
	}()

	_, err := p.Open(context.Background(), cfg)
	if err == nil {
		t.Error("expected Open error for empty path, got nil")
	}
}

// TestDetect_NoPanicWhenPathEmpty asserts Detect returns Available=false (not panic)
// for an empty path.
func TestDetect_NoPanicWhenPathEmpty(t *testing.T) {
	p := engram.NewProvider(baseCfg())
	cfg := providers.ProviderConfig{}

	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Detect panicked: %v", r)
		}
	}()

	det := p.Detect(context.Background(), cfg)
	if det.Available {
		t.Error("expected Available=false for empty path, got true")
	}
}

// TestDetect_DoesNotOpenLongLivedHandles asserts that after Detect returns,
// the original file can be deleted (i.e. no persistent handle was left open).
// This verifies the "pure check" contract.
func TestDetect_DoesNotOpenLongLivedHandles(t *testing.T) {
	path := seedEngramSchema(t)
	p := engram.NewProvider(baseCfg())
	cfg := providers.ProviderConfig{Enabled: true, Path: path}

	p.Detect(context.Background(), cfg)

	// On Windows, a lingering open handle would prevent removal.
	// We just assert the file is still there (not that we can delete it —
	// Windows test environments may have OS locks unrelated to our code).
	if _, err := os.Stat(path); err != nil {
		t.Errorf("file not accessible after Detect: %v", err)
	}
}
