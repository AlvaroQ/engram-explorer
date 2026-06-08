// Package services — Claude Code session reader.
// Reads transcript .jsonl files from ~/.claude/projects on-disk.
// No SQLite involved: data is live from the filesystem.
package services

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// Public types
// ---------------------------------------------------------------------------

// CCSessionListItem is one row in the Claude Code sessions list.
type CCSessionListItem struct {
	// ID is the session UUID (file stem, without .jsonl).
	ID string `json:"id"`
	// ProjectFolder is the raw encoded folder name on disk.
	ProjectFolder string `json:"project_folder"`
	// ProjectDisplay is derived from the cwd field inside the file: last path segment.
	ProjectDisplay string `json:"project_display"`
	// CWD is the working directory recorded in the transcript.
	CWD string `json:"cwd"`
	// GitBranch is the git branch recorded in the first relevant line.
	GitBranch string `json:"git_branch"`
	// CCVersion is the Claude Code version string from the transcript.
	CCVersion string `json:"cc_version"`
	// FirstPrompt is the text of the first user message (truncated).
	FirstPrompt string `json:"first_prompt"`
	// StartedAt is the timestamp of the first event in the transcript.
	StartedAt time.Time `json:"started_at"`
	// EndedAt is the file modification time (best approximation).
	EndedAt time.Time `json:"ended_at"`
	// FileSizeBytes is the size of the .jsonl file.
	FileSizeBytes int64 `json:"file_size_bytes"`
	// Usage holds token totals and the computed cost for the session.
	// For the list it is populated only for the rows actually returned (the
	// current page), via a full-file scan — the list scan itself reads only HEAD.
	Usage CCSessionUsage `json:"usage"`
}

// CCSessionUsage holds the aggregated token usage and computed cost for a
// session, summed over every assistant message's message.usage block.
// Token counts are EXACT (read straight from the transcript). CostUSD is exact
// too when every model is priced; UnknownModel flags that some assistant turns
// used a model absent from the pricing table, so their cost was counted as 0.
type CCSessionUsage struct {
	InputTokens  int64   `json:"input_tokens"`
	OutputTokens int64   `json:"output_tokens"`
	CacheWrite5m int64   `json:"cache_write_5m_tokens"`
	CacheWrite1h int64   `json:"cache_write_1h_tokens"`
	CacheRead    int64   `json:"cache_read_tokens"`
	TotalTokens  int64   `json:"total_tokens"`
	CostUSD      float64 `json:"cost_usd"`
	UnknownModel bool    `json:"unknown_model"`
}

// finalize derives the total token count from the per-category counters.
func (u *CCSessionUsage) finalize() {
	u.TotalTokens = u.InputTokens + u.OutputTokens + u.CacheWrite5m + u.CacheWrite1h + u.CacheRead
}

// CCSessionListParams holds query parameters for the cc-sessions list.
type CCSessionListParams struct {
	// Project filters by ProjectDisplay (derived from cwd). Empty = all.
	Project string
	// Cursor is the opaque cursor for load-more pagination.
	Cursor string
	// Limit is rows per page (default 50, max 500).
	Limit int
}

// CCSessionListResult is the paginated list response.
type CCSessionListResult struct {
	Items      []CCSessionListItem `json:"items"`
	Projects   []string            `json:"projects"`   // unique project display names for the filter selector
	NextCursor *string             `json:"nextCursor"` // nil when no more pages
}

// ---------------------------------------------------------------------------
// Detail types
// ---------------------------------------------------------------------------

// CCMessageBlock is one block inside a message content array.
type CCMessageBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
	// For tool_use blocks.
	ID   string `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
	// For tool_result blocks.
	ToolUseID string `json:"tool_use_id,omitempty"`
	// Raw content for tool_use/tool_result — displayed as summary.
	RawInput json.RawMessage `json:"input,omitempty"`
	Content  json.RawMessage `json:"content,omitempty"`
}

// CCTurn is one conversation turn (user or assistant).
type CCTurn struct {
	Role      string `json:"role"`
	Timestamp string `json:"timestamp"`
	// Text holds the plain-text content (string message).
	Text string `json:"text,omitempty"`
	// Blocks holds structured content when message.content is an array.
	Blocks []CCMessageBlock `json:"blocks,omitempty"`
}

// CCSessionDetail is the full detail for one Claude Code session.
type CCSessionDetail struct {
	ID             string         `json:"id"`
	ProjectFolder  string         `json:"project_folder"`
	ProjectDisplay string         `json:"project_display"`
	CWD            string         `json:"cwd"`
	GitBranch      string         `json:"git_branch"`
	CCVersion      string         `json:"cc_version"`
	StartedAt      time.Time      `json:"started_at"`
	EndedAt        time.Time      `json:"ended_at"`
	FileSizeBytes  int64          `json:"file_size_bytes"`
	Usage          CCSessionUsage `json:"usage"`
	Turns          []CCTurn       `json:"turns"`
}

// ---------------------------------------------------------------------------
// Internal raw line structs (unexported)
// ---------------------------------------------------------------------------

// ccRawLine is the minimal shape of one .jsonl line.
type ccRawLine struct {
	Type      string          `json:"type"`
	Timestamp string          `json:"timestamp"`
	SessionID string          `json:"sessionId"`
	CWD       string          `json:"cwd"`
	GitBranch string          `json:"gitBranch"`
	Version   string          `json:"version"`
	Message   json.RawMessage `json:"message"`
}

// ccRawMessage is the minimal shape of the "message" field.
type ccRawMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

// ccRawUsage mirrors the message.usage block emitted on every assistant line.
// The split ephemeral_5m / ephemeral_1h fields let us price cache writes at the
// correct multiplier (1.25x for 5-minute TTL, 2x for 1-hour).
type ccRawUsage struct {
	InputTokens              int64 `json:"input_tokens"`
	OutputTokens             int64 `json:"output_tokens"`
	CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
	CacheCreation            struct {
		Ephemeral5m int64 `json:"ephemeral_5m_input_tokens"`
		Ephemeral1h int64 `json:"ephemeral_1h_input_tokens"`
	} `json:"cache_creation"`
}

// ccUsageMessage is the assistant-line shape needed for usage accounting:
// id (for dedup), model (for pricing), and the usage block.
type ccUsageMessage struct {
	ID    string     `json:"id"`
	Model string     `json:"model"`
	Usage ccRawUsage `json:"usage"`
}

// ---------------------------------------------------------------------------
// Pricing (USD per 1M tokens). Cache write = input x{1.25 (5m), 2 (1h)};
// cache read = input x0.1. Source: Anthropic public pricing. Edit here when
// rates change — token counts above are exact regardless of this table.
// ---------------------------------------------------------------------------

type ccPricing struct {
	InputPerMTok  float64
	OutputPerMTok float64
}

// ccModelPricing matches by substring so both full IDs ("claude-opus-4-8") and
// bare aliases ("opus", "sonnet", "haiku") found in transcripts resolve.
var ccModelPricing = []struct {
	Match   string
	Pricing ccPricing
}{
	{Match: "opus", Pricing: ccPricing{InputPerMTok: 5.00, OutputPerMTok: 25.00}},
	{Match: "sonnet", Pricing: ccPricing{InputPerMTok: 3.00, OutputPerMTok: 15.00}},
	{Match: "haiku", Pricing: ccPricing{InputPerMTok: 1.00, OutputPerMTok: 5.00}},
}

func ccPriceForModel(model string) (ccPricing, bool) {
	m := strings.ToLower(model)
	for _, e := range ccModelPricing {
		if e.Match != "" && strings.Contains(m, e.Match) {
			return e.Pricing, true
		}
	}
	return ccPricing{}, false
}

// addMessageUsage folds one assistant message's usage into the accumulator,
// pricing each message by its own model (a session may mix models).
func addMessageUsage(u *CCSessionUsage, model string, usage ccRawUsage) {
	u.InputTokens += usage.InputTokens
	u.OutputTokens += usage.OutputTokens

	// Prefer the split TTL counters; fall back to the aggregate as 5m.
	cw5 := usage.CacheCreation.Ephemeral5m
	cw1 := usage.CacheCreation.Ephemeral1h
	if cw5 == 0 && cw1 == 0 {
		cw5 = usage.CacheCreationInputTokens
	}
	u.CacheWrite5m += cw5
	u.CacheWrite1h += cw1
	u.CacheRead += usage.CacheReadInputTokens

	if p, ok := ccPriceForModel(model); ok {
		const m = 1_000_000.0
		u.CostUSD += float64(usage.InputTokens)/m*p.InputPerMTok +
			float64(usage.OutputTokens)/m*p.OutputPerMTok +
			float64(cw5)/m*(p.InputPerMTok*1.25) +
			float64(cw1)/m*(p.InputPerMTok*2.0) +
			float64(usage.CacheReadInputTokens)/m*(p.InputPerMTok*0.1)
	} else if model != "" {
		u.UnknownModel = true
	}
}

// computeSessionUsage scans a full transcript and sums assistant-message usage,
// deduping by message id so a repeated line is never double-counted.
func computeSessionUsage(r io.Reader) CCSessionUsage {
	var u CCSessionUsage
	seen := make(map[string]struct{})

	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 1024*1024), 16*1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var raw struct {
			Type    string         `json:"type"`
			Message ccUsageMessage `json:"message"`
		}
		if err := json.Unmarshal(line, &raw); err != nil {
			continue
		}
		if raw.Type != "assistant" {
			continue
		}
		if raw.Message.ID != "" {
			if _, ok := seen[raw.Message.ID]; ok {
				continue
			}
			seen[raw.Message.ID] = struct{}{}
		}
		addMessageUsage(&u, raw.Message.Model, raw.Message.Usage)
	}

	u.finalize()
	return u
}

// ---------------------------------------------------------------------------
// List
// ---------------------------------------------------------------------------

// CCScanProjectsDir is the projects directory source injected into functions.
// This allows tests to override it without touching the real filesystem.
type CCProjectsReader interface {
	// ProjectEntries returns (name, isDir) pairs for the top-level entries.
	ProjectEntries() ([]fs.DirEntry, error)
	// SessionEntries returns (name, isRegular) pairs for .jsonl files in a project folder.
	SessionEntries(projectFolder string) ([]fs.DirEntry, error)
	// OpenSession returns a reader for a specific session file.
	OpenSession(projectFolder, sessionFile string) (io.ReadCloser, error)
	// SessionStat returns file info for a specific session file.
	SessionStat(projectFolder, sessionFile string) (fs.FileInfo, error)
}

// DiskProjectsReader reads from the real filesystem.
type DiskProjectsReader struct {
	BaseDir string
}

func (d DiskProjectsReader) ProjectEntries() ([]fs.DirEntry, error) {
	return os.ReadDir(d.BaseDir)
}

func (d DiskProjectsReader) SessionEntries(projectFolder string) ([]fs.DirEntry, error) {
	return os.ReadDir(filepath.Join(d.BaseDir, projectFolder))
}

func (d DiskProjectsReader) OpenSession(projectFolder, sessionFile string) (io.ReadCloser, error) {
	return os.Open(filepath.Join(d.BaseDir, projectFolder, sessionFile))
}

func (d DiskProjectsReader) SessionStat(projectFolder, sessionFile string) (fs.FileInfo, error) {
	return os.Stat(filepath.Join(d.BaseDir, projectFolder, sessionFile))
}

// ccSessionHead holds the metadata extracted from the HEAD of a .jsonl file.
type ccSessionHead struct {
	cwd         string
	gitBranch   string
	version     string
	firstAt     time.Time
	firstAtSet  bool
	firstPrompt string
}

// parseSessionHead reads up to headLineLimit lines and extracts metadata.
// The bufio.Scanner is pre-configured with a large buffer to handle big attachment lines.
const headLineLimit = 40

func parseSessionHead(r io.Reader) (ccSessionHead, error) {
	var h ccSessionHead
	scanner := bufio.NewScanner(r)
	// Mandatory: large buffer to handle attachment lines (deferred_tools, mcp_instructions
	// can be hundreds of KB). Without this, Scanner returns "token too long".
	scanner.Buffer(make([]byte, 1024*1024), 16*1024*1024)

	linesRead := 0
	for scanner.Scan() {
		linesRead++
		if linesRead > headLineLimit {
			break
		}
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var raw ccRawLine
		if err := json.Unmarshal(line, &raw); err != nil {
			continue // skip malformed lines robustly
		}

		// Collect metadata from any line that carries it.
		if raw.CWD != "" && h.cwd == "" {
			h.cwd = raw.CWD
		}
		if raw.GitBranch != "" && h.gitBranch == "" {
			h.gitBranch = raw.GitBranch
		}
		if raw.Version != "" && h.version == "" {
			h.version = raw.Version
		}
		if raw.Timestamp != "" && !h.firstAtSet {
			if t, err := time.Parse(time.RFC3339Nano, raw.Timestamp); err == nil {
				h.firstAt = t
				h.firstAtSet = true
			}
		}

		// Extract the first user prompt text.
		if h.firstPrompt == "" && raw.Type == "user" && len(raw.Message) > 0 {
			if text := extractFirstUserText(raw.Message); text != "" {
				h.firstPrompt = text
			}
		}
	}
	// Scanner.Err() returns nil on EOF, non-nil only on real errors.
	// We ignore "token too long" only for the lines we already processed.
	return h, nil
}

// extractFirstUserText returns the plain text from a raw message JSON.
// Handles both string content and array-of-blocks content.
func extractFirstUserText(rawMsg json.RawMessage) string {
	var msg ccRawMessage
	if err := json.Unmarshal(rawMsg, &msg); err != nil {
		return ""
	}
	if msg.Role != "user" {
		return ""
	}
	return extractTextFromContent(msg.Content)
}

// extractTextFromContent parses the content field (string or []block) and returns plain text.
func extractTextFromContent(content json.RawMessage) string {
	if len(content) == 0 {
		return ""
	}
	// Try string first.
	var s string
	if err := json.Unmarshal(content, &s); err == nil {
		return s
	}
	// Try array of blocks.
	var blocks []CCMessageBlock
	if err := json.Unmarshal(content, &blocks); err != nil {
		return ""
	}
	var sb strings.Builder
	for _, b := range blocks {
		if b.Type == "text" && b.Text != "" {
			if sb.Len() > 0 {
				sb.WriteByte('\n')
			}
			sb.WriteString(b.Text)
		}
	}
	return sb.String()
}

// projectDisplayName returns the last non-empty segment of a path.
// Used to get a human-readable name from the cwd field.
func projectDisplayName(cwd string) string {
	if cwd == "" {
		return ""
	}
	// Normalize separators.
	cwd = filepath.ToSlash(cwd)
	// Remove trailing slash.
	cwd = strings.TrimRight(cwd, "/")
	idx := strings.LastIndex(cwd, "/")
	if idx < 0 {
		return cwd
	}
	return cwd[idx+1:]
}

// CCSessionsList lists Claude Code sessions from a projects directory reader.
// It reads only the HEAD of each .jsonl to keep it fast.
func CCSessionsList(reader CCProjectsReader, p CCSessionListParams) (CCSessionListResult, error) {
	limit := p.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}

	projectDirs, err := reader.ProjectEntries()
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) || os.IsNotExist(err) {
			return CCSessionListResult{Items: []CCSessionListItem{}, Projects: []string{}}, nil
		}
		return CCSessionListResult{}, fmt.Errorf("list projects dir: %w", err)
	}

	var allItems []CCSessionListItem
	projectSet := make(map[string]struct{})

	for _, pd := range projectDirs {
		if !pd.IsDir() {
			continue
		}
		projectFolder := pd.Name()

		entries, err := reader.SessionEntries(projectFolder)
		if err != nil {
			continue // skip unreadable project dirs
		}

		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			name := e.Name()
			// Only .jsonl files in the project root (skip subdirs handled above).
			if !strings.HasSuffix(name, ".jsonl") {
				continue
			}

			fi, err := reader.SessionStat(projectFolder, name)
			if err != nil {
				continue
			}

			// Parse HEAD for metadata.
			f, err := reader.OpenSession(projectFolder, name)
			if err != nil {
				continue
			}
			head, _ := parseSessionHead(f)
			f.Close()

			sessionID := strings.TrimSuffix(name, ".jsonl")
			display := projectDisplayName(head.cwd)
			if display == "" {
				display = projectFolder
			}

			// Apply project filter if set.
			if p.Project != "" && display != p.Project {
				continue
			}

			startedAt := head.firstAt
			if startedAt.IsZero() {
				// Fall back to file mtime as best effort.
				startedAt = fi.ModTime()
			}

			item := CCSessionListItem{
				ID:             sessionID,
				ProjectFolder:  projectFolder,
				ProjectDisplay: display,
				CWD:            head.cwd,
				GitBranch:      head.gitBranch,
				CCVersion:      head.version,
				// First prompt is the most important column and is shown in full
				// (no ellipsis) in the UI, so store it untruncated here.
				FirstPrompt:   head.firstPrompt,
				StartedAt:     startedAt,
				EndedAt:       fi.ModTime(),
				FileSizeBytes: fi.Size(),
			}
			allItems = append(allItems, item)
			projectSet[display] = struct{}{}
		}
	}

	// Sort by EndedAt desc (most-recently-modified file first).
	sort.Slice(allItems, func(i, j int) bool {
		return allItems[i].EndedAt.After(allItems[j].EndedAt)
	})

	// Cursor-based pagination: the cursor encodes the index into the sorted slice.
	// Simple offset cursor: encode the "skip N" count as a base-10 string.
	// This is intentionally simple since the list is rebuilt from disk on every request.
	skip := 0
	if p.Cursor != "" {
		if n, err := strconv.Atoi(p.Cursor); err == nil && n > 0 {
			skip = n
		}
	}
	if skip > len(allItems) {
		skip = len(allItems)
	}
	allItems = allItems[skip:]

	var nextCursor *string
	if len(allItems) > limit {
		allItems = allItems[:limit]
		next := fmt.Sprintf("%d", skip+limit)
		nextCursor = &next
	}

	// Second phase: token/cost accounting for the page only. The HEAD scan above
	// touched every file cheaply; here we full-scan ONLY the rows being returned
	// so a large project doesn't make the list pay to read every transcript.
	for i := range allItems {
		f, err := reader.OpenSession(allItems[i].ProjectFolder, allItems[i].ID+".jsonl")
		if err != nil {
			continue
		}
		allItems[i].Usage = computeSessionUsage(f)
		f.Close()
	}

	// Stable unique project list (sorted).
	projects := make([]string, 0, len(projectSet))
	for k := range projectSet {
		projects = append(projects, k)
	}
	sort.Strings(projects)

	if allItems == nil {
		allItems = []CCSessionListItem{}
	}

	return CCSessionListResult{
		Items:      allItems,
		Projects:   projects,
		NextCursor: nextCursor,
	}, nil
}

// ---------------------------------------------------------------------------
// Detail
// ---------------------------------------------------------------------------

// CCSessionGetDetail parses the full .jsonl for one session and returns its turns.
func CCSessionGetDetail(reader CCProjectsReader, projectFolder, sessionID string) (*CCSessionDetail, error) {
	fileName := sessionID + ".jsonl"

	fi, err := reader.SessionStat(projectFolder, fileName)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) || os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("stat session file: %w", err)
	}

	f, err := reader.OpenSession(projectFolder, fileName)
	if err != nil {
		return nil, fmt.Errorf("open session file: %w", err)
	}
	defer f.Close()

	detail := &CCSessionDetail{
		ID:            sessionID,
		ProjectFolder: projectFolder,
		FileSizeBytes: fi.Size(),
		EndedAt:       fi.ModTime(),
	}

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 16*1024*1024)

	firstAtSet := false
	usageSeen := make(map[string]struct{})

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var raw ccRawLine
		if err := json.Unmarshal(line, &raw); err != nil {
			continue // skip malformed lines robustly
		}

		// Collect metadata from any line.
		if raw.CWD != "" && detail.CWD == "" {
			detail.CWD = raw.CWD
			detail.ProjectDisplay = projectDisplayName(raw.CWD)
		}
		if raw.GitBranch != "" && detail.GitBranch == "" {
			detail.GitBranch = raw.GitBranch
		}
		if raw.Version != "" && detail.CCVersion == "" {
			detail.CCVersion = raw.Version
		}
		if raw.Timestamp != "" && !firstAtSet {
			if t, err := time.Parse(time.RFC3339Nano, raw.Timestamp); err == nil {
				detail.StartedAt = t
				firstAtSet = true
			}
		}

		// Token/cost accounting: assistant lines carry message.usage.
		if raw.Type == "assistant" && len(raw.Message) > 0 {
			var um ccUsageMessage
			if err := json.Unmarshal(raw.Message, &um); err == nil {
				if um.ID == "" {
					addMessageUsage(&detail.Usage, um.Model, um.Usage)
				} else if _, ok := usageSeen[um.ID]; !ok {
					usageSeen[um.ID] = struct{}{}
					addMessageUsage(&detail.Usage, um.Model, um.Usage)
				}
			}
		}

		// Only conversation turns (user/assistant).
		if raw.Type != "user" && raw.Type != "assistant" {
			continue
		}
		if len(raw.Message) == 0 {
			continue
		}

		var msg ccRawMessage
		if err := json.Unmarshal(raw.Message, &msg); err != nil {
			continue
		}

		// Filter non-conversation user lines (e.g. tool_result-only turns can still be useful,
		// but queue-operation/attachment type lines are already excluded by raw.Type check).
		turn := buildTurn(msg, raw.Timestamp)
		if turn == nil {
			continue
		}
		detail.Turns = append(detail.Turns, *turn)
	}

	// A scan error other than an oversized line means real I/O trouble — surface it
	// rather than silently returning a partial conversation. Oversized lines (rare
	// giant attachment blobs beyond the 16MB cap) are tolerated: we keep what we read.
	if err := scanner.Err(); err != nil && !errors.Is(err, bufio.ErrTooLong) {
		return nil, fmt.Errorf("scan session %s: %w", sessionID, err)
	}

	detail.Usage.finalize()

	if detail.ProjectDisplay == "" {
		detail.ProjectDisplay = projectFolder
	}

	if detail.Turns == nil {
		detail.Turns = []CCTurn{}
	}

	return detail, nil
}

// buildTurn converts a ccRawMessage into a CCTurn, returning nil if the message
// has no user-visible content worth displaying.
func buildTurn(msg ccRawMessage, timestamp string) *CCTurn {
	turn := &CCTurn{
		Role:      msg.Role,
		Timestamp: timestamp,
	}

	// Try string content.
	var s string
	if err := json.Unmarshal(msg.Content, &s); err == nil {
		if strings.TrimSpace(s) == "" {
			return nil
		}
		turn.Text = s
		return turn
	}

	// Try array of blocks.
	var blocks []CCMessageBlock
	if err := json.Unmarshal(msg.Content, &blocks); err != nil {
		return nil
	}
	if len(blocks) == 0 {
		return nil
	}
	turn.Blocks = blocks
	return turn
}

// ---------------------------------------------------------------------------
// Path safety
// ---------------------------------------------------------------------------

// ValidateCCSessionPath validates that projectFolder and sessionID are safe
// to use as filesystem path components (no traversal, no separators).
// Returns an error if validation fails.
func ValidateCCSessionPath(projectFolder, sessionID string) error {
	for _, s := range []string{projectFolder, sessionID} {
		if strings.Contains(s, "..") ||
			strings.ContainsAny(s, "/\\") ||
			s == "" {
			return fmt.Errorf("invalid path component: %q", s)
		}
	}
	return nil
}
