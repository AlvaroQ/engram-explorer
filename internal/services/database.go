package services

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// Error types
// ---------------------------------------------------------------------------

// DatabaseImportCode enumerates import-specific error codes.
type DatabaseImportCode string

const (
	DatabaseImportCodeBadSQLite      DatabaseImportCode = "BAD_SQLITE"
	DatabaseImportCodeIntegrity      DatabaseImportCode = "INTEGRITY"
	DatabaseImportCodeSchemaMismatch DatabaseImportCode = "SCHEMA_MISMATCH"
	DatabaseImportCodeBusy           DatabaseImportCode = "BUSY"
)

// DatabaseImportError is a typed error returned by ImportMerge.
type DatabaseImportError struct {
	Code    DatabaseImportCode
	Message string
}

func (e *DatabaseImportError) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// ---------------------------------------------------------------------------
// Result types
// ---------------------------------------------------------------------------

// MergeCounts holds per-table inserted/skipped counts.
type MergeCounts struct {
	Observations int64 `json:"observations"`
	Sessions     int64 `json:"sessions"`
	Prompts      int64 `json:"prompts"`
	Relations    int64 `json:"relations"`
}

// MergeResult is the response body for /api/db/import.
type MergeResult struct {
	BackupPath string      `json:"backupPath"`
	Inserted   MergeCounts `json:"inserted"`
	Skipped    MergeCounts `json:"skipped"`
}

// ---------------------------------------------------------------------------
// contentTables enumerates the tables we copy during import.
// Sync-state tables are deliberately excluded.
// ---------------------------------------------------------------------------

// contentTables are the tables included in export/import.
// memory_relations is optional — older engram schemas may not have it.
var contentTables = []string{"sessions", "observations", "user_prompts", "memory_relations"}

// requiredTables is the subset that MUST be present for schema validation to pass.
var requiredTables = []string{"sessions", "observations", "user_prompts"}

// ---------------------------------------------------------------------------
// ExportSnapshot
// ---------------------------------------------------------------------------

// ExportSnapshot creates a consistent read-only snapshot of the DB at rwDB
// using VACUUM INTO and writes it to a temp file. Returns the temp file path.
// The caller is responsible for deleting the temp file.
func ExportSnapshot(ctx context.Context, rwDB *sql.DB) (string, error) {
	dest := filepath.Join(os.TempDir(), fmt.Sprintf("engram-export-%s.db", fileTimestamp()))

	// VACUUM INTO works with WAL mode and is safe for online backup.
	_, err := rwDB.ExecContext(ctx, `VACUUM INTO ?`, dest)
	if err != nil {
		_ = os.Remove(dest)
		return "", fmt.Errorf("VACUUM INTO: %w", err)
	}
	return dest, nil
}

// fileTimestamp returns a sortable timestamp string safe for filenames.
func fileTimestamp() string {
	return time.Now().UTC().Format("2006-01-02T15-04-05Z")
}

// ---------------------------------------------------------------------------
// ImportMerge
// ---------------------------------------------------------------------------

// ImportMerge validates uploadPath as an engram SQLite database, takes a
// pre-import safety backup, then merges it into rwDB inside a single pinned
// connection. Returns MergeResult on success or a DatabaseImportError on
// known failures.
func ImportMerge(ctx context.Context, rwDB *sql.DB, uploadPath, dataDir string) (MergeResult, error) {
	// 1. Validate schema before touching the live DB.
	if err := validateImportSchema(ctx, rwDB, uploadPath); err != nil {
		return MergeResult{}, err
	}

	// 2. Safety backup via VACUUM INTO.
	backupPath := filepath.Join(dataDir,
		fmt.Sprintf("engram-backup-before-import-%s.db", fileTimestamp()),
	)
	if _, err := rwDB.ExecContext(ctx, `VACUUM INTO ?`, backupPath); err != nil {
		return MergeResult{}, fmt.Errorf("pre-import backup: %w", err)
	}

	// 3. Pin one connection for ATTACH → transaction → DETACH sequence.
	conn, err := rwDB.Conn(ctx)
	if err != nil {
		return MergeResult{}, fmt.Errorf("pin conn: %w", err)
	}
	defer conn.Close()

	// ATTACH cannot run inside a transaction.
	if _, err := conn.ExecContext(ctx, `ATTACH DATABASE ? AS srcdb`, uploadPath); err != nil {
		return MergeResult{}, &DatabaseImportError{
			Code:    DatabaseImportCodeBusy,
			Message: "could not attach the uploaded database (the engram daemon may be writing)",
		}
	}

	// 4. Run the data-copy merge inside an IMMEDIATE transaction.
	// FTS rebuild is done separately AFTER the DETACH because FTS5 shadow-table
	// updates interact poorly with an ATTACHed second database on the same connection.
	tx, err := conn.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		_, _ = conn.ExecContext(context.Background(), `DETACH DATABASE srcdb`)
		if isSQLiteBusy(err) {
			return MergeResult{}, &DatabaseImportError{
				Code:    DatabaseImportCodeBusy,
				Message: "The database is busy (the engram daemon may be writing). Please try again.",
			}
		}
		return MergeResult{}, fmt.Errorf("begin merge tx: %w", err)
	}

	result, err := runMerge(ctx, conn, tx, uploadPath, backupPath)
	if err != nil {
		tx.Rollback()
		_, _ = conn.ExecContext(context.Background(), `DETACH DATABASE srcdb`)
		return MergeResult{}, err
	}

	if err := tx.Commit(); err != nil {
		_, _ = conn.ExecContext(context.Background(), `DETACH DATABASE srcdb`)
		if isSQLiteBusy(err) {
			return MergeResult{}, &DatabaseImportError{
				Code:    DatabaseImportCodeBusy,
				Message: "The database is busy (the engram daemon may be writing). Please try again.",
			}
		}
		return MergeResult{}, fmt.Errorf("commit merge tx: %w", err)
	}

	// 5. DETACH before FTS rebuild (avoids FTS5 shadow-table conflict).
	_, _ = conn.ExecContext(context.Background(), `DETACH DATABASE srcdb`)

	// 6. Rebuild FTS indexes in a separate transaction on the pinned connection.
	if err := rebuildFTSIndexes(ctx, conn); err != nil {
		// FTS rebuild failure is non-fatal — data is committed; log and continue.
		// Callers can rebuild FTS by running `INSERT INTO observations_fts(observations_fts) VALUES('rebuild')`.
		_ = err
	}

	return result, nil
}

// ---------------------------------------------------------------------------
// validateImportSchema
// ---------------------------------------------------------------------------

func validateImportSchema(ctx context.Context, rwDB *sql.DB, uploadPath string) error {
	// Verify SQLite magic header (16 bytes: "SQLite format 3\000").
	f, err := os.Open(uploadPath)
	if err != nil {
		return &DatabaseImportError{Code: DatabaseImportCodeBadSQLite, Message: "could not open the uploaded file"}
	}
	header := make([]byte, 16)
	_, readErr := f.Read(header)
	f.Close()
	if readErr != nil || string(header[:15]) != "SQLite format 3" {
		return &DatabaseImportError{
			Code:    DatabaseImportCodeBadSQLite,
			Message: "The uploaded file is not a valid SQLite database.",
		}
	}

	// Open the source DB read-only via a temporary ATTACH on a separate conn.
	// We can't use the normal driver from here because we need PRAGMA queries.
	// Instead, pin a connection, attach srcdb, run pragmas, detach.
	conn, err := rwDB.Conn(ctx)
	if err != nil {
		return fmt.Errorf("pin conn for validation: %w", err)
	}
	defer conn.Close()

	if _, err := conn.ExecContext(ctx, `ATTACH DATABASE ? AS valdb`, uploadPath); err != nil {
		return &DatabaseImportError{
			Code:    DatabaseImportCodeBadSQLite,
			Message: "The uploaded file is not a valid SQLite database.",
		}
	}
	defer func() {
		_, _ = conn.ExecContext(context.Background(), `DETACH DATABASE valdb`)
	}()

	// quick_check on the source.
	var qc string
	if err := conn.QueryRowContext(ctx, `PRAGMA valdb.quick_check`).Scan(&qc); err != nil || qc != "ok" {
		return &DatabaseImportError{
			Code:    DatabaseImportCodeIntegrity,
			Message: "The uploaded database failed an integrity check.",
		}
	}

	// Verify required tables exist and columns match.
	for _, table := range requiredTables {
		srcCols, err := pragmaTableInfoCols(ctx, conn, "valdb", table)
		if err != nil || len(srcCols) == 0 {
			return &DatabaseImportError{
				Code:    DatabaseImportCodeSchemaMismatch,
				Message: fmt.Sprintf(`The uploaded database is missing the "%s" table. It does not look like an engram database.`, table),
			}
		}
		localCols, err := pragmaTableInfoCols(ctx, conn, "main", table)
		if err != nil {
			// Table may not exist locally yet — skip schema check.
			continue
		}
		if !stringSlicesMatch(localCols, srcCols) {
			return &DatabaseImportError{
				Code:    DatabaseImportCodeSchemaMismatch,
				Message: fmt.Sprintf(`Schema mismatch on "%s". The uploaded database was likely created by a different engram version.`, table),
			}
		}
	}
	// memory_relations is optional — check only if present in both source and local.
	{
		srcCols, err := pragmaTableInfoCols(ctx, conn, "valdb", "memory_relations")
		if err == nil && len(srcCols) > 0 {
			localCols, err := pragmaTableInfoCols(ctx, conn, "main", "memory_relations")
			if err == nil && len(localCols) > 0 && !stringSlicesMatch(localCols, srcCols) {
				return &DatabaseImportError{
					Code:    DatabaseImportCodeSchemaMismatch,
					Message: `Schema mismatch on "memory_relations". The uploaded database was likely created by a different engram version.`,
				}
			}
		}
	}
	return nil
}

// pragmaTableInfoCols returns sorted column names for a table in the given
// schema (e.g. "main" or "valdb") via PRAGMA <schema>.table_info(<table>).
func pragmaTableInfoCols(ctx context.Context, conn *sql.Conn, schema, table string) ([]string, error) {
	if err := checkIdent(schema, table); err != nil {
		return nil, err
	}
	rows, err := conn.QueryContext(ctx, fmt.Sprintf(`PRAGMA %s.table_info(%s)`, schema, table))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	// PRAGMA table_info columns: cid, name, type, notnull, dflt_value, pk
	nameIdx := -1
	for i, c := range cols {
		if c == "name" {
			nameIdx = i
			break
		}
	}
	if nameIdx < 0 {
		return nil, errors.New("unexpected PRAGMA table_info schema")
	}

	vals := make([]any, len(cols))
	ptrs := make([]any, len(cols))
	for i := range vals {
		ptrs[i] = &vals[i]
	}

	var names []string
	for rows.Next() {
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		if n, ok := vals[nameIdx].(string); ok {
			names = append(names, n)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return sortedStrings(names), nil
}

func sortedStrings(s []string) []string {
	// simple insertion sort — tables are small
	out := make([]string, len(s))
	copy(out, s)
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

func stringSlicesMatch(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// ---------------------------------------------------------------------------
// runMerge — the actual INSERT OR IGNORE logic
// ---------------------------------------------------------------------------

func runMerge(ctx context.Context, conn *sql.Conn, tx *sql.Tx, uploadPath, backupPath string) (MergeResult, error) {
	var inserted, skipped MergeCounts

	// --- sessions ---
	{
		cols, err := pragmaTableInfoCols(ctx, conn, "srcdb", "sessions")
		if err != nil {
			return MergeResult{}, fmt.Errorf("cols sessions: %w", err)
		}
		list := strings.Join(cols, ", ")
		srcTotal, err := countRows(tx, "srcdb.sessions")
		if err != nil {
			return MergeResult{}, err
		}
		before, err := countRows(tx, "sessions")
		if err != nil {
			return MergeResult{}, err
		}
		if _, err := tx.ExecContext(ctx,
			fmt.Sprintf(`INSERT OR IGNORE INTO sessions (%s) SELECT %s FROM srcdb.sessions`, list, list),
		); err != nil {
			return MergeResult{}, fmt.Errorf("insert sessions: %w", err)
		}
		after, err := countRows(tx, "sessions")
		if err != nil {
			return MergeResult{}, err
		}
		inserted.Sessions = after - before
		skipped.Sessions = srcTotal - inserted.Sessions
	}

	// --- observations ---
	// Dedupe by sync_id (preferred), then by normalized_hash, then import if both nil.
	{
		cols, err := pragmaTableInfoCols(ctx, conn, "srcdb", "observations")
		if err != nil {
			return MergeResult{}, fmt.Errorf("cols observations: %w", err)
		}
		// Exclude the auto-increment PK.
		cols = filterOut(cols, "id")
		list := strings.Join(cols, ", ")
		where := `
			(sync_id IS NOT NULL AND sync_id NOT IN (SELECT sync_id FROM observations WHERE sync_id IS NOT NULL))
			OR (sync_id IS NULL AND normalized_hash IS NOT NULL
				AND normalized_hash NOT IN (SELECT normalized_hash FROM observations WHERE normalized_hash IS NOT NULL))
			OR (sync_id IS NULL AND normalized_hash IS NULL)`
		srcTotal, err := countRows(tx, "srcdb.observations")
		if err != nil {
			return MergeResult{}, err
		}
		res, err := tx.ExecContext(ctx,
			fmt.Sprintf(`INSERT INTO observations (%s) SELECT %s FROM srcdb.observations WHERE %s`, list, list, where),
		)
		if err != nil {
			return MergeResult{}, fmt.Errorf("insert observations: %w", err)
		}
		n, _ := res.RowsAffected()
		inserted.Observations = n
		skipped.Observations = srcTotal - n
	}

	// --- user_prompts ---
	// Dedupe by sync_id; rows without one are always imported.
	{
		cols, err := pragmaTableInfoCols(ctx, conn, "srcdb", "user_prompts")
		if err != nil {
			return MergeResult{}, fmt.Errorf("cols user_prompts: %w", err)
		}
		cols = filterOut(cols, "id")
		list := strings.Join(cols, ", ")
		where := `
			(sync_id IS NOT NULL AND sync_id NOT IN (SELECT sync_id FROM user_prompts WHERE sync_id IS NOT NULL))
			OR sync_id IS NULL`
		srcTotal, err := countRows(tx, "srcdb.user_prompts")
		if err != nil {
			return MergeResult{}, err
		}
		res, err := tx.ExecContext(ctx,
			fmt.Sprintf(`INSERT INTO user_prompts (%s) SELECT %s FROM srcdb.user_prompts WHERE %s`, list, list, where),
		)
		if err != nil {
			return MergeResult{}, fmt.Errorf("insert user_prompts: %w", err)
		}
		n, _ := res.RowsAffected()
		inserted.Prompts = n
		skipped.Prompts = srcTotal - n
	}

	// --- memory_relations --- (optional table — skip if not present in src)
	{
		hasSrc, err := tableExistsTx(tx, "srcdb.sqlite_master", "memory_relations")
		if err != nil {
			return MergeResult{}, err
		}
		if hasSrc {
			cols, err := pragmaTableInfoCols(ctx, conn, "srcdb", "memory_relations")
			if err != nil {
				return MergeResult{}, fmt.Errorf("cols memory_relations: %w", err)
			}
			cols = filterOut(cols, "id")
			list := strings.Join(cols, ", ")
			// superseded_by_relation_id is a local FK — map to NULL.
			selectCols := make([]string, len(cols))
			for i, c := range cols {
				if c == "superseded_by_relation_id" {
					selectCols[i] = "NULL"
				} else {
					selectCols[i] = c
				}
			}
			selectList := strings.Join(selectCols, ", ")
			where := `
				(sync_id IS NOT NULL AND sync_id NOT IN (SELECT sync_id FROM memory_relations WHERE sync_id IS NOT NULL))
				OR sync_id IS NULL`
			srcTotal, err := countRows(tx, "srcdb.memory_relations")
			if err != nil {
				return MergeResult{}, err
			}
			// Ensure local table exists before inserting.
			hasLocal, err := tableExistsTx(tx, "sqlite_master", "memory_relations")
			if err != nil {
				return MergeResult{}, err
			}
			if hasLocal {
				res, err := tx.ExecContext(ctx,
					fmt.Sprintf(`INSERT INTO memory_relations (%s) SELECT %s FROM srcdb.memory_relations WHERE %s`,
						list, selectList, where),
				)
				if err != nil {
					return MergeResult{}, fmt.Errorf("insert memory_relations: %w", err)
				}
				n, _ := res.RowsAffected()
				inserted.Relations = n
				skipped.Relations = srcTotal - n
			} else {
				skipped.Relations = srcTotal
			}
		}
	}

	return MergeResult{
		BackupPath: backupPath,
		Inserted:   inserted,
		Skipped:    skipped,
	}, nil
}

// ---------------------------------------------------------------------------
// rebuildFTSIndexes rebuilds FTS5 indexes from the base tables.
// Must be called AFTER the ATTACH database has been DETACHed.
// ---------------------------------------------------------------------------

func rebuildFTSIndexes(ctx context.Context, conn *sql.Conn) error {
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin fts rebuild tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	if _, err := tx.ExecContext(ctx, `DELETE FROM observations_fts`); err != nil {
		return fmt.Errorf("clear observations_fts: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO observations_fts(rowid, title, content, tool_name, type, project, topic_key)
		 SELECT id, title, content, tool_name, type, project, topic_key FROM observations`,
	); err != nil {
		return fmt.Errorf("rebuild observations_fts: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM prompts_fts`); err != nil {
		return fmt.Errorf("clear prompts_fts: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO prompts_fts(rowid, content, project) SELECT id, content, project FROM user_prompts`,
	); err != nil {
		return fmt.Errorf("rebuild prompts_fts: %w", err)
	}
	return tx.Commit()
}

// ---------------------------------------------------------------------------
// Small helpers
// ---------------------------------------------------------------------------

// safeIdentRE matches a bare or schema-qualified SQL identifier (e.g. "sessions"
// or "srcdb.sessions"). SQLite cannot bind identifiers as parameters, so the few
// helpers that must interpolate a table/schema name into PRAGMA / FROM clauses
// validate against this allow-list first. All current callers pass compile-time
// constants; this is defense-in-depth against future misuse.
var safeIdentRE = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*(\.[A-Za-z_][A-Za-z0-9_]*)?$`)

// checkIdent returns an error if any part is not a safe SQL identifier.
func checkIdent(idents ...string) error {
	for _, id := range idents {
		if !safeIdentRE.MatchString(id) {
			return fmt.Errorf("unsafe SQL identifier %q", id)
		}
	}
	return nil
}

func countRows(tx *sql.Tx, qualifiedTable string) (int64, error) {
	if err := checkIdent(qualifiedTable); err != nil {
		return 0, err
	}
	var n int64
	err := tx.QueryRow(fmt.Sprintf(`SELECT COUNT(*) FROM %s`, qualifiedTable)).Scan(&n)
	return n, err
}

func tableExistsTx(tx *sql.Tx, masterTable, name string) (bool, error) {
	if err := checkIdent(masterTable); err != nil {
		return false, err
	}
	var n int
	err := tx.QueryRow(
		fmt.Sprintf(`SELECT COUNT(*) FROM %s WHERE type = 'table' AND name = ?`, masterTable), name,
	).Scan(&n)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

func filterOut(s []string, exclude string) []string {
	out := s[:0:len(s)]
	for _, v := range s {
		if v != exclude {
			out = append(out, v)
		}
	}
	return out
}

func isSQLiteBusy(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "SQLITE_BUSY") || strings.Contains(msg, "database is locked")
}
