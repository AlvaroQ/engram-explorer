package services

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/AlvaroQ/engram-explorer/internal/sqlite"
)

// ExportProjectSnapshot builds a backup of one project: a full-schema copy of
// the database (ExportSnapshot) from which every row of other projects is
// deleted, so ImportMerge accepts it and merges back exactly that project.
//
// What the snapshot keeps:
//   - observations, user_prompts and every other table with a project column
//     (prompt_tombstones, sync_mutations, sync_enrolled_projects,
//     cloud_upgrade_state, …): rows whose project is the project. Soft-deleted
//     memories of the project are kept.
//   - sessions of the project, plus any other session that a kept memory or
//     prompt belongs to (their foreign key needs it, and ImportMerge inserts
//     sessions first). Those other-project sessions keep only their id,
//     project and times: summary is set to NULL and directory to the empty
//     string (NOT NULL in the schema), so the backup carries no text about
//     other projects' work.
//   - observation_versions of kept memories; memory_relations whose source
//     and target are both kept memories (superseded_by links to a dropped
//     relation are cleared).
//   - sync_state rows of the global "cloud" target, the project's own
//     "cloud:<project>" target and any target a kept sync mutation uses.
//
// Every other table without a project column (sync_apply_deferred,
// sync_chunks, tables from newer Engram versions) is emptied: it may hold
// other projects' data and ImportMerge does not read it. FTS indexes are
// rebuilt from the kept rows and the file is vacuumed, so deleted rows do not
// linger in free pages.
//
// The project must exist (projectExistsVia); otherwise a NOT_FOUND
// WriteError is returned. On success the caller must call cleanup once the
// file has been served.
func ExportProjectSnapshot(ctx context.Context, rwDB sqlite.Querier, project string) (path string, cleanup func(), err error) {
	exists, err := projectExistsVia(rwDB, project)
	if err != nil {
		return "", nil, err
	}
	if !exists {
		return "", nil, &WriteError{Code: "NOT_FOUND", Message: fmt.Sprintf("project %q not found", project)}
	}

	path, cleanup, err = ExportSnapshot(ctx, rwDB)
	if err != nil {
		return "", nil, err
	}
	if err := filterSnapshotToProject(ctx, path, project); err != nil {
		cleanup()
		return "", nil, err
	}
	return path, cleanup, nil
}

// snapshotTable is one table of the snapshot as listed in sqlite_master.
type snapshotTable struct {
	name, sql string
	virtual   bool
}

// filterSnapshotToProject deletes other projects' rows from the snapshot at
// path (see ExportProjectSnapshot), rebuilds the FTS indexes and vacuums.
func filterSnapshotToProject(ctx context.Context, path, project string) error {
	// Foreign keys stay off: every dependent row is deleted explicitly, in an
	// order that does not rely on ON DELETE actions. journal_mode(DELETE)
	// keeps the snapshot a single file once closed.
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(off)&_pragma=journal_mode(DELETE)")
	if err != nil {
		return fmt.Errorf("open snapshot: %w", err)
	}
	defer func() { _ = db.Close() }() // writes are committed and quick-checked below
	db.SetMaxOpenConns(1)

	tables, err := listSnapshotTables(ctx, db)
	if err != nil {
		return err
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin snapshot filter: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	if err := deleteOtherProjects(ctx, tx, tables, project); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit snapshot filter: %w", err)
	}
	if _, err := db.ExecContext(ctx, `VACUUM`); err != nil {
		return fmt.Errorf("vacuum snapshot: %w", err)
	}
	var qc string
	if err := db.QueryRowContext(ctx, `PRAGMA quick_check`).Scan(&qc); err != nil {
		return fmt.Errorf("snapshot quick_check: %q %w", qc, err)
	}
	if qc != "ok" {
		return fmt.Errorf("snapshot quick_check: %q <nil>", qc)
	}
	return nil
}

// listSnapshotTables returns the snapshot's tables except SQLite's internal
// ones and the shadow tables of virtual tables (they follow their table).
func listSnapshotTables(ctx context.Context, db *sql.DB) ([]snapshotTable, error) {
	rows, err := db.QueryContext(ctx, `SELECT name, COALESCE(sql, '') FROM sqlite_master
		WHERE type = 'table' AND name NOT LIKE 'sqlite\_%' ESCAPE '\' ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("list snapshot tables: %w", err)
	}
	defer rows.Close()
	var all []snapshotTable
	for rows.Next() {
		var t snapshotTable
		if err := rows.Scan(&t.name, &t.sql); err != nil {
			return nil, err
		}
		t.virtual = strings.HasPrefix(strings.ToUpper(strings.TrimSpace(t.sql)), "CREATE VIRTUAL TABLE")
		all = append(all, t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	var out []snapshotTable
	for _, t := range all {
		if !isShadowTable(t.name, all) {
			out = append(out, t)
		}
	}
	return out, nil
}

// isShadowTable reports whether name is a shadow table ("<vtab>_data", …) of
// one of the virtual tables in all.
func isShadowTable(name string, all []snapshotTable) bool {
	for _, t := range all {
		if t.virtual && t.name != name && strings.HasPrefix(name, t.name+"_") {
			return true
		}
	}
	return false
}

// keptRefsObservations selects the sync ids of the kept memories, the keys
// memory_relations uses for its source and target.
const keptRefsObservations = `SELECT sync_id FROM observations WHERE sync_id IS NOT NULL`

// deleteOtherProjects applies the ExportProjectSnapshot rules inside tx.
// Order matters: memories and prompts first, then what depends on them.
func deleteOtherProjects(ctx context.Context, tx *sql.Tx, tables []snapshotTable, project string) error {
	has := map[string]bool{}
	for _, t := range tables {
		has[t.name] = true
	}
	exec := func(query string, args ...any) error {
		if _, err := tx.ExecContext(ctx, query, args...); err != nil {
			return fmt.Errorf("filter snapshot: %w", err)
		}
		return nil
	}

	steps := []struct {
		table, query string
		args         []any
	}{
		{"observations", `DELETE FROM observations WHERE project IS NOT ?`, []any{project}},
		{"user_prompts", `DELETE FROM user_prompts WHERE project IS NOT ?`, []any{project}},
		{"sessions", `DELETE FROM sessions WHERE project IS NOT ?
			AND id NOT IN (SELECT session_id FROM observations WHERE session_id IS NOT NULL)` +
			sessionsKeptByPrompts(has), []any{project}},
		{"observation_versions", `DELETE FROM observation_versions WHERE observation_id NOT IN (SELECT id FROM observations)`, nil},
		{"memory_relations", `DELETE FROM memory_relations WHERE source_id IS NULL OR target_id IS NULL
			OR source_id NOT IN (` + keptRefsObservations + `)
			OR target_id NOT IN (` + keptRefsObservations + `)`, nil},
		{"memory_relations", `UPDATE memory_relations SET superseded_by_relation_id = NULL
			WHERE superseded_by_relation_id IS NOT NULL
			AND superseded_by_relation_id NOT IN (SELECT id FROM memory_relations)`, nil},
	}
	handled := map[string]bool{"sync_state": true}
	for _, s := range steps {
		handled[s.table] = true
		if !has[s.table] {
			continue
		}
		if err := exec(s.query, s.args...); err != nil {
			return err
		}
	}
	if has["sessions"] {
		if err := blankForeignSessions(ctx, tx, project, exec); err != nil {
			return err
		}
	}

	// Every other plain table: filter by project, or empty it.
	for _, t := range tables {
		if t.virtual || handled[t.name] {
			continue
		}
		if err := checkIdent(t.name); err != nil {
			return err
		}
		cols, err := txColumns(ctx, tx, t.name)
		if err != nil {
			return err
		}
		if cols["project"] {
			err = exec(`DELETE FROM `+quoteIdent(t.name)+` WHERE project IS NOT ?`, project)
		} else {
			err = exec(`DELETE FROM ` + quoteIdent(t.name))
		}
		if err != nil {
			return err
		}
	}

	// sync_state after sync_mutations, whose rows reference it.
	if has["sync_state"] {
		query := `DELETE FROM sync_state WHERE target_key <> 'cloud' AND target_key <> 'cloud:' || ?`
		if has["sync_mutations"] {
			query += ` AND target_key NOT IN (SELECT target_key FROM sync_mutations)`
		}
		if err := exec(query, project); err != nil {
			return err
		}
	}

	return rebuildSnapshotFTS(ctx, tx, tables, exec)
}

// blankForeignSessions clears the summary and directory of the kept sessions
// of other projects (columns missing from older schemas are skipped).
func blankForeignSessions(ctx context.Context, tx *sql.Tx, project string, exec func(string, ...any) error) error {
	cols, err := txColumns(ctx, tx, "sessions")
	if err != nil {
		return err
	}
	var set []string
	if cols["summary"] {
		set = append(set, "summary = NULL")
	}
	if cols["directory"] {
		set = append(set, "directory = ''")
	}
	if len(set) == 0 {
		return nil
	}
	return exec(`UPDATE sessions SET `+strings.Join(set, ", ")+` WHERE project IS NOT ?`, project)
}

// sessionsKeptByPrompts extends the sessions rule to keep sessions of kept
// prompts when the snapshot has a user_prompts table.
func sessionsKeptByPrompts(has map[string]bool) string {
	if !has["user_prompts"] {
		return ""
	}
	return ` AND id NOT IN (SELECT session_id FROM user_prompts WHERE session_id IS NOT NULL)`
}

// rebuildSnapshotFTS rebuilds every external-content FTS5 index from its
// (already filtered) content table. A virtual table of any other kind could
// hold its own copy of other projects' text, so it is emptied.
func rebuildSnapshotFTS(ctx context.Context, tx *sql.Tx, tables []snapshotTable, exec func(string, ...any) error) error {
	for _, t := range tables {
		if !t.virtual {
			continue
		}
		if err := checkIdent(t.name); err != nil {
			return err
		}
		q := quoteIdent(t.name)
		lower := strings.ToLower(t.sql)
		if strings.Contains(lower, "using fts5") && strings.Contains(lower, "content='") && !strings.Contains(lower, "content=''") {
			if err := exec(`INSERT INTO ` + q + `(` + q + `) VALUES ('rebuild')`); err != nil {
				return err
			}
			continue
		}
		if err := exec(`DELETE FROM ` + q); err != nil {
			return err
		}
	}
	return nil
}

// txColumns returns the column names of table (a checked identifier).
func txColumns(ctx context.Context, tx *sql.Tx, table string) (map[string]bool, error) {
	rows, err := tx.QueryContext(ctx, `SELECT name FROM pragma_table_info(?)`, table)
	if err != nil {
		return nil, fmt.Errorf("columns of %s: %w", table, err)
	}
	defer rows.Close()
	cols := map[string]bool{}
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		cols[n] = true
	}
	return cols, rows.Err()
}
