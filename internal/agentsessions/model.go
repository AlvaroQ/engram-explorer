// Package agentsessions is the vendor-neutral model for coding-agent sessions
// (Claude Code, Codex, ...). Agent-specific readers map their on-disk formats
// into Session/Detail and expose them through the Source port; the host lists,
// filters, paginates and aggregates many sources with ListSessions and Stats.
//
// The package is a pure library: it must not import the HTTP, UI or provider
// layers.
package agentsessions

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrNotFound is returned by Source.Detail when no session matches the key.
var ErrNotFound = errors.New("agent session not found")

// ErrInvalidKey is returned by Source.Detail for keys that fail validation
// (path traversal, wrong shape). It matches ErrNotFound via errors.Is so hosts
// answer 404 without leaking whether a path exists.
var ErrInvalidKey = fmt.Errorf("%w: invalid session key", ErrNotFound)

// Usage holds token counts in DISJOINT buckets plus the computed cost.
//
// Input is uncached input only; CacheRead and CacheWrite are the cached input
// tokens read from / written to the prompt cache; Output includes reasoning.
// Reasoning is an informative subset of Output and is never added to Total.
type Usage struct {
	Input        int64   `json:"input_tokens"`
	CacheRead    int64   `json:"cache_read_tokens"`
	CacheWrite   int64   `json:"cache_write_tokens"`
	Output       int64   `json:"output_tokens"`
	Reasoning    int64   `json:"reasoning_tokens"`
	CostUSD      float64 `json:"cost_usd"`
	UnknownModel bool    `json:"unknown_model"`
	// Breakdown carries optional agent-specific counters that refine a bucket
	// (e.g. Claude's "cache_write_5m"/"cache_write_1h" split of CacheWrite).
	// It is informative and never feeds Total.
	Breakdown map[string]int64 `json:"breakdown,omitempty"`
}

// Total is Input + CacheRead + CacheWrite + Output (Reasoning excluded).
func (u Usage) Total() int64 {
	return u.Input + u.CacheRead + u.CacheWrite + u.Output
}

// Add folds o into u. UnknownModel is sticky. Breakdown maps are copied on
// write so values sharing a map (struct copies, the argument) never alias.
func (u *Usage) Add(o Usage) {
	u.Input += o.Input
	u.CacheRead += o.CacheRead
	u.CacheWrite += o.CacheWrite
	u.Output += o.Output
	u.Reasoning += o.Reasoning
	u.CostUSD += o.CostUSD
	u.UnknownModel = u.UnknownModel || o.UnknownModel
	if len(o.Breakdown) == 0 {
		return
	}
	merged := make(map[string]int64, len(u.Breakdown)+len(o.Breakdown))
	for k, v := range u.Breakdown {
		merged[k] = v
	}
	for k, v := range o.Breakdown {
		merged[k] += v
	}
	u.Breakdown = merged
}

// Session is one row of the cross-agent session list.
type Session struct {
	// Source is the ID of the Source that produced the session; ListSessions
	// stamps it, so readers may leave it empty.
	Source string `json:"source"`
	// Agent is the agent kind ("claude-code", "codex"); stamped like Source.
	Agent string `json:"agent"`
	// Key is opaque, URL-safe (may contain '/') and unique within the source.
	// It is what Source.Detail accepts.
	Key string `json:"key"`
	// ID is the agent's own session identifier.
	ID           string    `json:"id"`
	Project      string    `json:"project"`     // display name (cwd base name)
	ProjectKey   string    `json:"project_key"` // stable grouping key (usually the cwd)
	CWD          string    `json:"cwd"`
	GitBranch    string    `json:"git_branch"`
	AgentVersion string    `json:"agent_version"`
	Models       []string  `json:"models"`
	StartedAt    time.Time `json:"started_at"`
	EndedAt      time.Time `json:"ended_at"`
	FirstPrompt  string    `json:"first_prompt"`
	// TurnCount counts human prompts; MessageCount counts user + assistant
	// messages (tool calls and reasoning excluded).
	TurnCount    int   `json:"turn_count"`
	MessageCount int   `json:"message_count"`
	SizeBytes    int64 `json:"size_bytes"`
	Usage        Usage `json:"usage"`
	// ModelUsage optionally splits Usage per model (or model family). Stats
	// uses it for the per-model chart; when nil, Usage is attributed to the
	// single entry of Models, or to "unknown".
	ModelUsage map[string]Usage `json:"model_usage,omitempty"`
	// ForkedFrom is the parent session ID when the session is a fork.
	ForkedFrom string `json:"forked_from,omitempty"`
	// SkippedLines counts malformed or unrecognized records the reader skipped.
	SkippedLines int `json:"skipped_lines,omitempty"`
}

// BlockKind discriminates the content of a Block.
type BlockKind string

// Block kinds.
const (
	BlockText       BlockKind = "text"
	BlockReasoning  BlockKind = "reasoning"
	BlockToolCall   BlockKind = "tool_call"
	BlockToolResult BlockKind = "tool_result"
)

// Block is one piece of a turn. A tool call whose result was found carries
// both Input and Output with Paired=true; a result without its call is a
// BlockToolResult with Paired=false; a call without result keeps Paired=false.
type Block struct {
	Kind     BlockKind `json:"kind"`
	Text     string    `json:"text,omitempty"`
	ToolName string    `json:"tool_name,omitempty"`
	CallID   string    `json:"call_id,omitempty"`
	Input    string    `json:"input,omitempty"`
	Output   string    `json:"output,omitempty"`
	IsError  bool      `json:"is_error,omitempty"`
	Paired   bool      `json:"paired,omitempty"`
}

// Turn is one conversation turn.
type Turn struct {
	Role      string    `json:"role"`
	Timestamp time.Time `json:"timestamp"`
	Model     string    `json:"model,omitempty"`
	Blocks    []Block   `json:"blocks"`
	Usage     *Usage    `json:"usage,omitempty"`
}

// Detail is the full transcript of one session.
type Detail struct {
	Session Session `json:"session"`
	Turns   []Turn  `json:"turns"`
}

// Source is the port each agent reader implements and the host aggregates.
// Implementations must be safe for concurrent use.
type Source interface {
	// ID identifies the source instance (unique among the host's sources).
	ID() string
	// Agent is the agent kind, e.g. "claude-code" or "codex".
	Agent() string
	// Fingerprint is a cheap change detector (stat-only); it changes whenever
	// List would return different data. Hosts cache List/Stats by it.
	Fingerprint(ctx context.Context) (string, error)
	// List returns every session with Usage populated. Callers must treat the
	// returned values as read-only.
	List(ctx context.Context) ([]Session, error)
	// Detail returns one session by Key, or an error matching ErrNotFound.
	Detail(ctx context.Context, key string) (*Detail, error)
}

// ProjectName returns the last non-empty segment of a cwd ('/' or '\').
func ProjectName(cwd string) string {
	s := strings.TrimRight(strings.ReplaceAll(cwd, `\`, "/"), "/")
	if i := strings.LastIndex(s, "/"); i >= 0 {
		return s[i+1:]
	}
	return s
}
