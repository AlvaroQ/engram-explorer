package httpapi_test

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/httpapi"
)

// execOnDB runs statements on the database file behind a container.
func execOnDB(t *testing.T, path string, stmts ...string) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("exec %q: %v", s, err)
		}
	}
}

const seedOldEmptySession = `INSERT INTO sessions (id, project, directory, started_at) VALUES ('sess-empty', 'myproject', '/tmp', '2026-01-01 09:00:00')`

func serveReq(handler http.Handler, method, path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
	return rec
}

// TestMaintenanceAPI_EmptySessions_ListAndDelete: GET reports the empty
// session, POST deletes it (and only it), a second GET reports none.
func TestMaintenanceAPI_EmptySessions_ListAndDelete(t *testing.T) {
	c := newWriteContainer(t)
	execOnDB(t, c.Config.EngramDbPath, seedOldEmptySession)
	handler := httpapi.NewServeMux(c)

	rec := serveReq(handler, http.MethodGet, "/api/maintenance/empty-sessions")
	var rep struct {
		Total    int `json:"total"`
		Sessions []struct {
			ID string `json:"id"`
		} `json:"sessions"`
	}
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &rep) != nil {
		t.Fatalf("GET: status %d body %s", rec.Code, rec.Body.String())
	}
	if rep.Total != 1 || len(rep.Sessions) != 1 || rep.Sessions[0].ID != "sess-empty" {
		t.Fatalf("GET: got %+v, want only sess-empty", rep)
	}

	rec = serveReq(handler, http.MethodPost, "/api/maintenance/empty-sessions/delete")
	var del struct {
		Deleted int64 `json:"deleted"`
	}
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &del) != nil || del.Deleted != 1 {
		t.Fatalf("POST: status %d body %s, want 200 deleted=1", rec.Code, rec.Body.String())
	}

	rec = serveReq(handler, http.MethodGet, "/api/maintenance/empty-sessions")
	if err := json.Unmarshal(rec.Body.Bytes(), &rep); err != nil || rep.Total != 0 {
		t.Errorf("GET after delete: total %d err %v, want 0", rep.Total, err)
	}
	// The seeded non-empty session survives.
	rec = serveReq(handler, http.MethodGet, "/api/sessions/sess-1")
	if rec.Code != http.StatusOK {
		t.Errorf("sess-1 must survive the bulk delete, GET status %d", rec.Code)
	}
}

// TestMaintenanceAPI_EmptySessions_ReadOnly: GET works, POST answers 503
// RW_UNAVAILABLE and leaves the database unchanged.
func TestMaintenanceAPI_EmptySessions_ReadOnly(t *testing.T) {
	c, path := newReadOnlyContainer(t)
	execOnDB(t, path, seedOldEmptySession)
	handler := httpapi.NewServeMux(c)
	before := dbDigest(t, path)

	if rec := serveReq(handler, http.MethodGet, "/api/maintenance/empty-sessions"); rec.Code != http.StatusOK {
		t.Errorf("read-only GET: status %d, want 200", rec.Code)
	}
	rec := serveReq(handler, http.MethodPost, "/api/maintenance/empty-sessions/delete")
	if rec.Code != http.StatusServiceUnavailable || errorCode(rec) != "RW_UNAVAILABLE" {
		t.Errorf("read-only POST: status %d body %s, want 503 RW_UNAVAILABLE", rec.Code, rec.Body.String())
	}
	if after := dbDigest(t, path); after != before {
		t.Errorf("read-only POST changed the database:\nbefore %s\nafter  %s", before, after)
	}
}

// TestMaintenanceAPI_RegistryPathMounted: the registry-backed server (the
// production path) serves the maintenance routes too.
func TestMaintenanceAPI_RegistryPathMounted(t *testing.T) {
	handler := httpapi.NewServeMux(newTestRegistry(t))
	if rec := serveReq(handler, http.MethodGet, "/api/maintenance/empty-sessions"); rec.Code != http.StatusOK {
		t.Errorf("registry GET: status %d body %s, want 200", rec.Code, rec.Body.String())
	}
}
