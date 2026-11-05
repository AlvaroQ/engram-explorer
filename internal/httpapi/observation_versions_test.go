package httpapi_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/httpapi"
)

type versionsResp struct {
	Available *bool `json:"available"`
	Versions  []struct {
		Version   int64   `json:"version"`
		Title     *string `json:"title"`
		Content   *string `json:"content"`
		CreatedAt *string `json:"created_at"`
	} `json:"versions"`
}

func TestObservationVersionsAPI_WithTable(t *testing.T) {
	c := newWriteContainer(t)
	execOnDB(t, c.Config.EngramDbPath,
		`CREATE TABLE observation_versions (id INTEGER PRIMARY KEY AUTOINCREMENT, observation_id INTEGER NOT NULL,
			version INTEGER NOT NULL, title TEXT, content TEXT, created_at TEXT NOT NULL)`,
		`INSERT INTO observation_versions (observation_id, version, title, content, created_at) VALUES
			(1, 1, 'Old title', 'old body', '2026-01-01 09:00:00'),
			(1, 2, 'Newer title', 'newer body', '2026-01-01 10:00:00')`)
	rec := serveReq(httpapi.NewServeMux(c), http.MethodGet, "/api/observations/1/versions")
	var got versionsResp
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &got) != nil {
		t.Fatalf("status %d body %s, want 200 JSON", rec.Code, rec.Body.String())
	}
	if got.Available == nil || !*got.Available || len(got.Versions) != 2 ||
		got.Versions[0].Version != 2 || *got.Versions[0].Title != "Newer title" ||
		*got.Versions[1].Content != "old body" || *got.Versions[1].CreatedAt != "2026-01-01 09:00:00" {
		t.Errorf("got %s, want available, versions 2 then 1", rec.Body.String())
	}
}

func TestObservationVersionsAPI_WithoutTable(t *testing.T) {
	c := newWriteContainer(t)
	rec := serveReq(httpapi.NewServeMux(c), http.MethodGet, "/api/observations/1/versions")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s, want 200", rec.Code, rec.Body.String())
	}
	if body := rec.Body.String(); body != `{"available":false,"versions":[]}`+"\n" {
		t.Errorf("body %q, want available:false with an empty list", body)
	}
}

func TestObservationVersionsAPI_Errors(t *testing.T) {
	handler := httpapi.NewServeMux(newWriteContainer(t))
	for path, want := range map[string]int{
		"/api/observations/999/versions": http.StatusNotFound,
		"/api/observations/abc/versions": http.StatusBadRequest,
	} {
		if rec := serveReq(handler, http.MethodGet, path); rec.Code != want {
			t.Errorf("%s: status %d body %s, want %d", path, rec.Code, rec.Body.String(), want)
		}
	}
}

// The detail route keeps answering next to the new sub-route.
func TestObservationVersionsAPI_DetailRouteUnchanged(t *testing.T) {
	rec := serveReq(httpapi.NewServeMux(newWriteContainer(t)), http.MethodGet, "/api/observations/1")
	if rec.Code != http.StatusOK {
		t.Errorf("GET /api/observations/1: status %d, want 200", rec.Code)
	}
}
