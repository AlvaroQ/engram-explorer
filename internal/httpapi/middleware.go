package httpapi

import (
	"net/http"
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
	"http://localhost:5173":  true,
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
			h.Set("Vary", "Origin")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// fixedWindowBucket holds the state for one rate-limit key.
type fixedWindowBucket struct {
	mu      sync.Mutex
	count   int
	resetAt time.Time
}

// rateLimiter holds the per-path buckets for a single rate-limit rule.
type rateLimiter struct {
	windowMs time.Duration
	max      int
	mu       sync.Mutex
	buckets  map[string]*fixedWindowBucket
}

// newRateLimiter creates a rateLimiter matching the Node fixed-window implementation.
func newRateLimiter(windowMs int, max int) *rateLimiter {
	return &rateLimiter{
		windowMs: time.Duration(windowMs) * time.Millisecond,
		max:      max,
		buckets:  make(map[string]*fixedWindowBucket),
	}
}

func (rl *rateLimiter) bucket(key string) *fixedWindowBucket {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	b, ok := rl.buckets[key]
	if !ok {
		b = &fixedWindowBucket{}
		rl.buckets[key] = b
	}
	return b
}

// allow returns true if the request is within limits and increments the counter.
// Returns the retry-after seconds if rate-limited.
func (rl *rateLimiter) allow(key string) (ok bool, retryAfterSec int) {
	b := rl.bucket(key)
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()
	if b.resetAt.IsZero() || now.After(b.resetAt) {
		b.count = 0
		b.resetAt = now.Add(rl.windowMs)
	}
	b.count++
	if b.count > rl.max {
		remaining := b.resetAt.Sub(now)
		if remaining < 0 {
			remaining = 0
		}
		secs := int(remaining.Seconds())
		if remaining > 0 && secs == 0 {
			secs = 1 // round up
		}
		return false, secs
	}
	return true, 0
}

// withRateLimit wraps a handler with a fixed-window rate limiter keyed by path.
func withRateLimit(rl *rateLimiter, exposeDetails bool, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ok, retryAfter := rl.allow(r.URL.Path)
		if !ok {
			w.Header().Set("Retry-After", http.StatusText(retryAfter))
			writeError(w, http.StatusTooManyRequests, "BAD_INPUT",
				"Rate limit exceeded", nil, exposeDetails)
			return
		}
		next.ServeHTTP(w, r)
	})
}
