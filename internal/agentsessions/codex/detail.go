package codex

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/AlvaroQ/engram-explorer/internal/agentsessions"
)

var _ agentsessions.Source = (*Source)(nil)

// turnBuilder maps response items to turns. User/developer messages open a
// new turn; assistant messages, reasoning and tool calls join the current
// assistant turn. Outputs are folded into their call by call_id (S5).
type turnBuilder struct {
	turns   []agentsessions.Turn
	model   string
	pending map[string][2]int // call_id -> (turn, block) of an unpaired call
}

func (b *turnBuilder) turnContext(_ time.Time, tc turnContext) {
	if tc.Model != "" {
		b.model = tc.Model
	}
}

// current returns the index of the open turn with role, opening one if the
// last turn has another role.
func (b *turnBuilder) current(ts time.Time, role string) int {
	if n := len(b.turns); n > 0 && b.turns[n-1].Role == role {
		return n - 1
	}
	t := agentsessions.Turn{Role: role, Timestamp: ts}
	if role == "assistant" {
		t.Model = b.model
	}
	b.turns = append(b.turns, t)
	return len(b.turns) - 1
}

func (b *turnBuilder) add(ts time.Time, role string, blk agentsessions.Block) (int, int) {
	i := b.current(ts, role)
	b.turns[i].Blocks = append(b.turns[i].Blocks, blk)
	return i, len(b.turns[i].Blocks) - 1
}

func (b *turnBuilder) call(ts time.Time, name, input, callID string) {
	i, j := b.add(ts, "assistant", agentsessions.Block{
		Kind: agentsessions.BlockToolCall, ToolName: name, CallID: callID, Input: input,
	})
	if callID != "" {
		b.pending[callID] = [2]int{i, j}
	}
}

func (b *turnBuilder) item(ts time.Time, it item) {
	switch it.Type {
	case "message":
		text := contentText(it.Content)
		if text == "" {
			return
		}
		blk := agentsessions.Block{Kind: agentsessions.BlockText, Text: text}
		if it.Role == "assistant" {
			b.add(ts, "assistant", blk)
			return
		}
		b.turns = append(b.turns, agentsessions.Turn{Role: it.Role, Timestamp: ts, Blocks: []agentsessions.Block{blk}})
	case "reasoning":
		if text := summaryText(it.Summary); text != "" {
			b.add(ts, "assistant", agentsessions.Block{Kind: agentsessions.BlockReasoning, Text: text})
		}
	case "function_call":
		b.call(ts, it.Name, it.Arguments, it.CallID)
	case "custom_tool_call":
		b.call(ts, it.Name, it.Input, it.CallID)
	case "local_shell_call":
		b.call(ts, "local_shell", strings.Join(it.Action.Command, " "), it.CallID)
	case "web_search_call":
		b.add(ts, "assistant", agentsessions.Block{Kind: agentsessions.BlockToolCall, ToolName: "web_search", Input: it.Action.Query})
	case "function_call_output", "custom_tool_call_output":
		out, isErr := outputText(it.Output)
		if loc, ok := b.pending[it.CallID]; ok && it.CallID != "" {
			delete(b.pending, it.CallID)
			blk := &b.turns[loc[0]].Blocks[loc[1]]
			blk.Output, blk.IsError, blk.Paired = out, isErr, true
			return
		}
		role := "tool"
		if n := len(b.turns); n > 0 && b.turns[n-1].Role == "assistant" {
			role = "assistant"
		}
		b.add(ts, role, agentsessions.Block{Kind: agentsessions.BlockToolResult, CallID: it.CallID, Output: out, IsError: isErr})
	}
}

func (b *turnBuilder) meta(time.Time, sessionMeta)      {}
func (b *turnBuilder) tokenCount(time.Time, tokenUsage) {}
func (b *turnBuilder) record(time.Time, usageRecord)    {}
func (b *turnBuilder) stamp(time.Time)                  {}
func (b *turnBuilder) skip()                            {}

func summaryText(parts []textPart) string {
	b, _ := json.Marshal(parts)
	return contentText(b)
}

// outputText decodes a *_output payload: a plain string (possibly the legacy
// JSON {"output","metadata":{"exit_code"}}), a content array, or an object
// {"content","success"}. isErr reports a non-zero exit code or success=false.
func outputText(raw json.RawMessage) (string, bool) {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		var shell struct {
			Output   *string `json:"output"`
			Metadata struct {
				ExitCode int `json:"exit_code"`
			} `json:"metadata"`
		}
		if json.Unmarshal([]byte(s), &shell) == nil && shell.Output != nil {
			return *shell.Output, shell.Metadata.ExitCode != 0
		}
		return s, false
	}
	if t := contentText(raw); t != "" {
		return t, false
	}
	var obj struct {
		Content *string `json:"content"`
		Success *bool   `json:"success"`
	}
	if json.Unmarshal(raw, &obj) == nil && obj.Content != nil {
		return *obj.Content, obj.Success != nil && !*obj.Success
	}
	return string(raw), false
}

// validKey accepts only clean, relative, slash-separated .jsonl paths.
func validKey(key string) bool {
	if key == "" || strings.ContainsAny(key, "\\:\x00") || strings.HasPrefix(key, "/") ||
		!strings.HasSuffix(key, ".jsonl") || path.Clean(key) != key {
		return false
	}
	for _, seg := range strings.Split(key, "/") {
		if seg == "" || seg == "." || seg == ".." {
			return false
		}
	}
	return filepath.IsLocal(filepath.FromSlash(key))
}

// Detail implements agentsessions.Source. The key is the rollout path
// relative to the sessions dir; it must be valid and belong to List.
func (s *Source) Detail(ctx context.Context, key string) (*agentsessions.Detail, error) {
	if !validKey(key) {
		return nil, fmt.Errorf("codex: %w", agentsessions.ErrInvalidKey)
	}
	sessions, err := s.List(ctx)
	if err != nil {
		return nil, err
	}
	var found *agentsessions.Session
	for i := range sessions {
		if sessions[i].Key == key {
			found = &sessions[i]
			break
		}
	}
	if found == nil {
		return nil, fmt.Errorf("codex %s: %w", key, agentsessions.ErrNotFound)
	}
	root, err := filepath.EvalSymlinks(filepath.Join(s.home, "sessions"))
	if err != nil {
		return nil, fmt.Errorf("codex %s: %w", key, agentsessions.ErrNotFound)
	}
	p := filepath.Join(root, filepath.FromSlash(key))
	if fi, err := os.Lstat(p); err != nil || !fi.Mode().IsRegular() {
		return nil, fmt.Errorf("codex %s: %w", key, agentsessions.ErrNotFound)
	}
	tb := &turnBuilder{pending: map[string][2]int{}}
	if err := scanFile(p, tb); err != nil {
		return nil, fmt.Errorf("read codex session %s: %w", key, err)
	}
	if tb.turns == nil {
		tb.turns = []agentsessions.Turn{}
	}
	return &agentsessions.Detail{Session: *found, Turns: tb.turns}, nil
}
