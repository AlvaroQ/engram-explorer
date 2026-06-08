package services

import "database/sql"

// OrphanObservation is an observation with no project.
type OrphanObservation struct {
	ID        int64   `json:"id"`
	Type      string  `json:"type"`
	Title     *string `json:"title"`
	ToolName  *string `json:"tool_name"`
	TopicKey  *string `json:"topic_key"`
	CreatedAt *string `json:"created_at"`
	UpdatedAt *string `json:"updated_at"`
	SessionID *string `json:"session_id"`
	SyncID    *string `json:"sync_id"`
}

// OrphanSession is a session with no project.
type OrphanSession struct {
	ID        string  `json:"id"`
	Directory *string `json:"directory"`
	StartedAt *string `json:"started_at"`
	EndedAt   *string `json:"ended_at"`
	Summary   *string `json:"summary"`
}

// OrphanPrompt is a prompt with no project.
type OrphanPrompt struct {
	ID        int64   `json:"id"`
	Content   *string `json:"content"`
	SessionID *string `json:"session_id"`
	CreatedAt *string `json:"created_at"`
	SyncID    *string `json:"sync_id"`
}

// OrphansResponse is the /api/orphans response shape.
type OrphansResponse struct {
	Observations []OrphanObservation `json:"observations"`
	Sessions     []OrphanSession     `json:"sessions"`
	Prompts      []OrphanPrompt      `json:"prompts"`
	Totals       OrphanTotals        `json:"totals"`
}

// OrphanTotals holds the counts.
type OrphanTotals struct {
	Observations int `json:"observations"`
	Sessions     int `json:"sessions"`
	Prompts      int `json:"prompts"`
}

// OrphansList returns all orphaned entities (no project).
func OrphansList(db *sql.DB) (*OrphansResponse, error) {
	obsRows, err := db.Query(`
		SELECT id, type, title, tool_name, topic_key, created_at, updated_at, session_id, sync_id
		  FROM observations
		 WHERE (project IS NULL OR project = '')
		   AND deleted_at IS NULL
		 ORDER BY COALESCE(updated_at, created_at) DESC, id DESC`)
	if err != nil {
		return nil, err
	}
	defer obsRows.Close()
	observations := []OrphanObservation{}
	for obsRows.Next() {
		var o OrphanObservation
		if err := obsRows.Scan(&o.ID, &o.Type, &o.Title, &o.ToolName, &o.TopicKey, &o.CreatedAt, &o.UpdatedAt, &o.SessionID, &o.SyncID); err != nil {
			return nil, err
		}
		observations = append(observations, o)
	}
	if err := obsRows.Err(); err != nil {
		return nil, err
	}

	sessRows, err := db.Query(`
		SELECT id, directory, started_at, ended_at, summary
		  FROM sessions
		 WHERE project IS NULL OR project = ''
		 ORDER BY COALESCE(started_at, '') DESC`)
	if err != nil {
		return nil, err
	}
	defer sessRows.Close()
	sessions := []OrphanSession{}
	for sessRows.Next() {
		var s OrphanSession
		if err := sessRows.Scan(&s.ID, &s.Directory, &s.StartedAt, &s.EndedAt, &s.Summary); err != nil {
			return nil, err
		}
		sessions = append(sessions, s)
	}
	if err := sessRows.Err(); err != nil {
		return nil, err
	}

	promptRows, err := db.Query(`
		SELECT id, content, session_id, created_at, sync_id
		  FROM user_prompts
		 WHERE project IS NULL OR project = ''
		 ORDER BY COALESCE(created_at, '') DESC`)
	if err != nil {
		return nil, err
	}
	defer promptRows.Close()
	prompts := []OrphanPrompt{}
	for promptRows.Next() {
		var p OrphanPrompt
		if err := promptRows.Scan(&p.ID, &p.Content, &p.SessionID, &p.CreatedAt, &p.SyncID); err != nil {
			return nil, err
		}
		prompts = append(prompts, p)
	}
	if err := promptRows.Err(); err != nil {
		return nil, err
	}

	return &OrphansResponse{
		Observations: observations,
		Sessions:     sessions,
		Prompts:      prompts,
		Totals: OrphanTotals{
			Observations: len(observations),
			Sessions:     len(sessions),
			Prompts:      len(prompts),
		},
	}, nil
}
