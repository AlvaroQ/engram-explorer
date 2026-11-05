package ui_test

// htmx error feedback (Phase 4, S8): 4xx/5xx responses never replace the
// target (pinned responseHandling), and toasts.js announces them in a live
// region the layout renders with localized fallback messages. The server
// contract is the .inline-error-message span of ErrorPartial, which the toast
// reads; polled widgets opt out with data-no-error-toast.

import (
	"encoding/json"
	"html"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/a-h/templ"

	"github.com/AlvaroQ/engram-explorer/internal/ui"
)

var toastHtmxConfigRe = regexp.MustCompile(`<meta name="htmx-config" content=(?:"([^"]*)"|'([^']*)')`)

func renderLayouts(t *testing.T, lang string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for name, layout := range map[string]func(string, string, string, string, string, templ.Component) templ.Component{
		"Layout": ui.Layout, "LayoutFullBleed": ui.LayoutFullBleed,
	} {
		var b strings.Builder
		if err := layout("t", "overview", lang, "dark", "collapsed", templ.NopComponent).Render(t.Context(), &b); err != nil {
			t.Fatal(err)
		}
		out[name] = b.String()
	}
	return out
}

func TestToasts_HtmxConfigNeverSwapsErrors(t *testing.T) {
	for name, page := range renderLayouts(t, "en") {
		m := toastHtmxConfigRe.FindStringSubmatch(page)
		if m == nil {
			t.Fatalf("%s: no htmx-config meta", name)
		}
		var cfg struct {
			AllowEval        *bool `json:"allowEval"`
			ResponseHandling []struct {
				Code  string `json:"code"`
				Swap  bool   `json:"swap"`
				Error bool   `json:"error"`
			} `json:"responseHandling"`
		}
		if err := json.Unmarshal([]byte(html.UnescapeString(m[1]+m[2])), &cfg); err != nil {
			t.Fatalf("%s: htmx-config is not JSON: %v", name, err)
		}
		if cfg.AllowEval == nil || *cfg.AllowEval {
			t.Errorf("%s: allowEval must stay false", name)
		}
		rules := map[string]string{}
		for _, r := range cfg.ResponseHandling {
			rules[r.Code] = map[bool]string{true: "swap", false: "keep"}[r.Swap] + map[bool]string{true: "+error", false: ""}[r.Error]
		}
		want := map[string]string{"204": "keep", "[23]..": "swap", "[45]..": "keep+error"}
		for code, w := range want {
			if rules[code] != w {
				t.Errorf("%s: responseHandling %s = %q, want %q (all: %v)", name, code, rules[code], w, rules)
			}
		}
	}
}

func TestToasts_LayoutRegionAndScript(t *testing.T) {
	for _, tc := range []struct{ lang, generic, network, unavailable, close string }{
		{"en", "The request failed. Check the server log for details.", "Could not reach the server. Check that it is running.", "This feature is not available right now.", "Dismiss notification"},
		{"es", "La solicitud falló. Revisa el registro del servidor para más detalles.", "No se pudo contactar con el servidor. Comprueba que está en marcha.", "Esta función no está disponible ahora.", "Cerrar aviso"},
	} {
		for name, page := range renderLayouts(t, tc.lang) {
			region := regexp.MustCompile(`<div[^>]*id="toast-region"[^>]*>`).FindString(page)
			for _, want := range []string{
				`aria-live="polite"`, `class="toast-region"`,
				`data-msg-generic="` + tc.generic + `"`,
				`data-msg-network="` + tc.network + `"`,
				`data-msg-unavailable="` + tc.unavailable + `"`,
				`data-label-close="` + tc.close + `"`,
			} {
				if !strings.Contains(region, want) {
					t.Errorf("%s/%s: toast region %q lacks %s", name, tc.lang, region, want)
				}
			}
			if !strings.Contains(page, `<script src="/static/toasts.js" defer></script>`) {
				t.Errorf("%s/%s: layout does not load /static/toasts.js", name, tc.lang)
			}
		}
	}

	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{})
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/static/toasts.js", nil))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "htmx:responseError") || !strings.Contains(w.Body.String(), "htmx:sendError") {
		t.Fatalf("GET /static/toasts.js = %d, want the script handling htmx:responseError and htmx:sendError", w.Code)
	}
}

// TestToasts_ErrorPartialCarriesMessage: the toast reads the message from the
// .inline-error-message span; the visible label stays outside it.
func TestToasts_ErrorPartialCarriesMessage(t *testing.T) {
	var b strings.Builder
	if err := ui.ErrorPartial("Project not found.").Render(t.Context(), &b); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), `<span class="inline-error-message">Project not found.</span>`) {
		t.Errorf("ErrorPartial = %s, want the message in .inline-error-message", b.String())
	}

	// The 503 an htmx request gets for a disabled module is an ErrorPartial,
	// so its toast shows the localized "module disabled" text.
	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{Live: func(*http.Request) ui.LiveHandles { return ui.LiveHandles{} }})
	req := httptest.NewRequest(http.MethodGet, "/observations", nil)
	req.Header.Set("HX-Request", "true")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable || !strings.Contains(w.Body.String(), `<span class="inline-error-message">This module is disabled. Enable it in Settings → Modules.</span>`) {
		t.Errorf("htmx GET of a disabled module = %d %s, want 503 with the message span", w.Code, w.Body.String())
	}
}

// TestToasts_PolledWidgetsOptOut: the status pill polls every 30s and
// reports backend health itself, so its failures raise no toast.
func TestToasts_PolledWidgetsOptOut(t *testing.T) {
	page := renderLayouts(t, "en")["Layout"]
	pill := regexp.MustCompile(`<div[^>]*id="sidebar-status-pill"[^>]*>`).FindString(page)
	if !strings.Contains(pill, "data-no-error-toast") {
		t.Errorf("status pill wrapper %q lacks data-no-error-toast", pill)
	}
}
