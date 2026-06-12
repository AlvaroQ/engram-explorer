package services

import (
	"regexp"
	"strings"
)

// CCMarkerTags is the canonical list of IDE/harness wrapper tags injected into
// Claude Code session transcripts (e.g. <ide_opened_file>…</ide_opened_file>).
// It is the single source of truth shared by the services layer (stripping
// markers out of one-line prompt previews) and the ui layer (rendering markers
// as paperclip chips in the transcript detail view).
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

// ccMarkerReList holds one compiled regexp per marker tag. RE2 has no
// backreferences, so we compile one (?s)<tag\b[^>]*>.*?</tag> per tag.
// The slice is index-aligned with CCMarkerTags.
var ccMarkerReList = buildCCMarkerReList()

func buildCCMarkerReList() []*regexp.Regexp {
	res := make([]*regexp.Regexp, 0, len(CCMarkerTags))
	for _, tag := range CCMarkerTags {
		res = append(res, regexp.MustCompile(`(?s)<`+tag+`\b[^>]*>.*?</`+tag+`>`))
	}
	return res
}

// CCMarkerReList returns the compiled marker regexps, one per CCMarkerTags
// entry and index-aligned with it. The ui layer reuses these for positional
// marker splitting instead of recompiling the identical patterns; callers must
// treat the returned slice as read-only.
func CCMarkerReList() []*regexp.Regexp {
	return ccMarkerReList
}

// StripCCMarkers removes every marker span from s and returns the remaining
// plain text (whitespace-normalised) plus whether any marker was present.
func StripCCMarkers(s string) (plain string, hadMarker bool) {
	out := s
	for _, re := range ccMarkerReList {
		if re.MatchString(out) {
			hadMarker = true
			out = re.ReplaceAllString(out, " ")
		}
	}
	return strings.Join(strings.Fields(out), " "), hadMarker
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
