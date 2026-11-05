package ui_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/ui"
)

func esRequest(method, target string, form url.Values) *http.Request {
	var req *http.Request
	if form != nil {
		req = httptest.NewRequest(method, target, strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	} else {
		req = httptest.NewRequest(method, target, nil)
	}
	req.Header.Set("HX-Request", "true")
	req.AddCookie(&http.Cookie{Name: "lang", Value: "es"})
	return req
}

// TestUIErrors_Spanish: inline errors built in Go follow the lang cookie.
func TestUIErrors_Spanish(t *testing.T) {
	db := openTestDB(t)
	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{RoDB: db})

	cases := []struct {
		name string
		req  *http.Request
		want string
	}{
		{"invalid observation id", esRequest(http.MethodGet, "/observations/abc", nil), "Id de observación no válido: abc"},
		{"read-only write", esRequest(http.MethodPost, "/projects/alpha/merge", url.Values{"target": {"beta"}}),
			"Las operaciones de escritura no están disponibles en modo de solo lectura."},
	}
	for _, tc := range cases {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, tc.req)
		if !strings.Contains(w.Body.String(), `<span class="inline-error-message">`+tc.want+`</span>`) {
			t.Errorf("%s: body %q lacks %q", tc.name, w.Body.String(), tc.want)
		}
	}
}

// TestMergeConfirm_LocalizedTemplate: the combine form carries the localized
// confirm text with {source}/{target} placeholders for ui-actions.js.
func TestMergeConfirm_LocalizedTemplate(t *testing.T) {
	mux := openMergeDB(t)
	for lang, want := range map[string]string{
		"en": `data-merge-confirm-template="Combine &#34;{source}&#34; into &#34;{target}&#34;? This moves all its memories and cannot be undone."`,
		"es": `data-merge-confirm-template="¿Combinar «{source}» en «{target}»? Esto mueve todas sus memorias y no se puede deshacer."`,
	} {
		req := httptest.NewRequest(http.MethodGet, "/projects/alpha", nil)
		req.AddCookie(&http.Cookie{Name: "lang", Value: lang})
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if !strings.Contains(w.Body.String(), want) {
			t.Errorf("%s: project page lacks %s", lang, want)
		}
	}
}
