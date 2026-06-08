// Package services implements the business logic layer for engram-explorer.
package services

import (
	"database/sql"
	"strings"

	"github.com/AlvaroQ/engram-explorer/internal/cursor"
	"github.com/AlvaroQ/engram-explorer/internal/fts"
)

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
  o.last_seen_at, o.created_at, o.updated_at, o.deleted_at, o.sync_id`

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

// ObservationsList executes the keyset-paginated list query.
func ObservationsList(db *sql.DB, p ObservationListParams) (items []ObservationRow, nextCursor *string, err error) {
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

	rows, err := db.Query(query, params...)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	var all []ObservationRow
	for rows.Next() {
		var r ObservationRow
		if err := rows.Scan(
			&r.ID, &r.SessionID, &r.Type, &r.Title, &r.Content, &r.ToolName,
			&r.Project, &r.Scope, &r.TopicKey, &r.NormalizedHash,
			&r.RevisionCount, &r.DuplicateCount,
			&r.LastSeenAt, &r.CreatedAt, &r.UpdatedAt, &r.DeletedAt, &r.SyncID,
		); err != nil {
			return nil, nil, err
		}
		all = append(all, r)
	}
	if err := rows.Err(); err != nil {
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
func ObservationsGetByID(db *sql.DB, id int64) (obs *ObservationRow, revisions []ObservationRow, err error) {
	query := `SELECT ` + obsCols + ` FROM observations o WHERE o.id = ?`
	row := db.QueryRow(query, id)
	var r ObservationRow
	if err := row.Scan(
		&r.ID, &r.SessionID, &r.Type, &r.Title, &r.Content, &r.ToolName,
		&r.Project, &r.Scope, &r.TopicKey, &r.NormalizedHash,
		&r.RevisionCount, &r.DuplicateCount,
		&r.LastSeenAt, &r.CreatedAt, &r.UpdatedAt, &r.DeletedAt, &r.SyncID,
	); err == sql.ErrNoRows {
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
		revRows, err := db.Query(revQ, *obs.TopicKey, *obs.Project, id)
		if err != nil {
			return nil, nil, err
		}
		defer revRows.Close()
		for revRows.Next() {
			var rev ObservationRow
			if err := revRows.Scan(
				&rev.ID, &rev.SessionID, &rev.Type, &rev.Title, &rev.Content, &rev.ToolName,
				&rev.Project, &rev.Scope, &rev.TopicKey, &rev.NormalizedHash,
				&rev.RevisionCount, &rev.DuplicateCount,
				&rev.LastSeenAt, &rev.CreatedAt, &rev.UpdatedAt, &rev.DeletedAt, &rev.SyncID,
			); err != nil {
				return nil, nil, err
			}
			revisions = append(revisions, rev)
		}
		if err := revRows.Err(); err != nil {
			return nil, nil, err
		}
	}

	return obs, revisions, nil
}

// ObservationsSearch runs an FTS5 snippet search.
func ObservationsSearch(db *sql.DB, rawQ string, limit int) ([]ObservationWithSnippet, error) {
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
	rows, err := db.Query(query, sanitized, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []ObservationWithSnippet
	for rows.Next() {
		var r ObservationWithSnippet
		if err := rows.Scan(
			&r.ID, &r.SessionID, &r.Type, &r.Title, &r.Content, &r.ToolName,
			&r.Project, &r.Scope, &r.TopicKey, &r.NormalizedHash,
			&r.RevisionCount, &r.DuplicateCount,
			&r.LastSeenAt, &r.CreatedAt, &r.UpdatedAt, &r.DeletedAt, &r.SyncID,
			&r.Snippet,
		); err != nil {
			return nil, err
		}
		results = append(results, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if results == nil {
		results = []ObservationWithSnippet{}
	}
	return results, nil
}

// ObservationsListTypes returns all distinct, non-empty observation types ordered
// alphabetically case-insensitively (matching the Node listTypes implementation
// which uses ORDER BY type COLLATE NOCASE).
func ObservationsListTypes(db *sql.DB) ([]string, error) {
	rows, err := db.Query(`
		SELECT DISTINCT type FROM observations
		WHERE deleted_at IS NULL AND type IS NOT NULL AND type <> ''
		ORDER BY type COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var types []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, err
		}
		types = append(types, t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if types == nil {
		types = []string{}
	}
	return types, nil
}
