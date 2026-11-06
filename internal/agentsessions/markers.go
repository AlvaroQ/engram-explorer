package agentsessions

import (
	"regexp"
	"strings"
	"sync"
)

// MarkerSet is the set of harness wrapper tags an agent injects into its
// transcripts (e.g. Claude Code's <ide_opened_file>…</ide_opened_file>).
// Hosts render them as attachment chips and drop them from one-line previews.
// The zero value is an empty set: nothing is a marker.
type MarkerSet struct {
	tags     []string
	patterns []*regexp.Regexp
}

// Tags returns the tag names; callers must treat the slice as read-only.
func (m MarkerSet) Tags() []string { return m.tags }

// Patterns returns one compiled regexp per tag, index-aligned with Tags. Each
// matches a whole <tag ...>…</tag> span (multi-line). Read-only.
func (m MarkerSet) Patterns() []*regexp.Regexp { return m.patterns }

// Strip removes every marker span from s and returns the remaining text with
// whitespace collapsed to one line, plus whether any marker was present.
func (m MarkerSet) Strip(s string) (plain string, hadMarker bool) {
	for _, re := range m.patterns {
		if re.MatchString(s) {
			hadMarker = true
			s = re.ReplaceAllString(s, " ")
		}
	}
	return strings.Join(strings.Fields(s), " "), hadMarker
}

var (
	markersMu sync.RWMutex
	markers   = map[string]MarkerSet{}
)

// RegisterMarkers declares the marker tags of an agent kind (replacing any
// earlier set) and returns the compiled set. Agent readers call it at package
// initialization so the host never needs agent-specific tag lists.
func RegisterMarkers(agent string, tags ...string) MarkerSet {
	set := MarkerSet{tags: append([]string(nil), tags...), patterns: make([]*regexp.Regexp, 0, len(tags))}
	for _, tag := range tags {
		// RE2 has no backreferences: one pattern per tag.
		q := regexp.QuoteMeta(tag)
		set.patterns = append(set.patterns, regexp.MustCompile(`(?s)<`+q+`\b[^>]*>.*?</`+q+`>`))
	}
	markersMu.Lock()
	markers[agent] = set
	markersMu.Unlock()
	return set
}

// Markers returns the marker set registered for an agent kind, or the empty
// set when the agent declared none.
func Markers(agent string) MarkerSet {
	markersMu.RLock()
	defer markersMu.RUnlock()
	return markers[agent]
}
