package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/config"
	"github.com/AlvaroQ/engram-explorer/internal/httpapi"
)

const demoRejection = "Write operations are disabled in demo mode."

// demoState is everything a data-source change could alter: the database the
// server reads (as /api/health reports it), the active profile and the
// profiles, the module states and the Claude Code accounts.
type demoState struct {
	HealthDBPath string
	Active       string
	Profiles     []string
	Modules      map[string]string
	CCAccounts   []config.CCAccount
}

func snapshotDemoState(t *testing.T, c *httpapi.Container, h http.Handler) demoState {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/health", nil))
	var health struct {
		DB struct {
			Path string `json:"path"`
		} `json:"db"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &health); err != nil {
		t.Fatalf("decode /api/health: %v (%s)", err, rec.Body.String())
	}
	profiles := c.ProfileStore.ProfileNames()
	sort.Strings(profiles)
	modules := map[string]string{}
	for _, m := range c.Registry.AllProviderMetas() {
		modules[m.ID] = m.State.String()
	}
	return demoState{
		HealthDBPath: health.DB.Path,
		Active:       c.ProfileStore.ActiveProfileName(),
		Profiles:     profiles,
		Modules:      modules,
		CCAccounts:   c.ProfileStore.CCAccounts(),
	}
}

func postForm(h http.Handler, path string, form url.Values) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// TestDemo_DataSourceChangesRejected covers S1: in demo mode no settings POST
// may repoint the server at another database or data source. Each one answers
// the demo rejection and leaves the served DB, the profiles, the module states
// and the Claude Code accounts unchanged.
func TestDemo_DataSourceChangesRejected(t *testing.T) {
	realDB := makeEngramDB(t, filepath.Join(t.TempDir(), "real"))
	demoDB := makeEngramDB(t, filepath.Join(t.TempDir(), "demo"))
	ccDir := t.TempDir()
	otherCC := t.TempDir()
	if err := os.MkdirAll(filepath.Join(ccDir, "proj"), 0o755); err != nil {
		t.Fatal(err)
	}

	cfg := config.Config{
		ConfigHome:        t.TempDir(),
		EngramDbPath:      demoDB,
		ClaudeProjectsDir: ccDir,
		DemoMode:          true,
		DaemonTimeoutMs:   50,
	}
	c := buildContainer(cfg, quietLogger())
	c.CloseGrace = 0
	t.Cleanup(c.Close)

	// A second profile pointing at the real database, as an attacker would
	// want to switch to (seeded directly: creating it over HTTP is rejected).
	if err := c.ProfileStore.Update(func(st *config.ProfileStore) error {
		st.Profiles["real"] = config.Profile{Providers: map[string]config.ProviderConfig{
			"engram": {Enabled: true, Path: realDB},
		}}
		return nil
	}); err != nil {
		t.Fatalf("seed profile: %v", err)
	}

	h := httpapi.NewServeMux(c)
	before := snapshotDemoState(t, c, h)
	if before.HealthDBPath != demoDB {
		t.Fatalf("demo serves %q, want %q", before.HealthDBPath, demoDB)
	}
	ccID := before.CCAccounts[0].ID

	cases := []struct {
		name string
		path string
		form url.Values
	}{
		{"engram db", "/settings/engram-db", url.Values{"path": {realDB}}},
		{"claude dir", "/settings/claude-dir", url.Values{"path": {otherCC}}},
		{"module disable", "/settings/modules/engram/toggle", url.Values{"enabled": {"false"}}},
		{"module enable", "/settings/modules/cc-sessions/toggle", url.Values{"enabled": {"true"}}},
		{"module path", "/settings/modules/engram/path", url.Values{"path": {realDB}}},
		{"profile create", "/settings/profile/create", url.Values{"name": {"evil"}}},
		{"profile switch", "/settings/profile/switch", url.Values{"name": {"real"}}},
		{"profile delete", "/settings/profile/delete", url.Values{"name": {"real"}}},
		{"cc-account add", "/settings/cc-accounts", url.Values{"label": {"Work"}, "path": {otherCC}}},
		{"cc-account toggle", "/settings/cc-accounts/" + ccID + "/toggle", url.Values{"enabled": {"false"}}},
		{"cc-account remove", "/settings/cc-accounts/" + ccID + "/remove", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := postForm(h, tc.path, tc.form)
			if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), demoRejection) {
				t.Errorf("POST %s: status %d, body %.200q; want 200 with %q",
					tc.path, rec.Code, rec.Body.String(), demoRejection)
			}
			if rec.Header().Get("Location") != "" || rec.Header().Get("HX-Redirect") != "" {
				t.Errorf("POST %s redirected as if it succeeded", tc.path)
			}
			if after := snapshotDemoState(t, c, h); !reflect.DeepEqual(before, after) {
				t.Errorf("POST %s changed demo state:\nbefore %+v\nafter  %+v", tc.path, before, after)
			}
		})
	}
}

// TestDemo_CosmeticPreferencesAllowed: theme, language, sidebar visibility and
// the advanced view are display preferences and keep working in demo mode.
func TestDemo_CosmeticPreferencesAllowed(t *testing.T) {
	demoDB := makeEngramDB(t, filepath.Join(t.TempDir(), "demo"))
	c := buildContainer(config.Config{
		ConfigHome: t.TempDir(), EngramDbPath: demoDB, DemoMode: true, DaemonTimeoutMs: 50,
	}, quietLogger())
	c.CloseGrace = 0
	t.Cleanup(c.Close)
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
		if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Set-Cookie"), tc.cookie) {
			t.Errorf("POST %s: status %d, Set-Cookie %q; want 303 setting %q",
				tc.path, rec.Code, rec.Header().Get("Set-Cookie"), tc.cookie)
		}
	}

	rec := postForm(h, "/settings/advanced-view", url.Values{"enabled": {"true"}})
	if rec.Code != http.StatusSeeOther {
		t.Errorf("POST /settings/advanced-view: status %d, want 303", rec.Code)
	}
	if _, p, _ := c.ProfileStore.ActiveProfile(); !p.IsAdvancedView() {
		t.Error("advanced view was not enabled in demo mode")
	}
}
