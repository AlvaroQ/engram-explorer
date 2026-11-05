package ui_test

// Token-level WCAG AA guard (Phase 4, S1). The color tokens in
// static/tokens.css are resolved per theme and every text/background pair the
// UI actually renders is checked against the WCAG 2.x contrast thresholds:
// 4.5:1 for normal text and 3:1 for large text and non-text UI (dots, focus
// rings). Translucent backgrounds ("token/alpha@base") are alpha-composited
// over their base token before measuring, matching how the CSS paints
// hsl(var(--token) / alpha) on top of a surface.

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/ui"
)

const (
	aaText = 4.5 // normal text
	aaUI   = 3.0 // large text and non-text UI components
)

type contrastPair struct {
	fg, bg string
	min    float64
	usage  string
}

// contrastPairs lists every token pairing the UI renders. Keep it in sync with
// the AA table in design/system.md.
var contrastPairs = []contrastPair{
	// Body copy and secondary text on every neutral background.
	{"fg", "bg", aaText, "body text"},
	{"fg", "surface", aaText, "card and sidebar text"},
	{"fg", "surface-2", aaText, "inputs, menus, hover rows"},
	{"fg-muted", "bg", aaText, "page subtitles"},
	{"fg-muted", "surface", aaText, "labels, table heads, captions"},
	{"fg-muted", "surface-2", aaText, "secondary button, neutral badge"},
	// Accent as text.
	{"accent", "bg", aaText, "links in main content"},
	{"accent", "surface", aaText, "links in cards, active settings gear"},
	{"accent", "surface-2", aaText, "links on hover rows"},
	{"accent", "accent/0.18@surface", aaText, "active nav link, accent badge"},
	// Status text: pills on the sidebar, inline status, tinted badges.
	{"ok", "bg", aaText, "inline success text"},
	{"ok", "surface", aaText, "status pill (all ok)"},
	{"ok", "ok/0.18@surface", aaText, "success badge"},
	{"warn", "bg", aaText, "inline warning text"},
	{"warn", "surface", aaText, "status pill (daemon offline)"},
	{"warn", "warn/0.18@surface", aaText, "warning badge"},
	{"fail", "bg", aaText, "inline error text"},
	{"fail", "surface", aaText, "status pill (db error), form errors"},
	{"fail", "fail/0.18@surface", aaText, "error badge, doctor banner"},
	{"info", "info/0.18@surface", aaText, "doctor severity-low badge"},
	{"fg", "warn/0.15@bg", aaText, "overview sync alert"},
	// Overview hero and KPI cards (02-brain-first): glass surface over the
	// hero background, tonal CTA.
	{"fg", "surface/0.72@bg", aaText, "KPI values, search text on glass"},
	{"fg-muted", "surface/0.72@bg", aaText, "KPI labels and captions on glass"},
	{"fg", "accent/0.22@bg", aaText, "tonal CTA (hover layer)"},
	// Filled buttons.
	{"accent-fg", "accent", aaText, "primary button, active toggle button"},
	// Agent brand colors: text in KPI captions, dots as UI.
	{"brand-cc", "surface", aaText, "Claude Code caption text"},
	{"brand-codex", "surface", aaText, "Codex caption text"},
	{"brand-engram", "surface", aaUI, "Engram group dot"},
	{"brand-cc", "surface", aaUI, "Claude Code group dot"},
	{"brand-codex", "surface", aaUI, "Codex group dot"},
	{"accent", "bg", aaUI, "focus ring"},
}

func TestTokenContrastWCAGAA(t *testing.T) {
	themes := loadThemes(t)
	for _, theme := range []string{"dark", "light"} {
		tokens := themes[theme]
		for _, p := range contrastPairs {
			name := fmt.Sprintf("%s/%s_on_%s", theme, p.fg, p.bg)
			t.Run(name, func(t *testing.T) {
				fg := resolveColor(t, tokens, p.fg)
				bg := resolveColor(t, tokens, p.bg)
				ratio := contrastRatio(fg, bg)
				t.Logf("%-5s %-12s on %-20s %5.2f:1 (min %.1f) — %s", theme, p.fg, p.bg, ratio, p.min, p.usage)
				if ratio < p.min {
					t.Errorf("%s: %s on %s is %.2f:1, below WCAG AA %.1f:1 (%s)",
						theme, p.fg, p.bg, ratio, p.min, p.usage)
				}
			})
		}
	}
}

// TestContrastMath pins the luminance math against known WCAG reference
// values so a broken formula cannot make the guard pass vacuously.
func TestContrastMath(t *testing.T) {
	black, white := rgb{0, 0, 0}, rgb{1, 1, 1}
	if got := contrastRatio(black, white); math.Abs(got-21) > 0.01 {
		t.Fatalf("black/white = %.3f, want 21", got)
	}
	if got := contrastRatio(white, white); math.Abs(got-1) > 0.001 {
		t.Fatalf("white/white = %.3f, want 1", got)
	}
	// #767676 on white is the canonical 4.54:1 AA boundary grey.
	grey := hslToRGB(0, 0, 118.0/255.0)
	if got := contrastRatio(grey, white); math.Abs(got-4.54) > 0.01 {
		t.Fatalf("#767676/white = %.3f, want 4.54", got)
	}
}

// ---------------------------------------------------------------------------
// tokens.css parsing
// ---------------------------------------------------------------------------

var (
	cssComment = regexp.MustCompile(`(?s)/\*.*?\*/`)
	cssBlock   = regexp.MustCompile(`(?s)(:root(?:\.light)?)\s*\{(.*?)\}`)
	cssDecl    = regexp.MustCompile(`--([a-z0-9-]+)\s*:\s*([^;]+);`)
)

// loadThemes returns the custom properties of each theme: dark is every
// `:root` block; light is dark overlaid with every `:root.light` block.
func loadThemes(t *testing.T) map[string]map[string]string {
	t.Helper()
	raw, err := ui.StaticFS.ReadFile("static/tokens.css")
	if err != nil {
		t.Fatalf("read embedded static/tokens.css: %v", err)
	}
	src := cssComment.ReplaceAllString(string(raw), "")
	dark, lightOnly := map[string]string{}, map[string]string{}
	for _, m := range cssBlock.FindAllStringSubmatch(src, -1) {
		dst := dark
		if m[1] == ":root.light" {
			dst = lightOnly
		}
		for _, d := range cssDecl.FindAllStringSubmatch(m[2], -1) {
			dst[d[1]] = strings.TrimSpace(d[2])
		}
	}
	if len(dark) == 0 || len(lightOnly) == 0 {
		t.Fatalf("tokens.css: expected :root and :root.light blocks (dark=%d light=%d)", len(dark), len(lightOnly))
	}
	light := map[string]string{}
	for k, v := range dark {
		light[k] = v
	}
	for k, v := range lightOnly {
		light[k] = v
	}
	return map[string]map[string]string{"dark": dark, "light": light}
}

// resolveColor turns a pair operand into an opaque color. Supported forms:
// "token" and "token/alpha@base" (token at alpha composited over base).
func resolveColor(t *testing.T, tokens map[string]string, spec string) rgb {
	t.Helper()
	if fgPart, base, ok := strings.Cut(spec, "@"); ok {
		name, alphaStr, ok := strings.Cut(fgPart, "/")
		if !ok {
			t.Fatalf("bad composite spec %q", spec)
		}
		alpha, err := strconv.ParseFloat(alphaStr, 64)
		if err != nil {
			t.Fatalf("bad alpha in %q: %v", spec, err)
		}
		top := tokenColor(t, tokens, name)
		under := tokenColor(t, tokens, base)
		return rgb{
			top.r*alpha + under.r*(1-alpha),
			top.g*alpha + under.g*(1-alpha),
			top.b*alpha + under.b*(1-alpha),
		}
	}
	return tokenColor(t, tokens, spec)
}

var hslTriplet = regexp.MustCompile(`^(-?[\d.]+)\s+([\d.]+)%\s+([\d.]+)%$`)

func tokenColor(t *testing.T, tokens map[string]string, name string) rgb {
	t.Helper()
	v, ok := tokens[name]
	if !ok {
		t.Fatalf("token --%s is not defined in tokens.css", name)
	}
	m := hslTriplet.FindStringSubmatch(v)
	if m == nil {
		t.Fatalf("token --%s = %q is not a bare HSL triplet", name, v)
	}
	h, _ := strconv.ParseFloat(m[1], 64)
	s, _ := strconv.ParseFloat(m[2], 64)
	l, _ := strconv.ParseFloat(m[3], 64)
	return hslToRGB(h, s/100, l/100)
}

// ---------------------------------------------------------------------------
// WCAG 2.x color math
// ---------------------------------------------------------------------------

type rgb struct{ r, g, b float64 } // sRGB channels in [0,1]

// hslToRGB follows the CSS Color 4 hsl() conversion.
func hslToRGB(h, s, l float64) rgb {
	h = math.Mod(h, 360)
	if h < 0 {
		h += 360
	}
	f := func(n float64) float64 {
		k := math.Mod(n+h/30, 12)
		a := s * math.Min(l, 1-l)
		return l - a*math.Max(-1, math.Min(math.Min(k-3, 9-k), 1))
	}
	return rgb{f(0), f(8), f(4)}
}

func relativeLuminance(c rgb) float64 {
	lin := func(v float64) float64 {
		if v <= 0.04045 {
			return v / 12.92
		}
		return math.Pow((v+0.055)/1.055, 2.4)
	}
	return 0.2126*lin(c.r) + 0.7152*lin(c.g) + 0.0722*lin(c.b)
}

func contrastRatio(a, b rgb) float64 {
	la, lb := relativeLuminance(a), relativeLuminance(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}
