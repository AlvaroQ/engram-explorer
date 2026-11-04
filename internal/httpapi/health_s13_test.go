package httpapi_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/config"
	"github.com/AlvaroQ/engram-explorer/internal/httpapi"
	"github.com/AlvaroQ/engram-explorer/internal/providers"
	ccprovider "github.com/AlvaroQ/engram-explorer/internal/providers/ccsessions"
	engramprovider "github.com/AlvaroQ/engram-explorer/internal/providers/engram"
)

// closedDaemonURL returns a base URL whose port refuses connections: it binds a
// listener, records the address and closes it.
func closedDaemonURL(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()
	ln.Close()
	return "http://" + addr
}

func healthBody(t *testing.T, h http.Handler) map[string]any {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/health", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/health: status %d, want 200", rec.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v (body %s)", err, rec.Body.String())
	}
	return body
}

// TestHealth_DaemonErrorTextHidden covers S13: in production the daemon entry
// of /api/health carries a generic message and the error code, never the
// client error text (dial errors, URLs, "connection refused"). Development
// (ExposeDetails) keeps the detail.
func TestHealth_DaemonErrorTextHidden(t *testing.T) {
	for _, tc := range []struct {
		name       string
		expose     bool
		wantDetail bool
	}{
		{name: "production", expose: false, wantDetail: false},
		{name: "development", expose: true, wantDetail: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			daemonURL := closedDaemonURL(t)
			path := seedEngramDB(t)
			cfg := config.Config{
				EngramDbPath:    path,
				DaemonBaseURL:   daemonURL,
				DaemonTimeoutMs: 500,
				ExposeDetails:   tc.expose,
			}
			c, err := httpapi.NewContainer(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
			if err != nil {
				t.Fatalf("NewContainer: %v", err)
			}
			t.Cleanup(c.Close)

			body := healthBody(t, httpapi.NewServeMux(c))
			daemon, _ := body["daemon"].(map[string]any)
			if daemon["ok"] != false {
				t.Fatalf("daemon.ok: got %v, want false", daemon["ok"])
			}
			msg, _ := daemon["error"].(string)
			if msg == "" {
				t.Fatalf("daemon.error missing: %v", daemon)
			}
			if daemon["code"] != "NETWORK" {
				t.Errorf("daemon.code: got %v, want NETWORK", daemon["code"])
			}
			host := strings.TrimPrefix(daemonURL, "http://")
			leaky := strings.Contains(msg, "dial tcp") || strings.Contains(msg, "connection refused") ||
				strings.Contains(msg, host)
			if leaky == tc.wantDetail {
				return
			}
			if tc.wantDetail {
				t.Errorf("development daemon.error lost the detail: %q", msg)
			} else {
				t.Errorf("production daemon.error leaks client error text: %q", msg)
			}
		})
	}
}

// TestHealth_DBPathFollowsSettingsSwitch covers S13: after the Settings DB
// switch (POST /settings/engram-db) /api/health reports the new path in both
// the legacy db alias and the engram provider entry.
func TestHealth_DBPathFollowsSettingsSwitch(t *testing.T) {
	pathA := makeEngramDB(t, "a")
	pathB := makeEngramDB(t, "b")
	home := t.TempDir()
	cfg := config.Config{ConfigHome: home, EngramDbPath: pathA, DaemonTimeoutMs: 50}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	reg := providers.NewRegistry(logger)
	reg.Register(engramprovider.NewProvider(cfg))
	data := config.ProfileStore{
		Version:       1,
		ActiveProfile: "default",
		Profiles: map[string]config.Profile{
			"default": {Providers: map[string]config.ProviderConfig{"engram": {Enabled: true, Path: pathA}}},
		},
	}
	reg.Boot(context.Background(), httpapi.AdaptProfile(data.Profiles["default"]))
	c := httpapi.NewContainerWithRegistry(reg, cfg, logger)
	c.CloseGrace = 0
	c.ProfileStore = config.NewStore(home, data)
	t.Cleanup(c.Close)
	f := profileFixture{c: c, h: httpapi.NewServeMux(c), home: home}

	if code := f.post(t, "/settings/engram-db", url.Values{"path": {pathB}}); code != http.StatusSeeOther {
		t.Fatalf("engram-db: status %d, want 303", code)
	}

	body := healthBody(t, f.h)
	db, _ := body["db"].(map[string]any)
	if db["path"] != pathB {
		t.Errorf("db.path: got %v, want %q", db["path"], pathB)
	}
	provs, _ := body["providers"].([]any)
	for _, p := range provs {
		entry, _ := p.(map[string]any)
		if entry["id"] == "engram" && entry["path"] != pathB {
			t.Errorf("providers[engram].path: got %v, want %q", entry["path"], pathB)
		}
	}
}

// TestHealth_ProviderErrorTextHidden covers S13: when the Claude Code
// projects directory disappears, providers[].error in /api/health carries a
// generic message in production, never the OS error text or the path;
// development (ExposeDetails) keeps the detail.
func TestHealth_ProviderErrorTextHidden(t *testing.T) {
	for _, tc := range []struct {
		name   string
		expose bool
	}{{"production", false}, {"development", true}} {
		t.Run(tc.name, func(t *testing.T) {
			ccDir := filepath.Join(makeClaudeDir(t), "projects")
			if err := os.MkdirAll(filepath.Join(ccDir, "-Users-me-project"), 0o755); err != nil {
				t.Fatal(err)
			}
			cfg := config.Config{ClaudeProjectsDir: ccDir, ExposeDetails: tc.expose, DaemonBaseURL: closedDaemonURL(t), DaemonTimeoutMs: 200}
			logger := slog.New(slog.NewTextHandler(io.Discard, nil))
			reg := providers.NewRegistry(logger)
			reg.Register(ccprovider.NewProvider(cfg))
			reg.Boot(context.Background(), httpapi.AdaptProfile(config.Profile{Providers: map[string]config.ProviderConfig{
				"cc-sessions": {Enabled: true, Path: ccDir},
			}}))
			c := httpapi.NewContainerWithRegistry(reg, cfg, logger)
			t.Cleanup(c.Close)
			if err := os.RemoveAll(ccDir); err != nil {
				t.Fatal(err)
			}

			body := healthBody(t, httpapi.NewServeMux(c))
			provs, _ := body["providers"].([]any)
			var msg string
			for _, p := range provs {
				if e, _ := p.(map[string]any); e["id"] == "cc-sessions" {
					msg, _ = e["error"].(string)
				}
			}
			if msg == "" {
				t.Fatalf("providers: %v, want a cc-sessions entry with an error", provs)
			}
			leaks := strings.Contains(msg, "no such file") || strings.Contains(msg, ccDir) || strings.Contains(msg, "stat ")
			if !tc.expose && leaks {
				t.Errorf("production providers[cc-sessions].error = %q, want a generic message", msg)
			}
			if tc.expose && !leaks {
				t.Errorf("development providers[cc-sessions].error = %q, want the detail", msg)
			}
		})
	}
}
