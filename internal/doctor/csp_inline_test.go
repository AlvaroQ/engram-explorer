package doctor_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/doctor"
)

// Rendered-HTML patterns that a script-src without 'unsafe-inline' (or an
// htmx config with allowEval:false) would block. Mirrors the ui package check.
var (
	docHTMLCommentRe   = regexp.MustCompile(`(?s)<!--.*?-->`)
	docScriptTagRe     = regexp.MustCompile(`(?is)<script\b[^>]*>`)
	docSrcAttrRe       = regexp.MustCompile(`(?i)\ssrc\s*=`)
	docHTMLTagRe       = regexp.MustCompile(`(?s)<[a-zA-Z][^>]*>`)
	docEventAttrRe     = regexp.MustCompile(`(?i)\son[a-z]+\s*=`)
	docHxOnAttrRe      = regexp.MustCompile(`(?i)\s(data-)?hx-on`)
	docTriggerFilterRe = regexp.MustCompile(`(?i)hx-trigger="[^"]*\[`)
)

func docInlineCodeViolations(html string) []string {
	html = docHTMLCommentRe.ReplaceAllString(html, "")
	var v []string
	for _, tag := range docScriptTagRe.FindAllString(html, -1) {
		if !docSrcAttrRe.MatchString(tag) {
			v = append(v, "inline <script> without src: "+tag)
		}
	}
	for _, tag := range docHTMLTagRe.FindAllString(html, -1) {
		if m := docEventAttrRe.FindString(tag); m != "" {
			v = append(v, "inline event handler "+strings.TrimSpace(m)+" in "+tag)
		}
		if docHxOnAttrRe.MatchString(tag) {
			v = append(v, "hx-on attribute in "+tag)
		}
		if docTriggerFilterRe.MatchString(tag) {
			v = append(v, "hx-trigger event filter (needs eval) in "+tag)
		}
	}
	return v
}

// TestCSP_DoctorViewsHaveNoInlineScriptCode renders the maintenance page (sync
// polling shell), the orphans list and the orphan detail dialog. None may
// carry inline script code; each must carry the data-* hook that replaced it.
func TestCSP_DoctorViewsHaveNoInlineScriptCode(t *testing.T) {
	db := openTestDB(t)
	id := seedOrphanObservation(t, db)
	mux := http.NewServeMux()
	doctor.Mount(mux, doctor.Deps{RoDB: db})

	cases := []struct {
		path string
		htmx bool
		want map[string]int // substring -> minimum occurrences
	}{
		{"/settings/maintenance", false, map[string]int{
			`hx-trigger="load, every 15s"`: 2,
			"data-poll-visible-only":       2,
			"/static/ui-actions.js":        1,
		}},
		{"/doctor/orphans/list", true, map[string]int{`id="orphan-dialog-host"`: 1}},
		{fmt.Sprintf("/doctor/orphans/observations/%d/detail", id), true, map[string]int{
			`data-clear-target="orphan-dialog-host"`:        2, // backdrop + close button
			`data-escape-clear-target="orphan-dialog-host"`: 1,
		}},
	}
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			if tc.htmx {
				req.Header.Set("HX-Request", "true")
			}
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)
			if w.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
			}
			body := w.Body.String()
			for _, v := range docInlineCodeViolations(body) {
				t.Errorf("%s", v)
			}
			for s, n := range tc.want {
				if got := strings.Count(body, s); got < n {
					t.Errorf("%s: found %d, want at least %d", s, got, n)
				}
			}
		})
	}
}
