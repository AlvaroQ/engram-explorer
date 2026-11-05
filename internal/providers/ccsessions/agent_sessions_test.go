package ccsessions_test

import (
	"context"
	"errors"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/agentsessions"
	"github.com/AlvaroQ/engram-explorer/internal/config"
	"github.com/AlvaroQ/engram-explorer/internal/providers"
	"github.com/AlvaroQ/engram-explorer/internal/providers/ccsessions"
	"github.com/AlvaroQ/engram-explorer/internal/providers/providertest"
)

// claudeTranscript exercises usage dedupe by message id, the 5m/1h cache-write
// split, an unpriced model, marker-only prompts and tool_use/tool_result pairs.
var claudeTranscript = strings.Join([]string{
	`{"type":"user","timestamp":"2026-01-02T10:00:00Z","sessionId":"s1","cwd":"/tmp/alpha","gitBranch":"main","version":"2.0.1","message":{"role":"user","content":"<ide_opened_file>x.go</ide_opened_file>"}}`,
	`{"type":"user","timestamp":"2026-01-02T10:00:01Z","message":{"role":"user","content":"Fix the bug"}}`,
	`{"type":"assistant","timestamp":"2026-01-02T10:00:02Z","message":{"id":"m1","role":"assistant","model":"claude-opus-4-8","content":[{"type":"thinking","thinking":"hmm"},{"type":"text","text":"Looking"},{"type":"tool_use","id":"tu1","name":"Read","input":{"path":"a.go"}}],"usage":{"input_tokens":100,"output_tokens":50,"cache_read_input_tokens":1000,"cache_creation_input_tokens":300,"cache_creation":{"ephemeral_5m_input_tokens":200,"ephemeral_1h_input_tokens":100}}}}`,
	`{"type":"assistant","timestamp":"2026-01-02T10:00:03Z","message":{"id":"m1","role":"assistant","model":"claude-opus-4-8","content":[{"type":"text","text":"Still looking"}],"usage":{"input_tokens":100,"output_tokens":50,"cache_read_input_tokens":1000,"cache_creation_input_tokens":300,"cache_creation":{"ephemeral_5m_input_tokens":200,"ephemeral_1h_input_tokens":100}}}}`,
	`{"type":"user","timestamp":"2026-01-02T10:00:04Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"tu1","content":"file contents"}]}}`,
	`{"type":"user","timestamp":"2026-01-02T10:00:05Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"tu-orphan","content":[{"type":"text","text":"lost"}]}]}}`,
	`{"type":"assistant","timestamp":"2026-01-02T10:00:06Z","message":{"id":"m2","role":"assistant","model":"mystery-model","content":[{"type":"text","text":"Done"}],"usage":{"input_tokens":10,"output_tokens":0}}}`,
}, "\n") + "\n"

func writeTranscript(t *testing.T, dir, folder, id, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, folder), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, folder, id+".jsonl"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// openSources opens the cc-sessions provider through a registry (path dir,
// optional accounts) and returns the agent-session sources of its instance.
func openSources(t *testing.T, dir string, accounts func() []config.CCAccount) []agentsessions.Source {
	t.Helper()
	var opts []ccsessions.Option
	if accounts != nil {
		opts = append(opts, ccsessions.WithAccounts(accounts))
	}
	reg := providertest.NewRegistry(t, providertest.NewFakeProfile(map[string]providers.ProviderConfig{
		"cc-sessions": {Enabled: true, Path: dir},
	}), ccsessions.NewProvider(config.Config{}, opts...))
	src, ok := reg.Instance("cc-sessions").(providers.AgentSessionSource)
	if !ok {
		t.Fatalf("cc-sessions instance %T does not implement providers.AgentSessionSource", reg.Instance("cc-sessions"))
	}
	return src.AgentSessionSources()
}

func sourceIDs(srcs []agentsessions.Source) []string {
	out := []string{}
	for _, s := range srcs {
		out = append(out, s.ID()+"|"+s.Agent())
	}
	return out
}

// TestAgentSessions_OneSourcePerEnabledAccount: every enabled Claude account
// is one source "claude-code/<account>"; without accounts the provider path
// is the "default" account.
func TestAgentSessions_OneSourcePerEnabledAccount(t *testing.T) {
	dir := seedProjectsDir(t)
	if got := sourceIDs(openSources(t, dir, nil)); !reflect.DeepEqual(got, []string{"claude-code/default|claude-code"}) {
		t.Fatalf("sources without accounts = %v", got)
	}
	accounts := func() []config.CCAccount {
		return []config.CCAccount{
			{ID: "default", Path: dir, Enabled: true},
			{ID: "old", Path: dir, Enabled: false},
			{ID: "work", Path: t.TempDir(), Enabled: true},
		}
	}
	if got := sourceIDs(openSources(t, dir, accounts)); !reflect.DeepEqual(got, []string{"claude-code/default|claude-code", "claude-code/work|claude-code"}) {
		t.Fatalf("sources with accounts = %v", got)
	}
	none := func() []config.CCAccount { return []config.CCAccount{{ID: "off", Path: dir}} }
	if got := sourceIDs(openSources(t, dir, none)); !reflect.DeepEqual(got, []string{"claude-code/default|claude-code"}) {
		t.Fatalf("sources with every account disabled = %v, want the provider path as default", got)
	}
}

// TestAgentSessions_ListMapsUsageAndMetadata: List maps a transcript into the
// neutral session with disjoint buckets, the 5m/1h breakdown, cost, models
// and counts.
func TestAgentSessions_ListMapsUsageAndMetadata(t *testing.T) {
	dir := t.TempDir()
	writeTranscript(t, dir, "-tmp-alpha", "s1", claudeTranscript)
	src := openSources(t, dir, nil)[0]

	ss, err := src.List(context.Background())
	if err != nil || len(ss) != 1 {
		t.Fatalf("List = %d sessions, err %v; want 1", len(ss), err)
	}
	s := ss[0]
	if s.Key != "-tmp-alpha/s1" || s.ID != "s1" || s.Project != "alpha" || s.ProjectKey != "/tmp/alpha" ||
		s.CWD != "/tmp/alpha" || s.GitBranch != "main" || s.AgentVersion != "2.0.1" || s.FirstPrompt != "Fix the bug" {
		t.Errorf("metadata = %+v", s)
	}
	if s.TurnCount != 1 || s.MessageCount != 3 {
		t.Errorf("turns/messages = %d/%d, want 1/3", s.TurnCount, s.MessageCount)
	}
	if s.StartedAt.Format("15:04:05") != "10:00:00" || s.SizeBytes != int64(len(claudeTranscript)) {
		t.Errorf("started %v size %d", s.StartedAt, s.SizeBytes)
	}
	u := s.Usage
	if u.Input != 110 || u.Output != 50 || u.CacheRead != 1000 || u.CacheWrite != 300 || u.Total() != 1460 {
		t.Errorf("usage buckets = %+v, want input 110 output 50 cache_read 1000 cache_write 300", u)
	}
	if !reflect.DeepEqual(u.Breakdown, map[string]int64{"cache_write_5m": 200, "cache_write_1h": 100}) {
		t.Errorf("breakdown = %v", u.Breakdown)
	}
	if want := 0.0045; math.Abs(u.CostUSD-want) > 1e-9 || !u.UnknownModel {
		t.Errorf("cost %v unknown %v, want %v and unknown model", u.CostUSD, u.UnknownModel, want)
	}
	if !reflect.DeepEqual(s.Models, []string{"claude-opus-4-8", "mystery-model"}) {
		t.Errorf("models = %v", s.Models)
	}
	if mu := s.ModelUsage["claude-opus-4-8"]; mu.Input != 100 || mu.UnknownModel || len(s.ModelUsage) != 2 {
		t.Errorf("model usage = %+v", s.ModelUsage)
	}
	if fp, err := src.Fingerprint(context.Background()); err != nil || fp == "" {
		t.Errorf("Fingerprint = %q, %v", fp, err)
	}
}

// TestAgentSessions_DetailPairsToolsAndRejectsBadKeys: Detail folds each
// tool_result into its tool_use, marks unpaired results, maps thinking to
// reasoning, keeps marker text, and answers ErrNotFound for invalid keys.
func TestAgentSessions_DetailPairsToolsAndRejectsBadKeys(t *testing.T) {
	dir := t.TempDir()
	writeTranscript(t, dir, "-tmp-alpha", "s1", claudeTranscript)
	src := openSources(t, dir, nil)[0]

	d, err := src.Detail(context.Background(), "-tmp-alpha/s1")
	if err != nil {
		t.Fatalf("Detail: %v", err)
	}
	if d.Session.ID != "s1" || d.Session.Usage.Input != 110 {
		t.Errorf("detail session = %+v", d.Session)
	}
	var kinds []string
	for _, turn := range d.Turns {
		for _, b := range turn.Blocks {
			kinds = append(kinds, turn.Role+":"+string(b.Kind))
		}
	}
	want := []string{"user:text", "user:text", "assistant:reasoning", "assistant:text", "assistant:tool_call", "assistant:text", "user:tool_result", "assistant:text"}
	if !reflect.DeepEqual(kinds, want) {
		t.Fatalf("blocks = %v, want %v", kinds, want)
	}
	if b := d.Turns[0].Blocks[0]; b.Text != "<ide_opened_file>x.go</ide_opened_file>" {
		t.Errorf("marker text = %q", b.Text)
	}
	if b := d.Turns[2].Blocks[0]; b.Text != "hmm" {
		t.Errorf("reasoning = %q", b.Text)
	}
	call := d.Turns[2].Blocks[2]
	if call.ToolName != "Read" || call.CallID != "tu1" || call.Input != `{"path":"a.go"}` || call.Output != "file contents" || !call.Paired {
		t.Errorf("tool call = %+v", call)
	}
	orphan := d.Turns[4].Blocks[0]
	if orphan.CallID != "tu-orphan" || orphan.Output != "lost" || orphan.Paired {
		t.Errorf("orphan result = %+v", orphan)
	}

	for _, key := range []string{"", "../etc/passwd", "-tmp-alpha/../s1", "a/b/c", "-tmp-alpha", "-tmp-alpha/missing"} {
		if _, err := src.Detail(context.Background(), key); !errors.Is(err, agentsessions.ErrNotFound) {
			t.Errorf("Detail(%q) err = %v, want ErrNotFound", key, err)
		}
	}
}

// TestAgentSessions_MissingDirectoryIsEmpty: an account whose projects
// directory is gone lists no sessions and no error (like the old list page).
func TestAgentSessions_MissingDirectoryIsEmpty(t *testing.T) {
	dir := seedProjectsDir(t)
	gone := filepath.Join(t.TempDir(), "gone")
	accounts := func() []config.CCAccount { return []config.CCAccount{{ID: "default", Path: gone, Enabled: true}} }
	ss, err := openSources(t, dir, accounts)[0].List(context.Background())
	if err != nil || len(ss) != 0 {
		t.Fatalf("List of a missing directory = %d sessions, err %v; want none, nil", len(ss), err)
	}
}
