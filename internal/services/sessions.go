package services

import (
	"database/sql"
	"encoding/base64"
	"sort"
	"strings"

	"github.com/AlvaroQ/engram-explorer/internal/cursor"
)

// SessionRow mirrors the sessions table columns used by the API.
type SessionRow struct {
	ID        string  `json:"id"`
	Project   *string `json:"project"`
	Directory *string `json:"directory"`
	StartedAt *string `json:"started_at"`
	EndedAt   *string `json:"ended_at"`
	Summary   *string `json:"summary"`
}

// SessionListItem extends SessionRow with computed counts and enrichment fields.
type SessionListItem struct {
	SessionRow
	ObsCount     int64   `json:"obs_count"`
	PromptsCount int64   `json:"prompts_count"`
	LastActivity *string `json:"last_activity"`
	// Enrichment fields — only present when enrich=true.
	Tags        []string `json:"tags,omitempty"`
	RecentTitle *string  `json:"recent_title,omitempty"`
}

// SessionListParams holds the query parameters for the sessions list endpoint.
type SessionListParams struct {
	Projects   []string // filter by project(s); empty = all
	From       string   // lower bound on started_at (inclusive)
	To         string   // upper bound on started_at (inclusive)
	HasSummary *bool    // nil = no filter; true = must have summary; false = no summary
	Cursor     string   // opaque keyset pagination cursor
	Limit      int      // rows per page (1–500, default 50)
	Sort       string   // "started" (default) | "recent_activity"
	Enrich     bool     // when true, add tags + recent_title per item
}

// SessionListResult is the paginated list response.
type SessionListResult struct {
	Items      []SessionListItem `json:"items"`
	NextCursor *string           `json:"nextCursor"`
}

// SessionTypeCount is one row in the session stats breakdown.
type SessionTypeCount struct {
	Type  string `json:"type"`
	Count int64  `json:"count"`
}

// SessionStats holds the aggregate stats for a single session.
type SessionStats struct {
	ObsTotal     int64              `json:"obs_total"`
	PromptsTotal int64              `json:"prompts_total"`
	ByType       []SessionTypeCount `json:"by_type"`
}

// SessionEvent is one timeline event (observation or prompt).
type SessionEvent struct {
	Kind     string  `json:"kind"`     // "observation" | "prompt"
	ID       int64   `json:"id"`
	At       *string `json:"at"`
	Type     *string `json:"type"`
	Title    *string `json:"title"`
	Content  *string `json:"content"`
	Project  *string `json:"project"`
	ToolName *string `json:"tool_name"`
}

// SessionDetailResponse is the full session detail response.
type SessionDetailResponse struct {
	Session *SessionRow    `json:"session"`
	Stats   SessionStats   `json:"stats"`
	Events  []SessionEvent `json:"events"`
}

// orderKeyExpr returns the SQL expression used for ordering.
// For recent_activity mode, this is the latest-observation time subquery.
// For started (default) mode, it is COALESCE(s.started_at, '').
const (
	orderKeyExprStarted        = "COALESCE(s.started_at, '')"
	orderKeyExprRecentActivity = "COALESCE((SELECT MAX(o.created_at) FROM observations o WHERE o.session_id = s.id AND o.deleted_at IS NULL), s.ended_at, s.started_at, '')"
)

// SessionsList returns a paginated list of sessions.
func SessionsList(db *sql.DB, p SessionListParams) (SessionListResult, error) {
	limit := p.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}

	useRecentActivity := p.Sort == "recent_activity"
	orderExpr := orderKeyExprStarted
	if useRecentActivity {
		orderExpr = orderKeyExprRecentActivity
	}

	var conditions []string
	var args []any

	if len(p.Projects) > 0 {
		ph := make([]string, len(p.Projects))
		for i := range ph {
			ph[i] = "?"
		}
		conditions = append(conditions, "s.project IN ("+strings.Join(ph, ",")+")")
		for _, proj := range p.Projects {
			args = append(args, proj)
		}
	}
	if p.From != "" {
		conditions = append(conditions, "s.started_at >= ?")
		args = append(args, p.From)
	}
	if p.To != "" {
		conditions = append(conditions, "s.started_at <= ?")
		args = append(args, p.To)
	}
	if p.HasSummary != nil {
		if *p.HasSummary {
			conditions = append(conditions, "s.summary IS NOT NULL AND s.summary <> ''")
		} else {
			conditions = append(conditions, "s.summary IS NULL")
		}
	}

	// Cursor decode — sessions use TEXT ids, so the real session id is
	// base64url-encoded after the last '|' in the orderKey field.
	var cursorClause string
	if p.Cursor != "" {
		decoded := cursor.Decode(p.Cursor)
		if decoded != nil {
			// Split on last '|' to separate the real orderKey from the embedded id.
			combined := decoded.OrderKey
			sep := strings.LastIndex(combined, "|")
			realOrderKey := combined
			var lastID string
			if sep >= 0 {
				realOrderKey = combined[:sep]
				idHash := combined[sep+1:]
				if idHash != "" {
					b, err := base64.RawURLEncoding.DecodeString(idHash)
					if err == nil {
						lastID = string(b)
					}
				}
			}
			if len(conditions) == 0 {
				cursorClause = "(" + orderExpr + " < ? OR (" + orderExpr + " = ? AND s.id < ?))"
			} else {
				cursorClause = "AND (" + orderExpr + " < ? OR (" + orderExpr + " = ? AND s.id < ?))"
			}
			args = append(args, realOrderKey, realOrderKey, lastID)
		}
	}

	var where string
	if len(conditions) > 0 || cursorClause != "" {
		parts := conditions
		if cursorClause != "" {
			parts = append(parts, cursorClause)
		}
		where = "WHERE " + strings.Join(parts, " ")
	}

	query := `
		SELECT s.id, s.project, s.directory, s.started_at, s.ended_at, s.summary,
		       (SELECT COUNT(*) FROM observations o WHERE o.session_id = s.id AND o.deleted_at IS NULL) AS obs_count,
		       (SELECT COUNT(*) FROM user_prompts p WHERE p.session_id = s.id) AS prompts_count,
		       (SELECT MAX(o.created_at) FROM observations o WHERE o.session_id = s.id AND o.deleted_at IS NULL) AS last_activity
		FROM sessions s
		` + where + `
		ORDER BY ` + orderExpr + ` DESC, s.id DESC
		LIMIT ?`

	args = append(args, limit+1)

	rows, err := db.Query(query, args...)
	if err != nil {
		return SessionListResult{}, err
	}
	defer rows.Close()

	var allRows []SessionListItem
	for rows.Next() {
		var item SessionListItem
		if err := rows.Scan(
			&item.ID, &item.Project, &item.Directory,
			&item.StartedAt, &item.EndedAt, &item.Summary,
			&item.ObsCount, &item.PromptsCount, &item.LastActivity,
		); err != nil {
			return SessionListResult{}, err
		}
		allRows = append(allRows, item)
	}
	if err := rows.Err(); err != nil {
		return SessionListResult{}, err
	}

	items := allRows
	if len(allRows) > limit {
		items = allRows[:limit]
	}

	var nextCursor *string
	if len(allRows) > limit {
		last := items[len(items)-1]
		var orderKeyValue string
		if useRecentActivity {
			switch {
			case last.LastActivity != nil:
				orderKeyValue = *last.LastActivity
			case last.EndedAt != nil:
				orderKeyValue = *last.EndedAt
			case last.StartedAt != nil:
				orderKeyValue = *last.StartedAt
			default:
				orderKeyValue = ""
			}
		} else {
			if last.StartedAt != nil {
				orderKeyValue = *last.StartedAt
			}
		}
		idHash := base64.RawURLEncoding.EncodeToString([]byte(last.ID))
		composite := orderKeyValue + "|" + idHash
		// cursor.Encode uses orderKey|id where id is int64; for sessions the id
		// is TEXT so we embed it in the orderKey field with id=0 as a sentinel.
		encoded := cursor.Encode(composite, 0)
		nextCursor = &encoded
	}

	// Enrichment pass.
	if p.Enrich && len(items) > 0 {
		items, err = enrichSessions(db, items)
		if err != nil {
			return SessionListResult{}, err
		}
	}

	if items == nil {
		items = []SessionListItem{}
	}

	return SessionListResult{Items: items, NextCursor: nextCursor}, nil
}

// enrichSessions adds Tags and RecentTitle to each item.
func enrichSessions(db *sql.DB, items []SessionListItem) ([]SessionListItem, error) {
	tagQuery := `
		SELECT topic_key,
		       COUNT(*) AS freq,
		       MAX(created_at) AS latest
		  FROM observations
		 WHERE session_id = ? AND deleted_at IS NULL AND topic_key IS NOT NULL
		 GROUP BY topic_key
		 ORDER BY freq DESC, latest DESC
		 LIMIT 3`
	recentTitleQuery := `
		SELECT title
		  FROM observations
		 WHERE session_id = ? AND deleted_at IS NULL
		 ORDER BY COALESCE(created_at, '') DESC, id DESC
		 LIMIT 1`

	for i := range items {
		rows, err := db.Query(tagQuery, items[i].ID)
		if err != nil {
			return nil, err
		}
		var tags []string
		for rows.Next() {
			var topicKey string
			var freq int64
			var latest *string
			if err := rows.Scan(&topicKey, &freq, &latest); err != nil {
				rows.Close()
				return nil, err
			}
			tags = append(tags, topicKey)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
		if tags == nil {
			tags = []string{}
		}
		items[i].Tags = tags

		var title *string
		err = db.QueryRow(recentTitleQuery, items[i].ID).Scan(&title)
		if err != nil && err != sql.ErrNoRows {
			return nil, err
		}
		items[i].RecentTitle = title
	}

	return items, nil
}

// SessionsGetDetail returns the full session detail including events and stats.
// Returns nil, nil if the session does not exist.
func SessionsGetDetail(db *sql.DB, id string) (*SessionDetailResponse, error) {
	var sess SessionRow
	err := db.QueryRow(
		`SELECT id, project, directory, started_at, ended_at, summary FROM sessions WHERE id = ?`,
		id,
	).Scan(&sess.ID, &sess.Project, &sess.Directory, &sess.StartedAt, &sess.EndedAt, &sess.Summary)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	// Fetch observations.
	type obsRow struct {
		ID        int64
		CreatedAt *string
		Type      string
		Title     *string
		Content   *string
		Project   *string
		ToolName  *string
	}
	obsRows, err := db.Query(
		`SELECT id, created_at, type, title, content, project, tool_name
		   FROM observations
		  WHERE session_id = ? AND deleted_at IS NULL
		  ORDER BY COALESCE(created_at, '') ASC, id ASC`,
		id,
	)
	if err != nil {
		return nil, err
	}
	defer obsRows.Close()
	var obsList []obsRow
	for obsRows.Next() {
		var r obsRow
		if err := obsRows.Scan(&r.ID, &r.CreatedAt, &r.Type, &r.Title, &r.Content, &r.Project, &r.ToolName); err != nil {
			return nil, err
		}
		obsList = append(obsList, r)
	}
	if err := obsRows.Err(); err != nil {
		return nil, err
	}

	// Fetch prompts.
	type promptRow struct {
		ID        int64
		CreatedAt *string
		Content   *string
		Project   *string
	}
	promptRows, err := db.Query(
		`SELECT id, created_at, content, project
		   FROM user_prompts WHERE session_id = ? ORDER BY COALESCE(created_at, '') ASC, id ASC`,
		id,
	)
	if err != nil {
		return nil, err
	}
	defer promptRows.Close()
	var promptList []promptRow
	for promptRows.Next() {
		var r promptRow
		if err := promptRows.Scan(&r.ID, &r.CreatedAt, &r.Content, &r.Project); err != nil {
			return nil, err
		}
		promptList = append(promptList, r)
	}
	if err := promptRows.Err(); err != nil {
		return nil, err
	}

	// Build merged event list.
	events := make([]SessionEvent, 0, len(obsList)+len(promptList))
	for _, o := range obsList {
		kind := "observation"
		otype := o.Type
		events = append(events, SessionEvent{
			Kind:     kind,
			ID:       o.ID,
			At:       o.CreatedAt,
			Type:     &otype,
			Title:    o.Title,
			Content:  o.Content,
			Project:  o.Project,
			ToolName: o.ToolName,
		})
	}
	for _, p := range promptList {
		events = append(events, SessionEvent{
			Kind:    "prompt",
			ID:      p.ID,
			At:      p.CreatedAt,
			Type:    nil,
			Title:   nil,
			Content: p.Content,
			Project: p.Project,
			ToolName: nil,
		})
	}

	// Stable sort by at (byte order, matching JS localeCompare on ASCII timestamps).
	sort.SliceStable(events, func(i, j int) bool {
		ai := ""
		if events[i].At != nil {
			ai = *events[i].At
		}
		aj := ""
		if events[j].At != nil {
			aj = *events[j].At
		}
		return strings.Compare(ai, aj) < 0
	})

	// Stats: by_type breakdown.
	typeRows, err := db.Query(
		`SELECT type, COUNT(*) AS count
		   FROM observations
		  WHERE session_id = ? AND deleted_at IS NULL
		  GROUP BY type ORDER BY count DESC`,
		id,
	)
	if err != nil {
		return nil, err
	}
	defer typeRows.Close()
	byType := []SessionTypeCount{}
	for typeRows.Next() {
		var tc SessionTypeCount
		if err := typeRows.Scan(&tc.Type, &tc.Count); err != nil {
			return nil, err
		}
		byType = append(byType, tc)
	}
	if err := typeRows.Err(); err != nil {
		return nil, err
	}

	return &SessionDetailResponse{
		Session: &sess,
		Stats: SessionStats{
			ObsTotal:     int64(len(obsList)),
			PromptsTotal: int64(len(promptList)),
			ByType:       byType,
		},
		Events: events,
	}, nil
}
