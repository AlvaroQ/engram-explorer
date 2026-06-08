package httpapi

import (
	"net/url"
	"strconv"
)

// parseLimit reads q["limit"], clamps it to [min,max], and returns def when the
// value is missing or not a valid integer.
func parseLimit(q url.Values, def, min, max int) int {
	l := q.Get("limit")
	if l == "" {
		return def
	}
	n, err := strconv.Atoi(l)
	if err != nil {
		return def
	}
	if n < min {
		return min
	}
	if n > max {
		return max
	}
	return n
}

// parseBool recognizes "true" or "1" as true (the backend's current semantics).
func parseBool(v string) bool { return v == "true" || v == "1" }
