package ui_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/config"
	"github.com/AlvaroQ/engram-explorer/internal/ui"
)

// TestMount_LiveGatesModulePagesPerRequest covers S4: with Deps.Live the
// module pages are gated on every request — 503 with a localized message
// while the module is off, served as soon as it is on, with one Mount.
func TestMount_LiveGatesModulePagesPerRequest(t *testing.T) {
	var live ui.LiveHandles
	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{
		Config: config.Config{ClaudeProjectsDir: t.TempDir()},
		Live:   func() ui.LiveHandles { return live },
	})
	get := func(route, lang string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, route, nil)
		req.AddCookie(&http.Cookie{Name: "lang", Value: lang})
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec
	}

	for _, tc := range []struct{ lang, want string }{
		{"en", "This module is disabled."},
		{"es", "Este módulo está desactivado."},
	} {
		for _, route := range []string{"/observations", "/projects", "/brain", "/cc-overview"} {
			rec := get(route, tc.lang)
			if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), tc.want) {
				t.Errorf("%s GET %s while off: status %d body %.120q, want 503 with %q", tc.lang, route, rec.Code, rec.Body.String(), tc.want)
			}
		}
	}
	if rec := get("/settings", "en"); rec.Code != http.StatusOK {
		t.Errorf("GET /settings (shell) while modules are off: status %d, want 200", rec.Code)
	}

	live.Claude = true
	if rec := get("/cc-overview", "en"); rec.Code != http.StatusOK {
		t.Errorf("GET /cc-overview after enabling: status %d, want 200 (body %.200q)", rec.Code, rec.Body.String())
	}
}
