package services

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/AlvaroQ/engram-explorer/internal/sqlite"
)

// Empty sessions (Maintenance → "Empty sessions").
//
// A session is EMPTY when it has:
//   - no summary (NULL or whitespace only),
//   - no observations at all — soft-deleted observations count as content, so
//     a session whose only memories are soft-deleted is NOT empty (restoring or
//     purging deleted memories is a separate concern, and removing the session
//     would break the session_id reference of those rows),
//   - no user prompts.
//
// An empty session is DELETABLE when, in addition:
//   - its project is not enrolled for cloud sync (same rule as DeleteProject:
//     enrolled projects are never touched locally, so no sync mutation is
//     needed and cloud state cannot be left dangling), and
//   - it started more than emptySessionGrace ago: a session an agent opened a
//     moment ago is empty until its first memory lands, and deleting it would
//     break the live agent. A started_at that SQLite cannot parse is treated as
//     recent (kept).
//
// The predicates are SQL fragments over the alias "s" shared by the list and
// the delete, so the delete re-checks exactly what the list showed.
const (
	emptySessionGrace = "-1 day"

	emptySessionSQL = `(s.summary IS NULL OR TRIM(s.summary) = '')
	  AND NOT EXISTS (SELECT 1 FROM observations o WHERE o.session_id = s.id)
	  AND NOT EXISTS (SELECT 1 FROM user_prompts p WHERE p.session_id = s.id)`

	emptySessionEnrolledSQL = `EXISTS (SELECT 1 FROM sync_enrolled_projects e WHERE e.project = s.project)`

	emptySessionOldSQL = `COALESCE(datetime(s.started_at) < datetime('now', '` + emptySessionGrace + `'), 0)`

	emptySessionDeletableSQL = emptySessionSQL + ` AND NOT ` + emptySessionEnrolledSQL + ` AND ` + emptySessionOldSQL
)

// EmptySessionsDefaultLimit bounds the rows the Maintenance list renders.
const EmptySessionsDefaultLimit = 50

// EmptySessionRow is one deletable empty session.
type EmptySessionRow struct {
	ID        string `json:"id"`
	Project   string `json:"project"`
	Directory string `json:"directory"`
	StartedAt string `json:"startedAt"`
}

// EmptySessionsReport is the detection result.
type EmptySessionsReport struct {
	// Total counts every deletable empty session (not only the listed ones).
	Total int `json:"total"`
	// SkippedEnrolled counts empty sessions kept because their project is
	// enrolled for cloud sync.
	SkippedEnrolled int `json:"skippedEnrolled"`
	// SkippedRecent counts empty sessions kept because they started within the
	// grace window (or have no parsable start time).
	SkippedRecent int `json:"skippedRecent"`
	// Limit is the cap applied to Sessions.
	Limit int `json:"limit"`
	// Sessions holds the first Limit deletable empty sessions, oldest first.
	Sessions []EmptySessionRow `json:"sessions"`
}

// DeleteEmptySessionsResult reports how many sessions the bulk delete removed.
type DeleteEmptySessionsResult struct {
	Deleted int64 `json:"deleted"`
}

// ListEmptySessions returns the deletable empty sessions (bounded by limit; a
// non-positive limit uses EmptySessionsDefaultLimit) and the skipped counts.
func ListEmptySessions(db sqlite.Querier, limit int) (EmptySessionsReport, error) {
	if limit <= 0 {
		limit = EmptySessionsDefaultLimit
	}
	rep := EmptySessionsReport{Limit: limit, Sessions: []EmptySessionRow{}}

	var total, enrolled, recent sql.NullInt64
	if err := db.QueryRow(`
		SELECT
		  SUM(CASE WHEN NOT `+emptySessionEnrolledSQL+` AND `+emptySessionOldSQL+` THEN 1 ELSE 0 END),
		  SUM(CASE WHEN `+emptySessionEnrolledSQL+` THEN 1 ELSE 0 END),
		  SUM(CASE WHEN NOT `+emptySessionEnrolledSQL+` AND NOT `+emptySessionOldSQL+` THEN 1 ELSE 0 END)
		FROM sessions s
		WHERE `+emptySessionSQL,
	).Scan(&total, &enrolled, &recent); err != nil {
		return EmptySessionsReport{}, fmt.Errorf("count empty sessions: %w", err)
	}
	rep.Total, rep.SkippedEnrolled, rep.SkippedRecent = int(total.Int64), int(enrolled.Int64), int(recent.Int64)

	rows, err := queryRows(db, `
		SELECT s.id, COALESCE(s.project, ''), COALESCE(s.directory, ''), COALESCE(s.started_at, '')
		FROM sessions s
		WHERE `+emptySessionDeletableSQL+`
		ORDER BY s.started_at ASC, s.id ASC
		LIMIT ?`,
		[]any{limit},
		func(sc scanner) (EmptySessionRow, error) {
			var r EmptySessionRow
			err := sc.Scan(&r.ID, &r.Project, &r.Directory, &r.StartedAt)
			return r, err
		},
	)
	if err != nil {
		return EmptySessionsReport{}, fmt.Errorf("list empty sessions: %w", err)
	}
	rep.Sessions = rows
	return rep, nil
}

// DeleteEmptySessions deletes, inside one transaction, every session that is
// deletable-empty at delete time. It never takes a client-supplied id list:
// the DELETE re-evaluates the predicate, so a session that gained a prompt or
// an observation after it was listed is kept.
func DeleteEmptySessions(ctx context.Context, db sqlite.Querier) (DeleteEmptySessionsResult, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return DeleteEmptySessionsResult{}, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	res, err := tx.ExecContext(ctx,
		`DELETE FROM sessions WHERE id IN (SELECT s.id FROM sessions s WHERE `+emptySessionDeletableSQL+`)`)
	if err != nil {
		return DeleteEmptySessionsResult{}, fmt.Errorf("delete empty sessions: %w", err)
	}
	n, _ := res.RowsAffected()
	if err := tx.Commit(); err != nil {
		return DeleteEmptySessionsResult{}, fmt.Errorf("commit: %w", err)
	}
	return DeleteEmptySessionsResult{Deleted: n}, nil
}
