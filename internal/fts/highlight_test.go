package fts

import (
	"reflect"
	"testing"
)

func TestSplitHighlight(t *testing.T) {
	in := "<b>x</b> " + HighlightStart + "needle" + HighlightEnd + " <mark>tail"
	got := SplitHighlight(in)
	want := []Segment{
		{Text: "<b>x</b> "},
		{Text: "needle", Marked: true},
		{Text: " <mark>tail"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("SplitHighlight = %#v, want %#v", got, want)
	}
	if got := SplitHighlight(""); got != nil {
		t.Fatalf("SplitHighlight(\"\") = %#v, want nil", got)
	}
}

func TestMarkupHighlight(t *testing.T) {
	in := "a " + HighlightStart + "b" + HighlightEnd + " c"
	if got, want := MarkupHighlight(in), "a <mark>b</mark> c"; got != want {
		t.Fatalf("MarkupHighlight = %q, want %q", got, want)
	}
}
