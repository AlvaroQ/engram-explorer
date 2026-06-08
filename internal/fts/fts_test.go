package fts_test

import (
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
