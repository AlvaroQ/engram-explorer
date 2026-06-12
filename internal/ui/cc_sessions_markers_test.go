package ui

import (
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/services"
)

// ---------------------------------------------------------------------------
// ccsSplitMarkers
// ---------------------------------------------------------------------------

func TestCCSSplitMarkers_PureText(t *testing.T) {
	segs := ccsSplitMarkers("hello world")
	if len(segs) != 1 {
		t.Fatalf("want 1 segment, got %d", len(segs))
	}
	if segs[0].IsMarker {
		t.Fatal("single segment must be plain text")
	}
	if segs[0].Text != "hello world" {
		t.Fatalf("want %q, got %q", "hello world", segs[0].Text)
	}
}

func TestCCSSplitMarkers_PureMarker(t *testing.T) {
	input := "<ide_opened_file>some content here</ide_opened_file>"
	segs := ccsSplitMarkers(input)
	if len(segs) != 1 {
		t.Fatalf("want 1 segment, got %d: %+v", len(segs), segs)
	}
	if !segs[0].IsMarker {
		t.Fatal("segment must be a marker")
	}
	if segs[0].Tag != "ide_opened_file" {
		t.Fatalf("want tag %q, got %q", "ide_opened_file", segs[0].Tag)
	}
	if segs[0].Text != input {
		t.Fatalf("marker Text must be the full raw marker")
	}
}

func TestCCSSplitMarkers_MixedTextMarkerText(t *testing.T) {
	input := "before\n<ide_selection>selected text</ide_selection>\nafter"
	segs := ccsSplitMarkers(input)
	if len(segs) != 3 {
		t.Fatalf("want 3 segments, got %d: %+v", len(segs), segs)
	}
	if segs[0].IsMarker {
		t.Errorf("seg[0] must be plain text, got marker %q", segs[0].Tag)
	}
	if !segs[1].IsMarker {
		t.Errorf("seg[1] must be a marker")
	}
	if segs[1].Tag != "ide_selection" {
		t.Errorf("seg[1] tag: want %q, got %q", "ide_selection", segs[1].Tag)
	}
	if segs[2].IsMarker {
		t.Errorf("seg[2] must be plain text")
	}
}

func TestCCSSplitMarkers_MultipleMarkers(t *testing.T) {
	input := "<ide_opened_file>f</ide_opened_file><ide_diagnostics>d</ide_diagnostics>"
	segs := ccsSplitMarkers(input)
	if len(segs) != 2 {
		t.Fatalf("want 2 marker segments, got %d: %+v", len(segs), segs)
	}
	if !segs[0].IsMarker || segs[0].Tag != "ide_opened_file" {
		t.Errorf("seg[0]: want ide_opened_file marker, got %+v", segs[0])
	}
	if !segs[1].IsMarker || segs[1].Tag != "ide_diagnostics" {
		t.Errorf("seg[1]: want ide_diagnostics marker, got %+v", segs[1])
	}
}

func TestCCSSplitMarkers_RealWorldMixedCase(t *testing.T) {
	// Mirrors the real-world case from the bug report:
	// <local-command-caveat>...</local-command-caveat>\n<command-name>/model</command-name>\nreal prompt text
	input := "<local-command-caveat>caveat body</local-command-caveat>\n" +
		"<command-name>/model</command-name>\n" +
		"real prompt text"

	segs := ccsSplitMarkers(input)

	// Expect: marker(local-command-caveat) + marker(command-name) + plain("real prompt text")
	var markerTags []string
	var plainTexts []string
	for _, seg := range segs {
		if seg.IsMarker {
			markerTags = append(markerTags, seg.Tag)
		} else {
			plainTexts = append(plainTexts, seg.Text)
		}
	}

	if len(markerTags) != 2 {
		t.Errorf("want 2 marker segments, got %d: %v", len(markerTags), markerTags)
	}
	if len(plainTexts) != 1 {
		t.Errorf("want 1 plain-text segment, got %d: %v", len(plainTexts), plainTexts)
	}
	if len(plainTexts) == 1 {
		plain := ccsPlainFromSegments(segs)
		if plain != "real prompt text" {
			t.Errorf("plain text: want %q, got %q", "real prompt text", plain)
		}
	}

	// Verify the marker tags
	found := map[string]bool{}
	for _, tag := range markerTags {
		found[tag] = true
	}
	if !found["local-command-caveat"] {
		t.Error("expected local-command-caveat marker segment")
	}
	if !found["command-name"] {
		t.Error("expected command-name marker segment")
	}
}

func TestCCSSplitMarkers_MultilineMarkerContent(t *testing.T) {
	input := "<system-reminder>\nline one\nline two\n</system-reminder>\nuser text"
	segs := ccsSplitMarkers(input)

	if len(segs) < 2 {
		t.Fatalf("want at least 2 segments, got %d", len(segs))
	}
	if !segs[0].IsMarker || segs[0].Tag != "system-reminder" {
		t.Errorf("seg[0]: want system-reminder marker, got %+v", segs[0])
	}
	plain := ccsPlainFromSegments(segs)
	if plain != "user text" {
		t.Errorf("plain from multiline case: want %q, got %q", "user text", plain)
	}
}

func TestCCSSplitMarkers_EmptyString(t *testing.T) {
	segs := ccsSplitMarkers("")
	// Should return one plain-text segment (empty) — or none if whitespace-only is dropped.
	// Either is fine but must not panic.
	_ = segs
}

// ---------------------------------------------------------------------------
// ccsTurnPreview — marker stripping in preview
// ---------------------------------------------------------------------------

func TestCCSTurnPreview_StripMarkers(t *testing.T) {
	turn := services.CCTurn{
		Role: "user",
		Text: "<local-command-caveat>caveat</local-command-caveat>\n<command-name>/model</command-name>\nreal prompt text",
	}
	preview := ccsTurnPreview(turn)
	if preview == "" {
		t.Fatal("preview must not be empty")
	}
	// Must NOT contain the raw tag
	if containsRawTag(preview) {
		t.Errorf("preview contains raw marker tag: %q", preview)
	}
	// Must contain the real text
	if preview != "real prompt text" && len(preview) > 0 {
		// Accept truncated version or full text — just must not be a raw tag
		// and must start with the real content
		if preview[:min(16, len(preview))] != "real prompt text"[:min(16, len(preview))] {
			t.Errorf("preview should start with 'real prompt text', got %q", preview)
		}
	}
}

func TestCCSTurnPreview_OnlyMarkers(t *testing.T) {
	turn := services.CCTurn{
		Role: "user",
		Text: "<ide_opened_file>some file content</ide_opened_file>",
	}
	preview := ccsTurnPreview(turn)
	// When only markers present, must show attachment label, not raw tags.
	if containsRawTag(preview) {
		t.Errorf("preview of marker-only turn contains raw tag: %q", preview)
	}
	if preview == "" {
		t.Error("preview must not be empty for marker-only turn")
	}
}

func TestCCSTurnPreview_PlainText(t *testing.T) {
	turn := services.CCTurn{
		Role: "user",
		Text: "a simple prompt without any markers",
	}
	preview := ccsTurnPreview(turn)
	if preview != "a simple prompt without any markers" {
		t.Errorf("plain-text turn preview: want full text, got %q", preview)
	}
}

// ---------------------------------------------------------------------------
// ccsStripAttachments — unified tag set includes new tags
// ---------------------------------------------------------------------------

func TestCCSStripAttachments_LocalCommandCaveat(t *testing.T) {
	input := "<local-command-caveat>Caveat text here</local-command-caveat>\nreal user message"
	clean, hasAttach := ccsStripAttachments(input)
	if !hasAttach {
		t.Error("local-command-caveat must be detected as an attachment")
	}
	if clean != "real user message" {
		t.Errorf("stripped result: want %q, got %q", "real user message", clean)
	}
}

func TestCCSStripAttachments_CommandName(t *testing.T) {
	input := "<command-name>/model</command-name>\nsome prompt"
	clean, hasAttach := ccsStripAttachments(input)
	if !hasAttach {
		t.Error("command-name must be detected as an attachment")
	}
	if clean != "some prompt" {
		t.Errorf("stripped result: want %q, got %q", "some prompt", clean)
	}
}

func TestCCSStripAttachments_SystemReminder(t *testing.T) {
	input := "<system-reminder>\nThe following skills are available...\n</system-reminder>\nuser text"
	clean, hasAttach := ccsStripAttachments(input)
	if !hasAttach {
		t.Error("system-reminder must be detected as an attachment")
	}
	if clean != "user text" {
		t.Errorf("stripped result: want %q, got %q", "user text", clean)
	}
}

func TestCCSStripAttachments_NoMarkers(t *testing.T) {
	input := "just a normal prompt"
	clean, hasAttach := ccsStripAttachments(input)
	if hasAttach {
		t.Error("no markers present — hasAttach must be false")
	}
	if clean != "just a normal prompt" {
		t.Errorf("clean: want original text, got %q", clean)
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// containsRawTag reports whether s contains any recognisable raw opening marker tag.
func containsRawTag(s string) bool {
	for _, tag := range ccsMarkerTags {
		if len(s) > len(tag)+2 {
			for i := range s {
				if i+len(tag)+2 <= len(s) && s[i] == '<' && s[i+1:i+1+len(tag)] == tag {
					return true
				}
			}
		}
	}
	return false
}
