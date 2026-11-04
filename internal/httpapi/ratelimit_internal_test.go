package httpapi

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// fakeClock is a manually advanced clock for deterministic window tests.
type fakeClock struct{ t time.Time }

func (f *fakeClock) now() time.Time          { return f.t }
func (f *fakeClock) advance(d time.Duration) { f.t = f.t.Add(d) }

func newTestLimiter(max int) (*rateLimiter, *fakeClock) {
	clk := &fakeClock{t: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	rl := newRateLimiter(time.Minute, max)
	rl.now = clk.now
	return rl, clk
}

func limitedMux(rl *rateLimiter) *http.ServeMux {
	mux := http.NewServeMux()
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
	mux.Handle("DELETE /items/{id}", withRateLimit(rl, false, ok))
	mux.Handle("DELETE /other/{id}", withRateLimit(rl, false, ok))
	return mux
}

func hit(h http.Handler, method, path, remote string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, nil)
	req.RemoteAddr = remote
	h.ServeHTTP(rec, req)
	return rec
}

func TestRateLimiter_KeyedByClientAndPattern(t *testing.T) {
	rl, _ := newTestLimiter(2)
	mux := limitedMux(rl)

	// Two different ids on the same pattern share one budget for one client.
	if c := hit(mux, http.MethodDelete, "/items/1", "192.0.2.1:1000").Code; c != http.StatusNoContent {
		t.Fatalf("first: got %d", c)
	}
	if c := hit(mux, http.MethodDelete, "/items/2", "192.0.2.1:1001").Code; c != http.StatusNoContent {
		t.Fatalf("second: got %d", c)
	}
	rec := hit(mux, http.MethodDelete, "/items/3", "192.0.2.1:1002")
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("third: got %d, want 429", rec.Code)
	}
	if ra := rec.Header().Get("Retry-After"); ra != "60" {
		t.Errorf("Retry-After: got %q, want 60", ra)
	}

	// Another client has an independent budget.
	if c := hit(mux, http.MethodDelete, "/items/3", "192.0.2.2:1000").Code; c != http.StatusNoContent {
		t.Errorf("other client: got %d, want 204", c)
	}
	// Another pattern has an independent budget for the same client.
	if c := hit(mux, http.MethodDelete, "/other/1", "192.0.2.1:1000").Code; c != http.StatusNoContent {
		t.Errorf("other pattern: got %d, want 204", c)
	}
}

func TestRateLimiter_WindowResets(t *testing.T) {
	rl, clk := newTestLimiter(1)
	mux := limitedMux(rl)
	hit(mux, http.MethodDelete, "/items/1", "192.0.2.1:1")
	rec := hit(mux, http.MethodDelete, "/items/1", "192.0.2.1:1")
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("got %d, want 429", rec.Code)
	}
	clk.advance(20 * time.Second)
	rec = hit(mux, http.MethodDelete, "/items/1", "192.0.2.1:1")
	if ra := rec.Header().Get("Retry-After"); ra != "40" {
		t.Errorf("Retry-After: got %q, want 40", ra)
	}
	clk.advance(41 * time.Second)
	if c := hit(mux, http.MethodDelete, "/items/1", "192.0.2.1:1").Code; c != http.StatusNoContent {
		t.Errorf("after window: got %d, want 204", c)
	}
}

func TestRateLimitKey(t *testing.T) {
	cases := []struct {
		name, remote, pattern, path, want string
	}{
		{"pattern used", "192.0.2.1:1234", "DELETE /items/{id}", "/items/7", "192.0.2.1 DELETE DELETE /items/{id}"},
		{"path fallback", "192.0.2.1:1234", "", "/items/7", "192.0.2.1 DELETE /items/7"},
		{"ipv6 host", "[::1]:8080", "DELETE /items/{id}", "/items/7", "::1 DELETE DELETE /items/{id}"},
		{"raw remote fallback", "not-a-hostport", "", "/x", "not-a-hostport DELETE /x"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodDelete, tc.path, nil)
			req.RemoteAddr = tc.remote
			req.Pattern = tc.pattern
			req.Header.Set("X-Forwarded-For", "203.0.113.9")
			if got := rateLimitKey(req); got != tc.want {
				t.Errorf("rateLimitKey: got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestRateLimiter_PrunesExpiredBuckets(t *testing.T) {
	rl, clk := newTestLimiter(5)
	rl.pruneThreshold = 4

	for i := 0; i < 4; i++ {
		rl.allow(fmt.Sprintf("old-%d", i))
	}
	if n := len(rl.buckets); n != 4 {
		t.Fatalf("buckets: got %d, want 4", n)
	}

	// Expire the first batch, keep one fresh bucket alive.
	clk.advance(61 * time.Second)
	rl.allow("old-0") // reused, window restarted: still live

	// Inserting a new key past the threshold sweeps expired buckets.
	rl.allow("new")
	if _, ok := rl.buckets["old-1"]; ok {
		t.Error("expired bucket old-1 was not pruned")
	}
	if _, ok := rl.buckets["old-0"]; !ok {
		t.Error("live bucket old-0 was pruned")
	}
	if _, ok := rl.buckets["new"]; !ok {
		t.Error("new bucket missing")
	}
	if n := len(rl.buckets); n != 2 {
		t.Errorf("buckets after prune: got %d, want 2", n)
	}
}
