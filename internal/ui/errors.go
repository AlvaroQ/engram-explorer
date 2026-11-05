package ui

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/AlvaroQ/engram-explorer/internal/services"
	"github.com/AlvaroQ/engram-explorer/internal/usererr"
)

// ErrorMessage returns the text an HTML response may show for err.
//
// prefix is an i18n key (or ""): its localized text precedes a user-safe
// message, and the key itself names the operation in the log.
//
// User-safe errors keep their message, prefixed with prefix: usererr values
// (validation messages written for the user) and services.WriteError (domain
// codes such as NOT_FOUND). Any other error (database, driver, OS, I/O) may
// disclose paths or internals, so its detail goes to the log with the request
// method and path, and the user sees the localized generic message instead.
// Shared by the doctor package, which renders the same partials.
func ErrorMessage(r *http.Request, prefix string, err error) string {
	lang := langForRequest(r)
	label := ""
	if prefix != "" {
		label = T(lang, prefix)
	}
	if msg, ok := usererr.Message(err); ok {
		return label + msg
	}
	var we *services.WriteError
	if errors.As(err, &we) {
		return label + we.Error()
	}
	slog.Default().ErrorContext(r.Context(), "request failed",
		"method", r.Method,
		"path", r.URL.Path,
		"op", prefix,
		"err", err,
	)
	return T(lang, "common.errorGeneric")
}

// RenderError renders an ErrorPartial with the message chosen by ErrorMessage.
// Shared by the doctor package.
func RenderError(w http.ResponseWriter, r *http.Request, prefix string, err error) {
	Render(w, r, ErrorPartial(ErrorMessage(r, prefix, err)))
}
