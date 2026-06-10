package ui_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/ui"
)

func postForm(t *testing.T, mux *http.ServeMux, path, value string) *httptest.ResponseRecorder {
	t.Helper()
	form := url.Values{"path": {value}}
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	return w
}

// TestClaudeDirPost_Valid verifies a successful change invokes the callback and
// redirects back to /settings.
func TestClaudeDirPost_Valid(t *testing.T) {
	db := openTestDB(t)
	var got string

	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{
		RoDB:         db,
		SetClaudeDir: func(p string) error { got = p; return nil },
	})

	w := postForm(t, mux, "/settings/claude-dir", "/some/projects")
	if w.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d", w.Code)
	}
	if got != "/some/projects" {
		t.Errorf("SetClaudeDir got %q, want /some/projects", got)
	}
}

// TestClaudeDirPost_Error re-renders the settings page with the error message
// when the callback fails.
func TestClaudeDirPost_Error(t *testing.T) {
	db := openTestDB(t)

	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{
		RoDB:         db,
		SetClaudeDir: func(p string) error { return errors.New("not a directory: /bad") },
	})

	w := postForm(t, mux, "/settings/claude-dir", "/bad")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 (re-render), got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "not a directory: /bad") {
		t.Error("expected the error message to be rendered on the page")
	}
}

// TestEngramDBPost_EmptyPath surfaces a validation error for a blank path.
func TestEngramDBPost_EmptyPath(t *testing.T) {
	db := openTestDB(t)

	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{
		RoDB:           db,
		ReloadEngramDB: func(p string) error { return nil },
	})

	w := postForm(t, mux, "/settings/engram-db", "   ")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 (re-render with error), got %d", w.Code)
	}
}
