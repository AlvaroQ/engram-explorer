package services_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/services"
)

// createVersionsTable adds the upstream Engram observation_versions table:
// one row per earlier title/content of an observation.
func createVersionsTable(t *testing.T, db *sql.DB) {
	t.Helper()
	if _, err := db.Exec(`CREATE TABLE observation_versions (
		id INTEGER PRIMARY KEY AUTOINCREMENT, observation_id INTEGER NOT NULL,
		version INTEGER NOT NULL, title TEXT, content TEXT, created_at TEXT NOT NULL)`); err != nil {
		t.Fatalf("create observation_versions: %v", err)
	}
}

func TestObservationVersions_NewestFirst(t *testing.T) {
	db := newSearchDB(t, tokenizeDefault)
	createVersionsTable(t, db)
	id := addObs(t, db, "Auth v3", "current", "arch/auth")
	other := addObs(t, db, "Other", "x", "")
	if _, err := db.Exec(`INSERT INTO observation_versions (observation_id, version, title, content, created_at) VALUES
		(?, 1, 'Auth v1', 'first', '2026-01-01 10:00:00'),
		(?, 2, 'Auth v2', NULL, '2026-01-02 10:00:00'),
		(?, 1, 'Other v1', 'y', '2026-01-03 10:00:00')`, id, id, other); err != nil {
		t.Fatalf("seed versions: %v", err)
	}

	res, found, err := services.ObservationVersions(context.Background(), db, id)
	if err != nil || !found {
		t.Fatalf("got found=%v err=%v, want found", found, err)
	}
	if !res.Available || len(res.Versions) != 2 {
		t.Fatalf("got %+v, want available with 2 versions", res)
	}
	v2, v1 := res.Versions[0], res.Versions[1]
	if v2.Version != 2 || *v2.Title != "Auth v2" || v2.Content != nil || *v2.CreatedAt != "2026-01-02 10:00:00" {
		t.Errorf("newest: got %+v", v2)
	}
	if v1.Version != 1 || *v1.Title != "Auth v1" || *v1.Content != "first" {
		t.Errorf("oldest: got %+v", v1)
	}
}

func TestObservationVersions_NoVersionsIsEmptyList(t *testing.T) {
	db := newSearchDB(t, tokenizeDefault)
	createVersionsTable(t, db)
	id := addObs(t, db, "Fresh", "x", "")
	res, found, err := services.ObservationVersions(context.Background(), db, id)
	if err != nil || !found || !res.Available || res.Versions == nil || len(res.Versions) != 0 {
		t.Errorf("got %+v found=%v err=%v, want available with an empty, non-nil list", res, found, err)
	}
}

// Older Engram schemas have no observation_versions table: that is
// "history unavailable", never an error.
func TestObservationVersions_WithoutTableIsUnavailable(t *testing.T) {
	db := newSearchDB(t, tokenizeDefault)
	id := addObs(t, db, "Old schema", "x", "")
	res, found, err := services.ObservationVersions(context.Background(), db, id)
	if err != nil || !found || res.Available || res.Versions == nil || len(res.Versions) != 0 {
		t.Errorf("got %+v found=%v err=%v, want found, unavailable, empty list", res, found, err)
	}
}

func TestObservationVersions_UnknownObservation(t *testing.T) {
	for name, withTable := range map[string]bool{"with table": true, "without table": false} {
		t.Run(name, func(t *testing.T) {
			db := newSearchDB(t, tokenizeDefault)
			if withTable {
				createVersionsTable(t, db)
			}
			_, found, err := services.ObservationVersions(context.Background(), db, 999)
			if err != nil || found {
				t.Errorf("got found=%v err=%v, want not found without error", found, err)
			}
		})
	}
}
