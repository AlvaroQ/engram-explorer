package ccsessions

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/AlvaroQ/engram-explorer/internal/agentsessions"
	"github.com/AlvaroQ/engram-explorer/internal/config"
	"github.com/AlvaroQ/engram-explorer/internal/services"
)

// Agent is the agent kind of every Claude Code source; it is the key of the
// Claude Code marker set the services layer registers with agentsessions.
const Agent = services.CCAgent

// sourcePrefix prefixes the account ID in a source ID ("claude-code/<account>").
const sourcePrefix = Agent + "/"

// DetailPrefix is the URL prefix of the detail route this provider mounts:
// agentsessions.DetailPath of a Claude session starts with it.
const DetailPrefix = agentsessions.RoutePrefix + "/" + sourcePrefix

// Option configures the provider (NewProvider).
type Option func(*ccProvider)

// WithAccounts injects the tracked Claude Code accounts (the host passes the
// profile store's effective accounts). Each enabled account becomes one
// agent-session source. Without it, or with no enabled account, the
// provider's own path is served as the "default" account.
func WithAccounts(fn func() []config.CCAccount) Option {
	return func(p *ccProvider) { p.accounts = fn }
}

// account is one Claude Code projects directory served as a source.
type account struct{ id, label, path string }

// liveAccounts returns the enabled accounts, falling back to the instance path.
func (i *ccInstance) liveAccounts() []account {
	var out []account
	if i.accounts != nil {
		for _, a := range i.accounts() {
			if a.Enabled {
				out = append(out, account{id: a.ID, label: a.Label, path: a.Path})
			}
		}
	}
	if len(out) == 0 {
		out = []account{{id: "default", label: "Personal", path: i.path}}
	}
	return out
}

// AgentSessionSources implements providers.AgentSessionSource: one source per
// enabled account. Sources are reused across calls while the account keeps
// its path, so their fingerprint caches survive between requests.
func (i *ccInstance) AgentSessionSources() []agentsessions.Source {
	accs := i.liveAccounts()
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.sources == nil {
		i.sources = map[string]*claudeSource{}
	}
	out := make([]agentsessions.Source, 0, len(accs))
	for _, a := range accs {
		k := a.id + "\x00" + a.path
		src := i.sources[k]
		if src == nil {
			src = &claudeSource{id: sourcePrefix + a.id, reader: services.DiskProjectsReader{BaseDir: a.path}}
			i.sources[k] = src
		}
		out = append(out, src)
	}
	return out
}

// accountSources returns the enabled accounts as services sources, for the
// deprecated Claude-only stats endpoint.
func (i *ccInstance) accountSources() []services.CCAccountSource {
	accs := i.liveAccounts()
	out := make([]services.CCAccountSource, 0, len(accs))
	for _, a := range accs {
		out = append(out, services.CCAccountSource{ID: a.id, Label: a.label, Reader: services.DiskProjectsReader{BaseDir: a.path}})
	}
	return out
}

// claudeSource is an agentsessions.Source over one Claude Code projects
// directory. It caches the summarized list by fingerprint.
type claudeSource struct {
	id     string
	reader services.CCProjectsReader

	mu       sync.Mutex
	fp       string
	sessions []agentsessions.Session
}

func (s *claudeSource) ID() string    { return s.id }
func (s *claudeSource) Agent() string { return Agent }

// Fingerprint implements agentsessions.Source (stat-only).
func (s *claudeSource) Fingerprint(context.Context) (string, error) {
	return services.CCSessionsFingerprint(s.reader), nil
}

// List implements agentsessions.Source with a full read of every transcript,
// reused while the fingerprint is unchanged.
func (s *claudeSource) List(ctx context.Context) ([]agentsessions.Session, error) {
	fp := services.CCSessionsFingerprint(s.reader)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sessions != nil && fp != "" && fp == s.fp {
		return s.sessions, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	sums, err := services.CCSessionSummaries(s.reader)
	if err != nil {
		return nil, err
	}
	out := make([]agentsessions.Session, 0, len(sums))
	for _, sum := range sums {
		out = append(out, toSession(sum))
	}
	s.fp, s.sessions = fp, out
	return out, nil
}

// splitKey validates a "<project folder>/<session id>" key.
func splitKey(key string) (folder, id string, ok bool) {
	folder, id, found := strings.Cut(key, "/")
	if !found || services.ValidateCCSessionPath(folder, id) != nil {
		return "", "", false
	}
	return folder, id, true
}

// Detail implements agentsessions.Source. Keys are "<folder>/<id>"; anything
// else (traversal, extra segments) is ErrInvalidKey.
func (s *claudeSource) Detail(_ context.Context, key string) (*agentsessions.Detail, error) {
	folder, id, ok := splitKey(key)
	if !ok {
		return nil, fmt.Errorf("claude code: %w", agentsessions.ErrInvalidKey)
	}
	d, err := services.CCSessionGetDetail(s.reader, folder, id)
	if err != nil {
		return nil, err
	}
	if d == nil {
		return nil, fmt.Errorf("claude code %s: %w", key, agentsessions.ErrNotFound)
	}
	sess := agentsessions.Session{
		Key: key, ID: d.ID, Project: d.ProjectDisplay, ProjectKey: d.CWD, CWD: d.CWD,
		GitBranch: d.GitBranch, AgentVersion: d.CCVersion, StartedAt: d.StartedAt,
		EndedAt: d.EndedAt, SizeBytes: d.FileSizeBytes, Usage: toUsage(d.Usage),
	}
	if sess.ProjectKey == "" {
		sess.ProjectKey = folder
	}
	return &agentsessions.Detail{Session: sess, Turns: toTurns(d.Turns)}, nil
}

func toSession(s services.CCSessionSummary) agentsessions.Session {
	out := agentsessions.Session{
		Key: s.ProjectFolder + "/" + s.ID, ID: s.ID, Project: s.ProjectDisplay, ProjectKey: s.CWD,
		CWD: s.CWD, GitBranch: s.GitBranch, AgentVersion: s.Version, Models: s.Models,
		StartedAt: s.StartedAt, EndedAt: s.EndedAt, FirstPrompt: s.FirstPrompt,
		TurnCount: s.UserTurns, MessageCount: s.Messages, SizeBytes: s.SizeBytes, Usage: toUsage(s.Usage),
	}
	if out.ProjectKey == "" {
		out.ProjectKey = s.ProjectFolder
	}
	if len(s.ModelUsage) > 0 {
		out.ModelUsage = make(map[string]agentsessions.Usage, len(s.ModelUsage))
		for m, u := range s.ModelUsage {
			out.ModelUsage[m] = toUsage(u)
		}
	}
	return out
}

// toUsage maps Claude usage (already disjoint: input excludes cache reads and
// writes) into neutral buckets; the 5m/1h cache-write split goes to Breakdown.
func toUsage(u services.CCSessionUsage) agentsessions.Usage {
	out := agentsessions.Usage{
		Input: u.InputTokens, CacheRead: u.CacheRead, CacheWrite: u.CacheWrite5m + u.CacheWrite1h,
		Output: u.OutputTokens, CostUSD: u.CostUSD, UnknownModel: u.UnknownModel,
	}
	if u.CacheWrite5m != 0 || u.CacheWrite1h != 0 {
		out.Breakdown = map[string]int64{"cache_write_5m": u.CacheWrite5m, "cache_write_1h": u.CacheWrite1h}
	}
	return out
}

// toTurns maps transcript turns into neutral turns. Each tool_result is folded
// into the tool_use with the same id (Paired); results without a call stay as
// unpaired tool_result blocks, and turns left empty by folding are dropped.
func toTurns(in []services.CCTurn) []agentsessions.Turn {
	results := map[string]services.CCMessageBlock{}
	calls := map[string]bool{}
	for _, t := range in {
		for _, b := range t.Blocks {
			switch b.Type {
			case "tool_use":
				calls[b.ID] = true
			case "tool_result":
				if _, dup := results[b.ToolUseID]; !dup {
					results[b.ToolUseID] = b
				}
			}
		}
	}
	out := make([]agentsessions.Turn, 0, len(in))
	for _, t := range in {
		turn := agentsessions.Turn{Role: t.Role, Timestamp: parseTime(t.Timestamp)}
		if t.Text != "" {
			turn.Blocks = append(turn.Blocks, agentsessions.Block{Kind: agentsessions.BlockText, Text: t.Text})
		}
		for _, b := range t.Blocks {
			if nb, keep := toBlock(b, results, calls); keep {
				turn.Blocks = append(turn.Blocks, nb)
			}
		}
		if len(turn.Blocks) > 0 {
			out = append(out, turn)
		}
	}
	return out
}

func toBlock(b services.CCMessageBlock, results map[string]services.CCMessageBlock, calls map[string]bool) (agentsessions.Block, bool) {
	switch b.Type {
	case "text":
		return agentsessions.Block{Kind: agentsessions.BlockText, Text: b.Text}, true
	case "thinking":
		return agentsessions.Block{Kind: agentsessions.BlockReasoning, Text: firstNonEmpty(b.Thinking, b.Text)}, true
	case "tool_use":
		nb := agentsessions.Block{Kind: agentsessions.BlockToolCall, ToolName: b.Name, CallID: b.ID, Input: string(b.RawInput)}
		if r, ok := results[b.ID]; ok && b.ID != "" {
			nb.Output, nb.Paired, nb.IsError = resultText(r.Content), true, r.IsError
		}
		return nb, true
	case "tool_result":
		if calls[b.ToolUseID] && b.ToolUseID != "" {
			return agentsessions.Block{}, false // folded into its call
		}
		return agentsessions.Block{Kind: agentsessions.BlockToolResult, CallID: b.ToolUseID, Output: resultText(b.Content), IsError: b.IsError}, true
	default:
		return agentsessions.Block{Kind: agentsessions.BlockText, Text: "[" + b.Type + "]"}, b.Type != ""
	}
}

// resultText renders tool_result content: a string, or the text of its blocks.
func resultText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &blocks) == nil {
		var parts []string
		for _, b := range blocks {
			if b.Text != "" {
				parts = append(parts, b.Text)
			}
		}
		return strings.Join(parts, "\n")
	}
	return string(raw)
}

func firstNonEmpty(vs ...string) string {
	for _, v := range vs {
		if v != "" {
			return v
		}
	}
	return ""
}

func parseTime(ts string) time.Time {
	t, _ := time.Parse(time.RFC3339Nano, ts)
	return t
}
