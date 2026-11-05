package agentsessions_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/AlvaroQ/engram-explorer/internal/agentsessions"
)

// fakeSource is an in-memory Source used by the multi-source tests.
type fakeSource struct {
	id, agent string
	sessions  []agentsessions.Session
	err       error
	panicMsg  string
	fp        string
}

func (f *fakeSource) ID() string    { return f.id }
func (f *fakeSource) Agent() string { return f.agent }
func (f *fakeSource) Fingerprint(context.Context) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	return f.fp, nil
}

func (f *fakeSource) List(context.Context) ([]agentsessions.Session, error) {
	if f.panicMsg != "" {
		panic(f.panicMsg)
	}
	if f.err != nil {
		return nil, f.err
	}
	return f.sessions, nil
}

func (f *fakeSource) Detail(_ context.Context, key string) (*agentsessions.Detail, error) {
	for _, s := range f.sessions {
		if s.Key == key {
			return &agentsessions.Detail{Session: s}, nil
		}
	}
	return nil, agentsessions.ErrNotFound
}

var day = time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)

func sess(key, project string, startH, endH int, models []string, u agentsessions.Usage) agentsessions.Session {
	return agentsessions.Session{
		Key: key, ID: "id-" + key, Project: project, ProjectKey: "/w/" + project, CWD: "/w/" + project,
		Models: models, StartedAt: day.Add(time.Duration(startH) * time.Hour),
		EndedAt: day.Add(time.Duration(endH) * time.Hour), FirstPrompt: "prompt " + key, Usage: u,
	}
}

func fixtureSources() []agentsessions.Source {
	claude := &fakeSource{id: "cc-main", agent: "claude-code", fp: "a", sessions: []agentsessions.Session{
		sess("c1", "engram", 1, 2, []string{"opus"}, agentsessions.Usage{Input: 100, Output: 10, CostUSD: 1}),
		sess("c2", "explorer", 30, 31, []string{"sonnet"}, agentsessions.Usage{Input: 50, CostUSD: 3}),
		sess("c3", "engram", 5, 6, []string{"opus", "haiku"}, agentsessions.Usage{Input: 10, CostUSD: 0.5}),
	}}
	codex := &fakeSource{id: "codex", agent: "codex", fp: "b", sessions: []agentsessions.Session{
		sess("x1", "engram", 3, 4, []string{"gpt-5-codex"}, agentsessions.Usage{Input: 400, CostUSD: 2}),
		sess("x2", "other", 26, 27, []string{"o3"}, agentsessions.Usage{Input: 7, UnknownModel: true}),
	}}
	return []agentsessions.Source{claude, codex}
}

func keys(items []agentsessions.Session) []string {
	out := make([]string, 0, len(items))
	for _, s := range items {
		out = append(out, s.Key)
	}
	return out
}

func TestCollectStampsSourceAndIsolatesFailures(t *testing.T) {
	boom := errors.New("disk on fire")
	srcs := append(fixtureSources(),
		&fakeSource{id: "broken", agent: "codex", err: boom},
		&fakeSource{id: "panicky", agent: "claude-code", panicMsg: "kaboom"},
	)
	c, err := agentsessions.Collect(context.Background(), srcs)
	if err != nil {
		t.Fatalf("a failing source must not fail the merge: %v", err)
	}
	if len(c.Sessions) != 5 {
		t.Fatalf("healthy sources hidden: %d sessions", len(c.Sessions))
	}
	for _, s := range c.Sessions {
		want := map[string]string{"cc-main": "claude-code", "codex": "codex"}[s.Source]
		if want == "" || s.Agent != want {
			t.Fatalf("session %s stamped Source=%q Agent=%q", s.Key, s.Source, s.Agent)
		}
	}
	wantInfos := []agentsessions.SourceInfo{
		{ID: "cc-main", Agent: "claude-code"}, {ID: "codex", Agent: "codex"},
		{ID: "broken", Agent: "codex"}, {ID: "panicky", Agent: "claude-code"},
	}
	if !reflect.DeepEqual(c.Sources, wantInfos) {
		t.Fatalf("Sources = %+v", c.Sources)
	}
	if len(c.Errors) != 2 || c.Errors[0].Source != "broken" || !errors.Is(c.Errors[0], boom) ||
		c.Errors[1].Source != "panicky" || !strings.Contains(c.Errors[1].Error(), "kaboom") {
		t.Fatalf("Errors = %+v", c.Errors)
	}
}

func TestCollectCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := agentsessions.Collect(ctx, fixtureSources()); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

func TestFingerprintCombinesSources(t *testing.T) {
	ctx := context.Background()
	a := &fakeSource{id: "a", agent: "codex", fp: "1"}
	b := &fakeSource{id: "b", agent: "codex", fp: "2"}
	fp1 := agentsessions.Fingerprint(ctx, []agentsessions.Source{a, b})
	if fp1 == "" || fp1 != agentsessions.Fingerprint(ctx, []agentsessions.Source{a, b}) {
		t.Fatal("fingerprint must be stable and non-empty")
	}
	b.fp = "3"
	if agentsessions.Fingerprint(ctx, []agentsessions.Source{a, b}) == fp1 {
		t.Fatal("fingerprint must change when a source changes")
	}
	b.err = errors.New("down")
	fpErr := agentsessions.Fingerprint(ctx, []agentsessions.Source{a, b})
	if fpErr == fp1 {
		t.Fatal("a failing source must change the fingerprint")
	}
}
