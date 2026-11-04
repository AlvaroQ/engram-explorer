package httpapi_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"
)

type usageJSON struct {
	Input        int64   `json:"input_tokens"`
	CacheRead    int64   `json:"cache_read_tokens"`
	CacheWrite   int64   `json:"cache_write_tokens"`
	Output       int64   `json:"output_tokens"`
	Reasoning    int64   `json:"reasoning_tokens"`
	CostUSD      float64 `json:"cost_usd"`
	UnknownModel bool    `json:"unknown_model"`
}

type statsJSON struct {
	GeneratedAt     string    `json:"generated_at"`
	Sessions        int       `json:"sessions"`
	HasUnknownModel bool      `json:"has_unknown_model"`
	Totals          usageJSON `json:"totals"`
	ByAgent         []struct {
		Key      string    `json:"key"`
		Sessions int       `json:"sessions"`
		Usage    usageJSON `json:"usage"`
	} `json:"by_agent"`
	OverTime []struct {
		Day     string               `json:"day"`
		ByAgent map[string]usageJSON `json:"by_agent"`
	} `json:"over_time"`
	TopSessions []struct {
		Source string `json:"source"`
		Key    string `json:"key"`
	} `json:"top_sessions"`
}

func decodeStats(t *testing.T, w *httptest.ResponseRecorder) statsJSON {
	t.Helper()
	if w.Code != http.StatusOK {
		t.Fatalf("status %d, want 200 (body %.300s)", w.Code, w.Body.String())
	}
	var s statsJSON
	if err := json.Unmarshal(w.Body.Bytes(), &s); err != nil {
		t.Fatalf("decode: %v (body %.300s)", err, w.Body.String())
	}
	return s
}

// TestAgentSessionsStatsAPI_NeutralBucketsPerAgent covers S1 (and S3 through
// the API): both agents in disjoint buckets, reasoning apart, per agent and
// per day, top sessions with their source and key.
func TestAgentSessionsStatsAPI_NeutralBucketsPerAgent(t *testing.T) {
	f := newAgentFixture(t)
	s := decodeStats(t, f.fetch(t, "/api/agent-sessions/stats"))

	if s.Sessions != 2 || s.HasUnknownModel {
		t.Errorf("sessions %d unknown %v, want 2 sessions, all models priced", s.Sessions, s.HasUnknownModel)
	}
	// Claude: input 100, output 50. Codex: 1000 input of which 400 cached →
	// input 600 + cache_read 400; output 200 incl. 50 reasoning.
	if got := s.Totals; got.Input != 700 || got.CacheRead != 400 || got.CacheWrite != 0 || got.Output != 250 || got.Reasoning != 50 || got.CostUSD <= 0 {
		t.Errorf("totals = %+v", got)
	}
	agents := map[string]usageJSON{}
	for _, g := range s.ByAgent {
		agents[g.Key] = g.Usage
	}
	if len(agents) != 2 || agents["claude-code"].Input != 100 || agents["codex"].CacheRead != 400 || agents["codex"].Reasoning != 50 {
		t.Errorf("by_agent = %+v", s.ByAgent)
	}
	if len(s.OverTime) != 2 || s.OverTime[0].Day != "2025-10-01" || s.OverTime[0].ByAgent["codex"].Output != 200 {
		t.Errorf("over_time = %+v", s.OverTime)
	}
	var tops []string
	for _, ts := range s.TopSessions {
		tops = append(tops, ts.Source+"|"+ts.Key)
	}
	sort.Strings(tops)
	if want := []string{"claude-code/default|-tmp-alpha/s1", "codex|" + codexKey}; !reflect.DeepEqual(tops, want) {
		t.Errorf("top sessions = %v, want %v", tops, want)
	}

	if s := decodeStats(t, f.fetch(t, "/api/agent-sessions/stats?agent=codex")); s.Sessions != 1 || s.Totals.Input != 600 {
		t.Errorf("?agent=codex: %d sessions, totals %+v", s.Sessions, s.Totals)
	}
}

// TestAgentSessionsStatsAPI_CachedByFingerprint: an unchanged tree is served
// from cache (same generated_at); a modified session recomputes.
func TestAgentSessionsStatsAPI_CachedByFingerprint(t *testing.T) {
	f := newAgentFixture(t)
	first := decodeStats(t, f.fetch(t, "/api/agent-sessions/stats"))
	time.Sleep(5 * time.Millisecond)
	if again := decodeStats(t, f.fetch(t, "/api/agent-sessions/stats")); again.GeneratedAt != first.GeneratedAt {
		t.Errorf("unchanged sources recomputed: %s → %s", first.GeneratedAt, again.GeneratedAt)
	}

	ccFile := findFile(t, f, "s1.jsonl")
	extra := `{"type":"assistant","timestamp":"2026-01-02T10:01:00Z","message":{"id":"m9","role":"assistant","model":"claude-opus-4-8","content":[{"type":"text","text":"more"}],"usage":{"input_tokens":5,"output_tokens":5}}}` + "\n"
	fh, err := os.OpenFile(ccFile, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = fh.WriteString(extra)
	fh.Close()
	later := time.Now().Add(time.Minute)
	_ = os.Chtimes(ccFile, later, later)

	if after := decodeStats(t, f.fetch(t, "/api/agent-sessions/stats")); after.GeneratedAt == first.GeneratedAt || after.Totals.Input != 705 {
		t.Errorf("after a change: generated_at %s (was %s), input %d, want recomputed with 705", after.GeneratedAt, first.GeneratedAt, after.Totals.Input)
	}
}

// findFile returns the path of the Claude transcript of the fixture.
func findFile(t *testing.T, f profileFixture, name string) string {
	t.Helper()
	inst := f.c.Registry.Instance("cc-sessions")
	if inst == nil {
		t.Fatal("cc-sessions not enabled")
	}
	p := filepath.Join(inst.Health(t.Context()).Path, "-tmp-alpha", name)
	if _, err := os.Stat(p); err != nil {
		t.Fatal(err)
	}
	return p
}

// TestAgentSessionsStatsAPI_GatedOnAnySource: the API follows the live
// agent-session providers (S7): one disabled leaves the other's sessions;
// none answers 503 MODULE_DISABLED.
func TestAgentSessionsStatsAPI_GatedOnAnySource(t *testing.T) {
	f := newAgentFixture(t)
	f.toggle(t, "codex-sessions", false)
	if s := decodeStats(t, f.fetch(t, "/api/agent-sessions/stats")); s.Sessions != 1 || len(s.ByAgent) != 1 || s.ByAgent[0].Key != "claude-code" {
		t.Errorf("with Codex off: %+v", s.ByAgent)
	}
	f.toggle(t, "cc-sessions", false)
	f.wantDisabled(t, "/api/agent-sessions/stats", "/agent-sessions", "/agent-sessions/list")
}

// TestDeprecatedCCStats_KeepsShapeClaudeOnly: GET /api/cc-sessions/stats keeps
// its exact pre-Phase-3 JSON shape over the Claude sources only, and is marked
// deprecated (Deprecation + successor Link headers).
func TestDeprecatedCCStats_KeepsShapeClaudeOnly(t *testing.T) {
	f := newAgentFixture(t)
	w := f.fetch(t, "/api/cc-sessions/stats")
	if w.Code != http.StatusOK {
		t.Fatalf("status %d (body %.300s)", w.Code, w.Body.String())
	}
	if w.Header().Get("Deprecation") != "true" || !strings.Contains(w.Header().Get("Link"), `</api/agent-sessions/stats>; rel="successor-version"`) {
		t.Errorf("headers Deprecation=%q Link=%q", w.Header().Get("Deprecation"), w.Header().Get("Link"))
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	keys := make([]string, 0, len(raw))
	for k := range raw {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if want := []string{"by_model", "by_project", "generated_at", "has_unknown_model", "over_time", "sessions", "top_sessions", "totals"}; !reflect.DeepEqual(keys, want) {
		t.Errorf("keys = %v, want %v", keys, want)
	}
	var totals map[string]any
	_ = json.Unmarshal(raw["totals"], &totals)
	tk := make([]string, 0, len(totals))
	for k := range totals {
		tk = append(tk, k)
	}
	sort.Strings(tk)
	if want := []string{"cache_read_tokens", "cache_write_1h_tokens", "cache_write_5m_tokens", "cost_usd", "input_tokens", "output_tokens", "total_tokens", "unknown_model"}; !reflect.DeepEqual(tk, want) {
		t.Errorf("totals keys = %v, want %v", tk, want)
	}
	if string(raw["sessions"]) != "1" {
		t.Errorf("sessions = %s, want 1 (Claude only)", raw["sessions"])
	}
	f.toggle(t, "cc-sessions", false)
	f.wantDisabled(t, "/api/cc-sessions/stats")
}

// TestLegacyCCDetail_RedirectsToClaudeDetail: the old Claude detail URL
// answers 301 to the source-scoped detail (default or ?account= account).
func TestLegacyCCDetail_RedirectsToClaudeDetail(t *testing.T) {
	f := newAgentFixture(t)
	for _, tc := range []struct{ from, to string }{
		{"/cc-sessions/-tmp-alpha/s1", claudeDetailURL},
		{"/cc-sessions/-tmp-alpha/s1?account=work", "/agent-sessions/claude-code/work/-tmp-alpha/s1"},
	} {
		w := f.fetch(t, tc.from)
		if w.Code != http.StatusMovedPermanently || w.Header().Get("Location") != tc.to {
			t.Errorf("GET %s: %d → %q, want 301 → %q", tc.from, w.Code, w.Header().Get("Location"), tc.to)
		}
	}
	if w := f.fetch(t, "/cc-sessions/..%2Fx/s1"); w.Code != http.StatusNotFound {
		t.Errorf("legacy detail with an invalid project: status %d, want 404", w.Code)
	}
}

// TestAgentSessionsSidebar_OneGroupForBothProviders: with both agent-session
// providers enabled the sidebar has one "Agent sessions" group with each link
// once.
func TestAgentSessionsSidebar_OneGroupForBothProviders(t *testing.T) {
	f := newAgentFixture(t)
	body := f.fetch(t, "/agent-sessions").Body.String()
	sidebar := sidebarRe.FindString(body)
	if n := strings.Count(sidebar, "sidebar-group-label"); n != 1 || !strings.Contains(sidebar, "sidebar-group-agent-sessions") {
		t.Errorf("sidebar groups = %d, want only the agent-sessions group:\n%s", n, sidebar)
	}
	if strings.Contains(sidebar, "/cc-overview") {
		t.Error("the sidebar still links the legacy /cc-overview page")
	}
	for _, href := range []string{`href="/agent-sessions"`, `href="/agent-sessions/list"`} {
		if n := strings.Count(sidebarRe.FindString(body), href); n != 1 {
			t.Errorf("sidebar links %s = %d, want 1", href, n)
		}
	}
}
