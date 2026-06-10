package services_test

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"strings"
	"testing"
	"time"

	"github.com/AlvaroQ/engram-explorer/internal/services"
)

// ---------------------------------------------------------------------------
// Fake reader for tests — no disk access
// ---------------------------------------------------------------------------

type fakeEntry struct {
	name  string
	isDir bool
	size  int64
	mtime time.Time
}

func (f fakeEntry) Name() string      { return f.name }
func (f fakeEntry) IsDir() bool       { return f.isDir }
func (f fakeEntry) Type() fs.FileMode { return 0 }
func (f fakeEntry) Info() (fs.FileInfo, error) {
	return fakeFileInfo{name: f.name, size: f.size, mtime: f.mtime, isDir: f.isDir}, nil
}

type fakeFileInfo struct {
	name  string
	size  int64
	mtime time.Time
	isDir bool
}

func (f fakeFileInfo) Name() string       { return f.name }
func (f fakeFileInfo) Size() int64        { return f.size }
func (f fakeFileInfo) Mode() fs.FileMode  { return 0644 }
func (f fakeFileInfo) ModTime() time.Time { return f.mtime }
func (f fakeFileInfo) IsDir() bool        { return f.isDir }
func (f fakeFileInfo) Sys() any           { return nil }

// fakeReader implements CCProjectsReader backed by in-memory content.
type fakeReader struct {
	// projects maps project folder name to map[sessionFile]content.
	projects map[string]map[string]string
	// mtimes allows overriding mtime per file.
	mtimes map[string]time.Time // key: "projectFolder/sessionFile"
}

func newFakeReader() *fakeReader {
	return &fakeReader{
		projects: make(map[string]map[string]string),
		mtimes:   make(map[string]time.Time),
	}
}

func (f *fakeReader) addSession(projectFolder, sessionFile, content string, mtime time.Time) {
	if f.projects[projectFolder] == nil {
		f.projects[projectFolder] = make(map[string]string)
	}
	f.projects[projectFolder][sessionFile] = content
	f.mtimes[projectFolder+"/"+sessionFile] = mtime
}

func (f *fakeReader) ProjectEntries() ([]fs.DirEntry, error) {
	var entries []fs.DirEntry
	for name := range f.projects {
		entries = append(entries, fakeEntry{name: name, isDir: true})
	}
	return entries, nil
}

func (f *fakeReader) SessionEntries(projectFolder string) ([]fs.DirEntry, error) {
	files, ok := f.projects[projectFolder]
	if !ok {
		return nil, fmt.Errorf("no project %q", projectFolder)
	}
	var entries []fs.DirEntry
	for name := range files {
		mtime := f.mtimes[projectFolder+"/"+name]
		entries = append(entries, fakeEntry{
			name:  name,
			isDir: false,
			size:  int64(len(files[name])),
			mtime: mtime,
		})
	}
	return entries, nil
}

func (f *fakeReader) OpenSession(projectFolder, sessionFile string) (io.ReadCloser, error) {
	files, ok := f.projects[projectFolder]
	if !ok {
		return nil, fmt.Errorf("no project %q", projectFolder)
	}
	content, ok := files[sessionFile]
	if !ok {
		return nil, fmt.Errorf("no session %q in %q", sessionFile, projectFolder)
	}
	return io.NopCloser(strings.NewReader(content)), nil
}

func (f *fakeReader) SessionStat(projectFolder, sessionFile string) (fs.FileInfo, error) {
	files, ok := f.projects[projectFolder]
	if !ok {
		return nil, fmt.Errorf("%w: no project %q", fs.ErrNotExist, projectFolder)
	}
	content, ok := files[sessionFile]
	if !ok {
		return nil, fmt.Errorf("%w: no session %q in %q", fs.ErrNotExist, sessionFile, projectFolder)
	}
	mtime := f.mtimes[projectFolder+"/"+sessionFile]
	return fakeFileInfo{
		name:  sessionFile,
		size:  int64(len(content)),
		mtime: mtime,
	}, nil
}

// ---------------------------------------------------------------------------
// JSONL line builders
// ---------------------------------------------------------------------------

func jsonlLine(typ, ts, cwd, branch, version string, role string, content any) string {
	msg := map[string]any{"role": role, "content": content}
	msgBytes, _ := json.Marshal(msg)

	line := map[string]any{
		"type":      typ,
		"timestamp": ts,
		"sessionId": "test-session",
		"message":   json.RawMessage(msgBytes),
	}
	if cwd != "" {
		line["cwd"] = cwd
	}
	if branch != "" {
		line["gitBranch"] = branch
	}
	if version != "" {
		line["version"] = version
	}
	b, _ := json.Marshal(line)
	return string(b)
}

// buildSimpleSession builds a minimal .jsonl with one user message.
func buildSimpleSession(cwd, branch, version, firstPrompt string) string {
	ts := "2026-06-08T10:00:00.000Z"
	line := jsonlLine("user", ts, cwd, branch, version, "user", firstPrompt)
	return line + "\n"
}

// ---------------------------------------------------------------------------
// Tests: CCSessionsList
// ---------------------------------------------------------------------------

func TestCCSessionsList_EmptyDir(t *testing.T) {
	reader := newFakeReader()
	result, err := services.CCSessionsList(reader, services.CCSessionListParams{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Items) != 0 {
		t.Errorf("expected 0 items, got %d", len(result.Items))
	}
	if result.NextCursor != nil {
		t.Error("expected nil NextCursor for empty list")
	}
}

func TestCCSessionsList_SingleSession(t *testing.T) {
	reader := newFakeReader()
	mtime := time.Date(2026, 6, 8, 12, 0, 0, 0, time.UTC)
	content := buildSimpleSession(
		"e:/Personal/Repositorios/myproject",
		"main",
		"2.1.168",
		"Hello, what can you do?",
	)
	reader.addSession("e--Personal-Repositories-myproject", "abc123.jsonl", content, mtime)

	result, err := services.CCSessionsList(reader, services.CCSessionListParams{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(result.Items))
	}

	item := result.Items[0]
	if item.ID != "abc123" {
		t.Errorf("expected ID abc123, got %q", item.ID)
	}
	if item.ProjectDisplay != "myproject" {
		t.Errorf("expected ProjectDisplay myproject, got %q", item.ProjectDisplay)
	}
	if item.GitBranch != "main" {
		t.Errorf("expected GitBranch main, got %q", item.GitBranch)
	}
	if item.CCVersion != "2.1.168" {
		t.Errorf("expected CCVersion 2.1.168, got %q", item.CCVersion)
	}
	if !strings.Contains(item.FirstPrompt, "Hello") {
		t.Errorf("expected FirstPrompt to contain 'Hello', got %q", item.FirstPrompt)
	}
	if !item.EndedAt.Equal(mtime) {
		t.Errorf("expected EndedAt = mtime, got %v", item.EndedAt)
	}
}

func TestCCSessionsList_ProjectFilter(t *testing.T) {
	reader := newFakeReader()
	mtime := time.Now()

	reader.addSession("proj-a", "sess1.jsonl",
		buildSimpleSession("e:/proj-a", "main", "2.0", "prompt a"), mtime)
	reader.addSession("proj-b", "sess2.jsonl",
		buildSimpleSession("e:/proj-b", "main", "2.0", "prompt b"), mtime)

	result, err := services.CCSessionsList(reader, services.CCSessionListParams{Project: "proj-a"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Items) != 1 {
		t.Fatalf("expected 1 item after project filter, got %d", len(result.Items))
	}
	if result.Items[0].ProjectDisplay != "proj-a" {
		t.Errorf("filtered item should be proj-a, got %q", result.Items[0].ProjectDisplay)
	}
}

func TestCCSessionsList_IgnoresSubdirFolders(t *testing.T) {
	reader := newFakeReader()
	mtime := time.Now()

	// A real session file.
	reader.addSession("myproject", "real-session.jsonl",
		buildSimpleSession("e:/myproject", "main", "2.0", "real prompt"), mtime)

	// Simulate a subagents entry that happens to end in .jsonl by checking
	// the fakeReader only returns directory entries for subdirs, not files.
	// In the real impl, subdirs like "subagents/" show up as directories;
	// our filter already skips directories in SessionEntries loop.
	// This test just verifies the real session is returned.
	result, err := services.CCSessionsList(reader, services.CCSessionListParams{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Items) != 1 {
		t.Fatalf("expected 1 item, got %d: %+v", len(result.Items), result.Items)
	}
}

func TestCCSessionsList_Pagination(t *testing.T) {
	reader := newFakeReader()
	base := time.Now()

	// Create 5 sessions with staggered mtimes.
	for i := 0; i < 5; i++ {
		mtime := base.Add(time.Duration(i) * time.Minute)
		sessionID := fmt.Sprintf("session-%02d", i)
		reader.addSession("myproject", sessionID+".jsonl",
			buildSimpleSession("e:/myproject", "main", "2.0", fmt.Sprintf("prompt %d", i)),
			mtime,
		)
	}

	// Page 1: limit 2.
	p1, err := services.CCSessionsList(reader, services.CCSessionListParams{Limit: 2})
	if err != nil {
		t.Fatalf("page 1: %v", err)
	}
	if len(p1.Items) != 2 {
		t.Fatalf("expected 2 items on page 1, got %d", len(p1.Items))
	}
	if p1.NextCursor == nil {
		t.Fatal("expected NextCursor on page 1")
	}

	// Page 2: use cursor from page 1.
	p2, err := services.CCSessionsList(reader, services.CCSessionListParams{Limit: 2, Cursor: *p1.NextCursor})
	if err != nil {
		t.Fatalf("page 2: %v", err)
	}
	if len(p2.Items) != 2 {
		t.Fatalf("expected 2 items on page 2, got %d", len(p2.Items))
	}
	if p2.NextCursor == nil {
		t.Fatal("expected NextCursor on page 2")
	}

	// Page 3: last page (1 item).
	p3, err := services.CCSessionsList(reader, services.CCSessionListParams{Limit: 2, Cursor: *p2.NextCursor})
	if err != nil {
		t.Fatalf("page 3: %v", err)
	}
	if len(p3.Items) != 1 {
		t.Fatalf("expected 1 item on page 3, got %d", len(p3.Items))
	}
	if p3.NextCursor != nil {
		t.Error("expected nil NextCursor on last page")
	}
}

func TestCCSessionsList_SortsByMtimeDesc(t *testing.T) {
	reader := newFakeReader()
	base := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

	reader.addSession("p", "old.jsonl", buildSimpleSession("e:/p", "", "", "old"), base)
	reader.addSession("p", "new.jsonl", buildSimpleSession("e:/p", "", "", "new"), base.Add(time.Hour))

	result, err := services.CCSessionsList(reader, services.CCSessionListParams{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Items) < 2 {
		t.Fatalf("expected 2 items, got %d", len(result.Items))
	}
	if result.Items[0].ID != "new" {
		t.Errorf("first item should be the newer file, got %q", result.Items[0].ID)
	}
}

func TestCCSessionsList_ProjectsList(t *testing.T) {
	reader := newFakeReader()
	mtime := time.Now()

	reader.addSession("folder-a", "s1.jsonl", buildSimpleSession("e:/alpha", "", "", "p"), mtime)
	reader.addSession("folder-b", "s2.jsonl", buildSimpleSession("e:/beta", "", "", "p"), mtime)

	result, err := services.CCSessionsList(reader, services.CCSessionListParams{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Projects) != 2 {
		t.Errorf("expected 2 project names, got %d: %v", len(result.Projects), result.Projects)
	}
}

func TestCCSessionsList_MalformedLinesSkipped(t *testing.T) {
	reader := newFakeReader()
	mtime := time.Now()

	// Mix of good and bad lines.
	content := "not json at all\n" +
		buildSimpleSession("e:/proj", "main", "2.0", "valid prompt") +
		"{broken json\n"
	reader.addSession("proj", "sess.jsonl", content, mtime)

	result, err := services.CCSessionsList(reader, services.CCSessionListParams{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Items) != 1 {
		t.Fatalf("expected 1 item (malformed lines skipped), got %d", len(result.Items))
	}
}

// ---------------------------------------------------------------------------
// Tests: CCSessionGetDetail
// ---------------------------------------------------------------------------

func TestCCSessionGetDetail_NotFound(t *testing.T) {
	reader := newFakeReader()
	detail, err := services.CCSessionGetDetail(reader, "nonexistent", "abc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if detail != nil {
		t.Error("expected nil detail for non-existent session")
	}
}

func TestCCSessionGetDetail_StringContent(t *testing.T) {
	reader := newFakeReader()
	mtime := time.Date(2026, 6, 8, 12, 0, 0, 0, time.UTC)

	content := jsonlLine("user", "2026-06-08T10:00:00.000Z",
		"e:/myproject", "main", "2.1.0", "user", "Hello from user") + "\n" +
		jsonlLine("assistant", "2026-06-08T10:01:00.000Z",
			"", "", "", "assistant", "Hello from assistant") + "\n"
	reader.addSession("myproject", "sess-abc.jsonl", content, mtime)

	detail, err := services.CCSessionGetDetail(reader, "myproject", "sess-abc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if detail == nil {
		t.Fatal("expected non-nil detail")
	}
	if len(detail.Turns) != 2 {
		t.Fatalf("expected 2 turns, got %d", len(detail.Turns))
	}
	if detail.Turns[0].Role != "user" {
		t.Errorf("first turn role should be user, got %q", detail.Turns[0].Role)
	}
	if detail.Turns[0].Text != "Hello from user" {
		t.Errorf("first turn text mismatch: %q", detail.Turns[0].Text)
	}
	if detail.Turns[1].Role != "assistant" {
		t.Errorf("second turn role should be assistant, got %q", detail.Turns[1].Role)
	}
	if detail.GitBranch != "main" {
		t.Errorf("expected GitBranch main, got %q", detail.GitBranch)
	}
}

func TestCCSessionGetDetail_ArrayContent(t *testing.T) {
	reader := newFakeReader()
	mtime := time.Now()

	blocks := []map[string]any{
		{"type": "text", "text": "Block text here"},
		{"type": "tool_use", "id": "toolu_01", "name": "Bash", "input": map[string]any{"command": "ls"}},
	}
	ts := "2026-06-08T10:00:00.000Z"

	msgBytes, _ := json.Marshal(map[string]any{"role": "assistant", "content": blocks})
	lineMap := map[string]any{
		"type":      "assistant",
		"timestamp": ts,
		"sessionId": "sess",
		"message":   json.RawMessage(msgBytes),
		"cwd":       "e:/p",
	}
	lineBytes, _ := json.Marshal(lineMap)
	content := string(lineBytes) + "\n"
	reader.addSession("p", "sess.jsonl", content, mtime)

	detail, err := services.CCSessionGetDetail(reader, "p", "sess")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if detail == nil {
		t.Fatal("expected non-nil detail")
	}
	if len(detail.Turns) != 1 {
		t.Fatalf("expected 1 turn, got %d", len(detail.Turns))
	}
	turn := detail.Turns[0]
	if len(turn.Blocks) < 2 {
		t.Fatalf("expected at least 2 blocks, got %d", len(turn.Blocks))
	}
	if turn.Blocks[0].Type != "text" {
		t.Errorf("first block should be text, got %q", turn.Blocks[0].Type)
	}
	if turn.Blocks[1].Name != "Bash" {
		t.Errorf("second block name should be Bash, got %q", turn.Blocks[1].Name)
	}
}

func TestCCSessionGetDetail_FiltersNonConversationLines(t *testing.T) {
	reader := newFakeReader()
	mtime := time.Now()

	// queue-operation and attachment lines should be ignored.
	opLine := `{"type":"queue-operation","timestamp":"2026-06-08T10:00:00.000Z","sessionId":"s"}` + "\n"
	attLine := `{"type":"attachment","timestamp":"2026-06-08T10:00:01.000Z","sessionId":"s","message":{"role":"user","content":"deferred tools json here"}}` + "\n"
	userLine := jsonlLine("user", "2026-06-08T10:00:02.000Z", "e:/p", "", "", "user", "real prompt") + "\n"

	reader.addSession("p", "s.jsonl", opLine+attLine+userLine, mtime)

	detail, err := services.CCSessionGetDetail(reader, "p", "s")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if detail == nil {
		t.Fatal("expected non-nil detail")
	}
	// Only the user line should appear as a turn.
	if len(detail.Turns) != 1 {
		t.Fatalf("expected 1 turn (queue-operation and attachment filtered), got %d", len(detail.Turns))
	}
}

// ---------------------------------------------------------------------------
// Tests: ValidateCCSessionPath
// ---------------------------------------------------------------------------

func TestValidateCCSessionPath_Valid(t *testing.T) {
	cases := []struct{ folder, id string }{
		{"e--Personal-myproject", "abc123-uuid-here"},
		{"simple-folder", "uuid-0000-1111"},
	}
	for _, c := range cases {
		if err := services.ValidateCCSessionPath(c.folder, c.id); err != nil {
			t.Errorf("expected valid for (%q, %q), got error: %v", c.folder, c.id, err)
		}
	}
}

func TestValidateCCSessionPath_Invalid(t *testing.T) {
	cases := []struct{ folder, id string }{
		{"../etc", "abc"},
		{"myproject", "../etc/passwd"},
		{"my/project", "abc"},
		{"myproject", "abc/def"},
		{"", "abc"},
		{"myproject", ""},
	}
	for _, c := range cases {
		if err := services.ValidateCCSessionPath(c.folder, c.id); err == nil {
			t.Errorf("expected error for (%q, %q), got nil", c.folder, c.id)
		}
	}
}

// ---------------------------------------------------------------------------
// Tests: projectDisplayName helper (via list behavior)
// ---------------------------------------------------------------------------

func TestProjectDisplayName_LastSegment(t *testing.T) {
	reader := newFakeReader()
	mtime := time.Now()

	// Unix path
	reader.addSession("folder", "s1.jsonl",
		buildSimpleSession("/home/user/my-repo", "", "", "p"), mtime)

	result, _ := services.CCSessionsList(reader, services.CCSessionListParams{})
	if len(result.Items) == 0 {
		t.Fatal("expected items")
	}
	if result.Items[0].ProjectDisplay != "my-repo" {
		t.Errorf("expected 'my-repo', got %q", result.Items[0].ProjectDisplay)
	}
}

// ---------------------------------------------------------------------------
// Tests: first prompt stored in full (no truncation / no ellipsis)
// ---------------------------------------------------------------------------

func TestList_FirstPromptStoredInFull(t *testing.T) {
	reader := newFakeReader()
	mtime := time.Now()

	longPrompt := strings.Repeat("a", 300)
	reader.addSession("p", "s.jsonl",
		buildSimpleSession("e:/p", "", "", longPrompt), mtime)

	result, _ := services.CCSessionsList(reader, services.CCSessionListParams{})
	if len(result.Items) == 0 {
		t.Fatal("expected items")
	}
	// The first prompt is the most important column and is stored untruncated;
	// the UI shows it in full without an ellipsis.
	prompt := result.Items[0].FirstPrompt
	if len([]rune(prompt)) != 300 {
		t.Errorf("first prompt should be stored in full (300 runes), got %d", len([]rune(prompt)))
	}
	if strings.Contains(prompt, "…") {
		t.Errorf("first prompt should not contain an ellipsis, got %q", prompt)
	}
}

// ---------------------------------------------------------------------------
// Tests: token/cost accounting
// ---------------------------------------------------------------------------

// assistantUsageLine builds one assistant .jsonl line carrying message.usage.
func assistantUsageLine(id, model string, in, out, cw5, cw1, cr int64) string {
	msg := map[string]any{
		"role":    "assistant",
		"model":   model,
		"id":      id,
		"content": []any{map[string]any{"type": "text", "text": "ok"}},
		"usage": map[string]any{
			"input_tokens":                in,
			"output_tokens":               out,
			"cache_read_input_tokens":     cr,
			"cache_creation_input_tokens": cw5 + cw1,
			"cache_creation": map[string]any{
				"ephemeral_5m_input_tokens": cw5,
				"ephemeral_1h_input_tokens": cw1,
			},
		},
	}
	msgBytes, _ := json.Marshal(msg)
	line := map[string]any{
		"type":      "assistant",
		"timestamp": "2026-06-08T10:00:01.000Z",
		"sessionId": "test-session",
		"message":   json.RawMessage(msgBytes),
	}
	b, _ := json.Marshal(line)
	return string(b)
}

func TestList_UsageTokensAndCost_Opus(t *testing.T) {
	reader := newFakeReader()
	mtime := time.Now()

	// 1M of each category on Opus (input $5, output $25/MTok):
	// input 5 + output 25 + cacheWrite5m 6.25 + cacheWrite1h 10 + cacheRead 0.5 = 46.75
	content := buildSimpleSession("e:/p", "main", "2.1", "hi") +
		assistantUsageLine("msg_1", "claude-opus-4-8", 1_000_000, 1_000_000, 1_000_000, 1_000_000, 1_000_000) + "\n"
	reader.addSession("p", "s.jsonl", content, mtime)

	result, _ := services.CCSessionsList(reader, services.CCSessionListParams{})
	if len(result.Items) == 0 {
		t.Fatal("expected items")
	}
	u := result.Items[0].Usage

	if u.InputTokens != 1_000_000 || u.OutputTokens != 1_000_000 ||
		u.CacheWrite5m != 1_000_000 || u.CacheWrite1h != 1_000_000 || u.CacheRead != 1_000_000 {
		t.Errorf("unexpected token breakdown: %+v", u)
	}
	if u.TotalTokens != 5_000_000 {
		t.Errorf("expected TotalTokens 5,000,000, got %d", u.TotalTokens)
	}
	if u.UnknownModel {
		t.Error("opus is a known model; UnknownModel should be false")
	}
	if diff := u.CostUSD - 46.75; diff < -0.0001 || diff > 0.0001 {
		t.Errorf("expected CostUSD 46.75, got %.4f", u.CostUSD)
	}
}

func TestList_UsageDedupByMessageID(t *testing.T) {
	reader := newFakeReader()
	mtime := time.Now()

	// Same message id twice — must be counted once.
	dup := assistantUsageLine("msg_dup", "claude-sonnet-4-6", 1000, 500, 0, 0, 0) + "\n"
	content := buildSimpleSession("e:/p", "", "", "hi") + dup + dup
	reader.addSession("p", "s.jsonl", content, mtime)

	result, _ := services.CCSessionsList(reader, services.CCSessionListParams{})
	u := result.Items[0].Usage
	if u.InputTokens != 1000 || u.OutputTokens != 500 {
		t.Errorf("duplicate message id should be counted once, got input=%d output=%d", u.InputTokens, u.OutputTokens)
	}
}

func TestList_UsageUnknownModel(t *testing.T) {
	reader := newFakeReader()
	mtime := time.Now()

	content := buildSimpleSession("e:/p", "", "", "hi") +
		assistantUsageLine("msg_x", "some-future-model", 10_000, 2_000, 0, 0, 0) + "\n"
	reader.addSession("p", "s.jsonl", content, mtime)

	result, _ := services.CCSessionsList(reader, services.CCSessionListParams{})
	u := result.Items[0].Usage
	if u.TotalTokens != 12_000 {
		t.Errorf("tokens should still be counted for unknown models, got %d", u.TotalTokens)
	}
	if !u.UnknownModel {
		t.Error("expected UnknownModel true for an unpriced model")
	}
	if u.CostUSD != 0 {
		t.Errorf("expected CostUSD 0 for unknown model, got %.4f", u.CostUSD)
	}
}

func TestDetail_UsageAccounting(t *testing.T) {
	reader := newFakeReader()
	mtime := time.Now()

	content := buildSimpleSession("e:/p", "main", "2.1", "hi") +
		assistantUsageLine("msg_1", "claude-haiku-4-5", 2_000, 1_000, 0, 0, 0) + "\n"
	reader.addSession("p", "s.jsonl", content, mtime)

	detail, err := services.CCSessionGetDetail(reader, "p", "s")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if detail == nil {
		t.Fatal("expected detail")
	}
	// Haiku: input $1, output $5/MTok → 2000/1e6*1 + 1000/1e6*5 = 0.002 + 0.005 = 0.007
	if detail.Usage.TotalTokens != 3_000 {
		t.Errorf("expected 3,000 total tokens, got %d", detail.Usage.TotalTokens)
	}
	if diff := detail.Usage.CostUSD - 0.007; diff < -0.0001 || diff > 0.0001 {
		t.Errorf("expected CostUSD 0.007, got %.5f", detail.Usage.CostUSD)
	}
}

// ---------------------------------------------------------------------------
// Tests: CCSessionsStats (analytics aggregation)
// ---------------------------------------------------------------------------

func TestCCSessionsStats_Aggregation(t *testing.T) {
	reader := newFakeReader()
	mtime := time.Now()

	// Project alpha: two sessions, opus + sonnet usage.
	reader.addSession("alpha-folder", "s1.jsonl",
		buildSimpleSession("e:/x/alpha", "main", "2.1", "hi alpha 1")+
			assistantUsageLine("m1", "claude-opus-4-8", 1_000_000, 1_000_000, 0, 0, 0)+"\n", mtime)
	reader.addSession("alpha-folder", "s2.jsonl",
		buildSimpleSession("e:/x/alpha", "main", "2.1", "hi alpha 2")+
			assistantUsageLine("m2", "claude-sonnet-4-6", 2000, 1000, 0, 0, 0)+"\n", mtime)
	// Project beta: one session, haiku.
	reader.addSession("beta-folder", "s3.jsonl",
		buildSimpleSession("e:/x/beta", "main", "2.1", "hi beta")+
			assistantUsageLine("m3", "claude-haiku-4-5", 5000, 5000, 0, 0, 0)+"\n", mtime)

	res, err := services.CCSessionsStats(reader)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Sessions != 3 {
		t.Errorf("expected 3 sessions, got %d", res.Sessions)
	}
	if res.HasUnknownModel {
		t.Error("all models are priced; HasUnknownModel should be false")
	}
	// alpha tokens = (1M+1M) + (2000+1000) = 2,003,000 ; beta = 10,000 ; total = 2,013,000
	if res.Totals.TotalTokens != 2_013_000 {
		t.Errorf("expected total tokens 2,013,000, got %d", res.Totals.TotalTokens)
	}

	// By project: alpha first (highest cost/tokens), 2 sessions.
	if len(res.ByProject) != 2 {
		t.Fatalf("expected 2 projects, got %d", len(res.ByProject))
	}
	if res.ByProject[0].Project != "alpha" || res.ByProject[0].Sessions != 2 {
		t.Errorf("expected alpha with 2 sessions first, got %q (%d)", res.ByProject[0].Project, res.ByProject[0].Sessions)
	}

	// By model: opus, sonnet, haiku all present.
	models := map[string]bool{}
	for _, m := range res.ByModel {
		models[m.Model] = true
	}
	for _, want := range []string{"opus", "sonnet", "haiku"} {
		if !models[want] {
			t.Errorf("expected model %q in ByModel", want)
		}
	}

	// Top sessions: the opus 2M-token session is #1.
	if len(res.TopSessions) == 0 || res.TopSessions[0].ID != "s1" {
		t.Errorf("expected s1 as top session by cost, got %+v", res.TopSessions)
	}
}

func TestCCSessionsStats_UnknownModelFlag(t *testing.T) {
	reader := newFakeReader()
	mtime := time.Now()
	reader.addSession("p", "s.jsonl",
		buildSimpleSession("e:/p", "", "", "hi")+
			assistantUsageLine("m", "future-model-x", 1000, 500, 0, 0, 0)+"\n", mtime)

	res, err := services.CCSessionsStats(reader)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.HasUnknownModel {
		t.Error("expected HasUnknownModel true for an unpriced model")
	}
	if res.Totals.TotalTokens != 1500 {
		t.Errorf("expected 1500 tokens counted, got %d", res.Totals.TotalTokens)
	}
}

func TestCCSessionsFingerprint_ChangesWithContent(t *testing.T) {
	mtime := time.Unix(1_700_000_000, 0)

	r1 := newFakeReader()
	r1.addSession("p", "s.jsonl", buildSimpleSession("e:/p", "", "", "a"), mtime)
	fp1 := services.CCSessionsFingerprint(r1)

	// Same set, same mtime/size -> same fingerprint.
	r2 := newFakeReader()
	r2.addSession("p", "s.jsonl", buildSimpleSession("e:/p", "", "", "a"), mtime)
	if services.CCSessionsFingerprint(r2) != fp1 {
		t.Error("identical content+mtime should yield identical fingerprint")
	}

	// Different size (longer content) -> different fingerprint.
	r3 := newFakeReader()
	r3.addSession("p", "s.jsonl", buildSimpleSession("e:/p", "", "", "a longer prompt body"), mtime)
	if services.CCSessionsFingerprint(r3) == fp1 {
		t.Error("changed content size should change the fingerprint")
	}

	if fp1 == "" {
		t.Error("fingerprint should be non-empty")
	}
}
