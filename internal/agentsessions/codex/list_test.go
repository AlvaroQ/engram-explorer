package codex_test

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/agentsessions"
	"github.com/AlvaroQ/engram-explorer/internal/agentsessions/codex"
)

const (
	keyEnvelope = "2025/10/01/rollout-2025-10-01T10-00-00-0199a000-0000-7000-8000-00000000000a.jsonl"
	keyRecords  = "2025/10/01/rollout-2025-10-01T14-00-00-0199a000-0000-7000-8000-00000000000b.jsonl"
	keyParent   = "2025/10/02/rollout-2025-10-02T10-00-00-0199a000-0000-7000-8000-00000000000c.jsonl"
	keyFork     = "2025/10/02/rollout-2025-10-02T11-00-00-0199a000-0000-7000-8000-00000000000d.jsonl"
	keyResumed  = "2025/10/03/rollout-2025-10-03T09-00-00-0199a000-0000-7000-8000-00000000000e.jsonl"
	keyLegacy   = "2025/04/18/rollout-2025-04-18T09-00-00-0199a000-0000-7000-8000-00000000000f.jsonl"
)

func listFixture(t *testing.T) map[string]agentsessions.Session {
	t.Helper()
	src := codex.New("codex", filepath.Join("testdata", "home"))
	ss, err := src.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]agentsessions.Session{}
	for _, s := range ss {
		out[s.Key] = s
	}
	return out
}

func TestListWalksOnlyRollouts(t *testing.T) {
	got := listFixture(t)
	var keys []string
	for k := range got {
		keys = append(keys, k)
	}
	want := []string{keyEnvelope, keyRecords, keyParent, keyFork, keyResumed, keyLegacy}
	if len(got) != len(want) {
		t.Fatalf("listed %v, want %v", keys, want)
	}
	for _, k := range want {
		if _, ok := got[k]; !ok {
			t.Fatalf("missing %s in %v", k, keys)
		}
	}
}

func TestEnvelopeSessionFieldsAndDedupedUsage(t *testing.T) {
	s := listFixture(t)[keyEnvelope]
	want := agentsessions.Session{
		Source: "codex", Agent: "codex", Key: keyEnvelope, ID: uuid("0a"), Project: "engram-explorer",
		ProjectKey: "/Users/dev/code/engram-explorer", CWD: "/Users/dev/code/engram-explorer",
		GitBranch: "feat/codex", AgentVersion: "0.46.0", Models: []string{"gpt-5-codex"},
		StartedAt: ts("2025-10-01T10:00:00Z"), EndedAt: ts("2025-10-01T10:00:12Z"),
		FirstPrompt: "List the Go packages and run the tests", TurnCount: 1, MessageCount: 2,
		SkippedLines: 2,
	}
	got := s
	got.Usage, got.ModelUsage, got.SizeBytes = agentsessions.Usage{}, nil, 0
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("session =\n%+v\nwant\n%+v", got, want)
	}
	if s.SizeBytes <= 0 {
		t.Fatalf("SizeBytes = %d", s.SizeBytes)
	}
	// 5,200 cumulative tokens counted once despite the re-emitted token_count rows.
	if got, want := bucketsOf(s.Usage), (buckets{2800, 2000, 0, 400, 100, 5200}); got != want {
		t.Fatalf("usage = %+v, want %+v", got, want)
	}
	// gpt-5-codex: 2800*1.25 + 2000*0.125 + 400*10 per 1M.
	if !near(s.Usage.CostUSD, 0.00775) || s.Usage.UnknownModel {
		t.Fatalf("cost = %v unknown=%v", s.Usage.CostUSD, s.Usage.UnknownModel)
	}
	if mu := s.ModelUsage["gpt-5-codex"]; mu.Total() != 5200 || len(s.ModelUsage) != 1 {
		t.Fatalf("ModelUsage = %+v", s.ModelUsage)
	}
}

func TestTokenUsageRecordsDedupeByResponseID(t *testing.T) {
	s := listFixture(t)[keyRecords]
	// resp_1 counted once; token_count rows ignored when records exist.
	if got, want := bucketsOf(s.Usage), (buckets{1000, 1900, 100, 400, 20, 3400}); got != want {
		t.Fatalf("usage = %+v, want %+v", got, want)
	}
	// gpt-5-mini: (1000+100)*0.25 + 1900*0.025 + 400*2 per 1M.
	if !near(s.Usage.CostUSD, 0.0011225) || s.Usage.UnknownModel {
		t.Fatalf("cost = %v", s.Usage.CostUSD)
	}
	if s.Project != "api" || s.GitBranch != "" || !reflect.DeepEqual(s.Models, []string{"gpt-5-mini"}) {
		t.Fatalf("session = %+v", s)
	}
}

func TestForkReplayCountedOnceAttributedToEarliest(t *testing.T) {
	all := listFixture(t)
	parent, fork := all[keyParent], all[keyFork]
	if got, want := bucketsOf(parent.Usage), (buckets{1500, 1000, 0, 300, 0, 2800}); got != want {
		t.Fatalf("parent usage = %+v, want %+v", got, want)
	}
	if got, want := bucketsOf(fork.Usage), (buckets{0, 1500, 0, 200, 0, 1700}); got != want {
		t.Fatalf("fork usage = %+v, want %+v", got, want)
	}
	if !near(parent.Usage.CostUSD, 0.005) || !near(fork.Usage.CostUSD, 0.0021875) {
		t.Fatalf("costs parent=%v fork=%v", parent.Usage.CostUSD, fork.Usage.CostUSD)
	}
	if fork.ID != uuid("0d") || fork.ForkedFrom != uuid("0c") || parent.ForkedFrom != "" {
		t.Fatalf("fork ids: %q forked from %q", fork.ID, fork.ForkedFrom)
	}
	if fork.TurnCount != 3 || fork.StartedAt != ts("2025-10-02T11:00:00Z") {
		t.Fatalf("fork = %+v", fork)
	}
}

func TestForkAttributionUsesSessionStartNotWalkOrder(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, "sessions", "2025", "10", "02")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	// The fork sorts first on disk; the parent still started earlier.
	copyFile(t, filepath.Join("testdata", "home", "sessions", filepath.FromSlash(keyFork)), filepath.Join(dir, "rollout-a-fork.jsonl"))
	copyFile(t, filepath.Join("testdata", "home", "sessions", filepath.FromSlash(keyParent)), filepath.Join(dir, "rollout-b-parent.jsonl"))
	ss, err := codex.New("c", home).List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	totals := map[string]int64{}
	for _, s := range ss {
		totals[s.ID] = s.Usage.Total()
	}
	if totals[uuid("0c")] != 2800 || totals[uuid("0d")] != 1700 {
		t.Fatalf("totals = %v", totals)
	}
}

func TestResumeInSameFileIsOneSession(t *testing.T) {
	s := listFixture(t)[keyResumed]
	// 1,500 before the resume + 700 after the reset; replayed rows skipped.
	if got, want := bucketsOf(s.Usage), (buckets{1760, 0, 0, 440, 0, 2200}); got != want {
		t.Fatalf("usage = %+v, want %+v", got, want)
	}
	if !s.Usage.UnknownModel || s.Usage.CostUSD != 0 {
		t.Fatalf("o3 must be an unknown model at cost 0: %+v", s.Usage)
	}
	if s.CWD != "/Users/dev/code/scratch" || s.AgentVersion != "0.40.0" {
		t.Fatalf("first session_meta must win: cwd=%q version=%q", s.CWD, s.AgentVersion)
	}
	if s.TurnCount != 2 || !s.StartedAt.Equal(ts("2025-10-03T09:00:00Z")) || !s.EndedAt.Equal(ts("2025-10-04T08:05:00Z")) {
		t.Fatalf("session = %+v", s)
	}
	if !reflect.DeepEqual(s.Models, []string{"o3"}) {
		t.Fatalf("Models = %v", s.Models)
	}
}

func TestLegacyPreEnvelopeFile(t *testing.T) {
	s := listFixture(t)[keyLegacy]
	if s.ID != uuid("0f") || !s.StartedAt.Equal(ts("2025-04-18T09:00:00Z")) || s.EndedAt.Before(s.StartedAt) {
		t.Fatalf("legacy session = %+v", s)
	}
	if s.FirstPrompt != "Fix the failing test" || s.TurnCount != 1 || s.MessageCount != 2 || s.SkippedLines != 0 {
		t.Fatalf("legacy session = %+v", s)
	}
	if s.Usage.Total() != 0 || s.Project != "" {
		t.Fatalf("legacy usage/project = %+v %q", s.Usage, s.Project)
	}
}

func TestListMissingHome(t *testing.T) {
	ss, err := codex.New("codex", filepath.Join(t.TempDir(), "absent")).List(context.Background())
	if err != nil || len(ss) != 0 {
		t.Fatalf("missing home: %v, %v", ss, err)
	}
}

func TestLargeAndOversizedLines(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, "sessions", "2025", "11", "01")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	big := strings.Repeat("x", 2<<20)               // 2 MiB: beyond bufio.Scanner's default
	huge := strings.Repeat("y", codex.MaxLineBytes) // over the cap once wrapped in JSON
	content := strings.Join([]string{
		`{"timestamp":"2025-11-01T00:00:00Z","type":"session_meta","payload":{"id":"big","cwd":"/w/big"}}`,
		`{"timestamp":"2025-11-01T00:00:01Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"` + huge + `"}]}}`,
		`{"timestamp":"2025-11-01T00:00:02Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"` + big + `"}]}}`,
		`{"timestamp":"2025-11-01T00:00:03Z","type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"ok"}]}}`,
	}, "\n")
	writeFile(t, filepath.Join(dir, "rollout-big.jsonl"), content)
	ss, err := codex.New("codex", home).List(context.Background())
	if err != nil || len(ss) != 1 {
		t.Fatalf("list = %v, %v", ss, err)
	}
	s := ss[0]
	if s.FirstPrompt != big || s.SkippedLines != 1 || s.MessageCount != 2 || !s.EndedAt.Equal(ts("2025-11-01T00:00:03Z")) {
		t.Fatalf("prompt len=%d skipped=%d messages=%d ended=%v", len(s.FirstPrompt), s.SkippedLines, s.MessageCount, s.EndedAt)
	}
}

func TestListCacheRefreshesWhenRolloutChanges(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, "sessions", "2025", "11", "02")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "rollout-one.jsonl")
	writeFile(t, file, `{"timestamp":"2025-11-02T00:00:00Z","type":"session_meta","payload":{"id":"one"}}`+"\n")
	src := codex.New("codex", home)
	ctx := context.Background()
	fp1, err := src.Fingerprint(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if fp, _ := src.Fingerprint(ctx); fp != fp1 {
		t.Fatal("fingerprint must be stable")
	}
	if ss, _ := src.List(ctx); len(ss) != 1 || ss[0].TurnCount != 0 {
		t.Fatalf("list = %+v", ss)
	}
	f, err := os.OpenFile(file, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.WriteString(`{"timestamp":"2025-11-02T00:00:01Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"hi"}]}}` + "\n")
	_ = f.Close()
	if fp, _ := src.Fingerprint(ctx); fp == fp1 {
		t.Fatal("fingerprint must change after an append")
	}
	if ss, _ := src.List(ctx); len(ss) != 1 || ss[0].TurnCount != 1 || ss[0].FirstPrompt != "hi" {
		t.Fatalf("cached list not refreshed: %+v", ss)
	}
}
