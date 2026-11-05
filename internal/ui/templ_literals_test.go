package ui_test

// No hard-coded English in templ (Phase 4, S6): every user-visible string of
// the ui and doctor templates goes through T(...) so the es catalog applies.
// The check scans the templ blocks of each source file for text nodes, bare
// text lines, literal accessible-name/tooltip/confirm attributes, string
// literals passed as attribute expressions and literal EmptyState /
// ErrorPartial messages. Brand names and technical ids are allowlisted.

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

var (
	literalHTMLComment = regexp.MustCompile(`(?s)<!--.*?-->`)
	literalTextNode    = regexp.MustCompile(`>([^<>{}]*[A-Za-z]{2}[^<>{}]*)[<{]`)
	literalAttr        = regexp.MustCompile(`\b(aria-label|title|placeholder|alt|hx-confirm|data-submit-confirm)="([^"]*[A-Za-z]{2}[^"]*)"`)
	literalAttrExpr    = regexp.MustCompile(`\b(?:aria-label|title|placeholder|alt|hx-confirm|data-submit-confirm)=\{\s*"([^"]*[A-Za-z]{3}[^"]*)"`)
	literalComponent   = regexp.MustCompile(`@(?:ui\.)?(?:EmptyState|ErrorPartial)\("([^"]*)"`)
	literalBareLine    = regexp.MustCompile(`^[A-Z][A-Za-z ,.'()…-]*[:#]?(\s*\{.*\})?$`)
)

// literalAllowed are strings that stay as-is in every language.
var literalAllowed = map[string]bool{
	"ID": true, "topic_key": true, "Engram": true, "Explorer": true,
	"Claude Code": true, "Codex": true, "&times;": true, "&larr;": true,
}

// templLiterals returns "file:line: text" for every English literal found in
// the templ blocks of the given templ source.
func templLiterals(name, src string) []string {
	src = literalHTMLComment.ReplaceAllStringFunc(src, func(c string) string {
		return strings.Repeat("\n", strings.Count(c, "\n"))
	})
	var out []string
	inTempl := false
	for i, line := range strings.Split(src, "\n") {
		if strings.HasPrefix(line, "templ ") {
			inTempl = true
			continue
		}
		if line == "}" {
			inTempl = false
			continue
		}
		trimmed := strings.TrimSpace(line)
		if !inTempl || strings.HasPrefix(trimmed, "//") {
			continue
		}
		var found []string
		for _, m := range literalTextNode.FindAllStringSubmatch(line, -1) {
			found = append(found, m[1])
		}
		for _, m := range literalAttr.FindAllStringSubmatch(line, -1) {
			found = append(found, m[2])
		}
		for _, m := range literalAttrExpr.FindAllStringSubmatch(line, -1) {
			found = append(found, m[1])
		}
		for _, m := range literalComponent.FindAllStringSubmatch(line, -1) {
			found = append(found, m[1])
		}
		if !strings.ContainsAny(trimmed, "<>@=") && literalBareLine.MatchString(trimmed) {
			found = append(found, strings.TrimSpace(strings.SplitN(trimmed, "{", 2)[0]))
		}
		for _, f := range found {
			f = strings.TrimSpace(f)
			if f == "" || literalAllowed[f] || !regexp.MustCompile(`[A-Za-z]{2}`).MatchString(f) {
				continue
			}
			out = append(out, name+":"+strconv.Itoa(i+1)+": "+f)
		}
	}
	return out
}

func TestTemplLiterals_Detects(t *testing.T) {
	for _, src := range []string{
		"templ a() {\n\t<p>Loading…</p>\n}",
		"templ a() {\n\t<button aria-label=\"Close\">x</button>\n}",
		"templ a() {\n\t<th>Started At</th>\n}",
		"templ a() {\n\t<span>Hint: { h }</span>\n}",
		"templ a() {\n\t\tEnrolled:\n}",
		"templ a() {\n\t<button hx-confirm={ \"Delete this \" + x }>\n}",
		"templ a() {\n\t@ui.EmptyState(\"No pending mutations.\")\n}",
	} {
		if len(templLiterals("x.templ", src)) == 0 {
			t.Errorf("checker missed the literal in %q", src)
		}
	}
	for _, src := range []string{
		"templ a() {\n\t<p>{ T(lang, \"common.loading\") }</p>\n}",
		"templ a() {\n\t<th>ID</th>\n}",
		"templ a() {\n\t<!-- a <b>Comment</b> -->\n}",
		"templ a() {\n\t// Comment line\n}",
		"type data struct {\n\tUptimeS int\n}",
		"templ a() {\n\t<button aria-label={ T(lang, \"x\") }>\n}",
		"templ a() {\n\t<a class={ \"btn btn-sm\", x } hx-post={ \"/settings/modules/\" + id }>\n}",
	} {
		if got := templLiterals("x.templ", src); len(got) != 0 {
			t.Errorf("checker false positive on %q: %v", src, got)
		}
	}
}

func TestTemplSources_NoEnglishLiterals(t *testing.T) {
	var files []string
	for _, glob := range []string{"*.templ", "../doctor/*.templ"} {
		m, err := filepath.Glob(glob)
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, m...)
	}
	if len(files) == 0 {
		t.Fatal("no templ sources found")
	}
	var all []string
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		all = append(all, templLiterals(f, string(raw))...)
	}
	for _, l := range all {
		t.Errorf("hard-coded user-visible text (use T): %s", l)
	}
}
