package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Sample Claude Code transcripts. `make demo` points CLAUDE_PROJECTS_DIR at
// <out>/claude, so the Claude Code pages show these instead of the visitor's
// real ~/.claude/projects. The line shapes follow what
// internal/services/cc_sessions.go reads: one JSON object per line with type,
// timestamp, sessionId, cwd, gitBranch, version and message; assistant lines
// carry message.id, message.model and message.usage.
//
// Everything is deterministic except the calendar anchor (seedBase), so a
// re-run on the same day rewrites identical files.

const ccDemoVersion = "2.0.14"

// ccUsage mirrors the message.usage block of an assistant line.
type ccUsage struct {
	InputTokens              int64           `json:"input_tokens"`
	CacheCreationInputTokens int64           `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int64           `json:"cache_read_input_tokens"`
	CacheCreation            ccCacheCreation `json:"cache_creation"`
	OutputTokens             int64           `json:"output_tokens"`
	ServiceTier              string          `json:"service_tier"`
}

type ccCacheCreation struct {
	Ephemeral5m int64 `json:"ephemeral_5m_input_tokens"`
	Ephemeral1h int64 `json:"ephemeral_1h_input_tokens"`
}

// ccStep is one transcript line before rendering. Exactly one of the shapes
// is used: a user prompt (prompt), a tool result (toolResultFor/result), or an
// assistant reply (text, optional tool call, usage).
type ccStep struct {
	prompt        string
	toolResultFor string
	result        string

	text      string
	toolID    string
	toolName  string
	toolInput map[string]any
	usage     ccUsage
}

// ccSampleSession is one sample transcript.
type ccSampleSession struct {
	id       string
	cwd      string
	branch   string
	model    string
	daysAgo  int
	hourFrom int // start hour offset before seedBase
	steps    []ccStep
}

// usage builds a usage block; cw1h moves the cache write to the 1-hour TTL.
func usage(in, cacheWrite, cacheRead, out int64, cw1h bool) ccUsage {
	u := ccUsage{
		InputTokens:              in,
		CacheCreationInputTokens: cacheWrite,
		CacheReadInputTokens:     cacheRead,
		OutputTokens:             out,
		ServiceTier:              "standard",
	}
	if cw1h {
		u.CacheCreation.Ephemeral1h = cacheWrite
	} else {
		u.CacheCreation.Ephemeral5m = cacheWrite
	}
	return u
}

func ccSampleSessions() []ccSampleSession {
	return []ccSampleSession{
		{
			id: "6b1f0c2e-4a57-4d8e-9c31-2f7a8e5d0a11", cwd: "/home/user/demo-api", branch: "feat/rate-limit",
			model: "claude-sonnet-4-5-20250929", daysAgo: 3, hourFrom: 2,
			steps: []ccStep{
				{prompt: "Add per-client rate limiting to the write routes. Keep the read routes unlimited."},
				{text: "I'll start by reading how the write routes are registered.",
					toolID: "toolu_01DemoApiRead0001", toolName: "Read",
					toolInput: map[string]any{"file_path": "/home/user/demo-api/src/routes/write.ts"},
					usage:     usage(12, 18240, 0, 96, false)},
				{toolResultFor: "toolu_01DemoApiRead0001",
					result: "export function writeRoutes(app: Hono) {\n  app.patch('/api/items/:id', patchItem)\n  app.delete('/api/items/:id', deleteItem)\n}"},
				{text: "Both write routes go through `writeRoutes`, so I'll wrap them in a fixed-window limiter keyed by client address (120 requests per minute) and leave the read routes untouched.",
					usage: usage(8, 1310, 18240, 412, false)},
				{prompt: "Looks good. Add a test for the 429 response."},
				{text: "Added `rate-limit.test.ts`: the 121st request inside a minute gets 429 with a Retry-After header.",
					usage: usage(6, 860, 19550, 238, false)},
			},
		},
		{
			id: "9d3e7a40-1c8b-4f62-8a05-c4b19e7f3d22", cwd: "/home/user/demo-web", branch: "fix/table-rerender",
			model: "claude-opus-4-1-20250805", daysAgo: 6, hourFrom: 5,
			steps: []ccStep{
				{prompt: "The data table re-renders every row on each keystroke in the filter box. Can you find out why?"},
				{text: "Let me find where the filter state lives.",
					toolID: "toolu_01DemoWebGrep0001", toolName: "Grep",
					toolInput: map[string]any{"pattern": "useState\\(.*filter", "path": "/home/user/demo-web/src"},
					usage:     usage(20, 22410, 0, 74, true)},
				{toolResultFor: "toolu_01DemoWebGrep0001",
					result: "src/components/DataTable.tsx:41:  const [filter, setFilter] = useState('')"},
				{text: "The filter state sits in `DataTable`, so every keystroke re-renders the whole table and recreates the `columns` array. Moving the filter into its own component and memoising `columns` with `useMemo` keeps the rows stable.",
					usage: usage(9, 1904, 22410, 540, true)},
			},
		},
		{
			id: "c2a85f19-7e04-4b3d-b6f8-0e9d21a4c533", cwd: "/home/user/infra", branch: "main",
			model: "claude-haiku-4-5-20251001", daysAgo: 1, hourFrom: 1,
			steps: []ccStep{
				{prompt: "Why is the CI job failing on the migration step?"},
				{text: "I'll run the migration locally to reproduce it.",
					toolID: "toolu_01DemoInfraBash001", toolName: "Bash",
					toolInput: map[string]any{"command": "goose -dir migrations sqlite3 ./tmp.db up", "description": "Run migrations locally"},
					usage:     usage(15, 9620, 0, 58, false)},
				{toolResultFor: "toolu_01DemoInfraBash001",
					result: "2026/01/01 10:00:00 OK   0001_init.sql\n2026/01/01 10:00:00 goose run: 0002_add_index.sql: duplicate column name: created_at"},
				{text: "Migration 0002 adds `created_at` again although 0001 already created it. Drop the `ALTER TABLE` line from 0002 and keep only the index.",
					usage: usage(7, 640, 9620, 186, false)},
			},
		},
	}
}

// ccProjectFolder encodes a cwd the way Claude Code names project folders.
func ccProjectFolder(cwd string) string {
	b := []byte(cwd)
	for i, c := range b {
		if c == '/' || c == '.' {
			b[i] = '-'
		}
	}
	return string(b)
}

// renderTranscript renders s as JSONL and returns it with the last timestamp.
func renderTranscript(s ccSampleSession) ([]byte, time.Time, error) {
	var buf bytes.Buffer
	at := seedBase.Add(-time.Duration(s.daysAgo)*24*time.Hour - time.Duration(s.hourFrom)*time.Hour)
	parent := any(nil)
	for i, st := range s.steps {
		at = at.Add(time.Duration(20+7*i) * time.Second)
		uuid := fmt.Sprintf("%s-%04d", s.id[:23], i+1)
		line := map[string]any{
			"parentUuid":  parent,
			"isSidechain": false,
			"userType":    "external",
			"cwd":         s.cwd,
			"sessionId":   s.id,
			"version":     ccDemoVersion,
			"gitBranch":   s.branch,
			"uuid":        uuid,
			"timestamp":   at.Format("2006-01-02T15:04:05.000Z"),
		}
		switch {
		case st.prompt != "":
			line["type"] = "user"
			line["message"] = map[string]any{"role": "user", "content": st.prompt}
		case st.toolResultFor != "":
			line["type"] = "user"
			line["message"] = map[string]any{"role": "user", "content": []map[string]any{
				{"type": "tool_result", "tool_use_id": st.toolResultFor, "content": st.result},
			}}
		default:
			content := []map[string]any{{"type": "text", "text": st.text}}
			stop := "end_turn"
			if st.toolID != "" {
				content = append(content, map[string]any{
					"type": "tool_use", "id": st.toolID, "name": st.toolName, "input": st.toolInput,
				})
				stop = "tool_use"
			}
			line["type"] = "assistant"
			line["requestId"] = fmt.Sprintf("req_demo%s%02d", s.id[:8], i)
			line["message"] = map[string]any{
				"id":          fmt.Sprintf("msg_demo%s%02d", s.id[:8], i),
				"type":        "message",
				"role":        "assistant",
				"model":       s.model,
				"content":     content,
				"stop_reason": stop,
				"usage":       st.usage,
			}
		}
		b, err := json.Marshal(line)
		if err != nil {
			return nil, time.Time{}, err
		}
		buf.Write(b)
		buf.WriteByte('\n')
		parent = uuid
	}
	return buf.Bytes(), at, nil
}

// writeClaudeSessions writes the sample transcripts under dir (the
// CLAUDE_PROJECTS_DIR of `make demo`) and returns how many it wrote. Each
// file's mtime is set to its last timestamp, since the transcript reader uses
// the mtime as the session end.
func writeClaudeSessions(dir string) (int, error) {
	sessions := ccSampleSessions()
	for _, s := range sessions {
		folder := filepath.Join(dir, ccProjectFolder(s.cwd))
		if err := os.MkdirAll(folder, 0o755); err != nil {
			return 0, fmt.Errorf("create %s: %w", folder, err)
		}
		data, last, err := renderTranscript(s)
		if err != nil {
			return 0, fmt.Errorf("render session %s: %w", s.id, err)
		}
		path := filepath.Join(folder, s.id+".jsonl")
		if err := os.WriteFile(path, data, 0o644); err != nil {
			return 0, fmt.Errorf("write %s: %w", path, err)
		}
		if err := os.Chtimes(path, last, last); err != nil {
			return 0, fmt.Errorf("set mtime of %s: %w", path, err)
		}
	}
	return len(sessions), nil
}
