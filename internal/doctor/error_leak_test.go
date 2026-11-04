package doctor_test

import (
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/doctor"
)

const genericErrorEN = "Something went wrong. Check the server log for details."

// closedDB returns a SQLite handle that fails every query with
// "sql: database is closed".
func closedDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", fmt.Sprintf("file:%s/closed.db", t.TempDir()))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	db.Close()
	return db
}

func serve(mux *http.ServeMux, req *http.Request) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	return w
}

func assertGeneric(t *testing.T, w *httptest.ResponseRecorder, wantStatus int) {
	t.Helper()
	if w.Code != wantStatus {
		t.Fatalf("status = %d, want %d (unchanged)", w.Code, wantStatus)
	}
	body := w.Body.String()
	for _, leak := range []string{"sql: database is closed", "begin tx", "closed.db"} {
		if strings.Contains(body, leak) {
			t.Errorf("response leaks internal error text %q", leak)
		}
	}
	if !strings.Contains(body, genericErrorEN) {
		t.Errorf("body must contain the generic message; got: %s", body)
	}
}

func TestErrorLeak_OrphansListHidesDBError(t *testing.T) {
	mux := http.NewServeMux()
	doctor.Mount(mux, doctor.Deps{RoDB: closedDB(t)})

	w := serve(mux, httptest.NewRequest(http.MethodGet, "/doctor/orphans/list", nil))
	assertGeneric(t, w, http.StatusOK)
}

func TestErrorLeak_SyncProjectsHidesDBError(t *testing.T) {
	mux := http.NewServeMux()
	doctor.Mount(mux, doctor.Deps{RoDB: closedDB(t)})

	w := serve(mux, httptest.NewRequest(http.MethodGet, "/doctor/sync/projects", nil))
	assertGeneric(t, w, http.StatusOK)
}

// TestErrorLeak_AssignWriteErrorHidden: the read pool works (the list reloads)
// but the write pool is closed, so the inline flash must be generic.
func TestErrorLeak_AssignWriteErrorHidden(t *testing.T) {
	db := openTestDB(t)
	id := seedOrphanObservation(t, db)

	mux := http.NewServeMux()
	doctor.Mount(mux, doctor.Deps{RoDB: db, RWDB: closedDB(t)})

	form := url.Values{"project": {"myproject"}}
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/doctor/orphans/observations/%d/project", id),
		strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	assertGeneric(t, serve(mux, req), http.StatusOK)
}

// TestErrorLeak_DomainErrorsStayVisible: service WriteErrors and handler-built
// input validation messages are user-safe and must not be replaced.
func TestErrorLeak_DomainErrorsStayVisible(t *testing.T) {
	db := openTestDB(t)
	mux := http.NewServeMux()
	doctor.Mount(mux, doctor.Deps{RoDB: db, RWDB: db})

	w := serve(mux, httptest.NewRequest(http.MethodDelete, "/doctor/orphans/observations/999", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if body := w.Body.String(); !strings.Contains(body, "observation 999 not found") {
		t.Errorf("WriteError message must stay visible; got: %s", body)
	}

	w = serve(mux, httptest.NewRequest(http.MethodDelete, "/doctor/orphans/observations/abc", nil))
	body := w.Body.String()
	if w.Code != http.StatusOK || !strings.Contains(body, `invalid id &#34;abc&#34;`) {
		t.Errorf("invalid id message must stay visible (200); got %d: %s", w.Code, body)
	}
	if strings.Contains(body, "strconv") {
		t.Error("invalid id message must not include the parser error detail")
	}

	w = serve(mux, httptest.NewRequest(http.MethodDelete, "/doctor/orphans/bogus/1", nil))
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "unknown entity type") {
		t.Errorf("unknown entity must stay a visible 400; got %d: %s", w.Code, w.Body.String())
	}
}
