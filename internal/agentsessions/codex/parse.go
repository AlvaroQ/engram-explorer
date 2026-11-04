package codex

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"strings"
	"time"
)

// MaxLineBytes caps one rollout line; longer lines are skipped and counted.
const MaxLineBytes = 16 << 20

// line is the union of the envelope shape ({timestamp,type,payload}) and the
// legacy pre-envelope shapes (bare meta {id,timestamp,instructions},
// {record_type:"state"} and bare response items).
type line struct {
	Timestamp  string          `json:"timestamp"`
	Type       string          `json:"type"`
	Payload    json.RawMessage `json:"payload"`
	ID         string          `json:"id"`
	RecordType string          `json:"record_type"`
}

type sessionMeta struct {
	ID           string `json:"id"`
	SessionID    string `json:"session_id"`
	Timestamp    string `json:"timestamp"`
	CWD          string `json:"cwd"`
	CLIVersion   string `json:"cli_version"`
	ForkedFromID string `json:"forked_from_id"`
	Git          *struct {
		Branch string `json:"branch"`
	} `json:"git"`
	legacy bool // decoded from a pre-envelope first line
}

type turnContext struct {
	CWD   string `json:"cwd"`
	Model string `json:"model"`
}

type eventMsg struct {
	Type string `json:"type"`
	Info *struct {
		Total tokenUsage `json:"total_token_usage"`
	} `json:"info"`
}

type usageRecord struct {
	ResponseID string     `json:"response_id"`
	Model      string     `json:"model"`
	Usage      tokenUsage `json:"usage"`
}

// item is a Codex ResponseItem (only the fields the reader uses).
type item struct {
	Type      string          `json:"type"`
	Role      string          `json:"role"`
	Content   json.RawMessage `json:"content"`
	Summary   []textPart      `json:"summary"`
	Name      string          `json:"name"`
	Arguments string          `json:"arguments"`
	Input     string          `json:"input"`
	CallID    string          `json:"call_id"`
	Output    json.RawMessage `json:"output"`
	Action    struct {
		Command []string `json:"command"`
		Query   string   `json:"query"`
	} `json:"action"`
}

type textPart struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

var knownItems = map[string]bool{
	"message": true, "reasoning": true, "function_call": true, "function_call_output": true,
	"custom_tool_call": true, "custom_tool_call_output": true, "local_shell_call": true, "web_search_call": true,
}

// visitor receives the decoded records of one file in order.
type visitor interface {
	meta(ts time.Time, m sessionMeta)
	turnContext(ts time.Time, tc turnContext)
	item(ts time.Time, it item)
	tokenCount(ts time.Time, total tokenUsage)
	record(ts time.Time, r usageRecord)
	stamp(ts time.Time) // any line with a valid timestamp
	skip()              // malformed, oversized or unknown line
}

// scan reads r line by line and dispatches to v. Bad lines are never fatal.
func scan(r io.Reader, v visitor) error {
	br := bufio.NewReaderSize(r, 64<<10)
	var buf []byte
	first := true
	for {
		raw, tooLong, err := readLine(br, buf[:0])
		buf = raw
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if tooLong {
			v.skip()
			continue
		}
		if len(bytes.TrimSpace(raw)) == 0 {
			continue
		}
		var l line
		if json.Unmarshal(raw, &l) != nil {
			v.skip()
			continue
		}
		dispatch(raw, l, first, v)
		first = false
	}
}

func dispatch(raw []byte, l line, first bool, v visitor) {
	ts, tsErr := time.Parse(time.RFC3339Nano, l.Timestamp)
	if tsErr == nil {
		v.stamp(ts)
	}
	switch {
	case l.RecordType != "":
		return // legacy state snapshot: known, carries nothing we show
	case len(l.Payload) > 0:
		if !envelope(ts, l, v) {
			v.skip()
		}
	case l.Type == "" && l.ID != "" && first:
		v.meta(ts, sessionMeta{ID: l.ID, Timestamp: l.Timestamp, legacy: true}) // legacy first line
	case knownItems[l.Type]:
		var it item
		if json.Unmarshal(raw, &it) != nil {
			v.skip()
			return
		}
		v.item(ts, it) // legacy bare item
	default:
		v.skip()
	}
}

// envelope handles one {timestamp,type,payload} line; false = skipped.
func envelope(ts time.Time, l line, v visitor) bool {
	switch l.Type {
	case "session_meta":
		var m sessionMeta
		if json.Unmarshal(l.Payload, &m) != nil {
			return false
		}
		v.meta(ts, m)
	case "turn_context":
		var tc turnContext
		if json.Unmarshal(l.Payload, &tc) != nil {
			return false
		}
		v.turnContext(ts, tc)
	case "response_item":
		var it item
		if json.Unmarshal(l.Payload, &it) != nil || !knownItems[it.Type] {
			return false
		}
		v.item(ts, it)
	case "event_msg":
		var e eventMsg
		if json.Unmarshal(l.Payload, &e) != nil {
			return false
		}
		if e.Type == "token_count" && e.Info != nil {
			v.tokenCount(ts, e.Info.Total)
		}
	case "token_usage_record":
		var r usageRecord
		if json.Unmarshal(l.Payload, &r) != nil {
			return false
		}
		v.record(ts, r)
	case "compacted":
	default:
		return false
	}
	return true
}

// readLine returns the next line without its terminator. Lines longer than
// MaxLineBytes are drained and reported with tooLong=true.
func readLine(br *bufio.Reader, buf []byte) ([]byte, bool, error) {
	tooLong := false
	for {
		frag, isPrefix, err := br.ReadLine()
		if err != nil {
			if len(buf) > 0 || tooLong {
				return buf, tooLong, nil
			}
			return nil, false, err
		}
		if !tooLong {
			if len(buf)+len(frag) > MaxLineBytes {
				tooLong, buf = true, buf[:0]
			} else {
				buf = append(buf, frag...)
			}
		}
		if !isPrefix {
			return buf, tooLong, nil
		}
	}
}

// contentText joins the text parts of a message content array.
func contentText(raw json.RawMessage) string {
	var parts []textPart
	if json.Unmarshal(raw, &parts) != nil {
		return ""
	}
	var texts []string
	for _, p := range parts {
		switch p.Type {
		case "input_text", "output_text", "text", "summary_text":
			if p.Text != "" {
				texts = append(texts, p.Text)
			}
		}
	}
	return strings.Join(texts, "\n")
}

// syntheticTag matches a message that is entirely one harness tag, e.g.
// "<environment_context>...</environment_context>" or "<user_instructions>".
var syntheticTag = regexp.MustCompile(`(?s)^<([a-z0-9]+_[a-z0-9_]+)>.*</([a-z0-9_]+)>$`)

// isSynthetic reports harness-injected user messages that are not prompts.
func isSynthetic(text string) bool {
	t := strings.TrimSpace(text)
	if strings.HasPrefix(t, "# AGENTS.md instructions for ") {
		return true
	}
	m := syntheticTag.FindStringSubmatch(t)
	return m != nil && m[1] == m[2]
}
