package services

import (
	"database/sql"
	"fmt"
	"time"
)

// projectEntitiesUnionSQL is the core UNION ALL used by both ProjectsList and overview.
const projectEntitiesUnionSQL = `
  SELECT project, COUNT(*) AS obs_count, 0 AS sessions_count, 0 AS prompts_count,
         MAX(COALESCE(updated_at, created_at)) AS last_activity
    FROM observations WHERE deleted_at IS NULL GROUP BY project
  UNION ALL
  SELECT project, 0, COUNT(*), 0, MAX(COALESCE(ended_at, started_at)) FROM sessions GROUP BY project
  UNION ALL
  SELECT project, 0, 0, COUNT(*), MAX(created_at) FROM user_prompts GROUP BY project
`

// ProjectStats holds per-project aggregate counts and sync state.
type ProjectStats struct {
	Project          string  `json:"project"`
	ObsCount         int64   `json:"obs_count"`
	SessionsCount    int64   `json:"sessions_count"`
	PromptsCount     int64   `json:"prompts_count"`
	LastActivity     *string `json:"last_activity"`
	PendingMutations int64   `json:"pending_mutations"`
	SyncUpdatedAt    *string `json:"sync_updated_at"`
}

// TopicRow is a single topic from the topics endpoint.
type TopicRow struct {
	TopicKey    string  `json:"topic_key"`
	Project     *string `json:"project"`
	ObsCount    int64   `json:"obs_count"`
	Revisions   int64   `json:"revisions"`
	LastUpdated *string `json:"last_updated"`
}

// ProjectKPIs holds the summary KPIs for a project overview.
type ProjectKPIs struct {
	Observations int64   `json:"observations"`
	Sessions     int64   `json:"sessions"`
	Prompts      int64   `json:"prompts"`
	Topics       int64   `json:"topics"`
	LastActivity *string `json:"last_activity"`
}

// ProjectOverview is the full project overview response.
type ProjectOverview struct {
	Project             string                    `json:"project"`
	KPIs                ProjectKPIs               `json:"kpis"`
	Activity30d         []ActivityDay             `json:"activity_30d"`
	ByType              []TypeCount               `json:"by_type"`
	ByTool              []ToolCount               `json:"by_tool"`
	TopTopics           []TopicSummary            `json:"top_topics"`
	RecentObservations  []RecentObs               `json:"recent_observations"`
	RecentSessions      []RecentSession           `json:"recent_sessions"`
	RecentPrompts       []RecentPrompt            `json:"recent_prompts"`
}

// ActivityDay is a (day, count) bucket.
type ActivityDay struct {
	Day   string `json:"day"`
	Count int64  `json:"count"`
}

// TypeCount is a (type, count) bucket.
type TypeCount struct {
	Type  string `json:"type"`
	Count int64  `json:"count"`
}

// ToolCount is a (tool_name, count) bucket.
type ToolCount struct {
	ToolName string `json:"tool_name"`
	Count    int64  `json:"count"`
}

// TopicSummary is used in project overview.
type TopicSummary struct {
	TopicKey    string  `json:"topic_key"`
	ObsCount    int64   `json:"obs_count"`
	LastUpdated *string `json:"last_updated"`
}

// RecentObs is a condensed observation row for recent-observations lists.
type RecentObs struct {
	ID        int64   `json:"id"`
	Type      string  `json:"type"`
	Title     *string `json:"title"`
	CreatedAt *string `json:"created_at"`
	TopicKey  *string `json:"topic_key"`
	ToolName  *string `json:"tool_name"`
}

// RecentSession is a condensed session row.
type RecentSession struct {
	ID        string  `json:"id"`
	StartedAt *string `json:"started_at"`
	EndedAt   *string `json:"ended_at"`
	ObsCount  int64   `json:"obs_count"`
	Summary   *string `json:"summary"`
}

// RecentPrompt is a condensed prompt row.
type RecentPrompt struct {
	ID        int64   `json:"id"`
	Content   *string `json:"content"`
	CreatedAt *string `json:"created_at"`
}

// cutoffDays returns the UTC calendar date `days` days before today, formatted
// as "YYYY-MM-DD", replicating SQLite's date('now', '-N days'). Passed as a
// bound parameter so queries stay testable/deterministic and consistent across
// every service that windows by recent activity (overview, projects, activity).
func cutoffDays(days int) string {
	return time.Now().UTC().AddDate(0, 0, -days).Format("2006-01-02")
}

// ProjectsList returns aggregate stats for all projects.
// Matches Node's projects.service.ts list() which joins pending mutations
// exclusively by cloud:<project> target_key (NOT by the project column of
// sync_mutations — that belongs to the global 'cloud' target and is not
// per-project-target-keyed in the daemon schema).
func ProjectsList(db *sql.DB) ([]ProjectStats, error) {
	query := fmt.Sprintf(`
		WITH agg AS (
		  SELECT COALESCE(project, '') AS project,
		         SUM(obs_count) AS obs_count,
		         SUM(sessions_count) AS sessions_count,
		         SUM(prompts_count) AS prompts_count,
		         MAX(last_activity) AS last_activity
		    FROM (%s)
		   GROUP BY COALESCE(project, '')
		),
		pend AS (
		  SELECT target_key, COUNT(*) AS pending
		    FROM sync_mutations
		   WHERE acked_at IS NULL
		   GROUP BY target_key
		)
		SELECT a.project,
		       a.obs_count,
		       a.sessions_count,
		       a.prompts_count,
		       a.last_activity,
		       COALESCE(p.pending, 0) AS pending_mutations,
		       s.updated_at AS sync_updated_at
		  FROM agg a
		  LEFT JOIN sync_state s ON s.target_key = 'cloud:' || a.project
		  LEFT JOIN pend p ON p.target_key = 'cloud:' || a.project
		 ORDER BY a.obs_count DESC`, projectEntitiesUnionSQL)

	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []ProjectStats
	for rows.Next() {
		var s ProjectStats
		if err := rows.Scan(
			&s.Project, &s.ObsCount, &s.SessionsCount, &s.PromptsCount,
			&s.LastActivity, &s.PendingMutations, &s.SyncUpdatedAt,
		); err != nil {
			return nil, err
		}
		result = append(result, s)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if result == nil {
		result = []ProjectStats{}
	}
	return result, nil
}

// projectExists checks whether any observation, session, or prompt exists for the project.
func projectExists(db *sql.DB, project string) (bool, error) {
	if project == "" {
		return false, nil
	}
	var n int
	err := db.QueryRow(`
		SELECT 1 FROM (
		  SELECT project FROM observations WHERE project = ? AND deleted_at IS NULL
		  UNION ALL SELECT project FROM sessions WHERE project = ?
		  UNION ALL SELECT project FROM user_prompts WHERE project = ?
		) LIMIT 1`, project, project, project).Scan(&n)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return err == nil, err
}

// ProjectsGetOverview builds the full overview for a single project.
// Returns nil if the project does not exist.
func ProjectsGetOverview(db *sql.DB, project string) (*ProjectOverview, error) {
	exists, err := projectExists(db, project)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, nil
	}

	var kpis ProjectKPIs
	if err := db.QueryRow(`
		SELECT
		  (SELECT COUNT(*) FROM observations WHERE project = ? AND deleted_at IS NULL) AS observations,
		  (SELECT COUNT(*) FROM sessions WHERE project = ?) AS sessions,
		  (SELECT COUNT(*) FROM user_prompts WHERE project = ?) AS prompts,
		  (SELECT COUNT(DISTINCT topic_key) FROM observations
		     WHERE project = ? AND deleted_at IS NULL AND topic_key IS NOT NULL AND topic_key <> '') AS topics,
		  (SELECT MAX(COALESCE(updated_at, created_at)) FROM observations WHERE project = ? AND deleted_at IS NULL) AS last_activity`,
		project, project, project, project, project,
	).Scan(&kpis.Observations, &kpis.Sessions, &kpis.Prompts, &kpis.Topics, &kpis.LastActivity); err != nil {
		return nil, err
	}

	cutoff := cutoffDays(30)
	act30dRows, err := db.Query(`
		SELECT date(created_at) AS day, COUNT(*) AS count
		  FROM observations
		 WHERE project = ? AND deleted_at IS NULL
		   AND created_at >= ?
		 GROUP BY day ORDER BY day ASC`, project, cutoff)
	if err != nil {
		return nil, err
	}
	defer act30dRows.Close()
	activity30d := []ActivityDay{}
	for act30dRows.Next() {
		var d ActivityDay
		if err := act30dRows.Scan(&d.Day, &d.Count); err != nil {
			return nil, err
		}
		activity30d = append(activity30d, d)
	}
	if err := act30dRows.Err(); err != nil {
		return nil, err
	}

	byTypeRows, err := db.Query(`
		SELECT type, COUNT(*) AS count FROM observations
		 WHERE project = ? AND deleted_at IS NULL
		 GROUP BY type ORDER BY count DESC`, project)
	if err != nil {
		return nil, err
	}
	defer byTypeRows.Close()
	byType := []TypeCount{}
	for byTypeRows.Next() {
		var tc TypeCount
		if err := byTypeRows.Scan(&tc.Type, &tc.Count); err != nil {
			return nil, err
		}
		byType = append(byType, tc)
	}
	if err := byTypeRows.Err(); err != nil {
		return nil, err
	}

	byToolRows, err := db.Query(`
		SELECT COALESCE(tool_name, '(none)') AS tool_name, COUNT(*) AS count
		  FROM observations
		 WHERE project = ? AND deleted_at IS NULL
		 GROUP BY tool_name ORDER BY count DESC LIMIT 10`, project)
	if err != nil {
		return nil, err
	}
	defer byToolRows.Close()
	byTool := []ToolCount{}
	for byToolRows.Next() {
		var tc ToolCount
		if err := byToolRows.Scan(&tc.ToolName, &tc.Count); err != nil {
			return nil, err
		}
		byTool = append(byTool, tc)
	}
	if err := byToolRows.Err(); err != nil {
		return nil, err
	}

	topTopicRows, err := db.Query(`
		SELECT topic_key, COUNT(*) AS obs_count,
		       MAX(COALESCE(updated_at, created_at)) AS last_updated
		  FROM observations
		 WHERE project = ? AND deleted_at IS NULL
		   AND topic_key IS NOT NULL AND topic_key <> ''
		 GROUP BY topic_key ORDER BY obs_count DESC LIMIT 10`, project)
	if err != nil {
		return nil, err
	}
	defer topTopicRows.Close()
	topTopics := []TopicSummary{}
	for topTopicRows.Next() {
		var ts TopicSummary
		if err := topTopicRows.Scan(&ts.TopicKey, &ts.ObsCount, &ts.LastUpdated); err != nil {
			return nil, err
		}
		topTopics = append(topTopics, ts)
	}
	if err := topTopicRows.Err(); err != nil {
		return nil, err
	}

	recentObsRows, err := db.Query(`
		SELECT id, type, title, created_at, topic_key, tool_name
		  FROM observations
		 WHERE project = ? AND deleted_at IS NULL
		 ORDER BY COALESCE(updated_at, created_at) DESC, id DESC LIMIT 15`, project)
	if err != nil {
		return nil, err
	}
	defer recentObsRows.Close()
	recentObs := []RecentObs{}
	for recentObsRows.Next() {
		var r RecentObs
		if err := recentObsRows.Scan(&r.ID, &r.Type, &r.Title, &r.CreatedAt, &r.TopicKey, &r.ToolName); err != nil {
			return nil, err
		}
		recentObs = append(recentObs, r)
	}
	if err := recentObsRows.Err(); err != nil {
		return nil, err
	}

	recentSessionRows, err := db.Query(`
		SELECT s.id, s.started_at, s.ended_at, s.summary,
		       (SELECT COUNT(*) FROM observations o WHERE o.session_id = s.id AND o.deleted_at IS NULL) AS obs_count
		  FROM sessions s
		 WHERE s.project = ?
		 ORDER BY COALESCE(s.started_at, '') DESC LIMIT 10`, project)
	if err != nil {
		return nil, err
	}
	defer recentSessionRows.Close()
	recentSessions := []RecentSession{}
	for recentSessionRows.Next() {
		var s RecentSession
		if err := recentSessionRows.Scan(&s.ID, &s.StartedAt, &s.EndedAt, &s.Summary, &s.ObsCount); err != nil {
			return nil, err
		}
		recentSessions = append(recentSessions, s)
	}
	if err := recentSessionRows.Err(); err != nil {
		return nil, err
	}

	recentPromptRows, err := db.Query(`
		SELECT id, content, created_at FROM user_prompts
		 WHERE project = ? ORDER BY COALESCE(created_at, '') DESC LIMIT 5`, project)
	if err != nil {
		return nil, err
	}
	defer recentPromptRows.Close()
	recentPrompts := []RecentPrompt{}
	for recentPromptRows.Next() {
		var rp RecentPrompt
		if err := recentPromptRows.Scan(&rp.ID, &rp.Content, &rp.CreatedAt); err != nil {
			return nil, err
		}
		recentPrompts = append(recentPrompts, rp)
	}
	if err := recentPromptRows.Err(); err != nil {
		return nil, err
	}

	return &ProjectOverview{
		Project:            project,
		KPIs:               kpis,
		Activity30d:        activity30d,
		ByType:             byType,
		ByTool:             byTool,
		TopTopics:          topTopics,
		RecentObservations: recentObs,
		RecentSessions:     recentSessions,
		RecentPrompts:      recentPrompts,
	}, nil
}

// TopicsListParams holds filter parameters for the topics endpoint.
type TopicsListParams struct {
	Project string // empty means all
}

// TopicsList returns all distinct topics, optionally filtered by project.
func TopicsList(db *sql.DB, p TopicsListParams) ([]TopicRow, error) {
	where := "WHERE deleted_at IS NULL AND topic_key IS NOT NULL AND topic_key <> ''"
	args := []any{}
	if p.Project != "" {
		where += " AND project = ?"
		args = append(args, p.Project)
	}
	query := `SELECT topic_key, project,
		COUNT(*) AS obs_count,
		COALESCE(MAX(revision_count), 0) AS revisions,
		MAX(COALESCE(updated_at, created_at)) AS last_updated
		FROM observations
		` + where + `
		GROUP BY topic_key, project
		ORDER BY revisions DESC, obs_count DESC
		LIMIT 500`
	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []TopicRow
	for rows.Next() {
		var t TopicRow
		if err := rows.Scan(&t.TopicKey, &t.Project, &t.ObsCount, &t.Revisions, &t.LastUpdated); err != nil {
			return nil, err
		}
		result = append(result, t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if result == nil {
		result = []TopicRow{}
	}
	return result, nil
}

