package services_test

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/services"
)

// engramSchemaSQL returns the upstream Engram schema dump without the
// statements SQLite creates on its own (sqlite_sequence, FTS5 shadow tables),
// plus the upstream observation_versions table (with its cascade FK).
func engramSchemaSQL(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "testdata", "schema", "engram-schema.sql"))
	if err != nil {
		t.Fatalf("read schema: %v", err)
	}
	s := regexp.MustCompile(`(?m)^CREATE TABLE (sqlite_sequence|'[a-z]+_fts_[a-z]+')\(.*;$`).ReplaceAllString(string(raw), "")
	return s + `
CREATE TABLE observation_versions (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	observation_id INTEGER NOT NULL REFERENCES observations(id) ON DELETE CASCADE,
	version INTEGER NOT NULL, title TEXT, content TEXT,
	created_at TEXT NOT NULL DEFAULT (datetime('now')));`
}

// openEngramDB creates a database with the full upstream schema (foreign keys
// on, like the dashboard's read-write pool).
func openEngramDB(t *testing.T, name string) (*sql.DB, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(on)&_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(engramSchemaSQL(t)); err != nil {
		t.Fatalf("schema: %v", err)
	}
	return db, path
}

// seedTwoProjects fills every project-scoped table with alpha and beta rows.
// "BETASECRET" appears only in rows of other projects; it must not survive
// anywhere in an alpha snapshot, not even in freed pages.
func seedTwoProjects(t *testing.T, db *sql.DB) {
	t.Helper()
	for _, s := range []string{
		`INSERT INTO sessions (id, project, directory, summary) VALUES
			('s-a', 'alpha', '/a', 'alpha work'),
			('s-b', 'beta', '/b', 'BETASECRET summary'),
			('s-x', 'beta', '/b', 'beta session holding an alpha memory')`,
		`INSERT INTO observations (id, session_id, type, title, content, project, sync_id, deleted_at) VALUES
			(1, 's-a', 'note', 'a1', 'alphaword one', 'alpha', 'obs-a1', NULL),
			(2, 's-a', 'note', 'a2', 'alphaword deleted', 'alpha', 'obs-a2', '2026-01-02 00:00:00'),
			(3, 's-x', 'note', 'ax', 'alphaword cross', 'alpha', 'obs-ax', NULL),
			(4, 's-b', 'note', 'b1', 'BETASECRET memory', 'beta', 'obs-b1', NULL),
			(5, 's-b', 'note', 'n1', 'BETASECRET no project', NULL, 'obs-n1', NULL)`,
		`INSERT INTO user_prompts (session_id, content, project, sync_id) VALUES
			('s-a', 'alpha prompt', 'alpha', 'p-a'),
			('s-b', 'BETASECRET prompt', 'beta', 'p-b'),
			('s-b', 'BETASECRET orphan prompt', NULL, 'p-n')`,
		`INSERT INTO prompt_tombstones (sync_id, session_id, project) VALUES ('t-a', 's-a', 'alpha'), ('t-b', 's-b', 'beta')`,
		`INSERT INTO memory_relations (id, sync_id, source_id, target_id, reason, superseded_by_relation_id) VALUES
			(1, 'r-keep', 'obs-a1', 'obs-ax', 'alpha link', 2),
			(2, 'r-cross', 'obs-a1', 'obs-b1', 'BETASECRET link', NULL),
			(3, 'r-beta', 'obs-b1', 'obs-n1', 'BETASECRET link', NULL),
			(4, 'r-null', 'obs-a1', NULL, 'BETASECRET dangling', NULL)`,
		`INSERT INTO observation_versions (observation_id, version, title, content) VALUES
			(1, 1, 'a1 v1', 'alpha old'), (4, 1, 'b1 v1', 'BETASECRET old')`,
		`INSERT INTO sync_state (target_key) VALUES ('cloud'), ('cloud:alpha'), ('cloud:beta')`,
		`INSERT INTO sync_mutations (target_key, entity, entity_key, op, payload, project) VALUES
			('cloud', 'observation', 'obs-a1', 'upsert', '{"content":"alpha"}', 'alpha'),
			('cloud:beta', 'observation', 'obs-b1', 'upsert', '{"content":"BETASECRET"}', 'beta')`,
		`INSERT INTO sync_enrolled_projects (project) VALUES ('alpha'), ('beta')`,
		`INSERT INTO cloud_upgrade_state (project, snapshot_json) VALUES ('alpha', '{}'), ('beta', '{"x":"BETASECRET"}')`,
		`INSERT INTO sync_apply_deferred (sync_id, entity, payload) VALUES ('d-1', 'observation', '{"content":"BETASECRET"}')`,
		`INSERT INTO sync_chunks (chunk_id) VALUES ('chunk-BETASECRET')`,
		// Tables this version does not know: with a project column they are
		// filtered, without one they are emptied.
		`CREATE TABLE future_proj (project TEXT, note TEXT)`,
		`INSERT INTO future_proj VALUES ('alpha', 'alpha note'), ('beta', 'BETASECRET note')`,
		`CREATE TABLE future_global (note TEXT)`,
		`INSERT INTO future_global VALUES ('BETASECRET global')`,
	} {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("seed %.60q: %v", s, err)
		}
	}
}

func openSnapshot(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatalf("open snapshot: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func column(t *testing.T, db *sql.DB, query string) []string {
	t.Helper()
	rows, err := db.Query(query)
	if err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var v sql.NullString
		if err := rows.Scan(&v); err != nil {
			t.Fatalf("scan: %v", err)
		}
		if v.Valid {
			out = append(out, v.String)
		} else {
			out = append(out, "<nil>")
		}
	}
	sort.Strings(out)
	return out
}

func schemaObjects(t *testing.T, db *sql.DB) []string {
	t.Helper()
	return column(t, db, `SELECT type || ':' || name FROM sqlite_master`)
}

func TestExportProjectSnapshot_KeepsOnlyTheProject(t *testing.T) {
	src, _ := openEngramDB(t, "engram.db")
	seedTwoProjects(t, src)
	srcSchema := schemaObjects(t, src)

	path, cleanup, err := services.ExportProjectSnapshot(context.Background(), src, "alpha")
	if err != nil {
		t.Fatalf("ExportProjectSnapshot: %v", err)
	}
	defer cleanup()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte("BETASECRET")) {
		t.Error("snapshot file still contains other-project bytes (freed pages not vacuumed)")
	}
	for _, side := range []string{"-wal", "-journal", "-shm"} {
		if _, err := os.Stat(path + side); err == nil {
			t.Errorf("snapshot left a %s sidecar file", side)
		}
	}

	snap := openSnapshot(t, path)

	want := map[string]string{
		`SELECT title FROM observations`:                         "[a1 a2 ax]",
		`SELECT id FROM sessions`:                                "[s-a s-x]", // s-x holds an alpha memory
		`SELECT sync_id FROM user_prompts`:                       "[p-a]",
		`SELECT sync_id FROM prompt_tombstones`:                  "[t-a]",
		`SELECT sync_id FROM memory_relations`:                   "[r-keep]",
		`SELECT superseded_by_relation_id FROM memory_relations`: "[<nil>]",
		`SELECT title FROM observation_versions`:                 "[a1 v1]",
		`SELECT target_key FROM sync_state`:                      "[cloud cloud:alpha]",
		`SELECT entity_key FROM sync_mutations`:                  "[obs-a1]",
		`SELECT project FROM sync_enrolled_projects`:             "[alpha]",
		`SELECT project FROM cloud_upgrade_state`:                "[alpha]",
		`SELECT sync_id FROM sync_apply_deferred`:                "[]",
		`SELECT chunk_id FROM sync_chunks`:                       "[]",
		`SELECT note FROM future_proj`:                           "[alpha note]",
		`SELECT note FROM future_global`:                         "[]",
		`SELECT o.title FROM observations o JOIN observations_fts f ON f.rowid = o.id WHERE observations_fts MATCH 'alphaword'`: "[a1 a2 ax]",
		`SELECT rowid FROM observations_fts WHERE observations_fts MATCH 'BETASECRET'`:                                          "[]",
		`SELECT rowid FROM prompts_fts WHERE prompts_fts MATCH 'BETASECRET'`:                                                    "[]",
		`SELECT u.content FROM user_prompts u JOIN prompts_fts f ON f.rowid = u.id WHERE prompts_fts MATCH 'alpha'`:             "[alpha prompt]",
		`PRAGMA quick_check`: "[ok]",
	}
	for q, w := range want {
		if got := fmt.Sprint(column(t, snap, q)); got != w {
			t.Errorf("%s = %s, want %s", q, got, w)
		}
	}
	for _, fts := range []string{"observations_fts", "prompts_fts"} {
		if _, err := snap.Exec(`INSERT INTO ` + fts + `(` + fts + `) VALUES ('integrity-check')`); err != nil {
			t.Errorf("%s integrity-check: %v", fts, err)
		}
	}
	if got := schemaObjects(t, snap); fmt.Sprint(got) != fmt.Sprint(srcSchema) {
		t.Errorf("snapshot schema differs:\n got %v\nwant %v", got, srcSchema)
	}

	// The source database is untouched.
	if got := fmt.Sprint(column(t, src, `SELECT title FROM observations`)); got != "[a1 a2 ax b1 n1]" {
		t.Errorf("source observations changed: %s", got)
	}

	cleanup()
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("cleanup left the snapshot: %v", err)
	}
}

// The snapshot passes ImportMerge's schema validation and merges into an
// empty database with exactly the project's rows.
func TestExportProjectSnapshot_ReimportsWithImportMerge(t *testing.T) {
	src, _ := openEngramDB(t, "engram.db")
	seedTwoProjects(t, src)
	path, cleanup, err := services.ExportProjectSnapshot(context.Background(), src, "alpha")
	if err != nil {
		t.Fatalf("ExportProjectSnapshot: %v", err)
	}
	defer cleanup()

	dst, _ := openEngramDB(t, "fresh.db")
	res, err := services.ImportMerge(context.Background(), dst, path, t.TempDir())
	if err != nil {
		t.Fatalf("ImportMerge: %v", err)
	}
	want := services.MergeCounts{Observations: 3, Sessions: 2, Prompts: 1, Relations: 1}
	if res.Inserted != want || res.Skipped != (services.MergeCounts{}) {
		t.Errorf("inserted %+v skipped %+v, want inserted %+v and nothing skipped", res.Inserted, res.Skipped, want)
	}
	if got := fmt.Sprint(column(t, dst, `SELECT DISTINCT project FROM observations`)); got != "[alpha]" {
		t.Errorf("imported projects %s, want [alpha]", got)
	}

	// Importing the same snapshot again dedupes everything.
	res, err = services.ImportMerge(context.Background(), dst, path, t.TempDir())
	if err != nil || res.Inserted != (services.MergeCounts{}) {
		t.Errorf("second import: inserted %+v err %v, want nothing new", res.Inserted, err)
	}
}

func TestExportProjectSnapshot_UnknownProject(t *testing.T) {
	src, _ := openEngramDB(t, "engram.db")
	seedTwoProjects(t, src)
	for _, p := range []string{"gamma", "", "ALPHA"} {
		_, cleanup, err := services.ExportProjectSnapshot(context.Background(), src, p)
		var we *services.WriteError
		if !errors.As(err, &we) || we.Code != "NOT_FOUND" || cleanup != nil {
			t.Errorf("%q: err %v cleanup-nil=%v, want NOT_FOUND and no cleanup", p, err, cleanup == nil)
		}
	}
}

// Older schemas without the optional tables still export.
func TestExportProjectSnapshot_MinimalSchema(t *testing.T) {
	db, _ := newWriteDB(t)
	seedObs(t, db) // project "proj"
	if _, err := db.Exec(`INSERT INTO sessions (id, project, directory) VALUES ('o', 'other', '/o')`); err != nil {
		t.Fatal(err)
	}
	path, cleanup, err := services.ExportProjectSnapshot(context.Background(), db, "proj")
	if err != nil {
		t.Fatalf("ExportProjectSnapshot: %v", err)
	}
	defer cleanup()
	snap := openSnapshot(t, path)
	if got := fmt.Sprint(column(t, snap, `SELECT id FROM sessions`)); got != "[s1]" {
		t.Errorf("sessions %s, want [s1]", got)
	}
	if got := fmt.Sprint(column(t, snap, `SELECT title FROM observations`)); got != "[Test Title]" {
		t.Errorf("observations %s", got)
	}
	// No FTS triggers here: the index is rebuilt from the kept rows.
	if got := fmt.Sprint(column(t, snap, `SELECT rowid FROM observations_fts WHERE observations_fts MATCH 'test'`)); got != "[1]" {
		t.Errorf("rebuilt observations_fts match = %s, want [1]", got)
	}
}

// A session of another project is kept only because a kept memory or prompt
// belongs to it; its summary and directory describe the other project's work,
// so they are blanked. The session's own project and its rows are untouched,
// and the snapshot still merges with ImportMerge.
func TestExportProjectSnapshot_BlanksForeignSessionDetails(t *testing.T) {
	src, _ := openEngramDB(t, "engram.db")
	seedTwoProjects(t, src)
	for _, s := range []string{
		`INSERT INTO sessions (id, project, directory, summary) VALUES
			('s-y', 'beta', '/home/FOREIGNDIRMARK', 'FOREIGNSUMMARYMARK')`,
		`INSERT INTO user_prompts (session_id, content, project, sync_id) VALUES ('s-y', 'alpha prompt two', 'alpha', 'p-a2')`,
	} {
		if _, err := src.Exec(s); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	path, cleanup, err := services.ExportProjectSnapshot(context.Background(), src, "alpha")
	if err != nil {
		t.Fatalf("ExportProjectSnapshot: %v", err)
	}
	defer cleanup()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range []string{"FOREIGNDIRMARK", "FOREIGNSUMMARYMARK", "beta session holding an alpha memory"} {
		if bytes.Contains(raw, []byte(m)) {
			t.Errorf("snapshot bytes contain the foreign session detail %q", m)
		}
	}

	snap := openSnapshot(t, path)
	want := map[string]string{
		`SELECT id || '|' || project || '|' || directory || '|' || COALESCE(summary, '<nil>') FROM sessions`: "[s-a|alpha|/a|alpha work s-x|beta||<nil> s-y|beta||<nil>]",
	}
	for q, w := range want {
		if got := fmt.Sprint(column(t, snap, q)); got != w {
			t.Errorf("%s = %s, want %s", q, got, w)
		}
	}
	// The source keeps the details.
	if got := fmt.Sprint(column(t, src, `SELECT summary FROM sessions WHERE id = 's-y'`)); got != "[FOREIGNSUMMARYMARK]" {
		t.Errorf("source session changed: %s", got)
	}

	dst, _ := openEngramDB(t, "fresh.db")
	res, err := services.ImportMerge(context.Background(), dst, path, t.TempDir())
	if err != nil {
		t.Fatalf("ImportMerge: %v", err)
	}
	if want := (services.MergeCounts{Observations: 3, Sessions: 3, Prompts: 2, Relations: 1}); res.Inserted != want {
		t.Errorf("inserted %+v, want %+v", res.Inserted, want)
	}
}
