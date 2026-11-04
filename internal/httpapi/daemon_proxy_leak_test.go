package httpapi_test

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/config"
	"github.com/AlvaroQ/engram-explorer/internal/httpapi"
)

// TestDaemonProxy_ErrorTextHidden (S13, same leak class as /api/health): in
// production a failed /api/daemon/* call answers 503 DAEMON_UNAVAILABLE with a
// generic message naming the daemon error code, never the client error text
// (dial errors, "connection refused", the daemon URL), and no details.
// Development (ExposeDetails) keeps the detail.
func TestDaemonProxy_ErrorTextHidden(t *testing.T) {
	for _, tc := range []struct {
		name   string
		expose bool
	}{
		{name: "production", expose: false},
		{name: "development", expose: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			daemonURL := closedDaemonURL(t)
			host := strings.TrimPrefix(daemonURL, "http://")
			c, err := httpapi.NewContainer(config.Config{
				EngramDbPath:    seedEngramDB(t),
				DaemonBaseURL:   daemonURL,
				DaemonTimeoutMs: 500,
				ExposeDetails:   tc.expose,
			}, slog.New(slog.NewTextHandler(io.Discard, nil)))
			if err != nil {
				t.Fatalf("NewContainer: %v", err)
			}
			t.Cleanup(c.Close)
			h := httpapi.NewServeMux(c)

			for _, p := range []string{"/api/daemon/health", "/api/daemon/sync-status", "/api/daemon/stats"} {
				rec := doGet(t, h, p)
				if rec.Code != http.StatusServiceUnavailable {
					t.Fatalf("GET %s: status %d, want 503", p, rec.Code)
				}
				raw := rec.Body.String()
				var env struct {
					Error struct {
						Code    string          `json:"code"`
						Message string          `json:"message"`
						Details json.RawMessage `json:"details"`
					} `json:"error"`
				}
				if err := json.Unmarshal([]byte(raw), &env); err != nil {
					t.Fatalf("GET %s: decode %v (%s)", p, err, raw)
				}
				if env.Error.Code != "DAEMON_UNAVAILABLE" {
					t.Errorf("GET %s: code %q, want DAEMON_UNAVAILABLE", p, env.Error.Code)
				}
				leaky := strings.Contains(raw, "dial tcp") || strings.Contains(raw, "connection refused") ||
					strings.Contains(raw, host)
				if tc.expose {
					if !leaky || len(env.Error.Details) == 0 {
						t.Errorf("GET %s in development lost the detail: %s", p, raw)
					}
					continue
				}
				if leaky || len(env.Error.Details) != 0 {
					t.Errorf("GET %s in production leaks daemon client error text: %s", p, raw)
				}
				if !strings.Contains(env.Error.Message, "NETWORK") {
					t.Errorf("GET %s in production: message %q does not name the error code NETWORK", p, env.Error.Message)
				}
			}
		})
	}
}
