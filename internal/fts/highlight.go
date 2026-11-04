package fts

import "strings"

// Highlight sentinels passed to FTS5 snippet() instead of HTML tags. They are
// Unicode private-use code points, so stored content cannot be confused with
// markup and the snippet text never has to be trusted as HTML.
const (
	HighlightStart = "\uE000"
	HighlightEnd   = "\uE001"
)

// Segment is one run of snippet text; Marked runs are search matches.
type Segment struct {
	Text   string
	Marked bool
}

// SplitHighlight splits a snippet produced with HighlightStart/HighlightEnd
// delimiters into plain and marked segments. Callers render each segment as
// escaped text; only the segment boundaries become highlight elements.
func SplitHighlight(snippet string) []Segment {
	var segs []Segment
	marked := false
	var b strings.Builder
	flush := func() {
		if b.Len() > 0 {
			segs = append(segs, Segment{Text: b.String(), Marked: marked})
			b.Reset()
		}
	}
	for _, r := range snippet {
		switch string(r) {
		case HighlightStart:
			flush()
			marked = true
		case HighlightEnd:
			flush()
			marked = false
		default:
			b.WriteRune(r)
		}
	}
	flush()
	return segs
}

// MarkupHighlight renders a sentinel-delimited snippet with literal <mark> and
// </mark> delimiters, preserving the JSON API contract. The text between the
// delimiters is NOT escaped: API consumers must render it as text, never as HTML.
func MarkupHighlight(snippet string) string {
	r := strings.NewReplacer(HighlightStart, "<mark>", HighlightEnd, "</mark>")
	return r.Replace(snippet)
}
