package ui_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	engramprovider "github.com/AlvaroQ/engram-explorer/internal/providers/engram"
	"github.com/AlvaroQ/engram-explorer/internal/ui"
	"github.com/AlvaroQ/engram-explorer/internal/usererr"
)

const (
	genericErrorEN = "Something went wrong. Check the server log for details."
	genericErrorES = "Algo salió mal. Revisa el registro del servidor para más detalles."
)

// assertNoLeak fails when body contains any of the internal error fragments.
func assertNoLeak(t *testing.T, body string, fragments ...string) {
	t.Helper()
	for _, f := range fragments {
		if strings.Contains(body, f) {
			t.Errorf("response leaks internal error text %q", f)
		}
	}
}

// TestErrorLeak_ListPartialHidesDBError forces a real SQLite error (the
// sessions table is missing) on an HTMX list partial: the body must show the
// generic message, not the driver text, and keep status 200.
func TestErrorLeak_ListPartialHidesDBError(t *testing.T) {
	db := openTestDB(t) // observations only — no sessions table

	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{RoDB: db})

	req := httptest.NewRequest(http.MethodGet, "/sessions", nil)
	req.Header.Set("HX-Request", "true")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (unchanged)", w.Code)
	}
	body := w.Body.String()
	assertNoLeak(t, body, "no such table", "SQL logic error")
	if !strings.Contains(body, genericErrorEN) {
		t.Errorf("body must contain the generic message; got: %s", body)
	}
}

// TestErrorLeak_GenericMessageIsLocalized checks the Spanish catalog entry.
func TestErrorLeak_GenericMessageIsLocalized(t *testing.T) {
	db := openTestDB(t)

	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{RoDB: db})

	req := httptest.NewRequest(http.MethodGet, "/sessions", nil)
	req.Header.Set("HX-Request", "true")
	req.AddCookie(&http.Cookie{Name: "lang", Value: "es"})
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	body := w.Body.String()
	assertNoLeak(t, body, "no such table")
	if !strings.Contains(body, genericErrorES) {
		t.Errorf("body must contain the Spanish generic message; got: %s", body)
	}
}

// TestErrorLeak_SettingsEngramDBHidesInternalError covers a settings flow whose
// callback fails with an unexpected (non-user-safe) error.
func TestErrorLeak_SettingsEngramDBHidesInternalError(t *testing.T) {
	db := openTestDB(t)

	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{
		RoDB: db,
		ReloadEngramDB: func(string) error {
			return errors.New("open /private/secret/engram.db: sql: database is closed")
		},
	})

	w := postForm(t, mux, "/settings/engram-db", "/private/secret/engram.db")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (re-render, unchanged)", w.Code)
	}
	body := w.Body.String()
	assertNoLeak(t, body, "sql: database is closed", "open /private/secret")
	if !strings.Contains(body, genericErrorEN) {
		t.Errorf("body must contain the generic message; got: %s", body)
	}
}

// TestErrorLeak_SettingsEngramDBShowsValidatorMessage runs the real Engram
// validator: its rejection is a user-safe message and must stay verbatim.
func TestErrorLeak_SettingsEngramDBShowsValidatorMessage(t *testing.T) {
	db := openTestDB(t)
	notSQLite := filepath.Join(t.TempDir(), "notes.db")
	if err := os.WriteFile(notSQLite, []byte("plain text, not sqlite"), 0o600); err != nil {
		t.Fatal(err)
	}

	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{RoDB: db, ReloadEngramDB: engramprovider.ValidateDBFile})

	w := postForm(t, mux, "/settings/engram-db", notSQLite)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "not an SQLite database: "+notSQLite) {
		t.Errorf("validator message must be shown verbatim; got: %s", body)
	}
	if strings.Contains(body, genericErrorEN) {
		t.Error("a user-safe message must not be replaced by the generic one")
	}
}

// TestErrorLeak_SettingsShowsUserErrVerbatim pins the usererr contract for the
// Settings engram-db rejection text.
func TestErrorLeak_SettingsShowsUserErrVerbatim(t *testing.T) {
	db := openTestDB(t)
	const msg = "not an Engram database (missing observations or sessions tables): /tmp/x.db"

	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{
		RoDB:           db,
		ReloadEngramDB: func(string) error { return usererr.New(msg) },
	})

	w := postForm(t, mux, "/settings/engram-db", "/tmp/x.db")
	if !strings.Contains(w.Body.String(), msg) {
		t.Errorf("usererr message must be shown verbatim; got: %s", w.Body.String())
	}
}

// TestErrorLeak_AdvancedViewHTTPErrorIsGeneric covers the plain http.Error
// path: status 500 is kept, the body is the generic message.
func TestErrorLeak_AdvancedViewHTTPErrorIsGeneric(t *testing.T) {
	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{
		SetAdvancedView: func(bool) error {
			return errors.New("write /home/me/.config/engram-explorer/config.json: permission denied")
		},
	})

	form := url.Values{"enabled": {"true"}}
	req := httptest.NewRequest(http.MethodPost, "/settings/advanced-view", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 (unchanged)", w.Code)
	}
	body := w.Body.String()
	assertNoLeak(t, body, "permission denied", "/home/me")
	if !strings.Contains(body, genericErrorEN) {
		t.Errorf("body must contain the generic message; got: %s", body)
	}
}

// TestErrorLeak_AccountsHidesInternalError covers the accounts flash.
func TestErrorLeak_AccountsHidesInternalError(t *testing.T) {
	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{
		Profiles:      func() []ui.ProfileInfo { return nil },
		ActiveProfile: func() string { return "default" },
		SwitchProfile: func(string) error { return nil },
		CreateProfile: func(string) error {
			return errors.New("rename /home/me/.config/profiles.json.tmp: no space left on device")
		},
		DeleteProfile: func(string) error { return nil },
	})

	form := url.Values{"name": {"work"}}
	req := httptest.NewRequest(http.MethodPost, "/settings/profile/create", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (unchanged)", w.Code)
	}
	body := w.Body.String()
	assertNoLeak(t, body, "no space left on device", "/home/me")
	if !strings.Contains(body, genericErrorEN) {
		t.Errorf("body must contain the generic message; got: %s", body)
	}
}
