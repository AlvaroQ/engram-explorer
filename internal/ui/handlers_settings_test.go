package ui_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/ui"
)

// ---------------------------------------------------------------------------
// GET /settings
// ---------------------------------------------------------------------------

// TestSettingsFullPage verifies that GET /settings without HX-Request
// returns a full HTML page (200, text/html, contains <html>).
func TestSettingsFullPage(t *testing.T) {
	db := openTestDB(t)

	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{RoDB: db})

	req := httptest.NewRequest(http.MethodGet, "/settings", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}
	ct := w.Header().Get("Content-Type")
	if !strings.HasPrefix(ct, "text/html") {
		t.Errorf("expected text/html content-type, got %q", ct)
	}
	body := w.Body.String()
	if !strings.Contains(body, "<html") {
		t.Error("full page must contain <html>")
	}
	if !strings.Contains(body, "<head") {
		t.Error("full page must contain <head>")
	}
}

// TestSettingsHTMXPartial verifies GET /settings with HX-Request: true
// returns a fragment without a full HTML shell.
func TestSettingsHTMXPartial(t *testing.T) {
	db := openTestDB(t)

	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{RoDB: db})

	req := httptest.NewRequest(http.MethodGet, "/settings", nil)
	req.Header.Set("HX-Request", "true")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if strings.Contains(body, "<html") {
		t.Error("HTMX partial must NOT contain <html>")
	}
}

// TestSettingsRoDBNil verifies that when RoDB is nil the settings handler
// renders without panicking.
func TestSettingsRoDBNil(t *testing.T) {
	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{RoDB: nil})

	req := httptest.NewRequest(http.MethodGet, "/settings", nil)
	w := httptest.NewRecorder()

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("handler panicked: %v", r)
		}
	}()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 with nil RoDB, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// POST /theme
// ---------------------------------------------------------------------------

// TestThemePostDark verifies POST /theme with value=dark sets the theme
// cookie and returns a redirect to /settings.
func TestThemePostDark(t *testing.T) {
	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{})

	form := url.Values{"value": {"dark"}}
	req := httptest.NewRequest(http.MethodPost, "/theme", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d", w.Code)
	}
	if got := w.Header().Get("Location"); got != "/settings" {
		t.Errorf("expected redirect to /settings, got %q", got)
	}

	found := false
	for _, c := range w.Result().Cookies() {
		if c.Name == "theme" && c.Value == "dark" {
			found = true
		}
	}
	if !found {
		t.Error("expected theme=dark cookie to be set")
	}
}

// TestThemePostLight verifies POST /theme with value=light sets the cookie.
func TestThemePostLight(t *testing.T) {
	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{})

	form := url.Values{"value": {"light"}}
	req := httptest.NewRequest(http.MethodPost, "/theme", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d", w.Code)
	}

	found := false
	for _, c := range w.Result().Cookies() {
		if c.Name == "theme" && c.Value == "light" {
			found = true
		}
	}
	if !found {
		t.Error("expected theme=light cookie to be set")
	}
}

// TestThemePostHTMX verifies POST /theme with HX-Request returns HX-Redirect
// instead of a 303 Location.
func TestThemePostHTMX(t *testing.T) {
	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{})

	form := url.Values{"value": {"dark"}}
	req := httptest.NewRequest(http.MethodPost, "/theme", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if got := w.Header().Get("HX-Redirect"); got != "/settings" {
		t.Errorf("expected HX-Redirect: /settings, got %q", got)
	}
}

// TestThemePostInvalid verifies POST /theme with an unknown value returns 400.
func TestThemePostInvalid(t *testing.T) {
	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{})

	form := url.Values{"value": {"blue"}}
	req := httptest.NewRequest(http.MethodPost, "/theme", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// POST /lang
// ---------------------------------------------------------------------------

// TestLangPostES verifies POST /lang with value=es sets the lang cookie.
func TestLangPostES(t *testing.T) {
	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{})

	form := url.Values{"value": {"es"}}
	req := httptest.NewRequest(http.MethodPost, "/lang", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d", w.Code)
	}
	if got := w.Header().Get("Location"); got != "/settings" {
		t.Errorf("expected redirect to /settings, got %q", got)
	}

	found := false
	for _, c := range w.Result().Cookies() {
		if c.Name == "lang" && c.Value == "es" {
			found = true
		}
	}
	if !found {
		t.Error("expected lang=es cookie to be set")
	}
}

// TestLangPostHTMX verifies POST /lang with HX-Request returns HX-Redirect.
func TestLangPostHTMX(t *testing.T) {
	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{})

	form := url.Values{"value": {"en"}}
	req := httptest.NewRequest(http.MethodPost, "/lang", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if got := w.Header().Get("HX-Redirect"); got != "/settings" {
		t.Errorf("expected HX-Redirect: /settings, got %q", got)
	}
}

// TestLangPostInvalid verifies POST /lang with an unsupported lang returns 400.
func TestLangPostInvalid(t *testing.T) {
	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{})

	form := url.Values{"value": {"fr"}}
	req := httptest.NewRequest(http.MethodPost, "/lang", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}
