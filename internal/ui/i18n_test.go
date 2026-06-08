package ui

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// ---------------------------------------------------------------------------
// T() — key resolution
// ---------------------------------------------------------------------------

func TestT_NestedKey_English(t *testing.T) {
	got := T("en", "sidebar.topics")
	if got != "Topics" {
		t.Errorf("T(en, sidebar.topics) = %q; want %q", got, "Topics")
	}
}

func TestT_NestedKey_Spanish(t *testing.T) {
	got := T("es", "sidebar.topics")
	if got != "Temas" {
		t.Errorf("T(es, sidebar.topics) = %q; want %q", got, "Temas")
	}
}

func TestT_DeepNested_English(t *testing.T) {
	got := T("en", "topics.filterPlaceholder")
	want := "Filter by topic key or project…"
	if got != want {
		t.Errorf("T(en, topics.filterPlaceholder) = %q; want %q", got, want)
	}
}

func TestT_FallbackToKey_Missing(t *testing.T) {
	got := T("en", "this.key.does.not.exist")
	if got != "this.key.does.not.exist" {
		t.Errorf("missing key should fall back to key, got %q", got)
	}
}

func TestT_Interpolation(t *testing.T) {
	// "topics.entries" = "{{count}} entries"
	got := T("en", "topics.entries", "count", "42")
	want := "42 entries"
	if got != want {
		t.Errorf("T interpolation = %q; want %q", got, want)
	}
}

func TestT_UnknownLangFallsBackToEnglish(t *testing.T) {
	got := T("fr", "sidebar.topics")
	// "fr" is not loaded; should fall back to en value
	if got != "Topics" {
		t.Errorf("unknown lang should fall back to en; got %q", got)
	}
}

func TestT_SpanishThenEnglishFallback(t *testing.T) {
	// "common.noProject" exists in both
	en := T("en", "common.noProject")
	es := T("es", "common.noProject")
	if en == "" || es == "" {
		t.Errorf("common.noProject should exist in both languages; en=%q es=%q", en, es)
	}
	if en == es {
		t.Errorf("en and es translations for common.noProject should differ; both = %q", en)
	}
}

// ---------------------------------------------------------------------------
// langForRequest() — language resolution
// ---------------------------------------------------------------------------

func TestLangForRequest_Cookie(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "lang", Value: "es"})
	got := langForRequest(req)
	if got != "es" {
		t.Errorf("cookie lang=es; got %q", got)
	}
}

func TestLangForRequest_AcceptLanguage(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept-Language", "es-AR,es;q=0.9,en;q=0.8")
	got := langForRequest(req)
	if got != "es" {
		t.Errorf("Accept-Language es-AR should resolve to es; got %q", got)
	}
}

func TestLangForRequest_Default(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	got := langForRequest(req)
	if got != "en" {
		t.Errorf("no cookie, no header: should default to en; got %q", got)
	}
}

func TestLangForRequest_CookiePriorityOverHeader(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "lang", Value: "en"})
	req.Header.Set("Accept-Language", "es-AR")
	got := langForRequest(req)
	if got != "en" {
		t.Errorf("cookie should take priority over Accept-Language; got %q", got)
	}
}

func TestLangForRequest_UnsupportedCookieFallsToHeader(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "lang", Value: "fr"}) // unsupported
	req.Header.Set("Accept-Language", "es")
	got := langForRequest(req)
	if got != "es" {
		t.Errorf("unsupported cookie lang should fall through to Accept-Language; got %q", got)
	}
}

// ---------------------------------------------------------------------------
// themeForRequest()
// ---------------------------------------------------------------------------

func TestThemeForRequest_Cookie(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "theme", Value: "light"})
	got := themeForRequest(req)
	if got != "light" {
		t.Errorf("theme cookie=light; got %q", got)
	}
}

func TestThemeForRequest_Default(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	got := themeForRequest(req)
	if got != "dark" {
		t.Errorf("no theme cookie; should default to dark; got %q", got)
	}
}
