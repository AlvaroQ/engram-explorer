package services

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/AlvaroQ/engram-explorer/internal/sqlite"
)

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

// EntityKind mirrors the TS EntityKind type.
type EntityKind string

const (
	EntityKindObservation EntityKind = "observation"
	EntityKindSession     EntityKind = "session"
	EntityKindPrompt      EntityKind = "prompt"
)

// AssignmentResult mirrors the TS AssignmentResult shape.
type AssignmentResult struct {
	Entity      EntityKind `json:"entity"`
	ID          any        `json:"id"` // int64 for obs/prompt, string for session
	Project     string     `json:"project"`
	MutationSeq *int64     `json:"mutation_seq"`
	Enrolled    bool       `json:"enrolled"`
}

// ---------------------------------------------------------------------------
// Internal row types
// ---------------------------------------------------------------------------

type assignObsRow struct {
	id             int64
	sessionID      *string
	typ            string
	title          *string
	content        *string
	toolName       *string
	project        *string
	scope          *string
	topicKey       *string
	normalizedHash *string
	revisionCount  *int64
	duplicateCount *int64
	lastSeenAt     *string
	createdAt      *string
	updatedAt      *string
	syncID         *string
}

type assignSessRow struct {
	id        string
	project   *string
	directory *string
	startedAt *string
	endedAt   *string
	summary   *string
}

type assignPromptRow struct {
	id        int64
	sessionID *string
	content   *string
	project   *string
	createdAt *string
	syncID    *string
}

// ---------------------------------------------------------------------------
// AssignProject
// ---------------------------------------------------------------------------

// AssignProject assigns a single entity (observation, session, or prompt) to a
// project. All mutations run inside a single sql.Tx. Returns WriteError with
// codes NOT_FOUND or BAD_INPUT on expected failures.
func AssignProject(ctx context.Context, db sqlite.Querier, entity EntityKind, id any, project string) (AssignmentResult, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return AssignmentResult{}, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	result, err := assignInTx(tx, entity, id, project)
	if err != nil {
		return AssignmentResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return AssignmentResult{}, fmt.Errorf("commit: %w", err)
	}
	return result, nil
}

func assignInTx(tx *sql.Tx, entity EntityKind, id any, project string) (AssignmentResult, error) {
	enrolled := IsEnrolledTx(tx, project)
	now := NowSqlite()
	var mutationSeq *int64

	switch entity {
	case EntityKindObservation:
		seq, err := assignObservation(tx, id, project, enrolled, now)
		if err != nil {
			return AssignmentResult{}, err
		}
		mutationSeq = seq

	case EntityKindSession:
		sid := fmt.Sprintf("%v", id)
		seq, err := assignSession(tx, sid, project, enrolled, now)
		if err != nil {
			return AssignmentResult{}, err
		}
		mutationSeq = seq

	case EntityKindPrompt:
		seq, err := assignPrompt(tx, id, project, enrolled, now)
		if err != nil {
			return AssignmentResult{}, err
		}
		mutationSeq = seq

	default:
		return AssignmentResult{}, NewWriteError("BAD_INPUT", "unsupported entity kind: "+string(entity))
	}

	return AssignmentResult{
		Entity:      entity,
		ID:          id,
		Project:     project,
		MutationSeq: mutationSeq,
		Enrolled:    enrolled,
	}, nil
}

// ---------------------------------------------------------------------------
// assignObservation — hash-group fan-out, tombstone on leave, upsert on enter
// ---------------------------------------------------------------------------

func scanObsRow(row *sql.Row) (*assignObsRow, error) {
	var r assignObsRow
	err := row.Scan(
		&r.id, &r.sessionID, &r.typ, &r.title, &r.content, &r.toolName,
		&r.project, &r.scope, &r.topicKey, &r.normalizedHash,
		&r.revisionCount, &r.duplicateCount, &r.lastSeenAt, &r.createdAt, &r.updatedAt, &r.syncID,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func scanObsRows(rows *sql.Rows) ([]assignObsRow, error) {
	var out []assignObsRow
	for rows.Next() {
		var r assignObsRow
		if err := rows.Scan(
			&r.id, &r.sessionID, &r.typ, &r.title, &r.content, &r.toolName,
			&r.project, &r.scope, &r.topicKey, &r.normalizedHash,
			&r.revisionCount, &r.duplicateCount, &r.lastSeenAt, &r.createdAt, &r.updatedAt, &r.syncID,
		); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

const assignObsSelect = `SELECT id, session_id, type, title, content, tool_name, project, scope,
	topic_key, normalized_hash, revision_count, duplicate_count,
	last_seen_at, created_at, updated_at, sync_id`

func assignObservation(tx *sql.Tx, id any, project string, enrolled bool, now string) (*int64, error) {
	// 1. Fetch the clicked row.
	clicked, err := scanObsRow(tx.QueryRow(
		assignObsSelect+` FROM observations WHERE id = ? AND deleted_at IS NULL`, id,
	))
	if err != nil {
		return nil, fmt.Errorf("get obs: %w", err)
	}
	if clicked == nil {
		return nil, NewWriteError("NOT_FOUND", fmt.Sprintf("observation %v not found", id))
	}

	// 2. Build hash group.
	group := []assignObsRow{*clicked}
	if clicked.normalizedHash != nil && *clicked.normalizedHash != "" {
		rows, err := tx.Query(
			assignObsSelect+` FROM observations WHERE normalized_hash = ? AND deleted_at IS NULL`,
			*clicked.normalizedHash,
		)
		if err != nil {
			return nil, fmt.Errorf("get hash group: %w", err)
		}
		group, err = scanObsRows(rows)
		rows.Close()
		if err != nil {
			return nil, err
		}
	}

	var mutationSeq *int64

	for _, row := range group {
		rowProject := ""
		if row.project != nil {
			rowProject = *row.project
		}
		// Skip members already in the target project.
		if rowProject == project {
			continue
		}

		// Backfill sync_id.
		syncID := ""
		if row.syncID != nil {
			syncID = *row.syncID
		}
		if syncID == "" {
			syncID = GenSyncID("obs")
			if _, err := tx.Exec(`UPDATE observations SET sync_id = ? WHERE id = ?`, syncID, row.id); err != nil {
				return nil, fmt.Errorf("backfill sync_id obs %d: %w", row.id, err)
			}
		}

		// Tombstone: if leaving an enrolled project, emit delete.
		if rowProject != "" && IsEnrolledTx(tx, rowProject) {
			if _, err := InsertSyncMutationTx(tx, "observation", syncID, "delete", "{}", rowProject, now); err != nil {
				return nil, fmt.Errorf("tombstone mutation obs %d: %w", row.id, err)
			}
		}

		// Update project.
		if _, err := tx.Exec(
			`UPDATE observations SET project = ?, updated_at = ? WHERE id = ?`,
			project, now, row.id,
		); err != nil {
			return nil, fmt.Errorf("set project obs %d: %w", row.id, err)
		}

		// Upsert mutation for target project if enrolled.
		if enrolled {
			payload := CompactPayload(map[string]any{
				"sync_id":         syncID,
				"session_id":      row.sessionID,
				"type":            row.typ,
				"title":           row.title,
				"content":         row.content,
				"tool_name":       row.toolName,
				"project":         project,
				"scope":           row.scope,
				"topic_key":       row.topicKey,
				"revision_count":  row.revisionCount,
				"duplicate_count": row.duplicateCount,
				"last_seen_at":    row.lastSeenAt,
				"created_at":      row.createdAt,
				"updated_at":      now,
			})
			seq, err := InsertSyncMutationTx(tx, "observation", syncID, "upsert", payload, project, now)
			if err != nil {
				return nil, fmt.Errorf("upsert mutation obs %d: %w", row.id, err)
			}
			mutationSeq = &seq
		}
	}

	return mutationSeq, nil
}

// ---------------------------------------------------------------------------
// assignSession
// ---------------------------------------------------------------------------

func assignSession(tx *sql.Tx, sid, project string, enrolled bool, now string) (*int64, error) {
	var row assignSessRow
	err := tx.QueryRow(
		`SELECT id, project, directory, started_at, ended_at, summary FROM sessions WHERE id = ?`, sid,
	).Scan(&row.id, &row.project, &row.directory, &row.startedAt, &row.endedAt, &row.summary)
	if err == sql.ErrNoRows {
		return nil, NewWriteError("NOT_FOUND", "session "+sid+" not found")
	}
	if err != nil {
		return nil, fmt.Errorf("get session: %w", err)
	}

	if _, err := tx.Exec(`UPDATE sessions SET project = ? WHERE id = ?`, project, sid); err != nil {
		return nil, fmt.Errorf("set project session: %w", err)
	}

	var mutationSeq *int64
	if enrolled {
		payload := CompactPayload(map[string]any{
			"id":         sid,
			"project":    project,
			"directory":  row.directory,
			"started_at": row.startedAt,
			"ended_at":   row.endedAt,
			"summary":    row.summary,
		})
		seq, err := InsertSyncMutationTx(tx, "session", sid, "upsert", payload, project, now)
		if err != nil {
			return nil, fmt.Errorf("upsert mutation session: %w", err)
		}
		mutationSeq = &seq
	}
	return mutationSeq, nil
}

// ---------------------------------------------------------------------------
// assignPrompt
// ---------------------------------------------------------------------------

func assignPrompt(tx *sql.Tx, id any, project string, enrolled bool, now string) (*int64, error) {
	var row assignPromptRow
	err := tx.QueryRow(
		`SELECT id, session_id, content, project, created_at, sync_id FROM user_prompts WHERE id = ?`, id,
	).Scan(&row.id, &row.sessionID, &row.content, &row.project, &row.createdAt, &row.syncID)
	if err == sql.ErrNoRows {
		return nil, NewWriteError("NOT_FOUND", fmt.Sprintf("prompt %v not found", id))
	}
	if err != nil {
		return nil, fmt.Errorf("get prompt: %w", err)
	}

	// Backfill sync_id.
	syncID := ""
	if row.syncID != nil {
		syncID = *row.syncID
	}
	if syncID == "" {
		syncID = GenSyncID("prompt")
		if _, err := tx.Exec(`UPDATE user_prompts SET sync_id = ? WHERE id = ?`, syncID, row.id); err != nil {
			return nil, fmt.Errorf("backfill sync_id prompt %d: %w", row.id, err)
		}
	}

	if _, err := tx.Exec(`UPDATE user_prompts SET project = ? WHERE id = ?`, project, row.id); err != nil {
		return nil, fmt.Errorf("set project prompt: %w", err)
	}

	var mutationSeq *int64
	if enrolled {
		payload := CompactPayload(map[string]any{
			"sync_id":    syncID,
			"session_id": row.sessionID,
			"content":    row.content,
			"project":    project,
			"created_at": row.createdAt,
		})
		seq, err := InsertSyncMutationTx(tx, "prompt", syncID, "upsert", payload, project, now)
		if err != nil {
			return nil, fmt.Errorf("upsert mutation prompt: %w", err)
		}
		mutationSeq = &seq
	}
	return mutationSeq, nil
}
