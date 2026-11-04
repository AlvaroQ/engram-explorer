package usererr_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/usererr"
)

func TestNew_MessageIsUserSafe(t *testing.T) {
	err := usererr.New("project name is required")
	if err.Error() != "project name is required" {
		t.Errorf("Error() = %q, want the message verbatim", err.Error())
	}
	msg, ok := usererr.Message(err)
	if !ok || msg != "project name is required" {
		t.Errorf("Message() = %q, %v; want message, true", msg, ok)
	}
}

func TestMessage_FoundThroughWrapping(t *testing.T) {
	err := fmt.Errorf("engram: %w", usererr.New("not an SQLite database: /x.db"))
	msg, ok := usererr.Message(err)
	if !ok || msg != "not an SQLite database: /x.db" {
		t.Errorf("Message() = %q, %v; want the inner user message", msg, ok)
	}
}

func TestMessage_PlainErrorIsNotUserSafe(t *testing.T) {
	if msg, ok := usererr.Message(errors.New("sql: database is closed")); ok || msg != "" {
		t.Errorf("Message() = %q, %v; want \"\", false", msg, ok)
	}
	if msg, ok := usererr.Message(nil); ok || msg != "" {
		t.Errorf("Message(nil) = %q, %v; want \"\", false", msg, ok)
	}
}

func TestWrap_KeepsCauseForLogsAndErrorsIs(t *testing.T) {
	cause := errors.New("unknown entity type")
	err := usererr.Wrap(`unknown entity type: "foo"`, cause)
	if !errors.Is(err, cause) {
		t.Error("errors.Is must see the wrapped cause")
	}
	msg, ok := usererr.Message(err)
	if !ok || msg != `unknown entity type: "foo"` {
		t.Errorf("Message() = %q, %v; want the user message only", msg, ok)
	}
	if err.Error() != `unknown entity type: "foo": unknown entity type` {
		t.Errorf("Error() = %q; want message plus cause for logs", err.Error())
	}
}

func TestSentinel_IdentityPreserved(t *testing.T) {
	sentinel := usererr.New("CC account not found")
	wrapped := fmt.Errorf("update: %w", sentinel)
	if !errors.Is(wrapped, sentinel) {
		t.Error("errors.Is must match a usererr sentinel by identity")
	}
	if errors.Is(wrapped, usererr.New("CC account not found")) {
		t.Error("distinct usererr values with equal text must not match")
	}
}
