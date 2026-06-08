// Package httpapi provides the HTTP server, middleware, and error helpers.
package httpapi

import (
	"encoding/json"
	"net/http"
)

// errorEnvelope matches the Node error envelope shape:
//
//	{"error":{"code":"...","message":"...","details":...}}
type errorEnvelope struct {
	Error errorBody `json:"error"`
}

type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}

// writeError writes a JSON error envelope and sets the status code.
// When exposeDetails is false, details is suppressed and message replaces any
// internal details — prevents leaking SQLite paths or internal stack traces to
// clients in production.
func writeError(w http.ResponseWriter, status int, code, message string, details any, exposeDetails bool) {
	body := errorBody{Code: code, Message: message}
	if exposeDetails {
		body.Details = details
	}
	env := errorEnvelope{Error: body}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(env)
}
