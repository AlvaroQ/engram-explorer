package ui

import (
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/agentsessions"
)

// The Claude Code marker set is agent-specific: Claude sessions strip their
// IDE/harness wrappers, other agents (Codex) keep the same text verbatim.
const markerPrompt = "<system-reminder>be brief</system-reminder> real prompt"

func TestAsStripMarkers_PerAgent(t *testing.T) {
	if clean, had := asStripMarkers("claude-code", markerPrompt); clean != "real prompt" || !had {
		t.Errorf("claude-code: got %q, %v; want %q, true", clean, had, "real prompt")
	}
	if clean, had := asStripMarkers("codex", markerPrompt); clean != markerPrompt || had {
		t.Errorf("codex: got %q, %v; want the prompt untouched, false", clean, had)
	}
}

func TestAsSplitMarkers_PerAgent(t *testing.T) {
	if segs := asSplitMarkers("claude-code", markerPrompt); len(segs) != 2 || !segs[0].IsMarker || segs[0].Tag != "system-reminder" {
		t.Errorf("claude-code: want [marker system-reminder, text], got %+v", segs)
	}
	if segs := asSplitMarkers("codex", markerPrompt); len(segs) != 1 || segs[0].IsMarker || segs[0].Text != markerPrompt {
		t.Errorf("codex: want one plain segment, got %+v", segs)
	}
}

func TestAsTurnPreviewFor_PerAgent(t *testing.T) {
	turn := agentsessions.Turn{Role: "user", Blocks: []agentsessions.Block{{Kind: agentsessions.BlockText, Text: markerPrompt}}}
	if got := asTurnPreviewFor("claude-code", turn); got != "real prompt" {
		t.Errorf("claude-code preview = %q, want %q", got, "real prompt")
	}
	if got := asTurnPreviewFor("codex", turn); got != markerPrompt {
		t.Errorf("codex preview = %q, want the raw text", got)
	}
	if got := asFirstUserPrompt("codex", []agentsessions.Turn{turn}); got != markerPrompt {
		t.Errorf("codex title = %q, want the raw text", got)
	}
	if got := asFirstUserPrompt("claude-code", []agentsessions.Turn{turn}); got != "real prompt" {
		t.Errorf("claude-code title = %q, want %q", got, "real prompt")
	}
}

// An unstamped session (empty agent) keeps the historical Claude Code set.
func TestAsStripMarkers_EmptyAgentKeepsClaudeSet(t *testing.T) {
	if clean, had := asStripMarkers("", markerPrompt); clean != "real prompt" || !had {
		t.Errorf("empty agent: got %q, %v; want %q, true", clean, had, "real prompt")
	}
}
