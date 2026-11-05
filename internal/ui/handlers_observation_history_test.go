package ui_test

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/ui"
)

const versionsTableSQL = `CREATE TABLE observation_versions (
	id INTEGER PRIMARY KEY AUTOINCREMENT, observation_id INTEGER NOT NULL,
	version INTEGER NOT NULL, title TEXT, content TEXT, created_at TEXT NOT NULL)`

func openHistoryDB(t *testing.T, stmts ...string) *sql.DB {
	t.Helper()
	db := openObservationsTestDB(t)
	stmts = append([]string{`INSERT INTO observations (id, type, title, content, project, created_at, updated_at)
		VALUES (1, 'note', 'Auth v3', 'current', 'proj', '2026-01-03 10:00:00', '2026-01-03 10:00:00')`}, stmts...)
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("seed %q: %v", s, err)
		}
	}
	return db
}

func serveHistory(t *testing.T, db *sql.DB, path, lang string) *httptest.ResponseRecorder {
	t.Helper()
	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{RoDB: db})
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set("HX-Request", "true")
	if lang != "" {
		req.AddCookie(&http.Cookie{Name: "lang", Value: lang})
	}
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	return w
}

// The detail drawer carries a closed "Revision history" disclosure that loads
// its body once, the first time it is opened (no trigger filter: htmx
// allowEval is false).
func TestObservationDetail_HistoryDisclosureIsLazy(t *testing.T) {
	db := openHistoryDB(t)
	w := serveHistory(t, db, "/observations/1", "")
	body := w.Body.String()
	if w.Code != http.StatusOK {
		t.Fatalf("status %d", w.Code)
	}
	for _, want := range []string{
		`<details id="obs-history"`,
		`hx-get="/observations/1/history"`,
		`hx-trigger="toggle once"`,
		`hx-target="#obs-history-body"`,
		`Revision history</summary>`,
		`id="obs-history-body"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("detail missing %q; body: %.2000s", want, body)
		}
	}
	if strings.Contains(body, `<details id="obs-history" open`) {
		t.Error("history must start closed")
	}
}

func TestObservationHistory_ListsVersionsNewestFirstEscaped(t *testing.T) {
	db := openHistoryDB(t, versionsTableSQL,
		`INSERT INTO observation_versions (observation_id, version, title, content, created_at) VALUES
			(1, 1, 'Auth v1', '<script>alert(1)</script>', '2026-01-01 09:05:00'),
			(1, 2, 'Auth v2', NULL, '2026-01-02 10:30:00'),
			(2, 1, 'Other memory', 'x', '2026-01-02 11:00:00')`)
	w := serveHistory(t, db, "/observations/1/history", "")
	body := w.Body.String()
	if w.Code != http.StatusOK {
		t.Fatalf("status %d body %s", w.Code, body)
	}
	i2, i1 := strings.Index(body, "Version 2"), strings.Index(body, "Version 1")
	if i2 < 0 || i1 < 0 || i2 > i1 {
		t.Errorf("want Version 2 before Version 1; body: %s", body)
	}
	for _, want := range []string{"Auth v2", "Auth v1", "02-01-2026 10:30", "01-01-2026 09:05", "&lt;script&gt;alert(1)&lt;/script&gt;"} {
		if !strings.Contains(body, want) {
			t.Errorf("history missing %q; body: %s", want, body)
		}
	}
	if strings.Contains(body, "<script>") || strings.Contains(body, "Other memory") {
		t.Errorf("content must be escaped and only this memory's versions shown; body: %s", body)
	}
}

func TestObservationHistory_States(t *testing.T) {
	cases := []struct {
		name, lang, want string
		stmts            []string
	}{
		{"no table", "", "History is not available for this database", nil},
		{"no table es", "es", "El historial no está disponible en esta base de datos", nil},
		{"no versions", "", "No earlier versions", []string{versionsTableSQL}},
		{"no versions es", "es", "No hay versiones anteriores", []string{versionsTableSQL}},
	}
	for _, tc := range cases {
		w := serveHistory(t, openHistoryDB(t, tc.stmts...), "/observations/1/history", tc.lang)
		if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), tc.want) {
			t.Errorf("%s: status %d body %s, want %q", tc.name, w.Code, w.Body.String(), tc.want)
		}
	}
}

func TestObservationHistory_Errors(t *testing.T) {
	db := openHistoryDB(t)
	if w := serveHistory(t, db, "/observations/999/history", ""); w.Code != http.StatusNotFound {
		t.Errorf("unknown memory: status %d, want 404", w.Code)
	}
	w := serveHistory(t, db, "/observations/abc/history", "")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `class="inline-error"`) {
		t.Errorf("bad id: status %d body %s, want 200 inline error", w.Code, w.Body.String())
	}

	// A history table the query cannot read is reported inline, localized,
	// without the database error text.
	broken := openHistoryDB(t, `CREATE TABLE observation_versions (id INTEGER PRIMARY KEY, observation_id INTEGER)`)
	w = serveHistory(t, broken, "/observations/1/history", "es")
	body := w.Body.String()
	if w.Code != http.StatusOK || !strings.Contains(body, `class="inline-error"`) || !strings.Contains(body, "No se pudo cargar el historial") {
		t.Errorf("query error: status %d body %s, want 200 localized inline error", w.Code, body)
	}
	if strings.Contains(body, "no such column") {
		t.Errorf("query error leaked the database message: %s", body)
	}
}

// Long histories render the 50 newest versions and say how many exist.
func TestObservationHistory_CapsAtFifty(t *testing.T) {
	stmts := []string{versionsTableSQL}
	for v := 1; v <= 55; v++ {
		stmts = append(stmts, `INSERT INTO observation_versions (observation_id, version, title, content, created_at)
			VALUES (1, `+strconv.Itoa(v)+`, 'T', 'c', '2026-01-01 09:00:00')`)
	}
	body := serveHistory(t, openHistoryDB(t, stmts...), "/observations/1/history", "").Body.String()
	if n := strings.Count(body, "<li"); n != 50 {
		t.Errorf("rendered %d versions, want 50", n)
	}
	if !strings.Contains(body, "Showing the 50 most recent of 55 versions.") || !strings.Contains(body, "Version 55") || strings.Contains(body, "Version 5<") {
		t.Errorf("want the 50 newest (55..6) and the cap note; body: %.600s", body)
	}
}
