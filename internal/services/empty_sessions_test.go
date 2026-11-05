package services_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/services"
)

// seedEmptySessionsFixture seeds one session per predicate of the "empty"
// definition. Only "empty-old" is a deletable empty session.
func seedEmptySessionsFixture(t *testing.T, db *sql.DB) {
	t.Helper()
	old := "2026-01-01 10:00:00"
	for _, s := range []struct{ id, project, started, summary string }{
		{"empty-old", "repo", old, ""},
		{"blank-summary", "repo", old, "   "},
		{"with-summary", "repo", old, "did things"},
		{"with-obs", "repo", old, ""},
		{"with-deleted-obs", "repo", old, ""},
		{"with-prompt", "repo", old, ""},
		{"enrolled-empty", "synced", old, ""},
	} {
		var summary any
		if s.summary != "" {
			summary = s.summary
		}
		if _, err := db.Exec(`INSERT INTO sessions (id, project, directory, started_at, summary) VALUES (?, ?, '/tmp', ?, ?)`,
			s.id, s.project, s.started, summary); err != nil {
			t.Fatalf("seed session %s: %v", s.id, err)
		}
	}
	// A session started just now: no content yet, but it may be live.
	if _, err := db.Exec(`INSERT INTO sessions (id, project, directory) VALUES ('recent', 'repo', '/tmp')`); err != nil {
		t.Fatalf("seed recent: %v", err)
	}
	stmts := []string{
		`INSERT INTO observations (session_id, type, title, content, project) VALUES ('with-obs', 'note', 'T', 'C', 'repo')`,
		`INSERT INTO observations (session_id, type, title, content, project, deleted_at) VALUES ('with-deleted-obs', 'note', 'T', 'C', 'repo', datetime('now'))`,
		`INSERT INTO user_prompts (session_id, content, project) VALUES ('with-prompt', 'hi', 'repo')`,
		`INSERT INTO sync_enrolled_projects (project) VALUES ('synced')`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
}

func sessionIDs(t *testing.T, db *sql.DB) map[string]bool {
	t.Helper()
	rows, err := db.Query(`SELECT id FROM sessions`)
	if err != nil {
		t.Fatalf("query sessions: %v", err)
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("scan: %v", err)
		}
		out[id] = true
	}
	return out
}

// TestListEmptySessions_Predicates: only a session with no summary (NULL or
// blank), no observations (soft-deleted ones included) and no prompts, from an
// unenrolled project and older than the grace window, is listed.
func TestListEmptySessions_Predicates(t *testing.T) {
	db, _ := newWriteDB(t)
	seedEmptySessionsFixture(t, db)
	// blank-summary is empty too: a whitespace-only summary carries nothing.
	rep, err := services.ListEmptySessions(db, 50)
	if err != nil {
		t.Fatalf("ListEmptySessions: %v", err)
	}
	if rep.Total != 2 {
		t.Errorf("Total = %d, want 2", rep.Total)
	}
	got := map[string]bool{}
	for _, s := range rep.Sessions {
		got[s.ID] = true
		if s.Project != "repo" || s.StartedAt != "2026-01-01 10:00:00" {
			t.Errorf("row %+v: want project repo, started 2026-01-01 10:00:00", s)
		}
	}
	if !got["empty-old"] || !got["blank-summary"] || len(got) != 2 {
		t.Errorf("listed %v, want exactly empty-old and blank-summary", got)
	}
	if rep.SkippedEnrolled != 1 {
		t.Errorf("SkippedEnrolled = %d, want 1", rep.SkippedEnrolled)
	}
	if rep.SkippedRecent != 1 {
		t.Errorf("SkippedRecent = %d, want 1", rep.SkippedRecent)
	}
}

// TestListEmptySessions_Bounded: the list is capped, the total is not.
func TestListEmptySessions_Bounded(t *testing.T) {
	db, _ := newWriteDB(t)
	for _, id := range []string{"a", "b", "c"} {
		if _, err := db.Exec(`INSERT INTO sessions (id, project, directory, started_at) VALUES (?, 'p', '/tmp', '2026-01-01 10:00:00')`, id); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	rep, err := services.ListEmptySessions(db, 2)
	if err != nil {
		t.Fatalf("ListEmptySessions: %v", err)
	}
	if rep.Total != 3 || len(rep.Sessions) != 2 || rep.Limit != 2 {
		t.Errorf("got total=%d rows=%d limit=%d, want 3/2/2", rep.Total, len(rep.Sessions), rep.Limit)
	}
}

// TestDeleteEmptySessions_DeletesOnlyEmpty: the bulk delete removes exactly the
// listed rows and leaves every other session, observation and prompt intact,
// and enqueues no sync mutation (enrolled projects are skipped).
func TestDeleteEmptySessions_DeletesOnlyEmpty(t *testing.T) {
	db, _ := newWriteDB(t)
	seedEmptySessionsFixture(t, db)

	res, err := services.DeleteEmptySessions(context.Background(), db)
	if err != nil {
		t.Fatalf("DeleteEmptySessions: %v", err)
	}
	if res.Deleted != 2 {
		t.Errorf("Deleted = %d, want 2", res.Deleted)
	}
	ids := sessionIDs(t, db)
	for _, gone := range []string{"empty-old", "blank-summary"} {
		if ids[gone] {
			t.Errorf("session %s must be deleted", gone)
		}
	}
	for _, kept := range []string{"with-summary", "with-obs", "with-deleted-obs", "with-prompt", "enrolled-empty", "recent"} {
		if !ids[kept] {
			t.Errorf("session %s must be kept", kept)
		}
	}
	var obs, prompts, muts int
	_ = db.QueryRow(`SELECT COUNT(*) FROM observations`).Scan(&obs)
	_ = db.QueryRow(`SELECT COUNT(*) FROM user_prompts`).Scan(&prompts)
	_ = db.QueryRow(`SELECT COUNT(*) FROM sync_mutations`).Scan(&muts)
	if obs != 2 || prompts != 1 || muts != 0 {
		t.Errorf("observations=%d prompts=%d mutations=%d, want 2/1/0", obs, prompts, muts)
	}

	// Idempotent: a second run finds nothing.
	res, err = services.DeleteEmptySessions(context.Background(), db)
	if err != nil || res.Deleted != 0 {
		t.Errorf("second run: deleted=%d err=%v, want 0/nil", res.Deleted, err)
	}
}

// TestDeleteEmptySessions_RechecksAtDeleteTime: a session that was empty when
// listed but gained a prompt (or an observation) before the delete is kept.
func TestDeleteEmptySessions_RechecksAtDeleteTime(t *testing.T) {
	db, _ := newWriteDB(t)
	for _, id := range []string{"s-prompt", "s-obs", "s-still-empty"} {
		if _, err := db.Exec(`INSERT INTO sessions (id, project, directory, started_at) VALUES (?, 'p', '/tmp', '2026-01-01 10:00:00')`, id); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	rep, err := services.ListEmptySessions(db, 50)
	if err != nil || rep.Total != 3 {
		t.Fatalf("list: total=%d err=%v, want 3", rep.Total, err)
	}
	// Content arrives between the list and the delete.
	if _, err := db.Exec(`INSERT INTO user_prompts (session_id, content, project) VALUES ('s-prompt', 'late', 'p')`); err != nil {
		t.Fatalf("late prompt: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO observations (session_id, type, title, content, project) VALUES ('s-obs', 'note', 'T', 'C', 'p')`); err != nil {
		t.Fatalf("late obs: %v", err)
	}
	res, err := services.DeleteEmptySessions(context.Background(), db)
	if err != nil {
		t.Fatalf("DeleteEmptySessions: %v", err)
	}
	if res.Deleted != 1 {
		t.Errorf("Deleted = %d, want 1", res.Deleted)
	}
	ids := sessionIDs(t, db)
	if !ids["s-prompt"] || !ids["s-obs"] || ids["s-still-empty"] {
		t.Errorf("sessions after delete = %v, want s-prompt and s-obs kept, s-still-empty gone", ids)
	}
}
