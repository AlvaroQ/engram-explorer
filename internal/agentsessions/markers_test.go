package agentsessions

import (
	"slices"
	"testing"
)

// TestMarkers_PerAgent: a marker set registered for one agent strips its
// wrapper tags (attributes and multi-line bodies included) and never applies
// to another agent, whose set is empty unless registered.
func TestMarkers_PerAgent(t *testing.T) {
	set := RegisterMarkers("test-agent-markers", "ctx", "note")
	if got := set.Tags(); !slices.Equal(got, []string{"ctx", "note"}) {
		t.Fatalf("Tags() = %v", got)
	}
	if got := Markers("test-agent-markers").Tags(); !slices.Equal(got, []string{"ctx", "note"}) {
		t.Fatalf("Markers(agent).Tags() = %v, want the registered tags", got)
	}
	in := "<ctx path=\"a\">line1\nline2</ctx> keep <note>n</note> me"
	if plain, had := Markers("test-agent-markers").Strip(in); plain != "keep me" || !had {
		t.Errorf("Strip = %q, %v; want %q, true", plain, had, "keep me")
	}
	if plain, had := Markers("other-agent").Strip(in); had || plain != "<ctx path=\"a\">line1 line2</ctx> keep <note>n</note> me" {
		t.Errorf("unregistered agent: Strip = %q, %v; want the text untouched (whitespace-collapsed), false", plain, had)
	}
	if n := len(Markers("other-agent").Patterns()); n != 0 {
		t.Errorf("unregistered agent has %d patterns, want 0", n)
	}
	if got, want := len(set.Patterns()), 2; got != want {
		t.Errorf("Patterns() len = %d, want %d (index-aligned with Tags)", got, want)
	}
}
