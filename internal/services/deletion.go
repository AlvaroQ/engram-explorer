package services

import (
	"context"
	"database/sql"
	"fmt"
)

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

// DeletionResult mirrors the TS DeletionResult shape.
type DeletionResult struct {
	Entity      EntityKind `json:"entity"`
	ID          any        `json:"id"`
	Mode        string     `json:"mode"` // "soft" | "hard"
	MutationSeq *int64     `json:"mutation_seq"`
	Enrolled    bool       `json:"enrolled"`
}

// ---------------------------------------------------------------------------
// DeleteEntity
// ---------------------------------------------------------------------------

// DeleteEntity soft-deletes observations and hard-deletes sessions/prompts.
// Returns WriteError with codes: NOT_FOUND, ALREADY_DELETED, HAS_PROMPTS,
// HAS_OBSERVATIONS.
func DeleteEntity(ctx context.Context, db *sql.DB, entity EntityKind, id any) (DeletionResult, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return DeletionResult{}, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	result, err := deleteInTx(tx, entity, id)
	if err != nil {
		return DeletionResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return DeletionResult{}, fmt.Errorf("commit: %w", err)
	}
	return result, nil
}

func deleteInTx(tx *sql.Tx, entity EntityKind, id any) (DeletionResult, error) {
	now := NowSqlite()

	switch entity {
	case EntityKindObservation:
		return deleteObservation(tx, id, now)
	case EntityKindSession:
		sid := fmt.Sprintf("%v", id)
		return deleteSession(tx, sid, now)
	case EntityKindPrompt:
		return deletePrompt(tx, id, now)
	default:
		return DeletionResult{}, NewWriteError("BAD_INPUT", "unsupported entity: "+string(entity))
	}
}

// ---------------------------------------------------------------------------
// deleteObservation — soft delete
// ---------------------------------------------------------------------------

func deleteObservation(tx *sql.Tx, id any, now string) (DeletionResult, error) {
	var obsID int64
	var project, syncID, deletedAt *string
	err := tx.QueryRow(
		`SELECT id, project, sync_id, deleted_at FROM observations WHERE id = ?`, id,
	).Scan(&obsID, &project, &syncID, &deletedAt)
	if err == sql.ErrNoRows {
		return DeletionResult{}, NewWriteError("NOT_FOUND", fmt.Sprintf("observation %v not found", id))
	}
	if err != nil {
		return DeletionResult{}, fmt.Errorf("get obs: %w", err)
	}
	if deletedAt != nil {
		return DeletionResult{}, NewWriteError("ALREADY_DELETED", fmt.Sprintf("observation %v is already deleted", id))
	}

	if _, err := tx.Exec(
		`UPDATE observations SET deleted_at = ?, updated_at = ? WHERE id = ?`,
		now, now, obsID,
	); err != nil {
		return DeletionResult{}, fmt.Errorf("soft-delete obs: %w", err)
	}

	var mutationSeq *int64
	projStr := ptrStr(project)
	enrolled := IsEnrolledTx(tx, projStr)
	if enrolled && syncID != nil && *syncID != "" && projStr != "" {
		seq, err := InsertSyncMutationTx(tx, "observation", *syncID, "delete", "{}", projStr, now)
		if err != nil {
			return DeletionResult{}, fmt.Errorf("delete mutation: %w", err)
		}
		mutationSeq = &seq
	}
	return DeletionResult{
		Entity:      EntityKindObservation,
		ID:          id,
		Mode:        "soft",
		MutationSeq: mutationSeq,
		Enrolled:    enrolled,
	}, nil
}

// ---------------------------------------------------------------------------
// deleteSession — hard delete (guards: HAS_PROMPTS, HAS_OBSERVATIONS)
// ---------------------------------------------------------------------------

func deleteSession(tx *sql.Tx, sid, now string) (DeletionResult, error) {
	var project *string
	err := tx.QueryRow(`SELECT project FROM sessions WHERE id = ?`, sid).Scan(&project)
	if err == sql.ErrNoRows {
		return DeletionResult{}, NewWriteError("NOT_FOUND", "session "+sid+" not found")
	}
	if err != nil {
		return DeletionResult{}, fmt.Errorf("get session: %w", err)
	}

	// Guard: HAS_PROMPTS
	var promptCount int64
	if err := tx.QueryRow(`SELECT COUNT(*) FROM user_prompts WHERE session_id = ?`, sid).Scan(&promptCount); err != nil {
		return DeletionResult{}, fmt.Errorf("count prompts: %w", err)
	}
	if promptCount > 0 {
		return DeletionResult{}, NewWriteError("HAS_PROMPTS", "cannot delete session: it has user_prompts attached")
	}

	// Guard: HAS_OBSERVATIONS
	var obsCount int64
	if err := tx.QueryRow(
		`SELECT COUNT(*) FROM observations WHERE session_id = ? AND deleted_at IS NULL`, sid,
	).Scan(&obsCount); err != nil {
		return DeletionResult{}, fmt.Errorf("count obs: %w", err)
	}
	if obsCount > 0 {
		return DeletionResult{}, NewWriteError("HAS_OBSERVATIONS", "cannot delete session: it has active observations")
	}

	if _, err := tx.Exec(`DELETE FROM sessions WHERE id = ?`, sid); err != nil {
		return DeletionResult{}, fmt.Errorf("hard-delete session: %w", err)
	}

	var mutationSeq *int64
	projStr := ptrStr(project)
	enrolled := IsEnrolledTx(tx, projStr)
	if enrolled && projStr != "" {
		seq, err := InsertSyncMutationTx(tx, "session", sid, "delete", "{}", projStr, now)
		if err != nil {
			return DeletionResult{}, fmt.Errorf("delete mutation session: %w", err)
		}
		mutationSeq = &seq
	}
	return DeletionResult{
		Entity:      EntityKindSession,
		ID:          sid,
		Mode:        "hard",
		MutationSeq: mutationSeq,
		Enrolled:    enrolled,
	}, nil
}

// ---------------------------------------------------------------------------
// deletePrompt — hard delete
// ---------------------------------------------------------------------------

func deletePrompt(tx *sql.Tx, id any, now string) (DeletionResult, error) {
	var promptID int64
	var project, syncID *string
	err := tx.QueryRow(`SELECT id, project, sync_id FROM user_prompts WHERE id = ?`, id).
		Scan(&promptID, &project, &syncID)
	if err == sql.ErrNoRows {
		return DeletionResult{}, NewWriteError("NOT_FOUND", fmt.Sprintf("prompt %v not found", id))
	}
	if err != nil {
		return DeletionResult{}, fmt.Errorf("get prompt: %w", err)
	}

	if _, err := tx.Exec(`DELETE FROM user_prompts WHERE id = ?`, promptID); err != nil {
		return DeletionResult{}, fmt.Errorf("hard-delete prompt: %w", err)
	}

	var mutationSeq *int64
	projStr := ptrStr(project)
	enrolled := IsEnrolledTx(tx, projStr)
	if enrolled && syncID != nil && *syncID != "" && projStr != "" {
		seq, err := InsertSyncMutationTx(tx, "prompt", *syncID, "delete", "{}", projStr, now)
		if err != nil {
			return DeletionResult{}, fmt.Errorf("delete mutation prompt: %w", err)
		}
		mutationSeq = &seq
	}
	return DeletionResult{
		Entity:      EntityKindPrompt,
		ID:          id,
		Mode:        "hard",
		MutationSeq: mutationSeq,
		Enrolled:    enrolled,
	}, nil
}
