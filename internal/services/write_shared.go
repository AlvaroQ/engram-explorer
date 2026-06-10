package services

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/AlvaroQ/engram-explorer/internal/sqlite"
)

// ---------------------------------------------------------------------------
// Timestamp helpers
// ---------------------------------------------------------------------------

// NowSqlite returns the current UTC time formatted as "YYYY-MM-DD HH:MM:SS",
// matching the Node nowSqlite() helper in shared.ts.
func NowSqlite() string {
	return time.Now().UTC().Format("2006-01-02 15:04:05")
}

// ---------------------------------------------------------------------------
// Sync ID generation
// ---------------------------------------------------------------------------

// GenSyncID generates a stable cloud-sync identifier.
// prefix must be "obs" or "prompt"; the result is "<prefix>-<16 hex chars>".
func GenSyncID(prefix string) string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		// Fallback: use time-based bytes (should never happen in practice).
		t := time.Now().UnixNano()
		for i := 0; i < 8; i++ {
			b[i] = byte(t >> (i * 8))
		}
	}
	return prefix + "-" + hex.EncodeToString(b)
}

// ---------------------------------------------------------------------------
// Enrollment check
// ---------------------------------------------------------------------------

// IsEnrolled returns true when the given project is registered in
// sync_enrolled_projects. Returns false for empty/nil project.
func IsEnrolled(db sqlite.Querier, project string) bool {
	if project == "" {
		return false
	}
	var n int
	err := db.QueryRow(`SELECT 1 FROM sync_enrolled_projects WHERE project = ?`, project).Scan(&n)
	return err == nil && n == 1
}

// IsEnrolledTx is the same check but runs inside an existing transaction.
func IsEnrolledTx(tx *sql.Tx, project string) bool {
	if project == "" {
		return false
	}
	var n int
	err := tx.QueryRow(`SELECT 1 FROM sync_enrolled_projects WHERE project = ?`, project).Scan(&n)
	return err == nil && n == 1
}

// ---------------------------------------------------------------------------
// Sync mutation insert
// ---------------------------------------------------------------------------

// InsertSyncMutationTx inserts a row into sync_mutations inside an existing
// transaction and returns the new row's seq (lastInsertRowid).
func InsertSyncMutationTx(tx *sql.Tx, entity, entityKey, op, payload, project, occurredAt string) (int64, error) {
	res, err := tx.Exec(`
		INSERT INTO sync_mutations
		  (target_key, entity, entity_key, op, payload, source, occurred_at, project)
		VALUES ('cloud', ?, ?, ?, ?, 'local', ?, ?)`,
		entity, entityKey, op, payload, occurredAt, project,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// ---------------------------------------------------------------------------
// Project existence check
// ---------------------------------------------------------------------------

// rowQuerier is implemented by both *sql.DB and *sql.Tx, so the project
// existence check can run either standalone or inside a transaction.
type rowQuerier interface {
	QueryRow(query string, args ...any) *sql.Row
}

// projectExistsSQL is the single source of truth for the project existence
// check: true when any live observation, session, or prompt has the project.
const projectExistsSQL = `
	SELECT 1 FROM (
	  SELECT project FROM observations WHERE project = ? AND deleted_at IS NULL
	  UNION ALL SELECT project FROM sessions WHERE project = ?
	  UNION ALL SELECT project FROM user_prompts WHERE project = ?
	) LIMIT 1`

// projectExistsVia runs the shared project existence check on any rowQuerier.
func projectExistsVia(q rowQuerier, project string) (bool, error) {
	if project == "" {
		return false, nil
	}
	var n int
	err := q.QueryRow(projectExistsSQL, project, project, project).Scan(&n)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return err == nil, err
}

// ProjectExistsTx returns true when any live observation, session, or prompt
// exists for the given project. Runs inside an existing transaction.
func ProjectExistsTx(tx *sql.Tx, project string) (bool, error) {
	return projectExistsVia(tx, project)
}

// ProjectExistsDB is the same check but uses a plain *sql.DB.
func ProjectExistsDB(db sqlite.Querier, project string) (bool, error) {
	return projectExistsVia(db, project)
}

// ---------------------------------------------------------------------------
// compactPayload — mirror of the TS helper in shared.ts:
// drops null/zero-value entries from a map so the JSON payload stays lean.
// ---------------------------------------------------------------------------

// CompactPayload serialises a map to JSON, dropping keys whose value is nil.
func CompactPayload(m map[string]any) string {
	out := make(map[string]any, len(m))
	for k, v := range m {
		if v == nil {
			continue
		}
		out[k] = v
	}
	b, err := json.Marshal(out)
	if err != nil {
		return "{}"
	}
	return string(b)
}

// ---------------------------------------------------------------------------
// Sentinel error helpers
// ---------------------------------------------------------------------------

// WriteError is a typed sentinel used by write services. Callers map the Code
// field to HTTP status codes.
type WriteError struct {
	Code    string // e.g. "NOT_FOUND", "ALREADY_DELETED", "HAS_PROMPTS"
	Message string
}

func (e *WriteError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("%s: %s", e.Code, e.Message)
	}
	return e.Code
}

// NewWriteError constructs a WriteError.
func NewWriteError(code, msg string) *WriteError { return &WriteError{Code: code, Message: msg} }
