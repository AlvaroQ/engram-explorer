package httpapi

import (
	"fmt"
	"io"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/config"
	"github.com/AlvaroQ/engram-explorer/internal/providers"
)

// TestUIRateLimit_BucketsBoundedUnderRandomPaths (S12): the shared UI limiter
// keys on the routed pattern, not the raw path, so 20k unsafe requests from
// one client with random paths, ids and methods create only a handful of
// buckets (one per method and pattern, random paths share "(unmatched)").
func TestUIRateLimit_BucketsBoundedUnderRandomPaths(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	c := NewContainerWithRegistry(providers.NewRegistry(logger), config.Config{DaemonTimeoutMs: 1}, logger)
	rl := newUIRateLimiter()
	h := withUnsafeUIRateLimit(newRouter(c), rl, false)

	rng := rand.New(rand.NewPCG(1, 2))
	methods := []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete}
	shapes := []string{
		"/%d",
		"/x%d/y/z",
		"/doctor/sync/p%d/delete",
		"/doctor/orphans/observations/%d",
		"/doctor/orphans/sessions/%d/project",
		"/settings/modules/m%d/toggle",
		"/settings/cc-accounts/a%d/remove",
		"/projects/p%d/merge",
	}
	for i := 0; i < 20000; i++ {
		target := fmt.Sprintf(shapes[rng.IntN(len(shapes))], rng.Uint64())
		req := httptest.NewRequest(methods[rng.IntN(len(methods))], target, nil)
		req.RemoteAddr = fmt.Sprintf("198.51.100.7:%d", 1024+rng.IntN(60000))
		h.ServeHTTP(httptest.NewRecorder(), req)
	}

	rl.mu.Lock()
	n := len(rl.buckets)
	rl.mu.Unlock()
	t.Logf("buckets after 20k requests: %d", n)
	if n == 0 || n > 16 {
		t.Errorf("limiter holds %d buckets after 20k random unsafe requests from one client, want 1..16", n)
	}
}
