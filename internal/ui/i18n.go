package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
)

// contextKey is the unexported type for context keys in this package.
type contextKey int

const (
	langContextKey  contextKey = iota
	themeContextKey contextKey = iota
)

// catalogs holds the parsed locale maps, loaded once at first use.
var (
	catalogOnce sync.Once
	catalogs    map[string]map[string]any
)

// supportedLangs is the set of language codes the loader handles.
var supportedLangs = map[string]bool{"en": true, "es": true}

// loadCatalogs parses en.json and es.json from the embedded locales FS.
// Panics on failure — the embedded files are compile-time guarantees.
func loadCatalogs() map[string]map[string]any {
	catalogOnce.Do(func() {
		catalogs = make(map[string]map[string]any)
		for _, lang := range []string{"en", "es"} {
			data, err := localesFS.ReadFile("locales/" + lang + ".json")
			if err != nil {
				panic(fmt.Sprintf("ui/i18n: could not read locales/%s.json: %v", lang, err))
			}
			var m map[string]any
			if err := json.Unmarshal(data, &m); err != nil {
				panic(fmt.Sprintf("ui/i18n: could not parse locales/%s.json: %v", lang, err))
			}
			catalogs[lang] = m
		}
	})
	return catalogs
}

// T resolves a dotted key (e.g. "sidebar.topics") in the given language.
// If the key is not found, T returns the raw key as a fallback.
// Simple {{name}} interpolation is supported via alternating key/value args:
//
//	T("en", "topics.entries", "count", "42")
func T(lang, key string, args ...string) string {
	cats := loadCatalogs()
	m := cats[lang]

	val := resolveKey(m, strings.Split(key, "."))
	if val == "" && lang != "en" {
		// Secondary fallback: try English before returning the bare key.
		val = resolveKey(cats["en"], strings.Split(key, "."))
	}
	if val == "" {
		return key
	}

	// Apply {{name}} substitution.  Args are alternating placeholder/value pairs.
	for i := 0; i+1 < len(args); i += 2 {
		val = strings.ReplaceAll(val, "{{"+args[i]+"}}", args[i+1])
	}
	return val
}

// resolveKey walks a nested map[string]any by the path segments.
// Returns "" when any segment is missing or the leaf is not a string.
func resolveKey(m map[string]any, path []string) string {
	if m == nil || len(path) == 0 {
		return ""
	}
	v, ok := m[path[0]]
	if !ok {
		return ""
	}
	if len(path) == 1 {
		s, ok := v.(string)
		if !ok {
			return ""
		}
		return s
	}
	nested, ok := v.(map[string]any)
	if !ok {
		return ""
	}
	return resolveKey(nested, path[1:])
}

// langForRequest resolves the display language from an *http.Request.
// Priority: cookie "lang" → Accept-Language → "en".
func langForRequest(r *http.Request) string {
	if c, err := r.Cookie("lang"); err == nil {
		if supportedLangs[c.Value] {
			return c.Value
		}
	}
	// Parse Accept-Language: pick first matching supported code.
	for _, part := range strings.Split(r.Header.Get("Accept-Language"), ",") {
		tag := strings.TrimSpace(strings.SplitN(part, ";", 2)[0])
		// Match full tag or language prefix (e.g. "es-AR" → "es").
		code := strings.ToLower(strings.SplitN(tag, "-", 2)[0])
		if supportedLangs[code] {
			return code
		}
	}
	return "en"
}

// withLang stores the resolved language in the request context.
func withLang(r *http.Request, lang string) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), langContextKey, lang))
}

// langFromContext retrieves the language set by withLang.
// Falls back to "en" when not present.
func langFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(langContextKey).(string); ok && v != "" {
		return v
	}
	return "en"
}

// themeForRequest resolves the theme from an *http.Request.
// Cookie "theme" ("dark"/"light"), default "dark".
func themeForRequest(r *http.Request) string {
	if c, err := r.Cookie("theme"); err == nil {
		if c.Value == "light" || c.Value == "dark" {
			return c.Value
		}
	}
	return "dark"
}

// LangForRequest is the exported wrapper around langForRequest, for sibling
// modules (e.g. internal/doctor) that render inside the shared app shell and
// need the same cookie/Accept-Language resolution as the ui handlers.
func LangForRequest(r *http.Request) string { return langForRequest(r) }

// ThemeForRequest is the exported wrapper around themeForRequest, for sibling
// modules that render inside the shared app shell.
func ThemeForRequest(r *http.Request) string { return themeForRequest(r) }

// withTheme stores the resolved theme in the request context.
func withTheme(r *http.Request, theme string) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), themeContextKey, theme))
}

// themeFromContext retrieves the theme set by withTheme.
func themeFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(themeContextKey).(string); ok && v != "" {
		return v
	}
	return "dark"
}
