package httpapi_test

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/httpapi"
)

// cspDirectives splits a Content-Security-Policy header into a map of
// directive name -> source list.
func cspDirectives(t *testing.T, header string) map[string][]string {
	t.Helper()
	out := map[string][]string{}
	for _, part := range strings.Split(header, ";") {
		fields := strings.Fields(part)
		if len(fields) == 0 {
			continue
		}
		out[fields[0]] = fields[1:]
	}
	return out
}

// TestCSP_PageScriptSrcDisallowsInline checks the CSP on a full HTML page:
// scripts and styles are restricted to 'self' (no 'unsafe-inline', no
// 'unsafe-eval').
func TestCSP_PageScriptSrcDisallowsInline(t *testing.T) {
	c := newTestContainer(t)
	handler := httpapi.NewServeMux(c)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/settings", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /settings status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Fatalf("GET /settings Content-Type = %q, want text/html", ct)
	}
	d := cspDirectives(t, rec.Header().Get("Content-Security-Policy"))

	if got := strings.Join(d["script-src"], " "); got != "'self'" {
		t.Errorf("script-src = %q, want %q", got, "'self'")
	}
	if got := strings.Join(d["style-src"], " "); got != "'self'" {
		t.Errorf("style-src = %q, want %q (no 'unsafe-inline')", got, "'self'")
	}
	if got := strings.Join(d["default-src"], " "); got != "'self'" {
		t.Errorf("default-src = %q, want %q", got, "'self'")
	}
}

// Source-level guard patterns. They scan the templ sources so a new inline
// script, inline event handler or eval-dependent htmx attribute fails CI even
// when no test renders that template.
var (
	templHTMLCommentRe = regexp.MustCompile(`(?s)<!--.*?-->`)
	templGoCommentRe   = regexp.MustCompile(`(?m)^\s*//.*$`)
	templScriptTagRe   = regexp.MustCompile(`(?is)<script\b[^>]*>`)
	templSrcAttrRe     = regexp.MustCompile(`(?i)\ssrc\s*=`)
	templTagRe         = regexp.MustCompile(`(?s)<[a-zA-Z][^>]*>`)
	templEventAttrRe   = regexp.MustCompile(`(?i)\son[a-z]+\s*=`)
	templHxOnRe        = regexp.MustCompile(`(?i)\s(data-)?hx-on`)
	templJSURLRe       = regexp.MustCompile(`(?i)javascript:`)
	templTriggerFiltRe = regexp.MustCompile(`(?i)hx-trigger\s*=\s*"[^"]*\[`)
	templJSPrefixRe    = regexp.MustCompile(`(?i)hx-(vals|vars|headers)\s*=\s*['"]\s*js:`)
	templScriptCompRe  = regexp.MustCompile(`(?m)^script\s+\w+\(`)
)

// templInlineCodeViolations lists every construct in a templ source that a
// script-src without 'unsafe-inline' (or an htmx config without eval) blocks.
func templInlineCodeViolations(src string) []string {
	src = templHTMLCommentRe.ReplaceAllString(src, "")
	src = templGoCommentRe.ReplaceAllString(src, "")
	var v []string
	for _, tag := range templScriptTagRe.FindAllString(src, -1) {
		if !templSrcAttrRe.MatchString(tag) {
			v = append(v, "inline <script> without src: "+tag)
		}
	}
	for _, tag := range templTagRe.FindAllString(src, -1) {
		if m := templEventAttrRe.FindString(tag); m != "" {
			v = append(v, "inline event handler "+strings.TrimSpace(m)+" in "+tag)
		}
		if templHxOnRe.MatchString(tag) {
			v = append(v, "hx-on attribute (needs eval) in "+tag)
		}
	}
	if templJSURLRe.MatchString(src) {
		v = append(v, "javascript: URL")
	}
	for _, m := range templTriggerFiltRe.FindAllString(src, -1) {
		v = append(v, "hx-trigger event filter (needs eval): "+m)
	}
	for _, m := range templJSPrefixRe.FindAllString(src, -1) {
		v = append(v, "js: expression (needs eval): "+m)
	}
	for _, m := range templScriptCompRe.FindAllString(src, -1) {
		v = append(v, "templ script component (renders inline script): "+m)
	}
	return v
}

// TestTemplGuard_DetectsInlineCode proves the guard catches each pattern, so
// a passing TestTemplSources_NoInlineScriptCode is meaningful.
func TestTemplGuard_DetectsInlineCode(t *testing.T) {
	bad := []string{
		"<script>alert(1)</script>",
		`<button onclick="x()">`,
		"<input\n\ttype=\"file\"\n\tonchange=\"this.form.submit()\"/>",
		`<button hx-on::after-request="x()">`,
		`<a href="javascript:void(0)">`,
		`<div hx-trigger="every 15s [document.visibilityState=='visible']">`,
		`<div hx-vals='js:{a: 1}'>`,
		"script doThing(a string) {\n}",
	}
	for _, src := range bad {
		if len(templInlineCodeViolations(src)) == 0 {
			t.Errorf("guard missed inline code in %q", src)
		}
	}
	good := []string{
		`<script src="/static/ui-actions.js" defer></script>`,
		`<script type="module" src="/islands/loader.js"></script>`,
		`<button data-hide-target="demo-banner">`,
		`<div hx-trigger="load, every 15s" hx-include="[name='q']">`,
		"// IslandScript emits the <script> tag that loads the island loader.",
		"<!-- inline <script> was removed -->",
	}
	for _, src := range good {
		if v := templInlineCodeViolations(src); len(v) != 0 {
			t.Errorf("guard false positive on %q: %v", src, v)
		}
	}
}

// TestTemplSources_NoInlineScriptCode scans every internal/**/*.templ file.
func TestTemplSources_NoInlineScriptCode(t *testing.T) {
	root := ".." // internal/
	var scanned int
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || filepath.Ext(path) != ".templ" {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		scanned++
		for _, v := range templInlineCodeViolations(string(raw)) {
			t.Errorf("%s: %s", path, v)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk templ sources: %v", err)
	}
	if scanned == 0 {
		t.Fatal("no .templ files scanned; guard is not running")
	}
}
