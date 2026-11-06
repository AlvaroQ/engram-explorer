package services_test

import (
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/services"
)

// TestCheckDataQuality seeds empty titles and exact duplicates, plus the rows
// each check must ignore (soft-deleted, unique hash, empty or missing hash).
func TestCheckDataQuality(t *testing.T) {
	db, _ := newWriteDB(t)
	stmts := []string{
		`INSERT INTO sessions (id, project, directory) VALUES ('s1', 'repo', '/tmp')`,
		`INSERT INTO observations (id, session_id, type, title, content, project, normalized_hash, deleted_at) VALUES
		 (1, 's1', 'note', '', 'c1', 'repo', 'u1', NULL),
		 (2, 's1', 'note', '   ', 'c2', 'repo', 'u2', NULL),
		 (3, 's1', 'note', '', 'c3', 'repo', 'u3', datetime('now')),
		 (4, 's1', 'note', 'Fine', 'c4', 'repo', 'u4', NULL),
		 (10, 's1', 'note', 'Dup A', 'same', 'repo', 'h1', NULL),
		 (11, 's1', 'note', 'Dup A', 'same', 'repo', 'h1', NULL),
		 (12, 's1', 'note', 'Dup A', 'same', 'other', 'h1', NULL),
		 (13, 's1', 'note', 'Dup A', 'same', 'repo', 'h1', datetime('now')),
		 (20, 's1', 'note', 'Dup B', 'b', 'repo', 'h2', NULL),
		 (21, 's1', 'note', 'Dup B', 'b', 'repo', 'h2', NULL),
		 (30, 's1', 'note', 'Gone', 'g', 'repo', 'h3', NULL),
		 (31, 's1', 'note', 'Gone', 'g', 'repo', 'h3', datetime('now')),
		 (40, 's1', 'note', 'No hash', 'n', 'repo', '', NULL),
		 (41, 's1', 'note', 'No hash', 'n', 'repo', '', NULL),
		 (50, 's1', 'note', 'Null hash', 'n', 'repo', NULL, NULL),
		 (51, 's1', 'note', 'Null hash', 'n', 'repo', NULL, NULL)`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	rep, err := services.CheckDataQuality(db, 0)
	if err != nil {
		t.Fatalf("CheckDataQuality: %v", err)
	}
	if rep.Limit != services.DataQualityDefaultLimit {
		t.Errorf("Limit = %d, want default %d", rep.Limit, services.DataQualityDefaultLimit)
	}

	// Empty titles: ids 1 and 2 (whitespace only); 3 is soft-deleted.
	if rep.EmptyTitleTotal != 2 || len(rep.EmptyTitles) != 2 || rep.EmptyTitles[0].ID != 1 || rep.EmptyTitles[1].ID != 2 {
		t.Fatalf("empty titles = %d %+v, want ids [1 2]", rep.EmptyTitleTotal, rep.EmptyTitles)
	}
	if rep.EmptyTitles[0].Project != "repo" || rep.EmptyTitles[0].Type != "note" {
		t.Errorf("empty title row = %+v, want project repo, type note", rep.EmptyTitles[0])
	}

	// Duplicates: h1 (3 live copies, across projects) then h2 (2); h3 has one
	// live copy; empty and NULL hashes never group.
	if rep.DuplicateGroupTotal != 2 || rep.DuplicateMemoryTotal != 5 || len(rep.Duplicates) != 2 {
		t.Fatalf("duplicates = groups %d, memories %d, %+v; want 2 groups, 5 memories", rep.DuplicateGroupTotal, rep.DuplicateMemoryTotal, rep.Duplicates)
	}
	h1, h2 := rep.Duplicates[0], rep.Duplicates[1]
	if h1.Count != 3 || h1.Title != "Dup A" || len(h1.Memories) != 3 || h1.Memories[0].ID != 10 || h1.Memories[2].ID != 12 || h1.Memories[2].Project != "other" {
		t.Errorf("first group = %+v, want Dup A with ids 10, 11, 12", h1)
	}
	if h2.Count != 2 || len(h2.Memories) != 2 || h2.Memories[0].ID != 20 {
		t.Errorf("second group = %+v, want Dup B with ids 20, 21", h2)
	}

	// The check only reads: nothing is deleted or changed.
	var live int
	if err := db.QueryRow(`SELECT COUNT(*) FROM observations WHERE deleted_at IS NULL`).Scan(&live); err != nil {
		t.Fatalf("count: %v", err)
	}
	if live != 13 {
		t.Errorf("live observations = %d after the check, want 13 (read-only)", live)
	}
}

// TestCheckDataQuality_Bounded: lists are capped by limit while the totals
// still count everything; each group lists at most limit memories.
func TestCheckDataQuality_Bounded(t *testing.T) {
	db, _ := newWriteDB(t)
	for _, s := range []string{
		`INSERT INTO sessions (id, project, directory) VALUES ('s1', 'repo', '/tmp')`,
		`INSERT INTO observations (session_id, type, title, content, project, normalized_hash) VALUES
		 ('s1', 'note', '', 'a', 'repo', 'e1'), ('s1', 'note', '', 'b', 'repo', 'e2'), ('s1', 'note', '', 'c', 'repo', 'e3'),
		 ('s1', 'note', 'X', 'x', 'repo', 'hx'), ('s1', 'note', 'X', 'x', 'repo', 'hx'), ('s1', 'note', 'X', 'x', 'repo', 'hx'),
		 ('s1', 'note', 'Y', 'y', 'repo', 'hy'), ('s1', 'note', 'Y', 'y', 'repo', 'hy')`,
	} {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	rep, err := services.CheckDataQuality(db, 2)
	if err != nil {
		t.Fatalf("CheckDataQuality: %v", err)
	}
	if rep.EmptyTitleTotal != 3 || len(rep.EmptyTitles) != 2 {
		t.Errorf("empty titles: total %d, listed %d; want 3 and 2", rep.EmptyTitleTotal, len(rep.EmptyTitles))
	}
	if rep.DuplicateGroupTotal != 2 || len(rep.Duplicates) != 2 {
		t.Fatalf("duplicate groups: total %d, listed %d; want 2 and 2", rep.DuplicateGroupTotal, len(rep.Duplicates))
	}
	if g := rep.Duplicates[0]; g.Count != 3 || len(g.Memories) != 2 {
		t.Errorf("largest group: count %d, listed %d; want 3 and 2", g.Count, len(g.Memories))
	}

	rep, err = services.CheckDataQuality(db, 1)
	if err != nil {
		t.Fatalf("CheckDataQuality: %v", err)
	}
	if rep.DuplicateGroupTotal != 2 || len(rep.Duplicates) != 1 || rep.Duplicates[0].Count != 3 {
		t.Errorf("limit 1: want the largest group only, got total %d, %+v", rep.DuplicateGroupTotal, rep.Duplicates)
	}
}
