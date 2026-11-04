package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
)

// maxJSONBodyBytes caps JSON request bodies accepted by the write handlers.
const maxJSONBodyBytes = 1 << 20 // 1 MiB

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

// decodeJSONBody decodes the request body into dst, reading at most
// maxJSONBodyBytes. It writes the error response and returns false when the
// body is too large (413 PAYLOAD_TOO_LARGE) or not valid JSON (400 BAD_INPUT).
func decodeJSONBody(w http.ResponseWriter, r *http.Request, dst any, exposeDetails bool) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxJSONBodyBytes)
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeError(w, http.StatusRequestEntityTooLarge, "PAYLOAD_TOO_LARGE",
				"request body exceeds 1 MiB", nil, exposeDetails)
			return false
		}
		writeError(w, http.StatusBadRequest, "BAD_INPUT", "invalid JSON body", nil, exposeDetails)
		return false
	}
	return true
}
