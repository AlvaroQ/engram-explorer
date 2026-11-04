package codex_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/agentsessions"
	"github.com/AlvaroQ/engram-explorer/internal/agentsessions/codex"
)

func TestSourceImplementsPort(t *testing.T) {
	var _ agentsessions.Source = codex.New("codex", t.TempDir())
}

func TestDetailEnvelopePairsToolsByCallID(t *testing.T) {
	src := codex.New("codex", filepath.Join("testdata", "home"))
	d, err := src.Detail(context.Background(), keyEnvelope)
	if err != nil {
		t.Fatal(err)
	}
	if d.Session.Key != keyEnvelope || d.Session.Usage.Total() != 5200 {
		t.Fatalf("detail session = %+v", d.Session)
	}
	var roles []string
	for _, tr := range d.Turns {
		roles = append(roles, tr.Role)
	}
	if !reflect.DeepEqual(roles, []string{"user", "user", "user", "assistant"}) {
		t.Fatalf("roles = %v", roles)
	}
	if d.Turns[2].Blocks[0].Text != "List the Go packages and run the tests" || !d.Turns[2].Timestamp.Equal(ts("2025-10-01T10:00:01Z")) {
		t.Fatalf("user turn = %+v", d.Turns[2])
	}
	a := d.Turns[3]
	if a.Model != "gpt-5-codex" {
		t.Fatalf("assistant model = %q", a.Model)
	}
	want := []agentsessions.Block{
		{Kind: agentsessions.BlockReasoning, Text: "**Exploring** the repo layout"},
		{Kind: agentsessions.BlockToolCall, ToolName: "shell", CallID: "call_A",
			Input: `{"command": ["bash", "-lc", "go list ./..."]}`, Output: "internal/agentsessions\n", Paired: true},
		{Kind: agentsessions.BlockToolCall, ToolName: "apply_patch", CallID: "call_B",
			Input: "*** Begin Patch\n*** End Patch", Output: "Success", Paired: true},
		{Kind: agentsessions.BlockToolCall, ToolName: "local_shell", CallID: "call_C", Input: "go test ./..."},
		{Kind: agentsessions.BlockToolResult, CallID: "call_Z", Output: "orphan output"},
		{Kind: agentsessions.BlockToolCall, ToolName: "web_search", Input: "codex rollout format"},
		{Kind: agentsessions.BlockText, Text: "All tests pass."},
	}
	if !reflect.DeepEqual(a.Blocks, want) {
		t.Fatalf("assistant blocks =\n%+v\nwant\n%+v", a.Blocks, want)
	}
}

func TestDetailLegacyMarksFailedShell(t *testing.T) {
	d, err := codex.New("codex", filepath.Join("testdata", "home")).Detail(context.Background(), keyLegacy)
	if err != nil {
		t.Fatal(err)
	}
	last := d.Turns[len(d.Turns)-1]
	want := []agentsessions.Block{
		{Kind: agentsessions.BlockReasoning, Text: "Checking test output"},
		{Kind: agentsessions.BlockToolCall, ToolName: "shell", CallID: "call_L", Input: `{"command":["go","test"]}`,
			Output: "FAIL", IsError: true, Paired: true},
		{Kind: agentsessions.BlockText, Text: "Fixed."},
	}
	if last.Role != "assistant" || !reflect.DeepEqual(last.Blocks, want) {
		t.Fatalf("legacy assistant turn = %+v", last)
	}
}

func TestDetailRejectsInvalidKeys(t *testing.T) {
	home := t.TempDir()
	sessions := filepath.Join(home, "sessions", "2025", "10", "01")
	if err := os.MkdirAll(sessions, 0o755); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(home, "secret.jsonl")
	writeFile(t, outside, `{"timestamp":"2025-10-01T10:00:00Z","type":"session_meta","payload":{"id":"x"}}`+"\n")
	if err := os.Symlink(outside, filepath.Join(sessions, "rollout-link.jsonl")); err != nil {
		t.Fatal(err)
	}
	src := codex.New("codex", home)
	if ss, err := src.List(context.Background()); err != nil || len(ss) != 0 {
		t.Fatalf("symlinked rollouts must not be listed: %+v, %v", ss, err)
	}
	for _, key := range []string{
		"", "../secret.jsonl", "/etc/passwd.jsonl", "2025/10/01/../../../../secret.jsonl",
		`2025\10\01\rollout-x.jsonl`, "2025/10/01/rollout-x.txt", "./2025/10/01/rollout-x.jsonl",
		"2025//10/rollout-x.jsonl", "2025/10/01/rollout-link.jsonl", "2025/10/01/rollout-missing.jsonl",
		"..", "2025/10/01/rollout-x.jsonl\x00",
	} {
		d, err := src.Detail(context.Background(), key)
		if d != nil || !errors.Is(err, agentsessions.ErrNotFound) {
			t.Errorf("Detail(%q) = %v, %v; want ErrNotFound", key, d, err)
		}
	}
	if _, err := src.Detail(context.Background(), "../secret.jsonl"); !errors.Is(err, agentsessions.ErrInvalidKey) {
		t.Errorf("traversal must be ErrInvalidKey, got %v", err)
	}
}
