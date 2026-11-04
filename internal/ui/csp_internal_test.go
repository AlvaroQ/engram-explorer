package ui

import (
	"bytes"
	"context"
	"regexp"
	"strings"
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/services"
	"github.com/a-h/templ"
)

// Patterns for rendered HTML that a script-src without 'unsafe-inline' (or an
// htmx config with allowEval:false) would block.
var (
	htmlCommentRe   = regexp.MustCompile(`(?s)<!--.*?-->`)
	scriptTagRe     = regexp.MustCompile(`(?is)<script\b[^>]*>`)
	srcAttrRe       = regexp.MustCompile(`(?i)\ssrc\s*=`)
	htmlTagRe       = regexp.MustCompile(`(?s)<[a-zA-Z][^>]*>`)
	eventAttrRe     = regexp.MustCompile(`(?i)\son[a-z]+\s*=`)
	hxOnAttrRe      = regexp.MustCompile(`(?i)\s(data-)?hx-on`)
	jsURLRe         = regexp.MustCompile(`(?i)javascript:`)
	triggerFilterRe = regexp.MustCompile(`(?i)hx-trigger="[^"]*\[`)
)

// InlineCodeViolations lists inline script code in rendered HTML: <script>
// without src, on*= event handler attributes, hx-on attributes, javascript:
// URLs and hx-trigger event filters. HTML comments are ignored. Exported for
// the external ui_test package.
func InlineCodeViolations(html string) []string {
	html = htmlCommentRe.ReplaceAllString(html, "")
	var v []string
	for _, tag := range scriptTagRe.FindAllString(html, -1) {
		if !srcAttrRe.MatchString(tag) {
			v = append(v, "inline <script> without src: "+tag)
		}
	}
	for _, tag := range htmlTagRe.FindAllString(html, -1) {
		if m := eventAttrRe.FindString(tag); m != "" {
			v = append(v, "inline event handler "+strings.TrimSpace(m)+" in "+tag)
		}
		if hxOnAttrRe.MatchString(tag) {
			v = append(v, "hx-on attribute in "+tag)
		}
		if jsURLRe.MatchString(tag) {
			v = append(v, "javascript: URL in "+tag)
		}
		if triggerFilterRe.MatchString(tag) {
			v = append(v, "hx-trigger event filter in "+tag)
		}
	}
	return v
}

func renderToString(t *testing.T, ctx context.Context, c templ.Component) string {
	t.Helper()
	var buf bytes.Buffer
	if err := c.Render(ctx, &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	return buf.String()
}

// TestInlineCodeViolations_Detects proves the rendered-HTML checker catches
// each pattern it is meant to block.
func TestInlineCodeViolations_Detects(t *testing.T) {
	for _, html := range []string{
		"<script>alert(1)</script>",
		`<button onclick="x()">x</button>`,
		`<input type="file" onchange="this.form.submit()">`,
		`<button hx-on::after-request="x()">x</button>`,
		`<a href="javascript:void(0)">x</a>`,
		`<div hx-trigger="every 15s [document.visibilityState=='visible']"></div>`,
	} {
		if len(InlineCodeViolations(html)) == 0 {
			t.Errorf("checker missed inline code in %q", html)
		}
	}
	for _, html := range []string{
		`<script src="/static/ui-actions.js" defer></script>`,
		`<p>&lt;img src=x onerror=alert(1)&gt;</p>`,
		`<!-- <script>old()</script> -->`,
	} {
		if v := InlineCodeViolations(html); len(v) != 0 {
			t.Errorf("checker false positive on %q: %v", html, v)
		}
	}
}

// TestCSPComponents_UseDataHooks renders every component whose behaviour moved
// from inline code to ui-actions.js. Each must be free of inline code and
// carry the data-* hook the static script listens for.
func TestCSPComponents_UseDataHooks(t *testing.T) {
	ctx := context.Background()
	title := "t"
	cases := []struct {
		name string
		c    templ.Component
		want []string
	}{
		{"demo banner", DemoBanner("en"), []string{`data-hide-target="demo-banner"`}},
		{
			"observation detail close",
			ObservationDetailPartial(&services.ObservationRow{ID: 1, Type: "decision", Title: &title}, nil, "Heading", "en"),
			[]string{`data-hide-target="obs-detail"`},
		},
		{
			"sessions load more",
			sessionsLoadMoreRow("c1", services.SessionListParams{}, "en"),
			[]string{`data-remove-after-request="sessions-load-more-row"`},
		},
		{
			"cc sessions load more",
			ccsLoadMoreRow("c1", services.CCSessionListParams{}, "en"),
			[]string{`data-remove-after-request="cc-sessions-load-more-row"`},
		},
		{
			"project combine card",
			projectDetailCombineCard("alpha", []string{"beta"}, "en"),
			[]string{`data-merge-confirm-source="alpha"`, `id="merge-target"`},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			html := renderToString(t, ctx, tc.c)
			for _, v := range InlineCodeViolations(html) {
				t.Errorf("%s", v)
			}
			for _, w := range tc.want {
				if !strings.Contains(html, w) {
					t.Errorf("missing hook %s in:\n%s", w, html)
				}
			}
		})
	}
}
