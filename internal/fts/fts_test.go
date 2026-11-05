package fts_test

import (
	"strings"
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/fts"
)

func TestSanitizeQuery_EmptyInput(t *testing.T) {
	if got := fts.SanitizeQuery(""); got != "" {
		t.Errorf("got %q, want %q", got, "")
	}
	if got := fts.SanitizeQuery("   "); got != "" {
		t.Errorf("got %q, want %q", got, "")
	}
}

func TestSanitizeQuery_SingleToken(t *testing.T) {
	if got := fts.SanitizeQuery("hello"); got != `"hello"` {
		t.Errorf("got %q, want %q", got, `"hello"`)
	}
}

func TestSanitizeQuery_MultipleTokens(t *testing.T) {
	if got := fts.SanitizeQuery("auth bug"); got != `"auth" AND "bug"` {
		t.Errorf("got %q, want %q", got, `"auth" AND "bug"`)
	}
}

func TestSanitizeQuery_DropsFTSOperators(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"auth AND bug", `"auth" AND "bug"`},
		{"NEAR bug", `"bug"`},
		{"NOT", ``},
	}
	for _, tc := range cases {
		got := fts.SanitizeQuery(tc.input)
		if got != tc.want {
			t.Errorf("SanitizeQuery(%q): got %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestSanitizeQuery_StripsQuotesAndParens(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{`"DROP TABLE"`, `"DROP" AND "TABLE"`},
		{"(foo)'bar'", `"foo" AND "bar"`},
	}
	for _, tc := range cases {
		got := fts.SanitizeQuery(tc.input)
		if got != tc.want {
			t.Errorf("SanitizeQuery(%q): got %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestSanitizeQuery_TrimsAndCollapsesWhitespace(t *testing.T) {
	if got := fts.SanitizeQuery("   foo    bar  "); got != `"foo" AND "bar"` {
		t.Errorf("got %q, want %q", got, `"foo" AND "bar"`)
	}
}

func TestTerms_ReturnsSanitizedUnquotedTerms(t *testing.T) {
	got := fts.Terms(`  "auth" (bug) AND fix* `)
	want := []string{"auth", "bug", "fix"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("Terms: got %q, want %q", got, want)
	}
	if got := fts.Terms("AND OR"); len(got) != 0 {
		t.Errorf("Terms of operators only: got %q, want empty", got)
	}
}

func TestTerms_CapsTokenCount(t *testing.T) {
	if got := fts.Terms(strings.Repeat("a ", 40)); len(got) != 32 {
		t.Errorf("Terms cap: got %d terms, want 32", len(got))
	}
}

func TestMatchAllAndMatchAny(t *testing.T) {
	terms := []string{"auth", "bug"}
	if got := fts.MatchAll(terms); got != `"auth" AND "bug"` {
		t.Errorf("MatchAll: got %q", got)
	}
	if got := fts.MatchAny(terms); got != `"auth" OR "bug"` {
		t.Errorf("MatchAny: got %q", got)
	}
	if got := fts.MatchAll(nil); got != "" {
		t.Errorf("MatchAll(nil): got %q, want empty", got)
	}
	if got := fts.MatchAny(nil); got != "" {
		t.Errorf("MatchAny(nil): got %q, want empty", got)
	}
}

func TestSanitizeQuery_EqualsMatchAllOfTerms(t *testing.T) {
	for _, in := range []string{"", "hello", "auth bug", `a "b" (c) NOT d`, strings.Repeat("x ", 50)} {
		if got, want := fts.SanitizeQuery(in), fts.MatchAll(fts.Terms(in)); got != want {
			t.Errorf("SanitizeQuery(%q) = %q, MatchAll(Terms) = %q", in, got, want)
		}
	}
}

func TestSplitShort(t *testing.T) {
	long, short := fts.SplitShort([]string{"go", "auth", "ñu", "ñuño", "ui"}, fts.TrigramMinRunes)
	if strings.Join(long, "|") != "auth|ñuño" {
		t.Errorf("long: got %q", long)
	}
	// Runes, not bytes: "ñu" is 3 bytes but 2 runes.
	if strings.Join(short, "|") != "go|ñu|ui" {
		t.Errorf("short: got %q", short)
	}
}
