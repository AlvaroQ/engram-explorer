package services_test

import (
	"database/sql"
	"testing"
	"time"

	"github.com/AlvaroQ/engram-explorer/internal/services"
)

// seedReviewObs inserts a live or deleted observation with the given project
// and review_after (nil for NULL) and returns its id.
func seedReviewObs(t *testing.T, db *sql.DB, title, project string, reviewAfter *string, deleted bool) int64 {
	t.Helper()
	if _, err := db.Exec(`INSERT OR IGNORE INTO sessions (id, project, directory) VALUES ('rv', 'proj', '/tmp')`); err != nil {
		t.Fatalf("seed session: %v", err)
	}
	var deletedAt any
	if deleted {
		deletedAt = "2026-01-02 00:00:00"
	}
	res, err := db.Exec(`INSERT INTO observations (session_id, type, title, content, project, review_after, deleted_at)
		VALUES ('rv', 'note', ?, 'c', ?, ?, ?)`, title, project, reviewAfter, deletedAt)
	if err != nil {
		t.Fatalf("seed obs %q: %v", title, err)
	}
	id, _ := res.LastInsertId()
	return id
}

func listTitles(t *testing.T, db *sql.DB, p services.ObservationListParams) map[string]services.ObservationRow {
	t.Helper()
	if p.Limit == 0 {
		p.Limit = 50
	}
	items, _, err := services.ObservationsList(db, p)
	if err != nil {
		t.Fatalf("ObservationsList: %v", err)
	}
	out := map[string]services.ObservationRow{}
	for _, it := range items {
		out[*it.Title] = it
	}
	return out
}

// The filter selects exactly the rows ReviewDue() reports as due: a past or
// current review date in the stored layout. Future, NULL, empty and values in
// another format (which ReviewDue cannot parse) are not due.
func TestObservationsList_ReviewDueFilterMatchesReviewDue(t *testing.T) {
	db, _ := newWriteDB(t)
	now := time.Now().UTC()
	at := func(d time.Duration) *string { s := now.Add(d).Format(sqliteLayout); return &s }
	seedReviewObs(t, db, "past", "proj", at(-24*time.Hour), false)
	seedReviewObs(t, db, "now", "proj", at(0), false)
	seedReviewObs(t, db, "future", "proj", at(time.Hour), false)
	seedReviewObs(t, db, "null", "proj", nil, false)
	seedReviewObs(t, db, "empty", "proj", strPtr(""), false)
	seedReviewObs(t, db, "iso", "proj", strPtr("2020-01-01T10:00:00Z"), false)
	seedReviewObs(t, db, "deleted-past", "proj", at(-time.Hour), true)

	got := listTitles(t, db, services.ObservationListParams{ReviewDue: true})
	if len(got) != 2 || got["past"].ID == 0 || got["now"].ID == 0 {
		t.Fatalf("review due list = %v, want exactly past and now", keys(got))
	}
	all := listTitles(t, db, services.ObservationListParams{IncludeDeleted: true})
	for title, row := range all {
		_, listed := got[title]
		if row.DeletedAt == nil && row.ReviewDue() != listed {
			t.Errorf("%s: ReviewDue()=%v but listed=%v", title, row.ReviewDue(), listed)
		}
	}

	// The deleted filter still applies on top of review=due.
	onlyDeleted := listTitles(t, db, services.ObservationListParams{ReviewDue: true, OnlyDeleted: true})
	if len(onlyDeleted) != 1 || onlyDeleted["deleted-past"].ID == 0 {
		t.Errorf("review due + only deleted = %v, want [deleted-past]", keys(onlyDeleted))
	}
	// Without the flag the list is unchanged (6 live rows).
	if n := len(listTitles(t, db, services.ObservationListParams{})); n != 6 {
		t.Errorf("unfiltered live list has %d rows, want 6", n)
	}
}

// The count uses the list filters (project, deleted…) but ignores the review
// flag itself and the cursor, so the chip shows how many rows enabling it
// would list.
func TestObservationsReviewDueCount(t *testing.T) {
	db, _ := newWriteDB(t)
	past := time.Now().UTC().Add(-time.Hour).Format(sqliteLayout)
	future := time.Now().UTC().Add(time.Hour).Format(sqliteLayout)
	seedReviewObs(t, db, "a1", "alpha", &past, false)
	seedReviewObs(t, db, "a2", "alpha", &past, false)
	seedReviewObs(t, db, "a3", "alpha", &future, false)
	seedReviewObs(t, db, "a4", "alpha", &past, true)
	seedReviewObs(t, db, "b1", "beta", &past, false)

	cases := []struct {
		name string
		p    services.ObservationListParams
		want int
	}{
		{"all live", services.ObservationListParams{}, 3},
		{"review flag ignored", services.ObservationListParams{ReviewDue: true}, 3},
		{"project filter", services.ObservationListParams{Projects: []string{"alpha"}}, 2},
		{"cursor ignored", services.ObservationListParams{Projects: []string{"alpha"}, Cursor: "garbage"}, 2},
		{"include deleted", services.ObservationListParams{Projects: []string{"alpha"}, IncludeDeleted: true}, 3},
		{"other project", services.ObservationListParams{Projects: []string{"gamma"}}, 0},
	}
	for _, tc := range cases {
		n, err := services.ObservationsReviewDueCount(db, tc.p)
		if err != nil || n != tc.want {
			t.Errorf("%s: count = %d, %v; want %d", tc.name, n, err, tc.want)
		}
	}
}

func keys(m map[string]services.ObservationRow) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// Values in the stored layout that are not real dates or times (they pass the
// layout GLOB and sort before now) are not due: ReviewDue() cannot parse them,
// so the filter must not list them either. SQLite's datetime() rejects some
// (month 13, second 60) and normalizes others (Feb 30 → Mar 2, hour 24 kept).
func TestObservationsList_ReviewDueIgnoresInvalidDates(t *testing.T) {
	db, _ := newWriteDB(t)
	seedReviewObs(t, db, "valid", "proj", strPtr("2024-02-29 10:00:00"), false)
	for _, v := range []string{
		"2020-13-45 99:99:99",
		"2026-02-30 10:00:00",
		"2025-02-29 10:00:00",
		"2025-01-01 24:00:00",
		"2025-01-01 23:59:60",
		"2020-00-10 10:00:00",
	} {
		seedReviewObs(t, db, v, "proj", strPtr(v), false)
	}

	got := listTitles(t, db, services.ObservationListParams{ReviewDue: true})
	if len(got) != 1 || got["valid"].ID == 0 {
		t.Errorf("review due list = %v, want exactly [valid]", keys(got))
	}
	for title, row := range listTitles(t, db, services.ObservationListParams{}) {
		if _, listed := got[title]; row.ReviewDue() != listed {
			t.Errorf("%s: ReviewDue()=%v but listed=%v", title, row.ReviewDue(), listed)
		}
	}
}
