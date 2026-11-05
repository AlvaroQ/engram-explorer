package doctor_test

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/doctor"
)

// seedOldSession inserts a session started long before the grace window.
func seedOldSession(t *testing.T, db *sql.DB, id, project string) {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO sessions (id, project, directory, started_at) VALUES (?, ?, '/tmp', '2026-01-01 10:00:00')`, id, project); err != nil {
		t.Fatalf("seed session %s: %v", id, err)
	}
}

func countSessions(t *testing.T, db *sql.DB) int {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sessions`).Scan(&n); err != nil {
		t.Fatalf("count sessions: %v", err)
	}
	return n
}

func doctorRequest(t *testing.T, mux http.Handler, method, path, lang string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	req.Header.Set("HX-Request", "true")
	if lang != "" {
		req.AddCookie(&http.Cookie{Name: "lang", Value: lang})
	}
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	return w
}

// TestMaintenancePage_EmptySessionsSection: the page defers the section.
func TestMaintenancePage_EmptySessionsSection(t *testing.T) {
	mux := http.NewServeMux()
	doctor.Mount(mux, doctor.Deps{})
	body := doctorRequest(t, mux, http.MethodGet, "/settings/maintenance", "en").Body.String()
	for _, want := range []string{`id="empty-sessions"`, `hx-get="/doctor/maintenance/empty-sessions"`, "Empty sessions"} {
		if !strings.Contains(body, want) {
			t.Errorf("maintenance page missing %q", want)
		}
	}
}

// TestEmptySessionsPartial_ListsAndOffersDelete (en and es).
func TestEmptySessionsPartial_ListsAndOffersDelete(t *testing.T) {
	db := openTestDB(t)
	seedOldSession(t, db, "sess-empty-a", "repo")
	seedOldSession(t, db, "sess-empty-b", "repo")
	mux := http.NewServeMux()
	doctor.Mount(mux, doctor.Deps{RoDB: db, RWDB: db})

	cases := map[string][]string{
		"en": {"2 empty sessions", "Delete all 2 empty sessions", `hx-confirm="Delete 2 empty sessions?`},
		"es": {"2 sesiones vacías", "Eliminar las 2 sesiones vacías", `hx-confirm="¿Eliminar 2 sesiones vacías?`},
	}
	for lang, wants := range cases {
		w := doctorRequest(t, mux, http.MethodGet, "/doctor/maintenance/empty-sessions", lang)
		body := w.Body.String()
		if w.Code != http.StatusOK {
			t.Fatalf("lang=%s status %d", lang, w.Code)
		}
		for _, want := range append(wants, "sess-empty-a", "sess-empty-b", `hx-post="/doctor/maintenance/empty-sessions/delete"`) {
			if !strings.Contains(body, want) {
				t.Errorf("lang=%s: body missing %q", lang, want)
			}
		}
	}
}

// TestEmptySessionsPartial_None renders the empty state and no button.
func TestEmptySessionsPartial_None(t *testing.T) {
	db := openTestDB(t)
	mux := http.NewServeMux()
	doctor.Mount(mux, doctor.Deps{RoDB: db, RWDB: db})
	body := doctorRequest(t, mux, http.MethodGet, "/doctor/maintenance/empty-sessions", "en").Body.String()
	if !strings.Contains(body, "No empty sessions found.") || strings.Contains(body, "hx-post") {
		t.Errorf("want empty state without delete button, got %s", body)
	}
}

// TestEmptySessionsPartial_ReadOnly hides the button, and the POST is blocked
// without touching the data.
func TestEmptySessionsPartial_ReadOnly(t *testing.T) {
	db := openTestDB(t)
	seedOldSession(t, db, "sess-empty", "repo")
	mux := http.NewServeMux()
	doctor.Mount(mux, doctor.Deps{RoDB: db}) // RWDB nil = read-only mode

	body := doctorRequest(t, mux, http.MethodGet, "/doctor/maintenance/empty-sessions", "en").Body.String()
	if strings.Contains(body, "hx-post") {
		t.Error("read-only mode must not offer the delete button")
	}
	if !strings.Contains(body, "Read-only mode: deleting is disabled.") {
		t.Error("read-only mode must explain why there is no delete button")
	}

	w := doctorRequest(t, mux, http.MethodPost, "/doctor/maintenance/empty-sessions/delete", "en")
	if !strings.Contains(w.Body.String(), "inline-error") {
		t.Errorf("read-only POST must render the read-only error, got %d %s", w.Code, w.Body.String())
	}
	if n := countSessions(t, db); n != 1 {
		t.Errorf("read-only POST changed data: %d sessions, want 1", n)
	}
}

// TestEmptySessionsDelete_DeletesAndRerenders: only empty sessions go, the
// enrolled one stays, and the refreshed section reports the deleted count.
func TestEmptySessionsDelete_DeletesAndRerenders(t *testing.T) {
	db := openTestDB(t)
	seedOldSession(t, db, "sess-empty", "repo")
	seedOldSession(t, db, "sess-enrolled", "synced")
	seedOldSession(t, db, "sess-busy", "repo")
	seedEnrolledProject(t, db, "synced")
	if _, err := db.Exec(`INSERT INTO user_prompts (session_id, content, project) VALUES ('sess-busy', 'hi', 'repo')`); err != nil {
		t.Fatalf("seed prompt: %v", err)
	}
	mux := http.NewServeMux()
	doctor.Mount(mux, doctor.Deps{RoDB: db, RWDB: db})

	for lang, want := range map[string]string{"en": "Deleted 1 empty session.", "es": "Se eliminó 1 sesión vacía."} {
		if lang == "es" {
			seedOldSession(t, db, "sess-empty", "repo")
		}
		w := doctorRequest(t, mux, http.MethodPost, "/doctor/maintenance/empty-sessions/delete", lang)
		body := w.Body.String()
		if w.Code != http.StatusOK || !strings.Contains(body, want) {
			t.Errorf("lang=%s: status %d, want %q in %s", lang, w.Code, want, body)
		}
		if strings.Contains(body, "sess-empty") {
			t.Errorf("lang=%s: refreshed section still lists the deleted session", lang)
		}
		if n := countSessions(t, db); n != 2 {
			t.Errorf("lang=%s: %d sessions left, want 2 (enrolled + busy)", lang, n)
		}
	}
	body := doctorRequest(t, mux, http.MethodGet, "/doctor/maintenance/empty-sessions", "en").Body.String()
	if !strings.Contains(body, "Kept because the project is cloud-enrolled: 1.") {
		t.Errorf("section must report the enrolled empty session as kept, got %s", body)
	}
}
