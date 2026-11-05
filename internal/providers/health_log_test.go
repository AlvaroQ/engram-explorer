package providers_test

import (
	"bytes"
	"log/slog"
	"strings"
	"sync"
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/providers"
)

// lockedBuffer is an io.Writer safe for concurrent slog handlers.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// hammer runs fn from n goroutines released together and waits for all.
func hammer(n int, fn func()) {
	var start, done sync.WaitGroup
	start.Add(1)
	done.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer done.Done()
			start.Wait()
			fn()
		}()
	}
	start.Done()
	done.Wait()
}

// TestHealthLog_ConcurrentFailingLogsOnce: concurrent health polls on one
// instance log the healthy→failing transition exactly once as a warning and
// the failing→healthy recovery exactly once as info (S13).
func TestHealthLog_ConcurrentFailingLogsOnce(t *testing.T) {
	out := &lockedBuffer{}
	h := providers.NewHealthLog(slog.New(slog.NewTextHandler(out, nil)))

	hammer(50, func() { h.Failing("x") })
	if got := strings.Count(out.String(), "level=WARN"); got != 1 {
		t.Fatalf("50 concurrent Failing calls logged %d warnings, want 1:\n%s", got, out.String())
	}

	hammer(50, func() { h.Healthy("y") })
	if got := strings.Count(out.String(), "level=INFO"); got != 1 {
		t.Fatalf("50 concurrent Healthy calls logged %d info lines, want 1:\n%s", got, out.String())
	}
	if got := strings.Count(out.String(), "level=WARN"); got != 1 {
		t.Errorf("warnings after recovery = %d, want still 1", got)
	}
}
