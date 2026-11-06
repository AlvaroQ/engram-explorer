package services_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/services"
)

// Regression (minimal schema, no memory_relations or prompt_tombstones): a
// NULL and an empty summary are both empty; an ISO started_at with a zone
// offset is compared in UTC (old → deletable, now → recent); a started_at
// SQLite cannot parse is kept as recent. The delete removes exactly Total.
func TestEmptySessions_EdgeCases(t *testing.T) {
	db, _ := newWriteDB(t)
	for _, s := range []string{
		`INSERT INTO sessions (id, project, directory, started_at, summary) VALUES
			('null-summary', 'p', '/z', '2020-01-01 00:00:00', NULL),
			('empty-summary', 'p', '/z', '2020-01-01 00:00:00', ''),
			('iso-old', 'p', '/z', '2020-01-01T00:00:00+02:00', NULL),
			('iso-now-offset', 'p', '/z', strftime('%Y-%m-%dT%H:%M:%S+05:00', 'now', '+5 hours'), NULL),
			('unparsable', 'p', '/z', 'yesterday', NULL)`,
	} {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	rep, err := services.ListEmptySessions(db, 50)
	if err != nil {
		t.Fatalf("ListEmptySessions: %v", err)
	}
	listed := map[string]bool{}
	for _, s := range rep.Sessions {
		listed[s.ID] = true
	}
	want := map[string]bool{"null-summary": true, "empty-summary": true, "iso-old": true}
	if rep.Total != 3 || rep.SkippedRecent != 2 || fmt.Sprint(listed) != fmt.Sprint(want) {
		t.Fatalf("total=%d recent=%d listed=%v, want 3/2 %v", rep.Total, rep.SkippedRecent, listed, want)
	}

	res, err := services.DeleteEmptySessions(context.Background(), db)
	if err != nil {
		t.Fatalf("DeleteEmptySessions: %v", err)
	}
	if int(res.Deleted) != rep.Total {
		t.Errorf("Deleted = %d, want the listed total %d", res.Deleted, rep.Total)
	}
	if got := fmt.Sprint(sessionIDs(t, db)); got != "map[iso-now-offset:true unparsable:true]" {
		t.Errorf("sessions after delete = %s, want iso-now-offset and unparsable", got)
	}
}
