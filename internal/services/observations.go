// Package services implements the business logic layer for engram-explorer.
package services

import (
	"database/sql"
	"strings"
	"time"

	"github.com/AlvaroQ/engram-explorer/internal/cursor"
	"github.com/AlvaroQ/engram-explorer/internal/fts"
	"github.com/AlvaroQ/engram-explorer/internal/sqlite"
)

// sqliteTimeLayout is the format used by all TEXT timestamp columns in the
// Engram DB (matching NowSqlite() in write_shared.go). All stored times are UTC.
const sqliteTimeLayout = "2006-01-02 15:04:05"

// ObservationRow mirrors the DB row exactly (snake_case, pointer fields for
// nullable columns so they serialize as JSON null, never "" or 0).
type ObservationRow struct {
	ID             int64   `json:"id"`
	SessionID      *string `json:"session_id"`
	Type           string  `json:"type"`
	Title          *string `json:"title"`
	Content        *string `json:"content"`
	ToolName       *string `json:"tool_name"`
	Project        *string `json:"project"`
	Scope          *string `json:"scope"`
	TopicKey       *string `json:"topic_key"`
	NormalizedHash *string `json:"normalized_hash"`
	RevisionCount  *int64  `json:"revision_count"`
	DuplicateCount *int64  `json:"duplicate_count"`
	LastSeenAt     *string `json:"last_seen_at"`
	CreatedAt      *string `json:"created_at"`
	UpdatedAt      *string `json:"updated_at"`
	DeletedAt      *string `json:"deleted_at"`
	SyncID         *string `json:"sync_id"`
	// ReviewAfter is a nullable ISO timestamp indicating when a review is due.
	// It is intentionally not exposed as a user-facing label (see ReviewDue).
	ReviewAfter *string `json:"review_after"`
}

// ReviewDue returns true when ReviewAfter is set and the review date is in the
// past or exactly now (UTC). Comparison uses second-level granularity to match
// the "YYYY-MM-DD HH:MM:SS" format stored in the DB (see sqliteTimeLayout).
func (r ObservationRow) ReviewDue() bool {
	if r.ReviewAfter == nil || *r.ReviewAfter == "" {
		return false
	}
	t, err := time.Parse(sqliteTimeLayout, *r.ReviewAfter)
	if err != nil {
		return false
	}
	return !t.After(time.Now().UTC())
}

// ObservationWithSnippet is used by the search endpoint.
type ObservationWithSnippet struct {
	ObservationRow
	Snippet string `json:"snippet"`
}

// ObservationListParams mirrors the Node ObservationListQuery shape.
type ObservationListParams struct {
	Projects       []string
	Types          []string
	ToolNames      []string
	Scope          string // "" or "all" means no filter
	TopicKey       string
	Q              string
	From           string
	To             string
	IncludeDeleted bool
	OnlyDeleted    bool
	Cursor         string
	Limit          int
}

const obsCols = `
  o.id, o.session_id, o.type, o.title, o.content, o.tool_name,
  o.project, o.scope, o.topic_key, o.normalized_hash,
  o.revision_count, o.duplicate_count,
  o.last_seen_at, o.created_at, o.updated_at, o.deleted_at, o.sync_id,
  o.review_after`

func placeholders(n int) string {
	parts := make([]string, n)
	for i := range parts {
		parts[i] = "?"
	}
	return strings.Join(parts, ", ")
}

func coalesceStr(s *string, fallback string) string {
	if s != nil {
		return *s
	}
	return fallback
}

// scanObservation is the single source of truth for scanning the 18 obsCols
// columns into an ObservationRow. The destination order MUST match obsCols.
func scanObservation(s scanner) (ObservationRow, error) {
	var r ObservationRow
	err := s.Scan(
		&r.ID, &r.SessionID, &r.Type, &r.Title, &r.Content, &r.ToolName,
		&r.Project, &r.Scope, &r.TopicKey, &r.NormalizedHash,
		&r.RevisionCount, &r.DuplicateCount,
		&r.LastSeenAt, &r.CreatedAt, &r.UpdatedAt, &r.DeletedAt, &r.SyncID,
		&r.ReviewAfter,
	)
	return r, err
}

// scanObservationWithSnippet scans the 18 obsCols columns plus the trailing
// snippet column produced by the FTS search query.
func scanObservationWithSnippet(s scanner) (ObservationWithSnippet, error) {
	var r ObservationWithSnippet
	err := s.Scan(
		&r.ID, &r.SessionID, &r.Type, &r.Title, &r.Content, &r.ToolName,
		&r.Project, &r.Scope, &r.TopicKey, &r.NormalizedHash,
		&r.RevisionCount, &r.DuplicateCount,
		&r.LastSeenAt, &r.CreatedAt, &r.UpdatedAt, &r.DeletedAt, &r.SyncID,
		&r.ReviewAfter,
		&r.Snippet,
	)
	return r, err
}

// ObservationsList executes the keyset-paginated list query.
func ObservationsList(db sqlite.Querier, p ObservationListParams) (items []ObservationRow, nextCursor *string, err error) {
	conditions := []string{}
	params := []any{}

	if !p.IncludeDeleted && !p.OnlyDeleted {
		conditions = append(conditions, "o.deleted_at IS NULL")
	} else if p.OnlyDeleted {
		conditions = append(conditions, "o.deleted_at IS NOT NULL")
	}

	if len(p.Projects) > 0 {
		conditions = append(conditions, "o.project IN ("+placeholders(len(p.Projects))+")")
		for _, v := range p.Projects {
			params = append(params, v)
		}
	}
	if len(p.Types) > 0 {
		conditions = append(conditions, "o.type IN ("+placeholders(len(p.Types))+")")
		for _, v := range p.Types {
			params = append(params, v)
		}
	}
	if len(p.ToolNames) > 0 {
		conditions = append(conditions, "o.tool_name IN ("+placeholders(len(p.ToolNames))+")")
		for _, v := range p.ToolNames {
			params = append(params, v)
		}
	}
	if p.Scope != "" && p.Scope != "all" {
		conditions = append(conditions, "o.scope = ?")
		params = append(params, p.Scope)
	}
	if p.TopicKey != "" {
		conditions = append(conditions, "o.topic_key = ?")
		params = append(params, p.TopicKey)
	}
	if p.From != "" {
		conditions = append(conditions, "o.created_at >= ?")
		params = append(params, p.From)
	}
	if p.To != "" {
		conditions = append(conditions, "o.created_at <= ?")
		params = append(params, p.To)
	}

	var whereClause string
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	var cursorClause string
	if p.Cursor != "" {
		dec := cursor.Decode(p.Cursor)
		if dec != nil {
			connector := "AND"
			if whereClause == "" {
				connector = "WHERE"
			}
			cursorClause = connector + ` (
				COALESCE(o.updated_at, o.created_at, '') < ?
				OR (COALESCE(o.updated_at, o.created_at, '') = ? AND o.id < ?)
			)`
			params = append(params, dec.OrderKey, dec.OrderKey, dec.ID)
		}
	}

	// FTS join — prepend fts param so it becomes the first positional arg.
	var ftsJoin string
	if p.Q != "" {
		sanitized := fts.SanitizeQuery(p.Q)
		if sanitized != "" {
			ftsJoin = `JOIN observations_fts fts ON fts.rowid = o.id AND observations_fts MATCH ?`
			// Prepend so FTS param comes before WHERE params.
			params = append([]any{sanitized}, params...)
		}
	}

	query := `SELECT ` + obsCols + `
		FROM observations o
		` + ftsJoin + `
		` + whereClause + `
		` + cursorClause + `
		ORDER BY COALESCE(o.updated_at, o.created_at, '') DESC, o.id DESC
		LIMIT ?`
	params = append(params, p.Limit+1)

	all, err := queryRows(db, query, params, scanObservation)
	if err != nil {
		return nil, nil, err
	}

	if len(all) > p.Limit {
		items = all[:p.Limit]
	} else {
		items = all
	}

	if len(all) > p.Limit && len(items) > 0 {
		last := items[len(items)-1]
		orderKey := coalesceStr(last.UpdatedAt, coalesceStr(last.CreatedAt, ""))
		c := cursor.Encode(orderKey, last.ID)
		nextCursor = &c
	}
	return items, nextCursor, nil
}

// ObservationsGetByID returns the observation and its topic revisions, or nil if not found.
func ObservationsGetByID(db sqlite.Querier, id int64) (obs *ObservationRow, revisions []ObservationRow, err error) {
	query := `SELECT ` + obsCols + ` FROM observations o WHERE o.id = ?`
	r, err := scanObservation(db.QueryRow(query, id))
	if err == sql.ErrNoRows {
		return nil, nil, nil
	} else if err != nil {
		return nil, nil, err
	}
	obs = &r

	revisions = []ObservationRow{}
	if obs.TopicKey != nil && *obs.TopicKey != "" && obs.Project != nil {
		revQ := `SELECT ` + obsCols + `
			FROM observations o
			WHERE o.topic_key = ? AND o.project = ? AND o.id <> ?
			ORDER BY COALESCE(o.updated_at, o.created_at, '') DESC, o.id DESC
			LIMIT 50`
		revisions, err = queryRows(db, revQ, []any{*obs.TopicKey, *obs.Project, id}, scanObservation)
		if err != nil {
			return nil, nil, err
		}
	}

	return obs, revisions, nil
}

// ObservationsSearch runs an FTS5 snippet search.
func ObservationsSearch(db sqlite.Querier, rawQ string, limit int) ([]ObservationWithSnippet, error) {
	sanitized := fts.SanitizeQuery(rawQ)
	if sanitized == "" {
		return []ObservationWithSnippet{}, nil
	}
	query := `SELECT ` + obsCols + `,
		snippet(observations_fts, 0, '<mark>', '</mark>', '…', 16) AS snippet
		FROM observations o
		JOIN observations_fts ON observations_fts.rowid = o.id
		WHERE observations_fts MATCH ? AND o.deleted_at IS NULL
		ORDER BY rank, o.id DESC
		LIMIT ?`
	return queryRows(db, query, []any{sanitized, limit}, scanObservationWithSnippet)
}

// ObservationsListTypes returns all distinct, non-empty observation types ordered
// alphabetically case-insensitively (matching the Node listTypes implementation
// which uses ORDER BY type COLLATE NOCASE).
func ObservationsListTypes(db sqlite.Querier) ([]string, error) {
	return queryRows(db, `
		SELECT DISTINCT type FROM observations
		WHERE deleted_at IS NULL AND type IS NOT NULL AND type <> ''
		ORDER BY type COLLATE NOCASE`, nil, func(s scanner) (string, error) {
		var t string
		err := s.Scan(&t)
		return t, err
	})
}
