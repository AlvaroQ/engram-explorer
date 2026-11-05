package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Sample Codex CLI rollouts. `make demo` points CODEX_HOME at <out>/codex, so
// the agent session pages show these instead of the visitor's real ~/.codex.
// The lines follow the rollout envelope read by internal/agentsessions/codex:
// {timestamp, type, payload} with session_meta, turn_context, response_item,
// event_msg (token_count with cumulative totals) and token_usage_record.
//
// Everything is deterministic except the calendar anchor (seedBase), so a
// re-run on the same day rewrites identical files.

const cxDemoVersion = "0.46.0"

// cxTokens mirrors Codex's TokenUsage (cached and cache-write tokens are a
// subset of input, reasoning a subset of output).
type cxTokens struct {
	InputTokens           int64 `json:"input_tokens"`
	CachedInputTokens     int64 `json:"cached_input_tokens"`
	CacheWriteInputTokens int64 `json:"cache_write_input_tokens,omitempty"`
	OutputTokens          int64 `json:"output_tokens"`
	ReasoningOutputTokens int64 `json:"reasoning_output_tokens"`
	TotalTokens           int64 `json:"total_tokens"`
}

func tokens(in, cached, cacheWrite, out, reasoning int64) cxTokens {
	return cxTokens{in, cached, cacheWrite, out, reasoning, in + out}
}

// cxLine is one rollout line before its timestamp is assigned.
type cxLine struct {
	typ     string
	payload any
}

func cxItem(p map[string]any) cxLine { return cxLine{"response_item", p} }

func cxUser(text string) cxLine {
	return cxItem(map[string]any{"type": "message", "role": "user",
		"content": []map[string]any{{"type": "input_text", "text": text}}})
}

func cxAssistant(text string) cxLine {
	return cxItem(map[string]any{"type": "message", "role": "assistant",
		"content": []map[string]any{{"type": "output_text", "text": text}}})
}

func cxReasoning(summary string) cxLine {
	return cxItem(map[string]any{"type": "reasoning",
		"summary": []map[string]any{{"type": "summary_text", "text": summary}}, "encrypted_content": nil})
}

// cxShell is a function_call to the shell tool (arguments are a JSON string).
func cxShell(callID, workdir string, cmd ...string) cxLine {
	args, _ := json.Marshal(map[string]any{"command": cmd, "workdir": workdir, "timeout_ms": 120000})
	return cxItem(map[string]any{"type": "function_call", "name": "shell", "arguments": string(args), "call_id": callID})
}

// cxShellOutput is the function_call_output of a shell call (JSON string).
func cxShellOutput(callID, output string, exitCode int) cxLine {
	body, _ := json.Marshal(map[string]any{"output": output,
		"metadata": map[string]any{"exit_code": exitCode, "duration_seconds": 0.4}})
	return cxItem(map[string]any{"type": "function_call_output", "call_id": callID, "output": string(body)})
}

func cxTokenCount(total *cxTokens) cxLine {
	if total == nil {
		return cxLine{"event_msg", map[string]any{"type": "token_count", "info": nil}}
	}
	return cxLine{"event_msg", map[string]any{"type": "token_count", "info": map[string]any{
		"total_token_usage": total, "last_token_usage": total, "model_context_window": 272000}}}
}

// cxSampleSession is one sample rollout.
type cxSampleSession struct {
	id, cwd, branch, commit, model string
	daysAgo, hourFrom              int // start offset before seedBase
	lines                          []cxLine
}

func cxEnvContext(cwd string) cxLine {
	return cxUser("<environment_context>\n  <cwd>" + cwd + "</cwd>\n  <approval_policy>on-request</approval_policy>\n" +
		"  <sandbox_mode>workspace-write</sandbox_mode>\n  <network_access>restricted</network_access>\n" +
		"  <shell>bash</shell>\n</environment_context>")
}

func cxSampleSessions() []cxSampleSession {
	t1 := tokens(14820, 0, 0, 612, 384)
	t2 := tokens(31950, 14592, 0, 1480, 896)
	c1 := tokens(6210, 0, 0, 298, 192)
	c2 := tokens(13870, 6016, 0, 742, 448)
	patch := "*** Begin Patch\n*** Update File: src/middleware/rate-limit.ts\n" +
		"@@\n-const WINDOW_MS = 60_000\n+const WINDOW_MS = Number(process.env.RATE_WINDOW_MS ?? 60_000)\n*** End Patch"
	return []cxSampleSession{
		{
			id: "0199f3a2-5c41-7d10-9e2b-6a1c0d4e8f01", cwd: "/home/user/demo-api", branch: "feat/rate-limit",
			commit: "4f9c2d1e8b7a6c5d4e3f2a1b0c9d8e7f6a5b4c3d", model: "gpt-5-codex", daysAgo: 2, hourFrom: 3,
			lines: []cxLine{
				cxEnvContext("/home/user/demo-api"),
				cxUser("Where is the rate-limit window configured? Make it configurable through an env var."),
				cxTokenCount(nil),
				cxReasoning("**Locating the limiter**\n\nSearching the middleware for the window constant."),
				cxShell("call_demoApi0001", "/home/user/demo-api", "bash", "-lc", "rg -n \"WINDOW_MS\" src"),
				cxShellOutput("call_demoApi0001", "src/middleware/rate-limit.ts:3:const WINDOW_MS = 60_000", 0),
				cxTokenCount(&t1),
				cxAssistant("The window is the `WINDOW_MS` constant in `src/middleware/rate-limit.ts`. I'll read it from `RATE_WINDOW_MS` with 60 s as the default."),
				cxTokenCount(&t1), // re-emitted with unchanged totals
				cxUser("Go ahead."),
				cxItem(map[string]any{"type": "custom_tool_call", "status": "completed", "call_id": "call_demoApi0002",
					"name": "apply_patch", "input": patch}),
				cxItem(map[string]any{"type": "custom_tool_call_output", "call_id": "call_demoApi0002",
					"output": "Success. Updated the following files:\nM src/middleware/rate-limit.ts"}),
				cxTokenCount(&t2),
				cxAssistant("Done: `RATE_WINDOW_MS` now overrides the window; unset keeps 60 s."),
			},
		},
		{
			id: "0199f6b8-2d07-7a3e-8c15-3f9e7b2a4d02", cwd: "/home/user/demo-web", branch: "chore/bundle-size",
			commit: "a1b2c3d4e5f60718293a4b5c6d7e8f9012345678", model: "gpt-5", daysAgo: 5, hourFrom: 6,
			lines: []cxLine{
				cxEnvContext("/home/user/demo-web"),
				cxUser("The production bundle grew by 180 kB last week. Which dependency is responsible?"),
				cxReasoning("**Measuring the bundle**\n\nBuilding with the size report to compare chunks."),
				cxItem(map[string]any{"type": "local_shell_call", "call_id": "call_demoWeb0001", "status": "completed",
					"action": map[string]any{"type": "exec", "command": []string{"pnpm", "build", "--report"},
						"working_directory": "/home/user/demo-web", "timeout_ms": 120000}}),
				cxShellOutput("call_demoWeb0001", "dist/assets/index.js  412.6 kB\ndist/assets/vendor-charts.js  183.9 kB", 0),
				cxLine{"token_usage_record", map[string]any{"response_id": "resp_demoWeb0001", "model": "gpt-5",
					"usage": tokens(9840, 0, 0, 455, 256)}},
				cxAssistant("A new `vendor-charts` chunk (184 kB) appeared: the full charting library is imported in `Dashboard.tsx`. Importing only the line chart module, or lazy-loading the dashboard route, removes most of it."),
				cxLine{"token_usage_record", map[string]any{"response_id": "resp_demoWeb0002", "model": "gpt-5",
					"usage": tokens(21304, 9728, 1024, 1210, 640)}},
			},
		},
		{
			id: "0199f8e1-9b3c-7f24-a6d8-1e5c4a7b9f03", cwd: "/home/user/infra", branch: "main",
			commit: "0f1e2d3c4b5a69788796a5b4c3d2e1f00f1e2d3c", model: "gpt-5-mini", daysAgo: 1, hourFrom: 2,
			lines: []cxLine{
				cxEnvContext("/home/user/infra"),
				cxUser("The terraform plan fails in CI. Reproduce it locally and tell me why."),
				cxTokenCount(nil),
				cxShell("call_demoInfra001", "/home/user/infra", "bash", "-lc", "terraform plan -input=false"),
				cxShellOutput("call_demoInfra001", "Error: Unsupported argument\n\n  on network.tf line 14:\n  14:   enable_classiclink = false\n\nAn argument named \"enable_classiclink\" is not expected here.", 1),
				cxTokenCount(&c1),
				cxReasoning("**Reading the provider changelog**\n\nThe argument was removed in AWS provider v5."),
				cxAssistant("`enable_classiclink` was removed in AWS provider 5.x and CI now resolves 5.x. Delete the argument from `network.tf` (it defaulted to false anyway) or pin the provider to `~> 4.67`."),
				cxTokenCount(&c2),
			},
		},
	}
}

// renderRollout renders s as rollout JSONL and returns it with the start and
// last timestamps.
func renderRollout(s cxSampleSession) ([]byte, time.Time, time.Time, error) {
	start := seedBase.Add(-time.Duration(s.daysAgo)*24*time.Hour - time.Duration(s.hourFrom)*time.Hour)
	stamp := func(t time.Time) string { return t.Format("2006-01-02T15:04:05.000Z") }
	lines := append([]cxLine{
		{"session_meta", map[string]any{
			"id": s.id, "timestamp": stamp(start), "cwd": s.cwd, "originator": "codex_cli_rs",
			"cli_version": cxDemoVersion, "source": "cli", "model_provider": "openai",
			"git": map[string]any{"commit_hash": s.commit, "branch": s.branch,
				"repository_url": "git@github.com:example/" + filepath.Base(s.cwd) + ".git"},
		}},
		{"turn_context", map[string]any{
			"cwd": s.cwd, "approval_policy": "on-request", "sandbox_policy": map[string]any{"mode": "workspace-write"},
			"model": s.model, "effort": "medium", "summary": "auto",
		}},
	}, s.lines...)

	var buf bytes.Buffer
	at := start
	for i, l := range lines {
		if i > 0 {
			at = at.Add(time.Duration(3+2*i) * time.Second)
		}
		b, err := json.Marshal(map[string]any{"timestamp": stamp(at), "type": l.typ, "payload": l.payload})
		if err != nil {
			return nil, time.Time{}, time.Time{}, err
		}
		buf.Write(b)
		buf.WriteByte('\n')
	}
	return buf.Bytes(), start, at, nil
}

// removeStaleRollouts deletes earlier copies of the sample rollouts (written
// on another day, so under another date dir) that are not in keep. Only files
// named rollout-*-<sample id>.jsonl are touched.
func removeStaleRollouts(sessionsDir string, ids []string, keep map[string]bool) error {
	err := filepath.WalkDir(sessionsDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || !d.Type().IsRegular() || keep[p] || !strings.HasPrefix(d.Name(), "rollout-") {
			return err
		}
		for _, id := range ids {
			if strings.HasSuffix(d.Name(), "-"+id+".jsonl") {
				return os.Remove(p)
			}
		}
		return nil
	})
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// writeCodexSessions writes the sample rollouts under
// dir/sessions/YYYY/MM/DD/ (dir is the CODEX_HOME of `make demo`) and
// returns how many it wrote. Each file's mtime is its last timestamp.
func writeCodexSessions(dir string) (int, error) {
	sessions := cxSampleSessions()
	sessionsDir := filepath.Join(dir, "sessions")
	ids := make([]string, 0, len(sessions))
	keep := map[string]bool{}
	type rendered struct {
		path string
		data []byte
		last time.Time
	}
	var files []rendered
	for _, s := range sessions {
		data, start, last, err := renderRollout(s)
		if err != nil {
			return 0, fmt.Errorf("render rollout %s: %w", s.id, err)
		}
		name := "rollout-" + start.Format("2006-01-02T15-04-05") + "-" + s.id + ".jsonl"
		p := filepath.Join(sessionsDir, start.Format("2006"), start.Format("01"), start.Format("02"), name)
		ids, keep[p] = append(ids, s.id), true
		files = append(files, rendered{p, data, last})
	}
	if err := removeStaleRollouts(sessionsDir, ids, keep); err != nil {
		return 0, fmt.Errorf("remove stale sample rollouts: %w", err)
	}
	for _, f := range files {
		if err := os.MkdirAll(filepath.Dir(f.path), 0o755); err != nil {
			return 0, fmt.Errorf("create %s: %w", filepath.Dir(f.path), err)
		}
		if err := os.WriteFile(f.path, f.data, 0o644); err != nil {
			return 0, fmt.Errorf("write %s: %w", f.path, err)
		}
		if err := os.Chtimes(f.path, f.last, f.last); err != nil {
			return 0, fmt.Errorf("set mtime of %s: %w", f.path, err)
		}
	}
	return len(files), nil
}
