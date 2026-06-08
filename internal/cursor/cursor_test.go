package cursor_test

import (
	"encoding/base64"
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/cursor"
)

func TestRoundTripSimple(t *testing.T) {
	c := cursor.Encode("2026-04-22T18:00:00Z", 42)
	got := cursor.Decode(c)
	if got == nil {
		t.Fatal("expected non-nil decoded cursor")
	}
	if got.OrderKey != "2026-04-22T18:00:00Z" {
		t.Errorf("OrderKey: got %q, want %q", got.OrderKey, "2026-04-22T18:00:00Z")
	}
	if got.ID != 42 {
		t.Errorf("ID: got %d, want 42", got.ID)
	}
}

func TestRoundTripOrderKeyWithPipes(t *testing.T) {
	c := cursor.Encode("2026|04|22", 7)
	got := cursor.Decode(c)
	if got == nil {
		t.Fatal("expected non-nil decoded cursor")
	}
	if got.OrderKey != "2026|04|22" {
		t.Errorf("OrderKey: got %q, want %q", got.OrderKey, "2026|04|22")
	}
	if got.ID != 7 {
		t.Errorf("ID: got %d, want 7", got.ID)
	}
}

func TestDecodeInvalidCursors(t *testing.T) {
	// Not valid base64url
	if cursor.Decode("!!!not-base64!!!") != nil {
		t.Error("expected nil for invalid base64")
	}
	// Valid base64 but no pipe
	noPipe := base64.RawURLEncoding.EncodeToString([]byte("no-pipe"))
	if cursor.Decode(noPipe) != nil {
		t.Error("expected nil for missing pipe")
	}
	// Valid base64, has pipe, but id is not a number
	notANumber := base64.RawURLEncoding.EncodeToString([]byte("orderkey|notanumber"))
	if cursor.Decode(notANumber) != nil {
		t.Error("expected nil for non-integer id")
	}
}

func TestRoundTripIDZero(t *testing.T) {
	c := cursor.Encode("key", 0)
	got := cursor.Decode(c)
	if got == nil {
		t.Fatal("expected non-nil decoded cursor")
	}
	if got.OrderKey != "key" {
		t.Errorf("OrderKey: got %q, want %q", got.OrderKey, "key")
	}
	if got.ID != 0 {
		t.Errorf("ID: got %d, want 0", got.ID)
	}
}
