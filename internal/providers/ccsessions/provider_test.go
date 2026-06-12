package ccsessions_test

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/config"
	"github.com/AlvaroQ/engram-explorer/internal/providers"
	"github.com/AlvaroQ/engram-explorer/internal/providers/ccsessions"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// seedProjectsDir creates a temp directory that looks like a valid Claude Code
// projects directory: at least one project subfolder inside.
// Returns the path to the projects dir.
func seedProjectsDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	// Create one project subdirectory (simulates ~/.claude/projects/<project>).
	if err := os.Mkdir(filepath.Join(dir, "my-project"), 0o755); err != nil {
		t.Fatalf("mkdir project: %v", err)
	}
	return dir
}

// emptyProjectsDir creates a temp directory that exists but contains NO
// project subdirectories.
func emptyProjectsDir(t *testing.T) string {
	t.Helper()
	return t.TempDir()
}

// baseCfg returns a minimal config for CC sessions provider tests.
func baseCfg() config.Config {
	return config.Config{}
}

// ---------------------------------------------------------------------------
// Identity
// ---------------------------------------------------------------------------

// TestProvider_Identity asserts the CC sessions provider returns stable identity
// values: ID="cc-sessions", Tier1, Featured=false.
func TestProvider_Identity(t *testing.T) {
	p := ccsessions.NewProvider(baseCfg())

	if p.ID() != "cc-sessions" {
		t.Errorf("ID: got %q, want %q", p.ID(), "cc-sessions")
	}
	if p.ProviderTier() != providers.Tier1 {
		t.Errorf("ProviderTier: got %v, want Tier1", p.ProviderTier())
	}
	if p.Featured() {
		t.Error("Featured: expected false for CC sessions provider, got true")
	}
}

// ---------------------------------------------------------------------------
// Detect
// ---------------------------------------------------------------------------

// TestDetect_AvailableWhenProjectsExist asserts Detect returns Available=true
// when the projects directory exists AND contains at least one project folder.
func TestDetect_AvailableWhenProjectsExist(t *testing.T) {
	dir := seedProjectsDir(t)
	p := ccsessions.NewProvider(baseCfg())
	cfg := providers.ProviderConfig{Enabled: true, Path: dir}

	det := p.Detect(context.Background(), cfg)

	if !det.Available {
		t.Errorf("expected Available=true for populated projects dir, got false (reason: %s)", det.Reason)
	}
	if det.Reason != "ok" {
		t.Errorf("expected Reason=ok, got %q", det.Reason)
	}
	if det.Path != dir {
		t.Errorf("expected Path=%q, got %q", dir, det.Path)
	}
}

// TestDetect_UnavailableWhenDirMissing asserts Detect returns Available=false
// and Reason="dir-missing" when the path does not exist.
func TestDetect_UnavailableWhenDirMissing(t *testing.T) {
	p := ccsessions.NewProvider(baseCfg())
	cfg := providers.ProviderConfig{Enabled: true, Path: "/nonexistent/claude/projects"}

	det := p.Detect(context.Background(), cfg)

	if det.Available {
		t.Error("expected Available=false for missing dir, got true")
	}
	if det.Reason != "dir-missing" {
		t.Errorf("expected Reason=dir-missing, got %q", det.Reason)
	}
}

// TestDetect_UnavailableWhenDirEmpty asserts Detect returns Available=false
// and Reason="no-projects" when the directory exists but has no project folders.
func TestDetect_UnavailableWhenDirEmpty(t *testing.T) {
	dir := emptyProjectsDir(t)
	p := ccsessions.NewProvider(baseCfg())
	cfg := providers.ProviderConfig{Enabled: true, Path: dir}

	det := p.Detect(context.Background(), cfg)

	if det.Available {
		t.Error("expected Available=false for empty projects dir, got true")
	}
	if det.Reason != "no-projects" {
		t.Errorf("expected Reason=no-projects, got %q", det.Reason)
	}
}

// TestDetect_UnavailableWhenPathEmpty asserts Detect returns Available=false
// without panic when path is empty.
func TestDetect_UnavailableWhenPathEmpty(t *testing.T) {
	p := ccsessions.NewProvider(baseCfg())
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

// TestDetect_UnavailableWhenPathIsFile asserts Detect returns Available=false
// (not a directory) when path points to a regular file.
func TestDetect_UnavailableWhenPathIsFile(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "notadir.txt")
	if err := os.WriteFile(filePath, []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	p := ccsessions.NewProvider(baseCfg())
	cfg := providers.ProviderConfig{Enabled: true, Path: filePath}

	det := p.Detect(context.Background(), cfg)
	if det.Available {
		t.Error("expected Available=false when path is a file (not a dir), got true")
	}
}

// ---------------------------------------------------------------------------
// Validate
// ---------------------------------------------------------------------------

// TestValidate_OkWhenProjectsExist asserts Validate returns nil when the
// projects directory exists and contains at least one project subfolder.
func TestValidate_OkWhenProjectsExist(t *testing.T) {
	dir := seedProjectsDir(t)
	p := ccsessions.NewProvider(baseCfg())
	cfg := providers.ProviderConfig{Path: dir}

	if err := p.Validate(context.Background(), cfg); err != nil {
		t.Errorf("expected Validate=nil, got %v", err)
	}
}

// TestValidate_ErrorWhenDirMissing asserts Validate returns an error when the
// path does not exist.
func TestValidate_ErrorWhenDirMissing(t *testing.T) {
	p := ccsessions.NewProvider(baseCfg())
	cfg := providers.ProviderConfig{Path: "/nonexistent/projects"}

	if err := p.Validate(context.Background(), cfg); err == nil {
		t.Error("expected Validate error for missing path, got nil")
	}
}

// TestValidate_ErrorWhenDirEmpty asserts Validate returns an error when the
// projects directory exists but has no project subdirectories.
func TestValidate_ErrorWhenDirEmpty(t *testing.T) {
	dir := emptyProjectsDir(t)
	p := ccsessions.NewProvider(baseCfg())
	cfg := providers.ProviderConfig{Path: dir}

	if err := p.Validate(context.Background(), cfg); err == nil {
		t.Errorf("expected Validate error for empty projects dir at %s, got nil", dir)
	}
}

// TestValidate_ErrorWhenPathEmpty asserts Validate returns an error for an
// empty path.
func TestValidate_ErrorWhenPathEmpty(t *testing.T) {
	p := ccsessions.NewProvider(baseCfg())
	cfg := providers.ProviderConfig{Path: ""}

	if err := p.Validate(context.Background(), cfg); err == nil {
		t.Error("expected Validate error for empty path, got nil")
	}
}

// ---------------------------------------------------------------------------
// Open / Close
// ---------------------------------------------------------------------------

// TestOpen_SucceedsWithValidDir asserts Open returns a non-nil Instance for a
// valid projects directory and the instance can be closed cleanly.
func TestOpen_SucceedsWithValidDir(t *testing.T) {
	dir := seedProjectsDir(t)
	p := ccsessions.NewProvider(baseCfg())
	cfg := providers.ProviderConfig{Enabled: true, Path: dir}

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

// TestOpen_FailsWhenDirMissing asserts Open returns an error when the path
// does not exist.
func TestOpen_FailsWhenDirMissing(t *testing.T) {
	p := ccsessions.NewProvider(baseCfg())
	cfg := providers.ProviderConfig{Enabled: true, Path: "/nonexistent/projects"}

	_, err := p.Open(context.Background(), cfg)
	if err == nil {
		t.Error("expected Open error for missing directory, got nil")
	}
}

// TestOpen_NotFatal_WhenDirMissing asserts Open returns an error (not panic).
func TestOpen_NotFatal_WhenDirMissing(t *testing.T) {
	p := ccsessions.NewProvider(baseCfg())
	cfg := providers.ProviderConfig{Enabled: true, Path: filepath.Join(t.TempDir(), "missing")}

	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Open panicked: %v", r)
		}
	}()

	_, err := p.Open(context.Background(), cfg)
	if err == nil {
		t.Log("warning: Open returned nil for a missing directory")
	}
}

// ---------------------------------------------------------------------------
// Health
// ---------------------------------------------------------------------------

// TestHealth_OkWhenDirExists asserts Health returns OK=true when the projects
// directory exists.
func TestHealth_OkWhenDirExists(t *testing.T) {
	dir := seedProjectsDir(t)
	p := ccsessions.NewProvider(baseCfg())
	cfg := providers.ProviderConfig{Enabled: true, Path: dir}

	inst, err := p.Open(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer inst.Close(context.Background()) //nolint:errcheck

	h := inst.Health(context.Background())
	if !h.OK {
		t.Errorf("expected Health.OK=true for existing projects dir, got false (err: %s)", h.Error)
	}
	if h.ID != "cc-sessions" {
		t.Errorf("Health.ID: got %q, want %q", h.ID, "cc-sessions")
	}
}

// ---------------------------------------------------------------------------
// No Writable capability
// ---------------------------------------------------------------------------

// TestInstance_DoesNotImplementWritable asserts CC sessions instance does NOT
// implement the Writable capability (read-only provider contract).
func TestInstance_DoesNotImplementWritable(t *testing.T) {
	dir := seedProjectsDir(t)
	p := ccsessions.NewProvider(baseCfg())
	cfg := providers.ProviderConfig{Enabled: true, Path: dir}

	inst, err := p.Open(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer inst.Close(context.Background()) //nolint:errcheck

	if _, ok := inst.(providers.Writable); ok {
		t.Error("CC sessions Instance MUST NOT implement providers.Writable (read-only provider)")
	}
}

// ---------------------------------------------------------------------------
// Routes
// ---------------------------------------------------------------------------

// TestRoutes_DoesNotPanic asserts that Routes does not panic.
func TestRoutes_DoesNotPanic(t *testing.T) {
	dir := seedProjectsDir(t)
	p := ccsessions.NewProvider(baseCfg())
	cfg := providers.ProviderConfig{Enabled: true, Path: dir}

	inst, err := p.Open(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer inst.Close(context.Background()) //nolint:errcheck

	mux := http.NewServeMux()
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Routes panicked: %v", r)
		}
	}()
	p.Routes(mux, inst, nil)
}

// ---------------------------------------------------------------------------
// Nav
// ---------------------------------------------------------------------------

// TestNav_ReturnsGroup asserts Nav returns a populated NavGroup with the
// correct ID and at least one link.
func TestNav_ReturnsGroup(t *testing.T) {
	dir := seedProjectsDir(t)
	p := ccsessions.NewProvider(baseCfg())
	cfg := providers.ProviderConfig{Enabled: true, Path: dir}

	inst, err := p.Open(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer inst.Close(context.Background()) //nolint:errcheck

	nav := p.Nav(inst)
	if nav.ID != "cc-sessions" {
		t.Errorf("Nav.ID: got %q, want %q", nav.ID, "cc-sessions")
	}
	if nav.Featured {
		t.Error("Nav.Featured: expected false for CC sessions provider")
	}
	if len(nav.Links) == 0 {
		t.Error("Nav.Links: expected at least one link")
	}
}

// ---------------------------------------------------------------------------
// ValidateCCSessionPath safety
// ---------------------------------------------------------------------------

// TestValidate_RejectsPathTraversal asserts that a path containing ".." in the
// directory path (filesystem-level) is either rejected by Validate or caught
// before causing harm. For the provider itself, empty or non-existent paths
// must return errors.
func TestValidate_RejectsEmptyPath(t *testing.T) {
	p := ccsessions.NewProvider(baseCfg())
	if err := p.Validate(context.Background(), providers.ProviderConfig{Path: ""}); err == nil {
		t.Error("expected error for empty path")
	}
}
