package doctor_test

import (
	"regexp"
	"strings"
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/doctor"
)

// TestSyncPolls_OptOutOfErrorToasts: the sync widgets poll every 15s and show
// their own loading/error state, so a failing poll must not raise a toast
// each time (toasts.js skips [data-no-error-toast]).
func TestSyncPolls_OptOutOfErrorToasts(t *testing.T) {
	var b strings.Builder
	if err := doctor.SyncShell().Render(t.Context(), &b); err != nil {
		t.Fatal(err)
	}
	polls := regexp.MustCompile(`<div[^>]*hx-trigger="load, every 15s"[^>]*>`).FindAllString(b.String(), -1)
	if len(polls) != 2 {
		t.Fatalf("polled sync widgets = %d, want 2", len(polls))
	}
	for _, p := range polls {
		if !strings.Contains(p, "data-no-error-toast") {
			t.Errorf("polled widget %s lacks data-no-error-toast", p)
		}
	}
}
