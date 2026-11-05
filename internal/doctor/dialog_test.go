package doctor_test

import (
	"regexp"
	"strings"
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/doctor"
	"github.com/AlvaroQ/engram-explorer/internal/services"
)

// TestOrphanDialog_OpensModally (S6 focus trap): the detail dialog is not
// rendered open; ui-actions.js opens it with showModal() (focus moves inside,
// Esc closes natively) and its close controls call close(), which clears the
// host and returns focus to the opener. Clearing the host directly would skip
// that focus return, so no control does it.
func TestOrphanDialog_OpensModally(t *testing.T) {
	var b strings.Builder
	d := &services.OrphanObservationDetail{ID: 7, Type: "decision"}
	if err := doctor.OrphanObservationDetailDialog(d, nil).Render(t.Context(), &b); err != nil {
		t.Fatal(err)
	}
	html := regexp.MustCompile(`(?s)<!--.*?-->`).ReplaceAllString(b.String(), "")
	dialog := regexp.MustCompile(`<dialog[^>]*>`).FindString(html)
	if regexp.MustCompile(`\sopen[\s>=]`).MatchString(dialog) {
		t.Errorf("dialog %s must not carry the open attribute (opened with showModal)", dialog)
	}
	for _, want := range []string{`data-modal-dialog="orphan-dialog-host"`, `aria-labelledby="orphan-dialog-title"`} {
		if !strings.Contains(dialog, want) {
			t.Errorf("dialog %s lacks %s", dialog, want)
		}
	}
	if n := strings.Count(html, "data-dialog-close"); n != 2 {
		t.Errorf("data-dialog-close controls = %d, want 2 (backdrop + close button)", n)
	}
	for _, gone := range []string{"data-clear-target", "data-escape-clear-target", `aria-modal="true"`} {
		if strings.Contains(html, gone) {
			t.Errorf("dialog still uses %s", gone)
		}
	}
}
