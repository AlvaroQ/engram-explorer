package httpapi_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AlvaroQ/engram-explorer/internal/config"
	"github.com/AlvaroQ/engram-explorer/internal/httpapi"
	"github.com/AlvaroQ/engram-explorer/internal/providers"
	ccprovider "github.com/AlvaroQ/engram-explorer/internal/providers/ccsessions"
	engramprovider "github.com/AlvaroQ/engram-explorer/internal/providers/engram"
)

// newLifecycleFixture builds a registry-backed container with the real Engram
// and Claude Code providers, both detected and enabled as asked. The handler
// is built ONCE; modules are switched with the real Settings toggle route.
func newLifecycleFixture(t *testing.T, engramOn, ccOn bool) profileFixture {
	t.Helper()
	return newLifecycleFixtureWith(t, engramOn, ccOn, nil)
}

// newLifecycleFixtureWith is newLifecycleFixture plus the extra profiles
// extra returns for the seeded database and Claude Code directory.
func newLifecycleFixtureWith(t *testing.T, engramOn, ccOn bool, extra func(dbPath, ccDir string) map[string]config.Profile) profileFixture {
	t.Helper()
	home := t.TempDir()
	dbPath := seedEngramDB(t)
	ccDir := makeClaudeDir(t)
	cfg := config.Config{
		ConfigHome:        home,
		EngramDbPath:      dbPath,
		ClaudeProjectsDir: ccDir,
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	reg := providers.NewRegistry(logger)
	reg.Register(engramprovider.NewProvider(cfg))
	reg.Register(ccprovider.NewProvider(cfg))
	data := config.ProfileStore{
		Version:       1,
		ActiveProfile: "default",
		Profiles: map[string]config.Profile{"default": {Providers: map[string]config.ProviderConfig{
			"engram":      {Enabled: engramOn, Path: dbPath},
			"cc-sessions": {Enabled: ccOn, Path: ccDir},
		}}},
		CCAccounts: []config.CCAccount{{ID: "default", Label: "Personal", Path: ccDir, Enabled: true}},
	}
	if extra != nil {
		for name, p := range extra(dbPath, ccDir) {
			data.Profiles[name] = p
		}
	}
	reg.Boot(context.Background(), httpapi.AdaptProfile(data.Profiles["default"]))
	c := httpapi.NewContainerWithRegistry(reg, cfg, logger)
	c.CloseGrace = 0
	c.ProfileStore = config.NewStore(home, data)
	t.Cleanup(c.Close)
	return profileFixture{c: c, h: httpapi.NewServeMux(c), home: home}
}

func (f profileFixture) toggle(t *testing.T, id string, on bool) {
	t.Helper()
	val := url.Values{"enabled": {strconv.FormatBool(on)}}
	if code := f.post(t, "/settings/modules/"+id+"/toggle", val); code != http.StatusSeeOther {
		t.Fatalf("toggle %s=%v: status %d, want 303", id, on, code)
	}
}

func (f profileFixture) fetch(t *testing.T, route string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	f.h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, route, nil))
	return w
}

// wantOK asserts every route answers 200.
func (f profileFixture) wantOK(t *testing.T, routes ...string) {
	t.Helper()
	for _, route := range routes {
		if w := f.fetch(t, route); w.Code != http.StatusOK {
			t.Errorf("GET %s: status %d, want 200 (body %.200s)", route, w.Code, w.Body.String())
		}
	}
}

// wantDisabled asserts every route answers 503: /api/ routes with the JSON
// MODULE_DISABLED envelope, pages with a localized HTML message.
func (f profileFixture) wantDisabled(t *testing.T, routes ...string) {
	t.Helper()
	for _, route := range routes {
		w := f.fetch(t, route)
		if w.Code != http.StatusServiceUnavailable {
			t.Errorf("GET %s: status %d, want 503 (body %.200s)", route, w.Code, w.Body.String())
			continue
		}
		if strings.HasPrefix(route, "/api/") {
			if errorCode(w) != "MODULE_DISABLED" {
				t.Errorf("GET %s: body %q, want error.code MODULE_DISABLED", route, w.Body.String())
			}
			continue
		}
		if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
			t.Errorf("GET %s: Content-Type %q, want text/html", route, ct)
		}
		if !strings.Contains(w.Body.String(), "module is disabled") {
			t.Errorf("GET %s: body %q lacks the disabled-module message", route, w.Body.String())
		}
	}
}

// errorCode returns error.code of a JSON error envelope ("" when absent).
func errorCode(w *httptest.ResponseRecorder) string {
	var env struct {
		Error struct{ Code string } `json:"error"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &env)
	return env.Error.Code
}

var (
	engramRoutes = []string{"/api/observations", "/api/overview", "/observations", "/doctor/orphans/list", "/settings/maintenance"}
	ccRoutes     = []string{"/api/cc-sessions/stats", "/cc-overview"}
)

// TestModuleLifecycle_EngramEnabledLater covers S4: Engram disabled at boot;
// enabling it through the Settings toggle serves its API, UI and doctor routes
// without rebuilding the handler, and disabling it answers 503 instead of
// touching the closed database.
func TestModuleLifecycle_EngramEnabledLater(t *testing.T) {
	f := newLifecycleFixture(t, false, false)

	f.wantDisabled(t, engramRoutes...)
	f.toggle(t, "engram", true)
	f.wantOK(t, append(engramRoutes, "/")...)
	f.toggle(t, "engram", false)
	f.wantDisabled(t, engramRoutes...)
	f.wantOK(t, "/", "/settings", "/api/health")
}

// TestModuleLifecycle_ClaudeSessions covers S4 for the Claude Code module.
func TestModuleLifecycle_ClaudeSessions(t *testing.T) {
	f := newLifecycleFixture(t, false, false)

	f.wantDisabled(t, ccRoutes...)
	f.toggle(t, "cc-sessions", true)
	f.wantOK(t, ccRoutes...)
	f.toggle(t, "cc-sessions", false)
	f.wantDisabled(t, ccRoutes...)
}

// S4: a module disabled while requests are in flight must never hand a
// request a closed pool ("sql: database is closed" → 500).
func TestModuleLifecycle_InFlightRequestsNeverHitClosedPool(t *testing.T) {
	f := newLifecycleFixture(t, true, true)
	done := make(chan struct{})
	var bad atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-done:
					return
				default:
				}
				for _, route := range []string{"/api/observations?limit=50", "/observations", "/doctor/orphans/list"} {
					rec := httptest.NewRecorder()
					f.h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, route, nil))
					if rec.Code == http.StatusInternalServerError ||
						(rec.Code == http.StatusOK && strings.Contains(rec.Body.String(), "inline-error")) {
						bad.Add(1)
					}
				}
			}
		}()
	}
	for i := 0; i < 40; i++ {
		f.toggle(t, "engram", false)
		f.toggle(t, "engram", true)
	}
	close(done)
	wg.Wait()
	if n := bad.Load(); n > 0 {
		t.Fatalf("%d requests failed on a closed pool during concurrent toggles", n)
	}
}

// inFlightReaders runs 6 readers over the Engram API, UI and doctor routes
// until the returned stop is called; stop reports how many requests failed on
// a closed pool (500, or a 200 page carrying an inline error).
func inFlightReaders(f profileFixture) (stop func() int64) {
	done := make(chan struct{})
	var bad atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-done:
					return
				default:
				}
				for _, route := range []string{"/api/observations?limit=50", "/observations", "/doctor/orphans/list"} {
					rec := httptest.NewRecorder()
					f.h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, route, nil))
					if rec.Code == http.StatusInternalServerError ||
						(rec.Code == http.StatusOK && strings.Contains(rec.Body.String(), "inline-error")) {
						bad.Add(1)
					}
				}
			}
		}()
	}
	return func() int64 {
		close(done)
		wg.Wait()
		return bad.Load()
	}
}

// S4: a profile switch closes the superseded Engram instance only after the
// requests using it finish — switching to a profile with Engram on another
// database and to one with Engram off.
func TestModuleLifecycle_ProfileSwitchNeverHandsClosedPool(t *testing.T) {
	f := newLifecycleFixtureWith(t, true, true, func(_, ccDir string) map[string]config.Profile {
		cc := config.ProviderConfig{Enabled: true, Path: ccDir}
		return map[string]config.Profile{
			"work": {Providers: map[string]config.ProviderConfig{
				"engram": {Enabled: true, Path: seedEngramDB(t)}, "cc-sessions": cc,
			}},
			"off": {Providers: map[string]config.ProviderConfig{
				"engram": {Enabled: false}, "cc-sessions": cc,
			}},
		}
	})
	stop := inFlightReaders(f)
	for i := 0; i < 40; i++ {
		for _, name := range []string{"work", "off", "default"} {
			if code := f.post(t, "/settings/profile/switch", url.Values{"name": {name}}); code != http.StatusSeeOther {
				t.Fatalf("switch to %s: status %d, want 303", name, code)
			}
		}
	}
	if n := stop(); n > 0 {
		t.Fatalf("%d requests failed on a closed pool during concurrent profile switches", n)
	}
}

// S4: shutting down while requests are in flight lets them finish on open
// pools; requests after the shutdown answer 503 MODULE_DISABLED.
func TestModuleLifecycle_ShutdownWaitsForInFlightRequests(t *testing.T) {
	f := newLifecycleFixture(t, true, true)
	stop := inFlightReaders(f)
	time.Sleep(20 * time.Millisecond)
	f.c.Close()
	if n := stop(); n > 0 {
		t.Fatalf("%d in-flight requests failed on a pool closed by shutdown", n)
	}
	f.wantDisabled(t, "/api/observations", "/observations", "/doctor/orphans/list")
}
