package ui_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/config"
	"github.com/AlvaroQ/engram-explorer/internal/ui"
)

// uiActionsHooks are the data-* attributes ui-actions.js listens for. Every
// hook must be implemented by the served script.
var uiActionsHooks = []string{
	"data-hide-target",
	"data-clear-target",
	"data-escape-clear-target",
	"data-auto-submit",
	"data-submit-confirm",
	"data-merge-confirm-source",
	"data-show-on-swap",
	"data-remove-after-request",
	"data-poll-visible-only",
	"data-modal-dialog",
	"data-dialog-close",
	"data-menu-button",
}

// newCSPMux mounts the UI on a DB seeded with two projects, one observation
// and 51 sessions (one more than the page size, so the load-more row renders).
func newCSPMux(t *testing.T) *http.ServeMux {
	t.Helper()
	db := openTestDB(t)
	if _, err := db.ExecContext(context.Background(), mergeSchema+
		`ALTER TABLE observations ADD COLUMN review_after TEXT;`); err != nil {
		t.Fatalf("schema: %v", err)
	}
	for _, p := range []string{"alpha", "beta"} {
		if _, err := db.Exec(
			`INSERT INTO observations (type, title, content, project, created_at, updated_at, scope)
			 VALUES ('decision', 'obs', 'body', ?, '2026-01-01 10:00:00', '2026-01-01 10:00:00', 'project')`, p,
		); err != nil {
			t.Fatalf("seed obs: %v", err)
		}
	}
	for i := 0; i < 51; i++ {
		if _, err := db.Exec(
			`INSERT INTO sessions (id, project, started_at) VALUES (?, 'alpha', ?)`,
			fmt.Sprintf("s-%02d", i), fmt.Sprintf("2026-01-01 10:%02d:00", i),
		); err != nil {
			t.Fatalf("seed session: %v", err)
		}
	}
	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{RoDB: db, RWDB: db})
	return mux
}

func cspGet(t *testing.T, mux *http.ServeMux, path string, htmx bool) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if htmx {
		req.Header.Set("HX-Request", "true")
	}
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	return w
}

// TestCSP_RenderedPagesHaveNoInlineScriptCode renders the pages and partials
// whose templates carried inline scripts or handlers. None may contain inline
// script code, and each must carry the data-* hook that replaced it.
func TestCSP_RenderedPagesHaveNoInlineScriptCode(t *testing.T) {
	mux := newCSPMux(t)
	cases := []struct {
		path string
		htmx bool
		want []string
	}{
		{"/", false, []string{`<script src="/static/ui-actions.js" defer></script>`}},
		{"/brain", false, []string{`<script src="/static/ui-actions.js" defer></script>`}},
		{"/observations", false, []string{`id="obs-detail"`, "hidden data-show-on-swap"}},
		{"/observations/1", true, []string{`data-hide-target="obs-detail"`}},
		{"/settings", false, []string{"data-auto-submit", `data-submit-confirm="Import database?`}},
		{"/sessions/list", true, []string{`data-remove-after-request="sessions-load-more-row"`}},
		{"/projects/alpha", false, []string{`data-merge-confirm-source="alpha"`}},
	}
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			w := cspGet(t, mux, tc.path, tc.htmx)
			if w.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
			}
			body := w.Body.String()
			for _, v := range ui.InlineCodeViolations(body) {
				t.Errorf("%s", v)
			}
			for _, h := range tc.want {
				if !strings.Contains(body, h) {
					t.Errorf("missing %s", h)
				}
			}
		})
	}
}

// TestCSP_RenderedPagesHaveNoInlineStyles renders full pages (both layouts)
// and partials. style-src is 'self' only, so none may carry a <style> element
// or a style= attribute.
func TestCSP_RenderedPagesHaveNoInlineStyles(t *testing.T) {
	mux := newCSPMux(t)
	cases := []struct {
		path string
		htmx bool
	}{
		{"/", false},
		{"/brain", false},
		{"/observations", false},
		{"/observations/list", true},
		{"/observations/1", true},
		{"/settings", false},
		{"/sessions", false},
		{"/sessions/s-01", false},
		{"/projects", false},
		{"/projects/alpha", false},
		{"/agent-sessions", false},
		{"/agent-sessions/list", false},
	}
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			w := cspGet(t, mux, tc.path, tc.htmx)
			if w.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
			}
			for _, v := range ui.InlineStyleViolations(w.Body.String()) {
				t.Errorf("%s", v)
			}
		})
	}
}

// TestCSP_ModuleDisabledPageHasNoInlineStyles covers S4 + S18: the
// disabled-module page (full layout on navigation, fragment for htmx) stays
// within style-src 'self'.
func TestCSP_ModuleDisabledPageHasNoInlineStyles(t *testing.T) {
	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{
		Config: config.Config{ClaudeProjectsDir: t.TempDir()},
		Live:   func(*http.Request) ui.LiveHandles { return ui.LiveHandles{} },
	})
	for _, htmx := range []bool{false, true} {
		w := cspGet(t, mux, "/observations", htmx)
		if w.Code != http.StatusServiceUnavailable {
			t.Fatalf("htmx=%v: status = %d, want 503", htmx, w.Code)
		}
		for _, v := range ui.InlineStyleViolations(w.Body.String()) {
			t.Errorf("htmx=%v: %s", htmx, v)
		}
	}
}

// newSettingsCSPMux mounts the UI with every Settings callback wired, so the
// Accounts tab (profile rows), the Modules rows in each state and the Claude
// Code account rows render. Not in demo mode, so the error branches render.
func newSettingsCSPMux(t *testing.T) *http.ServeMux {
	t.Helper()
	db := openTestDB(t)
	fail := errors.New("boom")
	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{
		RoDB: db,
		RWDB: db,
		Profiles: func() []ui.ProfileInfo {
			return []ui.ProfileInfo{{Name: "default", Active: true}, {Name: "work"}}
		},
		ActiveProfile: func() string { return "default" },
		SwitchProfile: func(string) error { return fail },
		CreateProfile: func(string) error { return fail },
		DeleteProfile: func(string) error { return fail },
		Modules: func() []ui.ModuleInfo {
			return []ui.ModuleInfo{
				{ID: "engram", DisplayName: "Engram", State: ui.ModuleEnabled, Path: "/data/engram.db"},
				{ID: "cc-sessions", DisplayName: "Claude Code", State: ui.ModuleDisabled},
				{ID: "detected", DisplayName: "Detected", State: ui.ModuleDetected},
				{ID: "errored", DisplayName: "Errored", State: ui.ModuleErrored, Cause: fail},
				{ID: "registered", DisplayName: "Registered", State: ui.ModuleRegistered},
				{ID: "tier2", DisplayName: "Tier 2", Tier: ui.ModuleTier2, State: ui.ModuleDetected},
			}
		},
		ToggleModule:       func(string, bool) error { return fail },
		ValidateModulePath: func(context.Context, string, string) error { return fail },
		CCAccounts: func() []ui.CCAccountInfo {
			return []ui.CCAccountInfo{
				{ID: "default", Label: "Personal", Path: "/home/u/.claude", Enabled: true},
				{ID: "work", Label: "Work", Path: "/home/u/.claude-work"},
			}
		},
		AddCCAccount:    func(string, string) error { return fail },
		UpdateCCAccount: func(string, string, string, bool) error { return fail },
		RemoveCCAccount: func(string) error { return fail },
	})
	return mux
}

// TestCSP_SettingsTabsHaveNoInlineStyles renders every Settings tab (full
// page and HTMX partial) with all rows populated, plus the error re-renders
// of each settings form, and rejects any <style> element or style= attribute.
func TestCSP_SettingsTabsHaveNoInlineStyles(t *testing.T) {
	mux := newSettingsCSPMux(t)
	cases := []struct {
		method, path string
		form         url.Values
		htmx         bool
		want         string
	}{
		{http.MethodGet, "/settings", nil, false, `id="settings-cc-accounts"`},
		{http.MethodGet, "/settings", nil, true, `id="settings-cc-accounts"`},
		{http.MethodGet, "/settings/accounts", nil, false, `id="settings-accounts"`},
		{http.MethodGet, "/settings/accounts", nil, true, `id="settings-accounts"`},
		{http.MethodPost, "/settings/profile/switch", url.Values{"name": {"work"}}, true, "error-hint"},
		{http.MethodPost, "/settings/profile/create", url.Values{"name": {"x"}}, false, "error-hint"},
		{http.MethodPost, "/settings/engram-db", url.Values{"path": {""}}, true, "error-hint"},
		{http.MethodPost, "/settings/claude-dir", url.Values{"path": {""}}, true, "error-hint"},
		{http.MethodPost, "/settings/modules/detected/path", url.Values{"path": {"/x"}}, true, "error-hint"},
		{http.MethodPost, "/settings/modules/engram/toggle", url.Values{"enabled": {"false"}}, true, "error-hint"},
		{http.MethodPost, "/settings/cc-accounts", url.Values{"label": {""}}, true, "error-hint"},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprintf("%s %s htmx=%v", tc.method, tc.path, tc.htmx), func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.form.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			if tc.htmx {
				req.Header.Set("HX-Request", "true")
			}
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)
			body := w.Body.String()
			if w.Code != http.StatusOK || !strings.Contains(body, tc.want) {
				t.Fatalf("status = %d, want 200 rendering %s; body: %.300s", w.Code, tc.want, body)
			}
			for _, v := range ui.InlineStyleViolations(body) {
				t.Errorf("%s", v)
			}
		})
	}
}

// TestTemplSources_NoInlineStyles scans every internal/ui/*.templ file, so a
// new style attribute or <style> element fails even when no test renders it.
func TestTemplSources_NoInlineStyles(t *testing.T) {
	files, err := filepath.Glob("*.templ")
	if err != nil || len(files) == 0 {
		t.Fatalf("no .templ files found (err=%v); guard is not running", err)
	}
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		for _, v := range ui.TemplStyleViolations(string(raw)) {
			t.Errorf("%s: %s", f, v)
		}
	}
}

// htmxConfigRe accepts a double- or single-quoted content attribute (templ
// keeps the single quotes written in the template).
var htmxConfigRe = regexp.MustCompile(`<meta name="htmx-config" content=(?:"([^"]*)"|'([^']*)')`)

// TestCSP_LayoutsLoadUIActionsAndDisableEval checks both layouts (Layout via
// /settings, LayoutFullBleed via /brain) load the static actions script and
// configure htmx with allowEval:false.
func TestCSP_LayoutsLoadUIActionsAndDisableEval(t *testing.T) {
	mux := newCSPMux(t)
	for _, path := range []string{"/settings", "/brain"} {
		t.Run(path, func(t *testing.T) {
			body := cspGet(t, mux, path, false).Body.String()
			if !strings.Contains(body, `<script src="/static/ui-actions.js" defer></script>`) {
				t.Error("layout must load /static/ui-actions.js with defer")
			}
			m := htmxConfigRe.FindStringSubmatch(body)
			if m == nil {
				t.Fatal("layout must declare <meta name=\"htmx-config\">")
			}
			var cfg map[string]any
			raw := m[1] + m[2]
			if err := json.Unmarshal([]byte(html.UnescapeString(raw)), &cfg); err != nil {
				t.Fatalf("htmx-config is not JSON: %v (%q)", err, raw)
			}
			if v, ok := cfg["allowEval"].(bool); !ok || v {
				t.Errorf("htmx-config allowEval = %v, want false", cfg["allowEval"])
			}
			// htmx otherwise injects a <style> element for its indicator classes,
			// which style-src 'self' blocks; styles.css carries those rules.
			if v, ok := cfg["includeIndicatorStyles"].(bool); !ok || v {
				t.Errorf("htmx-config includeIndicatorStyles = %v, want false", cfg["includeIndicatorStyles"])
			}
		})
	}
}

// TestCSP_UIActionsScriptServed checks the static script is served as
// JavaScript and implements every data-* hook the templates use.
func TestCSP_UIActionsScriptServed(t *testing.T) {
	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{})
	w := cspGet(t, mux, "/static/ui-actions.js", false)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, "javascript") {
		t.Errorf("Content-Type = %q, want javascript", ct)
	}
	body := w.Body.String()
	for _, h := range uiActionsHooks {
		if !strings.Contains(body, h) {
			t.Errorf("ui-actions.js does not handle %s", h)
		}
	}
	// data-show-on-swap panels start with the hidden attribute (no inline
	// display:none under style-src 'self'), so the hook must clear it.
	if !strings.Contains(body, "target.hidden = false") {
		t.Error("ui-actions.js data-show-on-swap must clear the hidden attribute")
	}
}

// submitCallRe matches a direct HTMLFormElement.submit() call. form.submit()
// fires no submit event, so the capture-phase data-submit-confirm hook would
// never run; auto-submit must go through requestSubmit() instead.
var submitCallRe = regexp.MustCompile(`\.submit\(\s*\)`)

// TestUIActions_AutoSubmitUsesRequestSubmit checks the data-auto-submit hook
// submits via form.requestSubmit(), which dispatches the submit event that the
// data-submit-confirm listener (window.confirm) relies on (Settings import).
func TestUIActions_AutoSubmitUsesRequestSubmit(t *testing.T) {
	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{})
	body := cspGet(t, mux, "/static/ui-actions.js", false).Body.String()
	if !strings.Contains(body, ".requestSubmit()") {
		t.Error("ui-actions.js must auto-submit with form.requestSubmit()")
	}
	if m := submitCallRe.FindAllString(body, -1); len(m) != 0 {
		t.Errorf("ui-actions.js calls form.submit() (%d times), which skips the submit event and its confirm", len(m))
	}
}
