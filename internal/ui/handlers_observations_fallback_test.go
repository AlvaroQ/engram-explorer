package ui_test

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/ui"
)

// matchAnyNoticeID marks the OR-fallback notice (locale key
// observations.search.matchedAny). The
// assertions use the stable element id, so they hold whether or not the
// locale catalog already carries the translation.
const matchAnyNoticeID = `id="obs-match-any-notice"`

func openSearchableObservationsDB(t *testing.T) *sql.DB {
	t.Helper()
	db := openObservationsTestDB(t)
	for _, s := range []string{
		`CREATE VIRTUAL TABLE observations_fts USING fts5(
			title, content, tool_name, type, project, topic_key,
			content='observations', content_rowid='id')`,
		`INSERT INTO observations (id, type, title, content, project, created_at, updated_at) VALUES
			(1, 'bugfix', 'Auth bug in login', 'token expired', 'proj', '2026-02-01 10:00:00', '2026-02-01 10:00:00'),
			(2, 'bugfix', 'Request timeout', 'gateway slow', 'proj', '2026-02-02 10:00:00', '2026-02-02 10:00:00')`,
		`INSERT INTO observations_fts(observations_fts) VALUES ('rebuild')`,
	} {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	return db
}

func getObservationsList(t *testing.T, db *sql.DB, query string) string {
	t.Helper()
	mux := http.NewServeMux()
	ui.Mount(mux, ui.Deps{RoDB: db, AdvancedView: func() bool { return false }})
	req := httptest.NewRequest(http.MethodGet, "/observations/list?"+query, nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GET /observations/list?%s: status %d", query, w.Code)
	}
	return w.Body.String()
}

func TestObservationsList_ORFallbackShowsNotice(t *testing.T) {
	db := openSearchableObservationsDB(t)
	body := getObservationsList(t, db, "q=auth+timeout")
	if !strings.Contains(body, matchAnyNoticeID) {
		t.Errorf("fallback notice missing; body: %.600s", body)
	}
	if !strings.Contains(body, "Showing memories that match any of the words.") {
		t.Errorf("notice must render the observations.search.matchedAny copy; body: %.600s", body)
	}
	if !strings.Contains(body, "Auth bug in login") || !strings.Contains(body, "Request timeout") {
		t.Errorf("both any-word rows must render; body: %.600s", body)
	}
}

func TestObservationsList_ExactMatchHasNoNotice(t *testing.T) {
	db := openSearchableObservationsDB(t)
	body := getObservationsList(t, db, "q=auth+bug")
	if strings.Contains(body, matchAnyNoticeID) {
		t.Error("exact match must not show the fallback notice")
	}
	if !strings.Contains(body, "Auth bug in login") || strings.Contains(body, "Request timeout") {
		t.Errorf("exact match must render only the matching row; body: %.600s", body)
	}
}

// The load-more URL of a fallback page keeps match=any so later pages stay in
// OR mode.
func TestObservationsList_ORFallbackLoadMoreKeepsMatchAny(t *testing.T) {
	db := openSearchableObservationsDB(t)
	body := getObservationsList(t, db, "q=auth+timeout&limit=1")
	if !strings.Contains(body, "match=any") {
		t.Errorf("load-more URL must carry match=any; body: %.800s", body)
	}
	exact := getObservationsList(t, db, "q=auth&limit=1")
	if strings.Contains(exact, "match=any") {
		t.Error("exact-match load-more URL must not carry match=any")
	}
}
