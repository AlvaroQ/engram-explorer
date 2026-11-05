package services_test

import (
	"database/sql"
	"testing"
	"time"

	"github.com/AlvaroQ/engram-explorer/internal/services"
)

// daysAgo is a SQLite datetime string n days before now (UTC), at noon so a
// test running near midnight cannot cross a day boundary.
func daysAgo(n int) string {
	d := time.Now().UTC().AddDate(0, 0, -n)
	return time.Date(d.Year(), d.Month(), d.Day(), 12, 0, 0, 0, time.UTC).Format("2006-01-02 15:04:05")
}

func seedOverviewObs(t *testing.T, db *sql.DB, project, created, updated string, deleted bool) {
	t.Helper()
	var del any
	if deleted {
		del = created
	}
	var upd any
	if updated != "" {
		upd = updated
	}
	if _, err := db.Exec(
		`INSERT INTO observations (type, title, project, created_at, updated_at, deleted_at) VALUES ('decision', ?, ?, ?, ?, ?)`,
		project+"@"+created, project, created, upd, del,
	); err != nil {
		t.Fatalf("seed: %v", err)
	}
}

// TestOverviewKPIs_Last7DaysAndMostActive: the Memories card counts the
// observations created in the activity chart's 7-day window, and the Projects
// card names the project with most observations in the 30-day window (the
// chart's default range); deleted rows and empty projects never count.
func TestOverviewKPIs_Last7DaysAndMostActive(t *testing.T) {
	db := openSessionTestDB(t)
	seedOverviewObs(t, db, "alpha", daysAgo(1), "", false)
	seedOverviewObs(t, db, "alpha", daysAgo(3), "", false)
	seedOverviewObs(t, db, "beta", daysAgo(2), "", false)
	seedOverviewObs(t, db, "beta", daysAgo(10), "", false)
	seedOverviewObs(t, db, "beta", daysAgo(12), "", false)
	seedOverviewObs(t, db, "gamma", daysAgo(40), "", false) // outside 30d
	seedOverviewObs(t, db, "gamma", daysAgo(41), "", false)
	seedOverviewObs(t, db, "gamma", daysAgo(42), "", false)
	seedOverviewObs(t, db, "gamma", daysAgo(43), "", false)
	seedOverviewObs(t, db, "alpha", daysAgo(1), "", true) // deleted
	seedOverviewObs(t, db, "", daysAgo(1), "", false)     // no project
	seedOverviewObs(t, db, "", daysAgo(2), "", false)
	seedOverviewObs(t, db, "", daysAgo(4), "", false)

	got, err := services.OverviewBuild(db)
	if err != nil {
		t.Fatal(err)
	}
	if got.KPIs.Observations != 12 {
		t.Errorf("Observations = %d, want 12", got.KPIs.Observations)
	}
	if got.KPIs.ObservationsLast7d != 6 {
		t.Errorf("ObservationsLast7d = %d, want 6 (alpha 2, beta 1, no-project 3)", got.KPIs.ObservationsLast7d)
	}
	if got.KPIs.MostActiveProject != "beta" {
		t.Errorf("MostActiveProject = %q, want beta (3 in 30d; gamma is older, empty project ignored)", got.KPIs.MostActiveProject)
	}

	empty := openSessionTestDB(t)
	got, err = services.OverviewBuild(empty)
	if err != nil {
		t.Fatal(err)
	}
	if got.KPIs.ObservationsLast7d != 0 || got.KPIs.MostActiveProject != "" {
		t.Errorf("empty DB KPIs = %+v, want zero last-7d and no most active project", got.KPIs)
	}
}

// TestOverviewRecent_AgreesWithActivity (S4 coherent dates): the recent list
// is ordered by creation, like the activity series buckets by date(created_at),
// so an old memory edited today never tops the list with an old date, and the
// newest row's day is the series' newest day.
func TestOverviewRecent_AgreesWithActivity(t *testing.T) {
	db := openSessionTestDB(t)
	seedOverviewObs(t, db, "alpha", daysAgo(9), daysAgo(0), false) // edited today
	seedOverviewObs(t, db, "beta", daysAgo(2), "", false)
	seedOverviewObs(t, db, "alpha", daysAgo(5), "", false)

	got, err := services.OverviewBuild(db)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.RecentObservations) != 3 || len(got.Activity30d) == 0 {
		t.Fatalf("recent = %d rows, activity = %d days", len(got.RecentObservations), len(got.Activity30d))
	}
	newest := *got.RecentObservations[0].CreatedAt
	lastDay := got.Activity30d[len(got.Activity30d)-1].Day
	if newest[:10] != lastDay {
		t.Errorf("newest recent memory is from %s but the activity series ends on %s", newest[:10], lastDay)
	}
	for i := 1; i < len(got.RecentObservations); i++ {
		if *got.RecentObservations[i].CreatedAt > *got.RecentObservations[i-1].CreatedAt {
			t.Errorf("recent list not ordered by created_at: %s after %s",
				*got.RecentObservations[i].CreatedAt, *got.RecentObservations[i-1].CreatedAt)
		}
	}
}
