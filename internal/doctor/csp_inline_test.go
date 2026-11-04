package doctor_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
	docStyleElemRe     = regexp.MustCompile(`(?i)<style\b`)
	docStyleAttrRe     = regexp.MustCompile(`(?i)\sstyle\s*=`)
	docGoCommentRe     = regexp.MustCompile(`(?m)^\s*//.*$`)
)

// docInlineStyleViolations lists <style> elements and style= attributes, which
// a style-src without 'unsafe-inline' blocks. Mirrors ui.InlineStyleViolations.
func docInlineStyleViolations(html string) []string {
	html = docHTMLCommentRe.ReplaceAllString(html, "")
	var v []string
	for _, m := range docStyleElemRe.FindAllString(html, -1) {
		v = append(v, "inline style element "+m)
	}
	for _, tag := range docHTMLTagRe.FindAllString(html, -1) {
		if docStyleAttrRe.MatchString(tag) {
			v = append(v, "inline style attribute in "+tag)
		}
	}
	return v
}

// TestDocInlineStyleViolations_Detects proves the style checker is meaningful.
func TestDocInlineStyleViolations_Detects(t *testing.T) {
	for _, bad := range []string{"<style>.a{}</style>", `<td style="width:1px">`} {
		if len(docInlineStyleViolations(bad)) == 0 {
			t.Errorf("checker missed inline style in %q", bad)
		}
	}
	for _, good := range []string{`<link rel="stylesheet" href="/doctor/static/styles.css"/>`, "<!-- <style> -->"} {
		if v := docInlineStyleViolations(good); len(v) != 0 {
			t.Errorf("checker false positive on %q: %v", good, v)
		}
	}
}

// TestTemplSources_DoctorNoInlineStyles scans every internal/doctor/*.templ.
func TestTemplSources_DoctorNoInlineStyles(t *testing.T) {
	files, err := filepath.Glob("*.templ")
	if err != nil || len(files) == 0 {
		t.Fatalf("no .templ files found (err=%v); guard is not running", err)
	}
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		for _, v := range docInlineStyleViolations(docGoCommentRe.ReplaceAllString(string(raw), "")) {
			t.Errorf("%s: %s", f, v)
		}
	}
}

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
			for _, v := range docInlineStyleViolations(body) {
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
