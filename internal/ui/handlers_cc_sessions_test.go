package ui_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/config"
	"github.com/AlvaroQ/engram-explorer/internal/ui"
)

// ---------------------------------------------------------------------------
// Helpers: build a real temp directory with .jsonl files
// ---------------------------------------------------------------------------

// buildCCTestDir creates a temp projects directory with one project folder
// containing one .jsonl session file. Returns the base dir path.
func buildCCTestDir(t *testing.T, projectFolder, sessionID, content string) string {
	t.Helper()
	base := t.TempDir()
	dir := filepath.Join(base, projectFolder)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, sessionID+".jsonl"), []byte(content), 0644); err != nil {
		t.Fatalf("write session file: %v", err)
	}
	return base
}

// minimalJSONL returns a minimal valid .jsonl content for one user turn.
func minimalJSONL(cwd, prompt string) string {
	return `{"type":"user","timestamp":"2026-06-08T10:00:00.000Z","sessionId":"s","cwd":"` +
		cwd + `","gitBranch":"main","version":"2.1.0","message":{"role":"user","content":"` +
		prompt + `"}}` + "\n"
}

// ---------------------------------------------------------------------------
// Tests: GET /cc-sessions
// ---------------------------------------------------------------------------

func TestCCSessionsListPage_FullPage(t *testing.T) {
	base := buildCCTestDir(t, "my-project-folder", "sess-001",
		minimalJSONL("e:/my-project", "Hello Claude"))

	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{Config: config.Config{ClaudeProjectsDir: base}})

	req := httptest.NewRequest(http.MethodGet, "/cc-sessions", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, "<html") {
		t.Error("full page must contain <html>")
	}
	if !strings.Contains(body, "sess-001") {
		t.Errorf("page should contain session id; body snippet: %s", body[:min(500, len(body))])
	}
}

func TestCCSessionsListPage_HTMXPartial(t *testing.T) {
	base := buildCCTestDir(t, "p", "s1", minimalJSONL("e:/p", "prompt"))

	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{Config: config.Config{ClaudeProjectsDir: base}})

	req := httptest.NewRequest(http.MethodGet, "/cc-sessions", nil)
	req.Header.Set("HX-Request", "true")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if strings.Contains(w.Body.String(), "<html") {
		t.Error("HTMX partial must NOT contain <html>")
	}
}

func TestCCSessionsListPartialEndpoint(t *testing.T) {
	base := buildCCTestDir(t, "p", "s1", minimalJSONL("e:/p", "prompt"))

	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{Config: config.Config{ClaudeProjectsDir: base}})

	req := httptest.NewRequest(http.MethodGet, "/cc-sessions/list", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "<html") {
		t.Error("list partial must NOT contain <html>")
	}
}

func TestCCSessionsListPage_NonexistentDir(t *testing.T) {
	base := filepath.Join(t.TempDir(), "does-not-exist")

	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{Config: config.Config{ClaudeProjectsDir: base}})

	req := httptest.NewRequest(http.MethodGet, "/cc-sessions", nil)
	w := httptest.NewRecorder()

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("handler panicked: %v", r)
		}
	}()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 (empty state), got %d; body: %s", w.Code, w.Body.String())
	}
}

func TestCCSessionsListPage_ProjectFilter(t *testing.T) {
	base := t.TempDir()

	// Project A.
	dirA := filepath.Join(base, "proj-a")
	os.MkdirAll(dirA, 0755)
	os.WriteFile(filepath.Join(dirA, "sa.jsonl"),
		[]byte(minimalJSONL("e:/alpha", "prompt from alpha")), 0644)

	// Project B.
	dirB := filepath.Join(base, "proj-b")
	os.MkdirAll(dirB, 0755)
	os.WriteFile(filepath.Join(dirB, "sb.jsonl"),
		[]byte(minimalJSONL("e:/beta", "prompt from beta")), 0644)

	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{Config: config.Config{ClaudeProjectsDir: base}})

	req := httptest.NewRequest(http.MethodGet, "/cc-sessions?project=alpha", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "alpha") {
		t.Error("filtered page must show alpha project")
	}
	if strings.Contains(body, "beta") {
		t.Error("filtered page must NOT show beta project")
	}
}

// ---------------------------------------------------------------------------
// Tests: GET /cc-sessions/{project}/{id}
// ---------------------------------------------------------------------------

func TestCCSessionDetailPage_FullPage(t *testing.T) {
	base := buildCCTestDir(t, "my-folder", "session-xyz",
		minimalJSONL("e:/myproject", "First user message"))

	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{Config: config.Config{ClaudeProjectsDir: base}})

	req := httptest.NewRequest(http.MethodGet, "/cc-sessions/my-folder/session-xyz", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, "<html") {
		t.Error("full page must contain <html>")
	}
	if !strings.Contains(body, "session-xyz") {
		t.Error("page must contain session id")
	}
}

func TestCCSessionDetailPage_HTMXPartial(t *testing.T) {
	base := buildCCTestDir(t, "my-folder", "session-xyz",
		minimalJSONL("e:/myproject", "A prompt"))

	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{Config: config.Config{ClaudeProjectsDir: base}})

	req := httptest.NewRequest(http.MethodGet, "/cc-sessions/my-folder/session-xyz", nil)
	req.Header.Set("HX-Request", "true")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if strings.Contains(w.Body.String(), "<html") {
		t.Error("HTMX partial must NOT contain <html>")
	}
}

func TestCCSessionDetailPage_NotFound(t *testing.T) {
	base := t.TempDir()
	// Create the project dir but no session file.
	os.MkdirAll(filepath.Join(base, "my-folder"), 0755)

	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{Config: config.Config{ClaudeProjectsDir: base}})

	req := httptest.NewRequest(http.MethodGet, "/cc-sessions/my-folder/nonexistent-session", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestCCSessionDetailPage_PathTraversal(t *testing.T) {
	base := t.TempDir()

	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{Config: config.Config{ClaudeProjectsDir: base}})

	// A traversal attempt via the URL.
	req := httptest.NewRequest(http.MethodGet, "/cc-sessions/..%2Fetc/passwd", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	// Should be rejected — either 400 or 404 is acceptable.
	if w.Code == http.StatusOK {
		t.Fatal("path traversal must not return 200")
	}
}

// ---------------------------------------------------------------------------
// Helper
// ---------------------------------------------------------------------------

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
