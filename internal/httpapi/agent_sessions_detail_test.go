package httpapi_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/config"
	"github.com/AlvaroQ/engram-explorer/internal/httpapi"
	"github.com/AlvaroQ/engram-explorer/internal/providers"
	ccprovider "github.com/AlvaroQ/engram-explorer/internal/providers/ccsessions"
	codexprovider "github.com/AlvaroQ/engram-explorer/internal/providers/codexsessions"
)

// claudeFixtureTranscript: one human prompt, reasoning, a paired tool call
// and an unpaired tool result.
const claudeFixtureTranscript = `{"type":"user","timestamp":"2026-01-02T10:00:00Z","cwd":"/tmp/alpha","gitBranch":"main","version":"2.0.1","message":{"role":"user","content":"Fix the bug"}}
{"type":"assistant","timestamp":"2026-01-02T10:00:02Z","message":{"id":"m1","role":"assistant","model":"claude-opus-4-8","content":[{"type":"thinking","thinking":"pondering"},{"type":"tool_use","id":"tu1","name":"Read","input":{"path":"a.go"}}],"usage":{"input_tokens":100,"output_tokens":50}}}
{"type":"user","timestamp":"2026-01-02T10:00:04Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"tu1","content":"file contents"}]}}
{"type":"user","timestamp":"2026-01-02T10:00:05Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"tu-orphan","content":"lost output"}]}}
`

const codexFixtureRollout = `{"timestamp":"2025-10-01T10:00:00.000Z","type":"session_meta","payload":{"id":"0199a000-0000-7000-8000-000000000001","timestamp":"2025-10-01T10:00:00.000Z","cwd":"/work/proj","cli_version":"0.50.0"}}
{"timestamp":"2025-10-01T10:00:01.000Z","type":"turn_context","payload":{"model":"gpt-5","cwd":"/work/proj"}}
{"timestamp":"2025-10-01T10:00:02.000Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"hello codex"}]}}
{"timestamp":"2025-10-01T10:00:03.000Z","type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":1000,"cached_input_tokens":400,"output_tokens":200,"reasoning_output_tokens":50,"total_tokens":1200}}}}
{"timestamp":"2025-10-01T10:00:04.000Z","type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"hi from codex"}]}}
`

const (
	claudeDetailURL = "/agent-sessions/claude-code/default/-tmp-alpha/s1"
	codexKey        = "2025/10/01/rollout-2025-10-01T10-00-00-0199a000-0000-7000-8000-000000000001.jsonl"
	codexDetailURL  = "/agent-sessions/codex/" + codexKey
)

func writeFixtureFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// newAgentFixture boots the real Claude Code and Codex providers, both
// enabled, each with one session; the handler is built once.
func newAgentFixture(t *testing.T) profileFixture {
	t.Helper()
	home := t.TempDir()
	ccDir := filepath.Join(t.TempDir(), "projects")
	writeFixtureFile(t, filepath.Join(ccDir, "-tmp-alpha", "s1.jsonl"), claudeFixtureTranscript)
	codexHome := filepath.Join(t.TempDir(), "codex")
	writeFixtureFile(t, filepath.Join(codexHome, "sessions", filepath.FromSlash(codexKey)), codexFixtureRollout)
	// A rollout-shaped file outside the sessions dir must never be served.
	writeFixtureFile(t, filepath.Join(codexHome, "secret.jsonl"), `{"secret":"do-not-serve"}`)

	cfg := config.Config{ConfigHome: home, ClaudeProjectsDir: ccDir, CodexHome: codexHome}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	reg := providers.NewRegistry(logger)
	reg.Register(ccprovider.NewProvider(cfg))
	reg.Register(codexprovider.NewProvider(cfg))
	data := config.ProfileStore{
		Version: 1, ActiveProfile: "default",
		Profiles: map[string]config.Profile{"default": {Providers: map[string]config.ProviderConfig{
			"cc-sessions":    {Enabled: true, Path: ccDir},
			"codex-sessions": {Enabled: true, Path: codexHome},
		}}},
		CCAccounts: []config.CCAccount{{ID: "default", Label: "Personal", Path: ccDir, Enabled: true}},
	}
	reg.Boot(context.Background(), httpapi.AdaptProfile(data.Profiles["default"]))
	c := httpapi.NewContainerWithRegistry(reg, cfg, logger)
	c.CloseGrace = 0
	c.ProfileStore = config.NewStore(home, data)
	t.Cleanup(c.Close)
	return profileFixture{c: c, h: httpapi.NewServeMux(c), home: home}
}

// TestAgentSessionDetail_ProviderRoutesRenderNeutralDetail covers S1 + S7:
// each provider mounts its own source-scoped detail route, both render the
// same neutral page (reasoning collapsed, tool call with paired output,
// unpaired results marked) inside the registry sidebar.
func TestAgentSessionDetail_ProviderRoutesRenderNeutralDetail(t *testing.T) {
	f := newAgentFixture(t)

	w := f.fetch(t, claudeDetailURL)
	body := w.Body.String()
	if w.Code != http.StatusOK {
		t.Fatalf("GET %s: status %d (body %.300s)", claudeDetailURL, w.Code, body)
	}
	for _, want := range []string{
		`<h1 class="cc-detail-title">Fix the bug</h1>`, `class="sidebar-nav"`,
		`data-cc-block="thinking"`, "pondering", ">Read<", "file contents", `data-tool-output`,
		`data-unpaired="result"`, "lost output", "Claude Code",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Claude detail lacks %q", want)
		}
	}
	if strings.Contains(body, `data-unpaired="call"`) {
		t.Error("the paired Claude tool call is marked unpaired")
	}

	req := httptest.NewRequest(http.MethodGet, claudeDetailURL, nil)
	req.Header.Set("HX-Request", "true")
	hx := httptest.NewRecorder()
	f.h.ServeHTTP(hx, req)
	if hx.Code != http.StatusOK || strings.Contains(strings.ToLower(hx.Body.String()), "<!doctype") || !strings.Contains(hx.Body.String(), "file contents") {
		t.Errorf("htmx detail: status %d, want the bare detail content (body %.200s)", hx.Code, hx.Body.String())
	}

	w = f.fetch(t, codexDetailURL)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "hello codex") || !strings.Contains(w.Body.String(), "hi from codex") {
		t.Fatalf("GET codex detail: status %d (body %.300s)", w.Code, w.Body.String())
	}
}

// TestAgentSessionDetail_InvalidKeysAre404: unknown sources, unknown sessions
// and traversal attempts answer 404 and never serve a file outside the
// sessions directories.
func TestAgentSessionDetail_InvalidKeysAre404(t *testing.T) {
	f := newAgentFixture(t)
	for _, route := range []string{
		"/agent-sessions/claude-code/nobody/-tmp-alpha/s1",
		"/agent-sessions/claude-code/default/-tmp-alpha/missing",
		"/agent-sessions/claude-code/default/-tmp-alpha",
		"/agent-sessions/claude-code/default/..%2F..%2Fetc/passwd",
		"/agent-sessions/claude-code/default/-tmp-alpha/s1/extra",
		"/agent-sessions/codex/2025/10/01/missing.jsonl",
		"/agent-sessions/codex/..%2Fsecret.jsonl",
		"/agent-sessions/codex/2025/..%2F..%2F..%2Fsecret.jsonl",
		"/agent-sessions/codex/%2Fetc%2Fpasswd",
	} {
		w := f.fetch(t, route)
		if w.Code != http.StatusNotFound {
			t.Errorf("GET %s: status %d, want 404 (body %.200s)", route, w.Code, w.Body.String())
		}
		if strings.Contains(w.Body.String(), "do-not-serve") {
			t.Errorf("GET %s served a file outside the sessions directory", route)
		}
	}
}

// TestAgentSessionDetail_DisabledProviderAnswers503: a disabled provider's
// detail route answers the standard disabled-module page; the other agent
// keeps serving.
func TestAgentSessionDetail_DisabledProviderAnswers503(t *testing.T) {
	f := newAgentFixture(t)
	f.toggle(t, "codex-sessions", false)
	f.wantDisabled(t, codexDetailURL)
	f.wantOK(t, claudeDetailURL)
	f.toggle(t, "codex-sessions", true)
	f.wantOK(t, codexDetailURL)
}
