package ui_test

// No hard-coded English in user-facing errors built in Go (Phase 4, S6): the
// ui and doctor handlers pass ErrorPartial / EmptyState a localized string
// (T or tr), never a literal, and the prefix of RenderError / ErrorMessage
// (shown before a user-safe error message) is an i18n key or "". Complements
// TestTemplSources_NoEnglishLiterals, which covers the templ sources.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/ui"
)

// prefixArg is the index of the prefix argument of the error helpers.
var prefixArg = map[string]int{"RenderError": 2, "ErrorMessage": 1}

// literalArgs are helpers whose message argument must not be a literal.
var literalArgs = map[string]bool{"ErrorPartial": true, "EmptyState": true}

func callName(fn ast.Expr) string {
	switch f := fn.(type) {
	case *ast.Ident:
		return f.Name
	case *ast.SelectorExpr:
		return f.Sel.Name
	}
	return ""
}

// hasTextLiteral reports a string literal with letters inside e (directly, in
// a concatenation, or as an fmt.Sprintf format).
func hasTextLiteral(e ast.Expr) bool {
	found := false
	ast.Inspect(e, func(n ast.Node) bool {
		if c, ok := n.(*ast.CallExpr); ok {
			if name := callName(c.Fun); name == "T" || name == "tr" || prefixArg[name] > 0 {
				return false // localized, or an error helper checked on its own
			}
		}
		if lit, ok := n.(*ast.BasicLit); ok && lit.Kind == token.STRING {
			if s, err := strconv.Unquote(lit.Value); err == nil && strings.IndexFunc(s, func(r rune) bool {
				return r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z'
			}) >= 0 {
				found = true
			}
		}
		return !found
	})
	return found
}

func goErrorLiterals(t *testing.T, path string) []string {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	ast.Inspect(f, func(n ast.Node) bool {
		c, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		name := callName(c.Fun)
		pos := fset.Position(c.Pos()).String()
		if literalArgs[name] && len(c.Args) > 0 && hasTextLiteral(c.Args[0]) {
			out = append(out, pos+": "+name+" with a literal message")
		}
		if i, ok := prefixArg[name]; ok && len(c.Args) > i {
			if _, passThrough := c.Args[i].(*ast.Ident); passThrough {
				return true // a helper forwarding its own prefix parameter
			}
			lit, isLit := c.Args[i].(*ast.BasicLit)
			if !isLit {
				out = append(out, pos+": "+name+" prefix must be a literal i18n key")
				return true
			}
			key, _ := strconv.Unquote(lit.Value)
			if key != "" && ui.T("en", key) == key {
				out = append(out, pos+": "+name+" prefix "+strconv.Quote(key)+" is not an i18n key")
			} else if key != "" && ui.T("es", key) == ui.T("en", key) {
				out = append(out, pos+": "+name+" prefix "+strconv.Quote(key)+" has no es translation")
			}
		}
		return true
	})
	return out
}

func TestGoSources_NoEnglishErrorLiterals(t *testing.T) {
	var files []string
	for _, glob := range []string{"*.go", "../doctor/*.go"} {
		m, err := filepath.Glob(glob)
		if err != nil {
			t.Fatal(err)
		}
		for _, f := range m {
			if !strings.HasSuffix(f, "_test.go") && !strings.HasSuffix(f, "_templ.go") {
				files = append(files, f)
			}
		}
	}
	if len(files) == 0 {
		t.Fatal("no Go sources found")
	}
	for _, f := range files {
		for _, l := range goErrorLiterals(t, f) {
			t.Errorf("%s", l)
		}
	}
}

// TestUIActions_NoEnglishMergeConfirm: the merge confirm text comes from the
// data-merge-confirm-template attribute, not from the script.
func TestUIActions_NoEnglishMergeConfirm(t *testing.T) {
	raw, err := os.ReadFile("static/ui-actions.js")
	if err != nil {
		t.Fatal(err)
	}
	src := string(raw)
	if strings.Contains(src, "Combine") || strings.Contains(src, "cannot be undone") {
		t.Error("ui-actions.js hard-codes the English merge confirm")
	}
	if !strings.Contains(src, "data-merge-confirm-template") {
		t.Error("ui-actions.js does not read data-merge-confirm-template")
	}
}
