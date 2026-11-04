package httpapi

import (
	"net/http"
	"path"
	"strings"
	"time"
)

// uiWriteRateLimit is the per-minute budget of the shared limiter for unsafe
// requests outside /api/ (UI form posts and doctor actions). It matches the
// API write routes (writeRateLimit).
const uiWriteRateLimit = 120

// unmatchedRoute keys unsafe requests that match no route (404/405), so random
// paths share one bucket per client and method instead of creating new ones.
const unmatchedRoute = "(unmatched)"

// isUnsafeMethod reports whether a method can change server state.
func isUnsafeMethod(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	}
	return false
}

// isAPIPath reports whether a request path belongs to the JSON API, whose write
// routes carry their own per-route limiters (writeRoutes).
func isAPIPath(p string) bool {
	clean := path.Clean("/" + p)
	return clean == "/api" || strings.HasPrefix(clean, "/api/")
}

// withUnsafeUIRateLimit wraps router with one shared fixed-window limiter for
// unsafe requests (POST, PUT, PATCH, DELETE) whose path is outside /api/: the
// UI form posts mounted by ui.Mount and the doctor actions mounted by
// doctor.Mount. Buckets are keyed by client IP + method + the pattern the
// router would dispatch to (router.Handler), so path parameters share one
// budget. Rejected requests get 429 with Retry-After and the RATE_LIMITED
// envelope and never reach the route handler. Safe methods and /api/ paths
// pass through untouched (no double limiting of the API write routes).
func withUnsafeUIRateLimit(router *http.ServeMux, rl *rateLimiter, exposeDetails bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isUnsafeMethod(r.Method) || isAPIPath(r.URL.Path) {
			router.ServeHTTP(w, r)
			return
		}
		_, pattern := router.Handler(r)
		if pattern == "" {
			pattern = unmatchedRoute
		}
		ok, retryAfter := rl.allow(rateLimitClient(r) + " " + r.Method + " " + pattern)
		if !ok {
			writeRateLimited(w, retryAfter, exposeDetails)
			return
		}
		router.ServeHTTP(w, r)
	})
}

// newUIRateLimiter returns the limiter used by withUnsafeUIRateLimit.
func newUIRateLimiter() *rateLimiter {
	return newRateLimiter(time.Minute, uiWriteRateLimit)
}
