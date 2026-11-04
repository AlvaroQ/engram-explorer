package httpapi

import (
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// secureHeaders adds HTTP security headers matching the original Hono
// secureHeaders middleware configuration carried over during migration.
func secureHeaders(next http.Handler) http.Handler {
	// Content-Security-Policy string mirrors the Hono secureHeaders config exactly.
	const csp = "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'"

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", csp)
		h.Set("X-Frame-Options", "DENY")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Cross-Origin-Opener-Policy", "same-origin")
		h.Set("Cross-Origin-Resource-Policy", "same-origin")
		next.ServeHTTP(w, r)
	})
}

// corsAllowedOrigins is the dev-only allow-list, matching the Node CORS config.
// In production the frontend is served from the same origin (Go single binary),
// so CORS headers are only emitted for the Vite dev server origin.
var corsAllowedOrigins = map[string]bool{
	"http://localhost:5173": true,
	"http://127.0.0.1:5173": true,
}

// corsMiddleware applies CORS headers for the allow-listed origins.
// In production the frontend is served from the same origin, so CORS headers
// are only emitted when the request Origin is in the allow-list.
func corsMiddleware(next http.Handler) http.Handler {
	const methods = "GET, POST, PATCH, DELETE, OPTIONS"
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" && corsAllowedOrigins[origin] {
			h := w.Header()
			h.Set("Access-Control-Allow-Origin", origin)
			h.Set("Access-Control-Allow-Methods", methods)
			h.Set("Access-Control-Allow-Headers", "Content-Type, Accept")
			h.Set("Vary", "Origin")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// defaultRateLimitPruneThreshold is the bucket count above which inserting a
// new key first sweeps expired buckets, bounding the limiter's memory.
const defaultRateLimitPruneThreshold = 1024

// fixedWindowBucket holds the state for one rate-limit key. It is guarded by
// the owning rateLimiter's mutex.
type fixedWindowBucket struct {
	count   int
	resetAt time.Time
}

// rateLimiter is a fixed-window limiter for a single rule (window + max).
// Buckets are keyed by rateLimitKey (client IP + method + route pattern), so
// every client gets its own budget per route and requests to different ids of
// the same pattern share it. Expired buckets are pruned when a new key is
// inserted and the map has grown past pruneThreshold.
type rateLimiter struct {
	window         time.Duration
	max            int
	now            func() time.Time
	pruneThreshold int
	mu             sync.Mutex
	buckets        map[string]*fixedWindowBucket
}

// newRateLimiter creates a fixed-window rateLimiter allowing max requests per
// window for each key.
func newRateLimiter(window time.Duration, max int) *rateLimiter {
	return &rateLimiter{
		window:         window,
		max:            max,
		now:            time.Now,
		pruneThreshold: defaultRateLimitPruneThreshold,
		buckets:        make(map[string]*fixedWindowBucket),
	}
}

// pruneLocked removes buckets whose window has ended. Caller holds rl.mu.
// If live buckets alone exceed the threshold, the threshold doubles so the
// sweep does not run on every insert.
func (rl *rateLimiter) pruneLocked(now time.Time) {
	for k, b := range rl.buckets {
		if now.After(b.resetAt) {
			delete(rl.buckets, k)
		}
	}
	if len(rl.buckets) >= rl.pruneThreshold {
		rl.pruneThreshold = 2 * len(rl.buckets)
	}
}

// allow returns true if the request is within limits and increments the counter.
// Returns the retry-after seconds if rate-limited.
func (rl *rateLimiter) allow(key string) (ok bool, retryAfterSec int) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := rl.now()
	b, found := rl.buckets[key]
	if !found {
		if len(rl.buckets) >= rl.pruneThreshold {
			rl.pruneLocked(now)
		}
		b = &fixedWindowBucket{}
		rl.buckets[key] = b
	}

	if b.resetAt.IsZero() || now.After(b.resetAt) {
		b.count = 0
		b.resetAt = now.Add(rl.window)
	}
	b.count++
	if b.count > rl.max {
		remaining := b.resetAt.Sub(now)
		if remaining < 0 {
			remaining = 0
		}
		secs := int(remaining.Seconds())
		if remaining > time.Duration(secs)*time.Second {
			secs++ // round up partial seconds
		}
		return false, secs
	}
	return true, 0
}

// rateLimitKey identifies the client and route for a request: the host part of
// r.RemoteAddr (falling back to the raw value), the method, and the matched
// mux pattern (falling back to r.URL.Path when the request was not routed by a
// ServeMux). Proxy headers such as X-Forwarded-For are deliberately ignored:
// the dashboard binds to loopback and those headers are client-controlled.
func rateLimitKey(r *http.Request) string {
	client := r.RemoteAddr
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		client = host
	}
	route := r.Pattern
	if route == "" {
		route = r.URL.Path
	}
	return client + " " + r.Method + " " + route
}

// withRateLimit wraps a handler with a fixed-window rate limiter keyed by
// rateLimitKey. Wrap the per-route handler at mux registration (not the mux
// itself) so r.Pattern is populated when the key is computed.
func withRateLimit(rl *rateLimiter, exposeDetails bool, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ok, retryAfter := rl.allow(rateLimitKey(r))
		if !ok {
			// Retry-After must be delay-seconds (RFC 7231 §7.1.3), not status text.
			w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
			writeError(w, http.StatusTooManyRequests, "BAD_INPUT",
				"Rate limit exceeded", nil, exposeDetails)
			return
		}
		next.ServeHTTP(w, r)
	})
}
