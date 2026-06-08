package httpapi

import (
	"encoding/json"
	"net/http"
)

// writeJSON serializes v to JSON and writes it to w with Content-Type application/json.
// HTML escaping is disabled so FTS snippet markup (<mark>…</mark>) is not mangled.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

// writeRawJSON writes pre-encoded JSON bytes directly to the response.
// Used by the daemon proxy to pass through the daemon's response body without
// re-encoding (preserving key order).
func writeRawJSON(w http.ResponseWriter, status int, data []byte) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(data)
	_, _ = w.Write([]byte("\n"))
}
