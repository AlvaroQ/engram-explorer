package ui_test

// Selected/primary button hover guard (Phase 4, S1/S6). A generic `.btn:hover`
// once out-ranked `.btn-primary` and `.btn.is-active` and repainted the fill
// with a neutral surface while the text stayed --accent-fg: dark text on a
// dark fill in the dark theme, white on white in the light one. This test runs
// a tiny cascade over static/styles.css for each selected-state button the
// templates render and checks the winning text and fill stay WCAG AA in both
// themes, at rest and on hover.

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/ui"
)

// cssRule is one top-level style rule; order is its position in the file.
type cssRule struct {
	selectors []string
	decls     map[string]string
	order     int
}

// topLevelRules returns the plain style rules of src; at-rule blocks
// (@media, @supports, @keyframes) are skipped as a whole.
func topLevelRules(src string) []cssRule {
	src = cssComment.ReplaceAllString(src, "")
	var out []cssRule
	for i := 0; i < len(src); {
		open := strings.IndexByte(src[i:], '{')
		if open < 0 {
			break
		}
		prelude := strings.TrimSpace(src[i : i+open])
		// Find the matching close brace.
		depth, j := 0, i+open
		for ; j < len(src); j++ {
			if src[j] == '{' {
				depth++
			} else if src[j] == '}' {
				depth--
				if depth == 0 {
					break
				}
			}
		}
		body := src[i+open+1 : j]
		i = j + 1
		if strings.HasPrefix(prelude, "@") {
			continue
		}
		decls := map[string]string{}
		for _, d := range strings.Split(body, ";") {
			k, v, ok := strings.Cut(d, ":")
			if !ok {
				continue
			}
			decls[strings.TrimSpace(k)] = strings.Join(strings.Fields(v), " ")
		}
		var sels []string
		for _, s := range strings.Split(prelude, ",") {
			sels = append(sels, strings.Join(strings.Fields(s), " "))
		}
		out = append(out, cssRule{selectors: sels, decls: decls, order: len(out)})
	}
	return out
}

// element is a button with classes and attributes inside ancestors that
// carry ancestorClasses.
type element struct {
	classes         []string
	attrs           map[string]string
	hover           bool
	ancestorClasses []string
}

var compoundPart = regexp.MustCompile(`\.[a-zA-Z0-9_-]+|\[[a-z-]+(?:="[^"]*")?\]|:[a-z-]+`)

// matchCompound reports whether a compound selector (classes, attributes and
// :hover only) matches the given classes/attrs; it returns the specificity.
func matchCompound(sel string, classes []string, attrs map[string]string, hover bool) (bool, int) {
	if compoundPart.ReplaceAllString(sel, "") != "" {
		return false, 0 // tag, universal or unsupported syntax
	}
	has := func(c string) bool {
		for _, x := range classes {
			if x == c {
				return true
			}
		}
		return false
	}
	spec := 0
	for _, p := range compoundPart.FindAllString(sel, -1) {
		spec++
		switch {
		case strings.HasPrefix(p, "."):
			if !has(p[1:]) {
				return false, 0
			}
		case strings.HasPrefix(p, "["):
			name, val, withVal := strings.Cut(strings.Trim(p, "[]"), "=")
			got, ok := attrs[name]
			if !ok || (withVal && got != strings.Trim(val, `"`)) {
				return false, 0
			}
		case p == ":hover":
			if !hover {
				return false, 0
			}
		default:
			return false, 0
		}
	}
	return true, spec
}

// matches handles a compound selector or "ancestor-compound compound".
func (e element) matches(sel string) (bool, int) {
	parts := strings.Split(sel, " ")
	switch len(parts) {
	case 1:
		return matchCompound(parts[0], e.classes, e.attrs, e.hover)
	case 2:
		okA, sa := matchCompound(parts[0], e.ancestorClasses, nil, false)
		okE, se := matchCompound(parts[1], e.classes, e.attrs, e.hover)
		return okA && okE, sa + se
	}
	return false, 0
}

type winner struct {
	value      string
	spec, ord  int
	found      bool
	declaredAt string
}

func (w winner) beats(spec, ord int) bool {
	return w.found && (w.spec > spec || (w.spec == spec && w.ord > ord))
}

// cascade returns the winning declaration for each property name.
func cascade(rules []cssRule, e element, props ...string) map[string]winner {
	out := map[string]winner{}
	for _, r := range rules {
		best := -1
		for _, s := range r.selectors {
			if ok, sp := e.matches(s); ok && sp > best {
				best = sp
			}
		}
		if best < 0 {
			continue
		}
		for _, p := range props {
			v, ok := r.decls[p]
			if !ok {
				continue
			}
			if cur := out[p]; !cur.beats(best, r.order) {
				out[p] = winner{value: v, spec: best, ord: r.order, found: true,
					declaredAt: strings.Join(r.selectors, ", ")}
			}
		}
	}
	return out
}

var hslVar = regexp.MustCompile(`^hsl\(var\(--([a-z0-9-]+)\)(?:\s*/\s*(var\(--([a-z0-9-]+)\)|[\d.]+))?\)$`)

// paint is a token color at alpha (alpha 0 = transparent).
type paint struct {
	token string
	alpha float64
}

func parsePaint(t *testing.T, tokens map[string]string, v string) paint {
	t.Helper()
	v = strings.TrimSpace(v)
	if v == "transparent" || v == "none" || v == "" {
		return paint{alpha: 0}
	}
	m := hslVar.FindStringSubmatch(v)
	if m == nil {
		t.Fatalf("unsupported color value %q", v)
	}
	alpha := 1.0
	switch {
	case m[3] != "":
		a, err := strconv.ParseFloat(tokens[m[3]], 64)
		if err != nil {
			t.Fatalf("token --%s is not a number: %q", m[3], tokens[m[3]])
		}
		alpha = a
	case m[2] != "":
		alpha, _ = strconv.ParseFloat(m[2], 64)
	}
	return paint{token: m[1], alpha: alpha}
}

func over(top rgb, alpha float64, under rgb) rgb {
	return rgb{top.r*alpha + under.r*(1-alpha), top.g*alpha + under.g*(1-alpha), top.b*alpha + under.b*(1-alpha)}
}

var gradientLayer = regexp.MustCompile(`^linear-gradient\(\s*(hsl\(var\([^)]*\)(?:\s*/\s*(?:var\([^)]*\)|[\d.]+))?\)),\s*(hsl\(var\([^)]*\)(?:\s*/\s*(?:var\([^)]*\)|[\d.]+))?\))\s*\)$`)

// effectiveColors resolves the painted text and fill of e over a --surface
// page, including a background-image state layer and element opacity.
func effectiveColors(t *testing.T, tokens map[string]string, rules []cssRule, e element) (fg, bg rgb, why string) {
	t.Helper()
	w := cascade(rules, e, "color", "background", "background-color", "background-image", "opacity")
	base := tokenColor(t, tokens, "surface")

	fill := w["background"]
	if bc := w["background-color"]; bc.found && !fill.beats(bc.spec, bc.ord) {
		fill = bc
	}
	bgPaint := parsePaint(t, tokens, fill.value)
	bg = base
	if bgPaint.alpha > 0 {
		bg = over(tokenColor(t, tokens, bgPaint.token), bgPaint.alpha, base)
	}
	// A background-image layer survives unless a later/stronger shorthand reset it.
	if img := w["background-image"]; img.found && !(w["background"].found && w["background"].beats(img.spec, img.ord)) {
		m := gradientLayer.FindStringSubmatch(img.value)
		if m == nil {
			t.Fatalf("unsupported background-image %q", img.value)
		}
		layer := parsePaint(t, tokens, m[1])
		bg = over(tokenColor(t, tokens, layer.token), layer.alpha, bg)
	}
	fgPaint := parsePaint(t, tokens, w["color"].value)
	fg = over(tokenColor(t, tokens, fgPaint.token), fgPaint.alpha, bg)
	if op := w["opacity"]; op.found {
		o, err := strconv.ParseFloat(op.value, 64)
		if err != nil {
			t.Fatalf("opacity %q", op.value)
		}
		fg, bg = over(fg, o, base), over(bg, o, base)
	}
	why = fmt.Sprintf("color %q from %q; fill %q from %q", w["color"].value, w["color"].declaredAt, fill.value, fill.declaredAt)
	return fg, bg, why
}

func TestSelectedButtonStatesStayReadable(t *testing.T) {
	raw, err := ui.StaticFS.ReadFile("static/styles.css")
	if err != nil {
		t.Fatal(err)
	}
	rules := topLevelRules(string(raw))
	themes := loadThemes(t)
	cases := []struct {
		name string
		el   element
	}{
		// Agent tabs, Memory tabs and other selected .btn-primary choices.
		{"primary", element{classes: []string{"btn", "btn-sm", "btn-primary"}}},
		// Conversation filter toggle (cc-conv-filter.js adds is-active + aria-pressed).
		{"is-active toggle", element{classes: []string{"btn", "btn-secondary", "btn-sm", "is-active"}, attrs: map[string]string{"aria-pressed": "true"}}},
		// Settings language/theme choices (outlined selected style).
		{"preference choice", element{classes: []string{"btn", "btn-sm", "btn-primary"}, ancestorClasses: []string{"preference-controls"}}},
	}
	for _, theme := range []string{"dark", "light"} {
		for _, c := range cases {
			for _, hover := range []bool{false, true} {
				el := c.el
				el.hover = hover
				name := fmt.Sprintf("%s/%s/hover=%v", theme, c.name, hover)
				t.Run(name, func(t *testing.T) {
					fg, bg, why := effectiveColors(t, themes[theme], rules, el)
					r := contrastRatio(fg, bg)
					t.Logf("%s: %.2f:1 (%s)", name, r, why)
					if r < aaText {
						t.Errorf("%s: text contrast %.2f:1 below %.1f:1 (%s)", name, r, aaText, why)
					}
				})
			}
		}
	}
}
