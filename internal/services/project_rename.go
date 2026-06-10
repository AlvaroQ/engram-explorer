package services

import (
	"context"
	"fmt"

	"github.com/AlvaroQ/engram-explorer/internal/sqlite"
)

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

// RenameProjectParams mirrors the TS RenameProjectParams shape.
type RenameProjectParams struct {
	Source string
	Target string
	Mode   string // "rename" | "merge"
}

// RenameProjectResult mirrors the TS RenameProjectResult shape.
type RenameProjectResult struct {
	Source   string               `json:"source"`
	Target   string               `json:"target"`
	Mode     string               `json:"mode"`
	Affected RenameAffectedCounts `json:"affected"`
}

// RenameAffectedCounts holds the per-table changed-row counts.
type RenameAffectedCounts struct {
	Observations int64 `json:"observations"`
	Sessions     int64 `json:"sessions"`
	UserPrompts  int64 `json:"userPrompts"`
}

// ---------------------------------------------------------------------------
// RenameProject
// ---------------------------------------------------------------------------

// RenameProject validates and executes a project rename/merge inside a single
// transaction. Validation order follows the spec exactly:
// SAME_NAME → SOURCE_NOT_FOUND → ENROLLED_SOURCE_UNSUPPORTED →
// TARGET_EXISTS/NOT_FOUND → ENROLLED_TARGET_UNSUPPORTED.
func RenameProject(ctx context.Context, db sqlite.Querier, params RenameProjectParams) (RenameProjectResult, error) {
	// Pre-tx validations that only read.
	if params.Source == params.Target {
		return RenameProjectResult{}, NewWriteError("SAME_NAME", "source and target project names are the same")
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return RenameProjectResult{}, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	// SOURCE_NOT_FOUND
	srcExists, err := ProjectExistsTx(tx, params.Source)
	if err != nil {
		return RenameProjectResult{}, fmt.Errorf("check source: %w", err)
	}
	if !srcExists {
		return RenameProjectResult{}, NewWriteError("SOURCE_NOT_FOUND", fmt.Sprintf("project '%s' not found", params.Source))
	}

	// ENROLLED_SOURCE_UNSUPPORTED
	if IsEnrolledTx(tx, params.Source) {
		return RenameProjectResult{}, NewWriteError(
			"ENROLLED_SOURCE_UNSUPPORTED",
			fmt.Sprintf("project '%s' is enrolled for cloud sync; rename/merge of enrolled projects will be supported in Phase 2", params.Source),
		)
	}

	// TARGET_EXISTS (rename mode) / TARGET_NOT_FOUND (merge mode)
	tgtExists, err := ProjectExistsTx(tx, params.Target)
	if err != nil {
		return RenameProjectResult{}, fmt.Errorf("check target: %w", err)
	}
	if params.Mode == "rename" && tgtExists {
		return RenameProjectResult{}, NewWriteError(
			"TARGET_EXISTS",
			fmt.Sprintf("project '%s' already exists; use mode='merge' to combine", params.Target),
		)
	}
	if params.Mode == "merge" && !tgtExists {
		return RenameProjectResult{}, NewWriteError(
			"TARGET_NOT_FOUND",
			fmt.Sprintf("merge target '%s' does not exist", params.Target),
		)
	}

	// ENROLLED_TARGET_UNSUPPORTED (merge mode only)
	if params.Mode == "merge" && IsEnrolledTx(tx, params.Target) {
		return RenameProjectResult{}, NewWriteError(
			"ENROLLED_TARGET_UNSUPPORTED",
			fmt.Sprintf("merge target '%s' is enrolled for cloud sync; merging into enrolled projects will be supported in Phase 2", params.Target),
		)
	}

	now := NowSqlite()

	// Execute updates.
	obsRes, err := tx.Exec(
		`UPDATE observations SET project = ?, updated_at = ? WHERE project = ? AND deleted_at IS NULL`,
		params.Target, now, params.Source,
	)
	if err != nil {
		return RenameProjectResult{}, fmt.Errorf("update observations: %w", err)
	}
	obsChanged, _ := obsRes.RowsAffected()

	sessRes, err := tx.Exec(`UPDATE sessions SET project = ? WHERE project = ?`, params.Target, params.Source)
	if err != nil {
		return RenameProjectResult{}, fmt.Errorf("update sessions: %w", err)
	}
	sessChanged, _ := sessRes.RowsAffected()

	promptRes, err := tx.Exec(`UPDATE user_prompts SET project = ? WHERE project = ?`, params.Target, params.Source)
	if err != nil {
		return RenameProjectResult{}, fmt.Errorf("update user_prompts: %w", err)
	}
	promptChanged, _ := promptRes.RowsAffected()

	if err := tx.Commit(); err != nil {
		return RenameProjectResult{}, fmt.Errorf("commit: %w", err)
	}

	return RenameProjectResult{
		Source: params.Source,
		Target: params.Target,
		Mode:   params.Mode,
		Affected: RenameAffectedCounts{
			Observations: obsChanged,
			Sessions:     sessChanged,
			UserPrompts:  promptChanged,
		},
	}, nil
}
