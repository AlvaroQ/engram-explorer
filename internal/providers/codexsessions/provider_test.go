package codexsessions_test

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/config"
	"github.com/AlvaroQ/engram-explorer/internal/providers"
	"github.com/AlvaroQ/engram-explorer/internal/providers/codexsessions"
	"github.com/AlvaroQ/engram-explorer/internal/providers/providertest"
	"github.com/AlvaroQ/engram-explorer/internal/usererr"
)

const rollout = `{"timestamp":"2025-10-01T10:00:00.000Z","type":"session_meta","payload":{"id":"0199a000-0000-7000-8000-000000000001","timestamp":"2025-10-01T10:00:00.000Z","cwd":"/work/proj","cli_version":"0.50.0"}}
{"timestamp":"2025-10-01T10:00:01.000Z","type":"turn_context","payload":{"model":"gpt-5","cwd":"/work/proj"}}
{"timestamp":"2025-10-01T10:00:02.000Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"hello codex"}]}}
{"timestamp":"2025-10-01T10:00:03.000Z","type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"hi"}]}}
`

// codexHome creates <tmp>/codex/sessions/2025/10/01/rollout-....jsonl.
func codexHome(t *testing.T) string {
	t.Helper()
	home := filepath.Join(t.TempDir(), "codex")
	day := filepath.Join(home, "sessions", "2025", "10", "01")
	if err := os.MkdirAll(day, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(day, "rollout-2025-10-01T10-00-00-0199a000-0000-7000-8000-000000000001.jsonl"), []byte(rollout), 0o644); err != nil {
		t.Fatal(err)
	}
	return home
}

func TestProvider_Identity(t *testing.T) {
	p := codexsessions.NewProvider(config.Config{})
	if p.ID() != "codex-sessions" || p.DisplayName() != "Codex" || p.ProviderTier() != providers.Tier1 || p.Featured() {
		t.Fatalf("identity = %q %q %v %v", p.ID(), p.DisplayName(), p.ProviderTier(), p.Featured())
	}
	if got := p.Nav(nil); !reflect.DeepEqual(got, providers.AgentSessionsNav()) {
		t.Errorf("Nav = %+v, want the shared agent-sessions group", got)
	}
}

// TestProvider_DetectAndValidate: the home is valid when <home>/sessions is a
// readable directory; Validate errors are user-facing and name only the path.
func TestProvider_DetectAndValidate(t *testing.T) {
	ctx := context.Background()
	p := codexsessions.NewProvider(config.Config{})
	good := codexHome(t)
	fileSessions := t.TempDir()
	if err := os.WriteFile(filepath.Join(fileSessions, "sessions"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, path, reason, msg string
	}{
		{"valid", good, "ok", ""},
		{"empty", "", "dir-missing", "codex home path is required"},
		{"missing", filepath.Join(t.TempDir(), "nope"), "dir-missing", "codex sessions directory not found: "},
		{"sessions is a file", fileSessions, "dir-missing", "not a directory: "},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := providers.ProviderConfig{Path: tc.path}
			if d := p.Detect(ctx, cfg); d.Available != (tc.reason == "ok") || d.Reason != tc.reason || d.Path != tc.path {
				t.Errorf("Detect = %+v, want reason %q", d, tc.reason)
			}
			err := p.Validate(ctx, cfg)
			if tc.msg == "" {
				if err != nil {
					t.Errorf("Validate = %v, want nil", err)
				}
				return
			}
			if msg, ok := usererr.Message(err); !ok || !strings.Contains(msg, tc.msg) {
				t.Errorf("Validate = %v, want a user error containing %q", err, tc.msg)
			}
		})
	}
}

type captureHandler struct {
	mu   sync.Mutex
	recs []slog.Level
}

func (h *captureHandler) Enabled(context.Context, slog.Level) bool { return true }
func (h *captureHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.recs = append(h.recs, r.Level)
	return nil
}
func (h *captureHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *captureHandler) WithGroup(string) slog.Handler      { return h }
func (h *captureHandler) count(l slog.Level) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	n := 0
	for _, r := range h.recs {
		if r == l {
			n++
		}
	}
	return n
}

// TestProvider_OpenServesOneCodexSource: the opened instance exposes one
// agent-session source ("codex") listing the rollouts; health follows the
// sessions directory and logs once per transition through the registry logger.
func TestProvider_OpenServesOneCodexSource(t *testing.T) {
	ctx := context.Background()
	home := codexHome(t)
	logs := &captureHandler{}
	reg := providers.NewRegistry(slog.New(logs))
	reg.Register(codexsessions.NewProvider(config.Config{}))
	reg.Boot(ctx, providertest.NewFakeProfile(map[string]providers.ProviderConfig{"codex-sessions": {Enabled: true, Path: home}}))
	t.Cleanup(func() { reg.Shutdown(ctx) })

	inst := reg.Instance("codex-sessions")
	as, ok := inst.(providers.AgentSessionSource)
	if !ok {
		t.Fatalf("instance %T does not implement providers.AgentSessionSource", inst)
	}
	srcs := as.AgentSessionSources()
	if len(srcs) != 1 || srcs[0].ID() != "codex" || srcs[0].Agent() != "codex" {
		t.Fatalf("sources = %v", srcs)
	}
	ss, err := srcs[0].List(ctx)
	if err != nil || len(ss) != 1 || ss[0].FirstPrompt != "hello codex" || ss[0].Project != "proj" {
		t.Fatalf("List = %+v, %v", ss, err)
	}

	if h := inst.Health(ctx); !h.OK || h.ID != "codex-sessions" || h.Path != home {
		t.Fatalf("Health = %+v", h)
	}
	if err := os.RemoveAll(filepath.Join(home, "sessions")); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if h := inst.Health(ctx); h.OK || h.Error != "codex sessions directory unavailable" {
			t.Fatalf("Health without sessions dir = %+v", h)
		}
	}
	if n := logs.count(slog.LevelWarn); n != 1 {
		t.Errorf("3 failing checks logged %d warnings, want 1", n)
	}
}
