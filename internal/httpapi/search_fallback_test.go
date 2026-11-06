package httpapi_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/config"
	"github.com/AlvaroQ/engram-explorer/internal/httpapi"
	"github.com/AlvaroQ/engram-explorer/internal/providers"
	engramprovider "github.com/AlvaroQ/engram-explorer/internal/providers/engram"
)

// searchContainers seeds the standard test DB plus FTS-indexed rows, then
// returns the legacy and the registry (provider) containers over it.
func searchContainers(t *testing.T, seed func(db *sql.DB)) map[string]http.Handler {
	t.Helper()
	path := seedEngramDB(t)
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	seed(db)
	if _, err := db.Exec(`INSERT INTO observations_fts(observations_fts) VALUES ('rebuild')`); err != nil {
		t.Fatalf("rebuild fts: %v", err)
	}
	db.Close()

	cfg := config.Config{
		Host: "127.0.0.1", Port: 8787, EngramDbPath: path,
		DaemonBaseURL: "http://127.0.0.1:7437", DaemonTimeoutMs: 100,
		Env: "development", ExposeDetails: true,
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	legacy, err := httpapi.NewContainer(cfg, logger)
	if err != nil {
		t.Fatalf("NewContainer: %v", err)
	}
	t.Cleanup(legacy.Close)

	reg := providers.NewRegistry(logger)
	reg.Register(engramprovider.NewProvider(cfg))
	reg.Boot(context.Background(), httpapi.AdaptProfile(config.ProfileFromConfig(cfg)))
	withReg := httpapi.NewContainerWithRegistry(reg, cfg, logger)
	t.Cleanup(withReg.Close)

	return map[string]http.Handler{
		"legacy":   httpapi.NewServeMux(legacy),
		"registry": httpapi.NewServeMux(withReg),
	}
}

func seedAuthTimeout(t *testing.T) func(db *sql.DB) {
	return func(db *sql.DB) {
		for _, s := range []string{
			`INSERT INTO observations (id, session_id, type, title, content, project, created_at, updated_at)
			 VALUES (10, 'sess-1', 'bugfix', 'Auth bug in login', 'token expired', 'myproject', '2026-02-01 10:00:00', '2026-02-01 10:00:00')`,
			`INSERT INTO observations (id, session_id, type, title, content, project, created_at, updated_at)
			 VALUES (11, 'sess-1', 'bugfix', 'Request timeout', 'gateway slow', 'myproject', '2026-02-02 10:00:00', '2026-02-02 10:00:00')`,
		} {
			if _, err := db.Exec(s); err != nil {
				t.Fatalf("seed: %v", err)
			}
		}
	}
}

func decodeBody(t *testing.T, body string) map[string]json.RawMessage {
	t.Helper()
	var m map[string]json.RawMessage
	if err := json.Unmarshal([]byte(body), &m); err != nil {
		t.Fatalf("decode %q: %v", body, err)
	}
	return m
}

func itemCount(t *testing.T, raw json.RawMessage) int {
	t.Helper()
	var items []map[string]any
	if err := json.Unmarshal(raw, &items); err != nil {
		t.Fatalf("items %s: %v", raw, err)
	}
	return len(items)
}

func TestObservationsSearchAPI_ReportsORFallback(t *testing.T) {
	for name, h := range searchContainers(t, seedAuthTimeout(t)) {
		t.Run(name, func(t *testing.T) {
			rec := doGet(t, h, "/api/observations/search?q=auth+timeout")
			if rec.Code != http.StatusOK {
				t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
			}
			body := decodeBody(t, rec.Body.String())
			if n := itemCount(t, body["items"]); n != 2 {
				t.Errorf("items: got %d, want 2", n)
			}
			if string(body["matched_any"]) != "true" {
				t.Errorf("matched_any: got %s, want true (body %s)", body["matched_any"], rec.Body.String())
			}
		})
	}
}

// The response shape is unchanged when every term matches: only "items".
func TestObservationsSearchAPI_ExactMatchKeepsShape(t *testing.T) {
	for name, h := range searchContainers(t, seedAuthTimeout(t)) {
		t.Run(name, func(t *testing.T) {
			rec := doGet(t, h, "/api/observations/search?q=auth+bug")
			if rec.Code != http.StatusOK {
				t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
			}
			body := decodeBody(t, rec.Body.String())
			if len(body) != 1 || itemCount(t, body["items"]) != 1 {
				t.Errorf("body: got %s, want only items with 1 row", rec.Body.String())
			}
		})
	}
}

func TestObservationsListAPI_ORFallbackAndMatchAnyParam(t *testing.T) {
	h := searchContainers(t, seedAuthTimeout(t))["legacy"]

	rec := doGet(t, h, "/api/observations?q=auth+timeout&limit=1")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	body := decodeBody(t, rec.Body.String())
	if string(body["matched_any"]) != "true" || itemCount(t, body["items"]) != 1 {
		t.Fatalf("page 1: got %s, want 1 item and matched_any true", rec.Body.String())
	}
	var next string
	if err := json.Unmarshal(body["nextCursor"], &next); err != nil || next == "" {
		t.Fatalf("nextCursor: got %s", body["nextCursor"])
	}

	rec = doGet(t, h, "/api/observations?q=auth+timeout&limit=1&match=any&cursor="+next)
	body = decodeBody(t, rec.Body.String())
	if string(body["matched_any"]) != "true" || itemCount(t, body["items"]) != 1 {
		t.Errorf("page 2: got %s, want 1 item and matched_any true", rec.Body.String())
	}

	// Exact matches keep the previous shape (no matched_any key).
	rec = doGet(t, h, "/api/observations?q=auth+bug")
	if strings.Contains(rec.Body.String(), "matched_any") {
		t.Errorf("exact match: unexpected matched_any in %s", rec.Body.String())
	}
}
