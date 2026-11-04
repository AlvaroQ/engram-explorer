package services

import "testing"

func TestQuoteIdentList(t *testing.T) {
	got := quoteIdentList([]string{"id", `we"ird`, "content) VALUES (1); --"})
	want := `"id", "we""ird", "content) VALUES (1); --"`
	if got != want {
		t.Fatalf("quoteIdentList = %s, want %s", got, want)
	}
}
