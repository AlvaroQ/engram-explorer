package services

import (
	"context"
	"database/sql"
	"fmt"
)

// DeleteProjectResult holds the per-table affected-row counts.
type DeleteProjectResult struct {
	Project  string               `json:"project"`
	Affected DeleteAffectedCounts `json:"affected"`
}

// DeleteAffectedCounts holds what happened per entity table.
type DeleteAffectedCounts struct {
	Observations       int64 `json:"observations"`        // rows deleted
	UserPrompts        int64 `json:"userPrompts"`         // rows deleted
	Sessions           int64 `json:"sessions"`            // rows deleted (empty sessions)
	SessionsReassigned int64 `json:"sessionsReassigned"`  // moved to another project, not deleted
}

// DeleteProject removes a project. It is conservative and lossless:
//
//   - ENROLLED_UNSUPPORTED: enrolled projects are blocked (mirrors RenameProject;
//     unenroll first so cloud sync state is not left dangling).
//   - HAS_OBSERVATIONS: a project with its OWN live observations (project tag) is
//     blocked — only observation-free projects can be deleted, so no captured
//     knowledge is removed by accident.
//   - NOT_FOUND: the project has no live observation, session, or prompt.
//
// AssignProject changes only observations.project, never session_id, so a
// session tagged to this project can still physically hold observations that
// were reassigned to OTHER projects. Deleting such a session would orphan (and
// FK-break) those observations. To stay lossless we therefore:
//  1. delete this project's own observations + prompts (project tag),
//  2. reassign any of this project's sessions that still hold another project's
//     content to that project — the session follows its content,
//  3. delete the sessions that are left empty.
func DeleteProject(ctx context.Context, db *sql.DB, project string) (DeleteProjectResult, error) {
	if project == "" {
		return DeleteProjectResult{}, NewWriteError("INVALID_PROJECT", "project name is empty")
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return DeleteProjectResult{}, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	// NOT_FOUND
	exists, err := ProjectExistsTx(tx, project)
	if err != nil {
		return DeleteProjectResult{}, fmt.Errorf("check project: %w", err)
	}
	if !exists {
		return DeleteProjectResult{}, NewWriteError("NOT_FOUND", fmt.Sprintf("project '%s' not found", project))
	}

	// ENROLLED_UNSUPPORTED
	if IsEnrolledTx(tx, project) {
		return DeleteProjectResult{}, NewWriteError(
			"ENROLLED_UNSUPPORTED",
			fmt.Sprintf("project '%s' is enrolled for cloud sync; unenroll it before deleting", project),
		)
	}

	// HAS_OBSERVATIONS — block only on the project's OWN live observations, which
	// is exactly what the projects table shows. Observations reassigned to other
	// projects (but still living in this project's sessions) are preserved below.
	var liveObs int64
	if err := tx.QueryRow(
		`SELECT COUNT(*) FROM observations WHERE project = ? AND deleted_at IS NULL`,
		project,
	).Scan(&liveObs); err != nil {
		return DeleteProjectResult{}, fmt.Errorf("count observations: %w", err)
	}
	if liveObs > 0 {
		return DeleteProjectResult{}, NewWriteError(
			"HAS_OBSERVATIONS",
			fmt.Sprintf("project '%s' still has %d observation(s); only observation-free projects can be deleted", project, liveObs),
		)
	}

	// (1) Delete the project's own observations and prompts (by project tag).
	obsRes, err := tx.Exec(`DELETE FROM observations WHERE project = ?`, project)
	if err != nil {
		return DeleteProjectResult{}, fmt.Errorf("delete observations: %w", err)
	}
	obsDeleted, _ := obsRes.RowsAffected()

	promptRes, err := tx.Exec(`DELETE FROM user_prompts WHERE project = ?`, project)
	if err != nil {
		return DeleteProjectResult{}, fmt.Errorf("delete user_prompts: %w", err)
	}
	promptDeleted, _ := promptRes.RowsAffected()

	// (2) Reassign this project's sessions that still hold content belonging to
	// another project, so the session follows its content. Prefer the project of
	// a live observation, then any observation, then a prompt; only non-null
	// targets qualify (sessions.project is NOT NULL).
	reassignRes, err := tx.Exec(
		`UPDATE sessions
		    SET project = COALESCE(
		      (SELECT o.project FROM observations o
		         WHERE o.session_id = sessions.id AND o.project IS NOT NULL
		         ORDER BY (o.deleted_at IS NULL) DESC, o.updated_at DESC LIMIT 1),
		      (SELECT p.project FROM user_prompts p
		         WHERE p.session_id = sessions.id AND p.project IS NOT NULL
		         ORDER BY p.created_at DESC LIMIT 1)
		    )
		  WHERE project = ?
		    AND (
		      EXISTS (SELECT 1 FROM observations o WHERE o.session_id = sessions.id AND o.project IS NOT NULL)
		      OR EXISTS (SELECT 1 FROM user_prompts p WHERE p.session_id = sessions.id AND p.project IS NOT NULL)
		    )`,
		project,
	)
	if err != nil {
		return DeleteProjectResult{}, fmt.Errorf("reassign sessions: %w", err)
	}
	sessReassigned, _ := reassignRes.RowsAffected()

	// (3) The sessions still tagged to this project are now empty (or hold only
	// project-less leftovers). Remove any such leftover children first to satisfy
	// the session_id foreign key, then delete the sessions.
	if _, err := tx.Exec(
		`DELETE FROM observations WHERE session_id IN (SELECT id FROM sessions WHERE project = ?)`,
		project,
	); err != nil {
		return DeleteProjectResult{}, fmt.Errorf("delete leftover observations: %w", err)
	}
	if _, err := tx.Exec(
		`DELETE FROM user_prompts WHERE session_id IN (SELECT id FROM sessions WHERE project = ?)`,
		project,
	); err != nil {
		return DeleteProjectResult{}, fmt.Errorf("delete leftover prompts: %w", err)
	}
	sessRes, err := tx.Exec(`DELETE FROM sessions WHERE project = ?`, project)
	if err != nil {
		return DeleteProjectResult{}, fmt.Errorf("delete sessions: %w", err)
	}
	sessDeleted, _ := sessRes.RowsAffected()

	if err := tx.Commit(); err != nil {
		return DeleteProjectResult{}, fmt.Errorf("commit: %w", err)
	}

	return DeleteProjectResult{
		Project: project,
		Affected: DeleteAffectedCounts{
			Observations:       obsDeleted,
			UserPrompts:        promptDeleted,
			Sessions:           sessDeleted,
			SessionsReassigned: sessReassigned,
		},
	}, nil
}
