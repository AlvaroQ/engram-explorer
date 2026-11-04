package ui_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/config"
	"github.com/AlvaroQ/engram-explorer/internal/ui"
	"github.com/AlvaroQ/engram-explorer/internal/usererr"
)

// TestSettingsPage_NoRawErrorText covers the two Settings hints that rendered
// Go error text: the daemon reachability error and a module's open error.
func TestSettingsPage_NoRawErrorText(t *testing.T) {
	db := openTestDB(t)
	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{
		RoDB: db,
		// Nothing listens on port 1: the daemon ping fails with a dial error.
		Config: config.Config{DaemonBaseURL: "http://127.0.0.1:1", DaemonTimeoutMs: 200},
		Modules: func() []ui.ModuleInfo {
			return []ui.ModuleInfo{
				{ID: "engram", DisplayName: "Engram", Tier: ui.ModuleTier1, State: ui.ModuleErrored,
					Cause: errors.New("open /private/secret/engram.db: permission denied")},
				{ID: "cc-sessions", DisplayName: "Claude Code", Tier: ui.ModuleTier1, State: ui.ModuleErrored,
					Cause: usererr.New("projects directory not found: /typed/by/user")},
			}
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/settings", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	for _, leak := range []string{"dial tcp", "connection refused", "/private/secret", "permission denied"} {
		if strings.Contains(body, leak) {
			t.Errorf("settings page leaks internal error text %q", leak)
		}
	}
	if !strings.Contains(body, "projects directory not found: /typed/by/user") {
		t.Error("user-safe module error must still be shown verbatim")
	}
	if !strings.Contains(body, "Something went wrong") {
		t.Error("generic message expected in place of the internal module error")
	}
	if !strings.Contains(body, "The Engram daemon did not respond") {
		t.Error("fixed daemon hint expected in place of the dial error")
	}
}
