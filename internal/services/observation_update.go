package services

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/AlvaroQ/engram-explorer/internal/sqlite"
)

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

// ObservationPatch models a PATCH request body for an observation.
// Each field uses an explicit Provided flag to distinguish "omitted" from
// "set to null/empty" — this is the KEY-PRESENCE semantics required by the spec.
type ObservationPatch struct {
	TypeProvided bool
	Type         string // only meaningful when TypeProvided == true

	TitleProvided bool
	Title         *string // nil means "set to null"; "" is normalised to nil before write

	TopicKeyProvided bool
	TopicKey         *string // same normalisation as Title

	ContentProvided bool
	Content         *string // nil means "set to null"; content left as-is (no normalisation)
}

// ObservationUpdateResult mirrors the TS ObservationUpdateResult shape.
type ObservationUpdateResult struct {
	ID          int64          `json:"id"`
	MutationSeq *int64         `json:"mutation_seq"`
	Enrolled    bool           `json:"enrolled"`
	Updated     map[string]any `json:"updated"`
}

// ---------------------------------------------------------------------------
// UpdateObservation
// ---------------------------------------------------------------------------

// UpdateObservation applies patch to observation id (and its hash group) inside
// a single transaction. Returns WriteError "NOT_FOUND" if the observation is
// absent or deleted.
func UpdateObservation(ctx context.Context, db sqlite.Querier, id int64, patch ObservationPatch) (ObservationUpdateResult, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return ObservationUpdateResult{}, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	result, err := updateObservationInTx(tx, id, patch)
	if err != nil {
		return ObservationUpdateResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return ObservationUpdateResult{}, fmt.Errorf("commit: %w", err)
	}
	return result, nil
}

func updateObservationInTx(tx *sql.Tx, id int64, patch ObservationPatch) (ObservationUpdateResult, error) {
	// 1. Fetch clicked row.
	clicked, err := scanObsRow(tx.QueryRow(
		assignObsSelect+` FROM observations WHERE id = ? AND deleted_at IS NULL`, id,
	))
	if err != nil {
		return ObservationUpdateResult{}, fmt.Errorf("get obs: %w", err)
	}
	if clicked == nil {
		return ObservationUpdateResult{}, NewWriteError("NOT_FOUND", fmt.Sprintf("observation %d not found", id))
	}

	now := NowSqlite()

	// 2. Normalise patch fields ('' → nil for title/topic_key).
	normalised := normalisePatch(patch)

	// 3. Build UPDATE SET clauses.
	setClauses, setArgs := buildSetClauses(normalised, now)
	if len(setClauses) == 0 {
		// Nothing to update — return as-is.
		enrolled := IsEnrolledTx(tx, ptrStr(clicked.project))
		return ObservationUpdateResult{
			ID:          clicked.id,
			MutationSeq: nil,
			Enrolled:    enrolled,
			Updated:     map[string]any{},
		}, nil
	}

	// 4. Build hash group.
	group := []assignObsRow{*clicked}
	if clicked.normalizedHash != nil && *clicked.normalizedHash != "" {
		rows, err := tx.Query(
			assignObsSelect+` FROM observations WHERE normalized_hash = ? AND deleted_at IS NULL`,
			*clicked.normalizedHash,
		)
		if err != nil {
			return ObservationUpdateResult{}, fmt.Errorf("get hash group: %w", err)
		}
		group, err = scanObsRows(rows)
		rows.Close()
		if err != nil {
			return ObservationUpdateResult{}, err
		}
	}

	var mutationSeq *int64

	for _, row := range group {
		// Backfill sync_id.
		syncID := ""
		if row.syncID != nil {
			syncID = *row.syncID
		}
		if syncID == "" {
			syncID = GenSyncID("obs")
			if _, err := tx.Exec(`UPDATE observations SET sync_id = ? WHERE id = ?`, syncID, row.id); err != nil {
				return ObservationUpdateResult{}, fmt.Errorf("backfill sync_id obs %d: %w", row.id, err)
			}
		}

		// Execute UPDATE.
		args := append(setArgs, row.id) //nolint:gocritic
		q := fmt.Sprintf(`UPDATE observations SET %s WHERE id = ?`, strings.Join(setClauses, ", "))
		if _, err := tx.Exec(q, args...); err != nil {
			return ObservationUpdateResult{}, fmt.Errorf("update obs %d: %w", row.id, err)
		}

		// Emit sync mutation if project is enrolled.
		rowProject := ptrStr(row.project)
		if IsEnrolledTx(tx, rowProject) {
			mergedPayload := buildMergedPayload(row, syncID, normalised, now)
			seq, err := InsertSyncMutationTx(tx, "observation", syncID, "upsert", mergedPayload, rowProject, now)
			if err != nil {
				return ObservationUpdateResult{}, fmt.Errorf("upsert mutation obs %d: %w", row.id, err)
			}
			if row.id == clicked.id || mutationSeq == nil {
				mutationSeq = &seq
			}
		}
	}

	enrolled := IsEnrolledTx(tx, ptrStr(clicked.project))
	return ObservationUpdateResult{
		ID:          clicked.id,
		MutationSeq: mutationSeq,
		Enrolled:    enrolled,
		Updated:     normalisedToMap(normalised),
	}, nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

type normalisedPatch struct {
	typeProvided     bool
	typ              string
	titleProvided    bool
	title            *string
	topicKeyProvided bool
	topicKey         *string
	contentProvided  bool
	content          *string
}

func normalisePatch(p ObservationPatch) normalisedPatch {
	n := normalisedPatch{}
	if p.TypeProvided {
		n.typeProvided = true
		n.typ = p.Type
	}
	if p.TitleProvided {
		n.titleProvided = true
		if p.Title != nil && *p.Title == "" {
			n.title = nil
		} else {
			n.title = p.Title
		}
	}
	if p.TopicKeyProvided {
		n.topicKeyProvided = true
		if p.TopicKey != nil && *p.TopicKey == "" {
			n.topicKey = nil
		} else {
			n.topicKey = p.TopicKey
		}
	}
	if p.ContentProvided {
		n.contentProvided = true
		n.content = p.Content
	}
	return n
}

func buildSetClauses(n normalisedPatch, now string) (clauses []string, args []any) {
	if n.typeProvided {
		clauses = append(clauses, "type = ?")
		args = append(args, n.typ)
	}
	if n.titleProvided {
		clauses = append(clauses, "title = ?")
		args = append(args, n.title)
	}
	if n.topicKeyProvided {
		clauses = append(clauses, "topic_key = ?")
		args = append(args, n.topicKey)
	}
	if n.contentProvided {
		clauses = append(clauses, "content = ?")
		args = append(args, n.content)
	}
	clauses = append(clauses, "updated_at = ?")
	args = append(args, now)
	return
}

func buildMergedPayload(row assignObsRow, syncID string, n normalisedPatch, now string) string {
	typ := row.typ
	if n.typeProvided {
		typ = n.typ
	}
	title := row.title
	if n.titleProvided {
		title = n.title
	}
	content := row.content
	if n.contentProvided {
		content = n.content
	}
	topicKey := row.topicKey
	if n.topicKeyProvided {
		topicKey = n.topicKey
	}
	return CompactPayload(map[string]any{
		"sync_id":         syncID,
		"session_id":      row.sessionID,
		"type":            typ,
		"title":           title,
		"content":         content,
		"tool_name":       row.toolName,
		"project":         row.project,
		"scope":           row.scope,
		"topic_key":       topicKey,
		"revision_count":  row.revisionCount,
		"duplicate_count": row.duplicateCount,
		"last_seen_at":    row.lastSeenAt,
		"created_at":      row.createdAt,
		"updated_at":      now,
	})
}

func normalisedToMap(n normalisedPatch) map[string]any {
	m := map[string]any{}
	if n.typeProvided {
		m["type"] = n.typ
	}
	if n.titleProvided {
		m["title"] = n.title
	}
	if n.topicKeyProvided {
		m["topic_key"] = n.topicKey
	}
	if n.contentProvided {
		m["content"] = n.content
	}
	return m
}

func ptrStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
