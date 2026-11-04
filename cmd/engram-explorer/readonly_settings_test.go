package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/config"
	"github.com/AlvaroQ/engram-explorer/internal/httpapi"
)

func postForm(h http.Handler, path string, form url.Values) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// bootMakeDemo boots the container the way `make demo` runs the binary:
// ENGRAM_DATA_DIR=<demo>, CLAUDE_PROJECTS_DIR=<demo>/claude and
// ENGRAM_DASH_READONLY=true, with HOME pointing at an empty directory. It
// returns the container, the demo directory and HOME.
func bootMakeDemo(t *testing.T) (*httpapi.Container, string, string) {
	t.Helper()
	home := t.TempDir()
	demo := filepath.Join(t.TempDir(), "demo")
	makeEngramDB(t, demo)
	if err := os.MkdirAll(filepath.Join(demo, "claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("ENGRAM_DATA_DIR", demo)
	t.Setenv("CLAUDE_PROJECTS_DIR", filepath.Join(demo, "claude"))
	t.Setenv("ENGRAM_DASH_READONLY", "true")
	t.Setenv("ENGRAM_DAEMON_URL", "")
	t.Setenv("DASHBOARD_HOST", "")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	c := buildContainer(cfg, quietLogger())
	c.CloseGrace = 0
	t.Cleanup(c.Close)
	return c, demo, home
}

// TestMakeDemo_ServesDemoDataReadOnlyAndPersistsInDemoDir covers the `make
// demo` replacement of demo mode: the binary serves ./demo/engram.db, JSON
// writes answer 503 RW_UNAVAILABLE, and settings changes (no settings lock in
// read-only mode) persist in ./demo/config.json, never under HOME.
func TestMakeDemo_ServesDemoDataReadOnlyAndPersistsInDemoDir(t *testing.T) {
	c, demo, home := bootMakeDemo(t)
	h := httpapi.NewServeMux(c)

	if got, want := servedEngramPath(t, c), filepath.Join(demo, "engram.db"); got != want {
		t.Fatalf("served %q, want the demo database %q", got, want)
	}

	req := httptest.NewRequest(http.MethodPatch, "/api/observations/1", strings.NewReader(`{"title":"x"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), "RW_UNAVAILABLE") {
		t.Errorf("PATCH in read-only: status %d body %q, want 503 RW_UNAVAILABLE", rec.Code, rec.Body.String())
	}

	if rec := postForm(h, "/settings/profile/create", url.Values{"name": {"second"}}); rec.Code != http.StatusSeeOther {
		t.Fatalf("POST /settings/profile/create: status %d, want 303; body %.200q", rec.Code, rec.Body.String())
	}
	if rec := postForm(h, "/settings/advanced-view", url.Values{"enabled": {"true"}}); rec.Code != http.StatusSeeOther {
		t.Fatalf("POST /settings/advanced-view: status %d, want 303", rec.Code)
	}

	saved, err := config.LoadProfileStore(demo)
	if err != nil {
		t.Fatalf("LoadProfileStore(demo): %v", err)
	}
	if _, ok := saved.Profiles["second"]; !ok {
		t.Errorf("created profile not persisted in %s/config.json: %v", demo, saved.Profiles)
	}
	if !saved.Profiles[saved.ActiveProfile].IsAdvancedView() {
		t.Error("advanced view not persisted in the demo config.json")
	}
	entries, err := os.ReadDir(home)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("make demo wrote under HOME: %v", entries)
	}
}

// TestMakeDemo_CosmeticPreferencesAllowed: theme, language and sidebar
// visibility are cookie preferences and keep working in read-only mode.
func TestMakeDemo_CosmeticPreferencesAllowed(t *testing.T) {
	c, _, _ := bootMakeDemo(t)
	h := httpapi.NewServeMux(c)

	cases := []struct {
		path   string
		form   url.Values
		cookie string
	}{
		{"/theme", url.Values{"value": {"light"}}, "theme=light"},
		{"/lang", url.Values{"value": {"es"}}, "lang=es"},
		{"/settings/nav-visibility", url.Values{"section": {"engram"}, "value": {"hide"}}, "nav_engram=0"},
	}
	for _, tc := range cases {
		rec := postForm(h, tc.path, tc.form)
		cookies := rec.Header().Values("Set-Cookie")
		if rec.Code != http.StatusSeeOther || !slices.ContainsFunc(cookies, func(s string) bool { return strings.Contains(s, tc.cookie) }) {
			t.Errorf("POST %s: status %d, Set-Cookie %q; want 303 setting %q", tc.path, rec.Code, cookies, tc.cookie)
		}
	}
}
