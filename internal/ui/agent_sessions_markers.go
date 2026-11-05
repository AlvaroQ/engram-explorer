package ui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/AlvaroQ/engram-explorer/internal/agentsessions"
)

// Transcript helpers shared by the agent-session pages: per-agent IDE /
// harness marker handling (marker chips in transcripts, marker-free prompt
// previews) and the token, size and time formatters. Moved here from the
// former Claude Code session templates.

// ccsMarkerInner returns the whitespace-normalised text between a marker's open
// and close tags, or "" when the marker has no inner text.
func ccsMarkerInner(full string) string {
	open := strings.IndexByte(full, '>')
	closeAt := strings.LastIndex(full, "</")
	if open < 0 || closeAt <= open {
		return ""
	}
	return strings.Join(strings.Fields(full[open+1:closeAt]), " ")
}

// ccsMarkerLabel is the chip label: the inner marker text (truncated) when
// present, otherwise the bare tag name.
func ccsMarkerLabel(tag, full string) string {
	if inner := ccsMarkerInner(full); inner != "" {
		return ccsPromptPreview(inner)
	}
	return tag
}

// Marker tags are agent-specific: each agent reader registers its own set with
// agentsessions.RegisterMarkers (Claude Code: services.CCMarkerTags) and these
// helpers look it up by the session's agent kind, so a Codex prompt is never
// stripped with Claude Code tags.

// ccsMarkerTags is the Claude Code marker set's tag list, read from the
// registry (used by ccsAttachment and the transcript tests).
var ccsMarkerTags = agentsessions.Markers(claudeAgent).Tags()

// asMarkers is the marker set of agent. Hosts stamp the agent on every session
// they render; an unstamped session (empty agent) keeps the historical Claude
// Code set, while any other agent only ever gets its own registered set.
func asMarkers(agent string) agentsessions.MarkerSet {
	if agent == "" {
		agent = claudeAgent
	}
	return agentsessions.Markers(agent)
}

// ccsSegment is one piece of a split transcript string.
type ccsSegment struct {
	IsMarker bool   // true → marker chip; false → plain text
	Tag      string // tag name when IsMarker
	Text     string // full marker text (IsMarker) or plain text (!IsMarker)
}

// markerSpan records the position and tag of one matched marker in a string.
type markerSpan struct {
	start, end int
	tag        string
}

// ccsSplitMarkers splits s with the Claude Code marker set (asSplitMarkers).
func ccsSplitMarkers(s string) []ccsSegment { return asSplitMarkers(claudeAgent, s) }

// asSplitMarkers splits s into an ordered sequence of plain-text and marker
// segments, using the marker set of agent. Marker spans are found by running
// each tag's regexp and collecting [start,end,tag] tuples; overlapping spans
// are dropped (first/leftmost wins). Whitespace-only plain segments are omitted.
func asSplitMarkers(agent, s string) []ccsSegment {
	set := asMarkers(agent)
	tags := set.Tags()
	// Collect all marker spans across all tags.
	var spans []markerSpan
	for i, re := range set.Patterns() {
		for _, loc := range re.FindAllStringIndex(s, -1) {
			spans = append(spans, markerSpan{start: loc[0], end: loc[1], tag: tags[i]})
		}
	}
	if len(spans) == 0 {
		// No markers: single plain-text segment.
		return []ccsSegment{{IsMarker: false, Text: s}}
	}

	// Sort by start position; on tie keep the longer span (greedy).
	sort.Slice(spans, func(i, j int) bool {
		if spans[i].start != spans[j].start {
			return spans[i].start < spans[j].start
		}
		return spans[i].end > spans[j].end
	})

	// Remove overlapping spans: keep a span only if its start is >= the end
	// of the last kept span.
	kept := spans[:0]
	cursor := 0
	for _, sp := range spans {
		if sp.start >= cursor {
			kept = append(kept, sp)
			cursor = sp.end
		}
	}
	spans = kept

	// Walk the string emitting segments.
	var out []ccsSegment
	pos := 0
	for _, sp := range spans {
		if pos < sp.start {
			plain := s[pos:sp.start]
			if strings.TrimSpace(plain) != "" {
				out = append(out, ccsSegment{IsMarker: false, Text: plain})
			}
		}
		out = append(out, ccsSegment{IsMarker: true, Tag: sp.tag, Text: s[sp.start:sp.end]})
		pos = sp.end
	}
	if pos < len(s) {
		plain := s[pos:]
		if strings.TrimSpace(plain) != "" {
			out = append(out, ccsSegment{IsMarker: false, Text: plain})
		}
	}
	return out
}

// ccsAttachment reports whether text is a single wrapped marker block and, if so,
// returns the tag name for the chip label. Used for the all-or-nothing fast path
// (e.g. list view first-prompt detection).
func ccsAttachment(text string) (string, bool) {
	t := strings.TrimSpace(text)
	for _, tag := range ccsMarkerTags {
		if strings.HasPrefix(t, "<"+tag+">") || strings.HasPrefix(t, "<"+tag+" ") {
			return tag, true
		}
	}
	return "", false
}

// ccsPlainFromSegments joins plain-text segments into a single normalised string.
func ccsPlainFromSegments(segs []ccsSegment) string {
	var parts []string
	for _, seg := range segs {
		if !seg.IsMarker {
			parts = append(parts, seg.Text)
		}
	}
	return strings.Join(strings.Fields(strings.Join(parts, " ")), " ")
}

// ccsPromptPreview truncates the prompt for the table preview.
func ccsPromptPreview(s string) string {
	runes := []rune(s)
	// Remove newlines for single-line display.
	for i, r := range runes {
		if r == '\n' || r == '\r' {
			runes[i] = ' '
		}
	}
	max := 80
	if len(runes) <= max {
		return string(runes)
	}
	return string(runes[:max]) + "…"
}

// ccsStripAttachments strips a prompt with the Claude Code marker set
// (asStripMarkers).
func ccsStripAttachments(s string) (string, bool) { return asStripMarkers(claudeAgent, s) }

// asStripMarkers removes every marker of agent's set from a prompt and reports
// whether any were present. Whitespace in the result is collapsed to one line.
func asStripMarkers(agent, s string) (string, bool) {
	return asMarkers(agent).Strip(s)
}

// ccsFormatTokens renders a token count compactly (e.g. 850, 12.3k, 1.45M).
func ccsFormatTokens(n int64) string {
	switch {
	case n <= 0:
		return "—"
	case n < 1000:
		return fmt.Sprintf("%d", n)
	case n < 1_000_000:
		return fmt.Sprintf("%.1fk", float64(n)/1000)
	default:
		return fmt.Sprintf("%.2fM", float64(n)/1_000_000)
	}
}

// ccsFormatTime formats a time.Time for display in the table.
func ccsFormatTime(t interface {
	IsZero() bool
	Format(string) string
}) string {
	if t.IsZero() {
		return "—"
	}
	return t.Format("2006-01-02 15:04")
}

// ccsFormatDate renders just the date as dd-mm-yyyy (top line of the time cell).
func ccsFormatDate(t interface {
	IsZero() bool
	Format(string) string
}) string {
	if t.IsZero() {
		return "—"
	}
	return t.Format("02-01-2006")
}

// ccsFormatClock renders just the time as HH:MM (second line of the time cell).
func ccsFormatClock(t interface {
	IsZero() bool
	Format(string) string
}) string {
	if t.IsZero() {
		return ""
	}
	return t.Format("15:04")
}

// ccsFormatSize formats a file size in human-readable form.
func ccsFormatSize(bytes int64) string {
	switch {
	case bytes < 1024:
		return fmt.Sprintf("%d B", bytes)
	case bytes < 1024*1024:
		return fmt.Sprintf("%d KB", bytes/1024)
	default:
		return fmt.Sprintf("%.1f MB", float64(bytes)/(1024*1024))
	}
}

// ccsTurnBadgeClass returns the badge class based on role.
func ccsTurnBadgeClass(role string) string {
	if role == "user" {
		return "badge badge-accent"
	}
	return "badge badge-neutral"
}
