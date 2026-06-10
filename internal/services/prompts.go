package services

import (
	"strings"

	"github.com/AlvaroQ/engram-explorer/internal/fts"
	"github.com/AlvaroQ/engram-explorer/internal/sqlite"
)

// PromptRow mirrors the user_prompts DB row.
type PromptRow struct {
	ID        int64   `json:"id"`
	SessionID *string `json:"session_id"`
	Content   *string `json:"content"`
	Project   *string `json:"project"`
	CreatedAt *string `json:"created_at"`
	SyncID    *string `json:"sync_id"`
}

// PromptWithSnippet is used by the search endpoint.
type PromptWithSnippet struct {
	PromptRow
	Snippet string `json:"snippet"`
}

// PromptsListParams holds filter parameters for the list endpoint.
type PromptsListParams struct {
	Project string // empty means all
	Limit   int
}

// PromptsList returns user_prompts ordered by created_at DESC, id DESC.
func PromptsList(db sqlite.Querier, p PromptsListParams) ([]PromptRow, error) {
	var conditions []string
	var args []any
	if p.Project != "" {
		conditions = append(conditions, "project = ?")
		args = append(args, p.Project)
	}
	where := ""
	if len(conditions) > 0 {
		where = "WHERE " + strings.Join(conditions, " AND ")
	}
	args = append(args, p.Limit)
	query := `SELECT id, session_id, content, project, created_at, sync_id
		FROM user_prompts
		` + where + `
		ORDER BY COALESCE(created_at, '') DESC, id DESC
		LIMIT ?`
	return queryRows(db, query, args, scanPromptRow)
}

// scanPromptRow scans a user_prompts row into a PromptRow.
func scanPromptRow(s scanner) (PromptRow, error) {
	var r PromptRow
	err := s.Scan(&r.ID, &r.SessionID, &r.Content, &r.Project, &r.CreatedAt, &r.SyncID)
	return r, err
}

// PromptsSearch runs FTS5 search on the prompts_fts table.
func PromptsSearch(db sqlite.Querier, rawQ string, limit int) ([]PromptWithSnippet, error) {
	sanitized := fts.SanitizeQuery(rawQ)
	if sanitized == "" {
		return []PromptWithSnippet{}, nil
	}
	query := `SELECT p.id, p.session_id, p.content, p.project, p.created_at, p.sync_id,
		snippet(prompts_fts, 0, '<mark>', '</mark>', '…', 16) AS snippet
		FROM user_prompts p
		JOIN prompts_fts ON prompts_fts.rowid = p.id
		WHERE prompts_fts MATCH ?
		ORDER BY rank, p.id DESC
		LIMIT ?`
	rows, err := db.Query(query, sanitized, limit)
	if err != nil {
		// FTS table may not exist; mirror Node's silent fallback.
		return []PromptWithSnippet{}, nil
	}
	defer rows.Close()
	var results []PromptWithSnippet
	for rows.Next() {
		var r PromptWithSnippet
		if err := rows.Scan(&r.ID, &r.SessionID, &r.Content, &r.Project, &r.CreatedAt, &r.SyncID, &r.Snippet); err != nil {
			return nil, err
		}
		results = append(results, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if results == nil {
		results = []PromptWithSnippet{}
	}
	return results, nil
}
