package services

import "github.com/AlvaroQ/engram-explorer/internal/agentsessions"

// CCMarkerTags is the canonical list of IDE/harness wrapper tags injected into
// Claude Code session transcripts (e.g. <ide_opened_file>…</ide_opened_file>).
// It is the single source of truth for the services layer (stripping markers
// out of one-line prompt previews) and, through the agentsessions marker
// registry, for the ui layer (paperclip chips in the transcript detail view).
var CCMarkerTags = []string{
	"ide_opened_file",
	"ide_selection",
	"ide_diagnostics",
	"system-reminder",
	"local-command-caveat",
	"command-name",
	"command-message",
	"command-args",
	"local-command-stdout",
}

// CCAgent is the agent kind of Claude Code sessions in the neutral
// agentsessions model.
const CCAgent = "claude-code"

// ccMarkers registers CCMarkerTags as the Claude Code marker set of the
// neutral model, so the host UI renders and strips them per agent without
// importing this list; the compiled patterns are shared with StripCCMarkers.
var ccMarkers = agentsessions.RegisterMarkers(CCAgent, CCMarkerTags...)

// StripCCMarkers removes every marker span from s and returns the remaining
// plain text (whitespace-normalised) plus whether any marker was present.
func StripCCMarkers(s string) (plain string, hadMarker bool) {
	return ccMarkers.Strip(s)
}

// IsMarkerOnly reports whether s collapses to nothing once IDE/harness markers
// are stripped — i.e. it carries no human prose. Such turns are injected by the
// harness (e.g. a lone <local-command-caveat> or <ide_opened_file>) and must be
// skipped when picking a session's first *human* prompt.
func IsMarkerOnly(s string) bool {
	plain, had := StripCCMarkers(s)
	return plain == "" && had
}

// CleanPromptPreview produces a one-line prompt preview with IDE/harness marker
// context removed. When the prompt was nothing but markers it collapses to a
// paperclip so list rows show a compact attachment indicator instead of raw
// <ide_opened_file> noise.
func CleanPromptPreview(s string) string {
	plain, had := StripCCMarkers(s)
	if plain == "" && had {
		return "📎"
	}
	return plain
}
