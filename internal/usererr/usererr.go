// Package usererr marks errors whose message is safe to show to the user.
//
// HTML handlers must never render arbitrary err.Error() text: driver, SQLite
// and OS errors can disclose file paths and internals. An error built with New
// or Wrap carries a message written for the user; handlers show that message
// verbatim and replace every other error with a generic one (logging the
// detail). The package is a leaf: it imports nothing from internal/*, so any
// layer (config, providers, httpapi) can use it without import cycles.
package usererr

import "errors"

// Error is an error with a user-facing message and an optional internal cause.
type Error struct {
	msg   string
	cause error
}

// Error returns the user message, followed by the cause when present so logs
// keep the full detail.
func (e *Error) Error() string {
	if e.cause != nil {
		return e.msg + ": " + e.cause.Error()
	}
	return e.msg
}

// Unwrap exposes the cause to errors.Is / errors.As.
func (e *Error) Unwrap() error { return e.cause }

// New returns an error whose whole text is safe to show to the user. Each call
// returns a distinct value, so it can back a sentinel compared by identity.
func New(msg string) error { return &Error{msg: msg} }

// Wrap returns an error showing msg to the user while keeping cause for logs
// and for errors.Is / errors.As.
func Wrap(msg string, cause error) error { return &Error{msg: msg, cause: cause} }

// Message returns the user-facing message of the first usererr in err's chain.
// ok is false when err carries no user-safe message.
func Message(err error) (msg string, ok bool) {
	var e *Error
	if errors.As(err, &e) {
		return e.msg, true
	}
	return "", false
}
