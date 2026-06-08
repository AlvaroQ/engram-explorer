package services

import (
	"database/sql"
	"fmt"
	"time"
)

// ActivityRow is one (day, project) observation bucket.
type ActivityRow struct {
	Day     string `json:"day"`
	Project string `json:"project"`
	Count   int64  `json:"count"`
}

// ActivityResponse is the /api/activity response shape (mirrors Node's
// ActivityResponse: { range, generated_at, rows }).
type ActivityResponse struct {
	Range       string        `json:"range"`
	GeneratedAt string        `json:"generated_at"`
	Rows        []ActivityRow `json:"rows"`
}

// orphansLabel is the placeholder project name for observations captured
// without a project (mirrors Node's ORPHANS_LABEL).
const orphansLabel = "(orphans)"

// activityRangeDays maps the allowed range tokens to their day windows.
var activityRangeDays = map[string]int{"7d": 7, "30d": 30, "90d": 90}

// ValidActivityRange reports whether rng is an accepted activity range token.
func ValidActivityRange(rng string) bool {
	_, ok := activityRangeDays[rng]
	return ok
}

// ActivityByProject returns per-day, per-project observation counts within the
// given range. Mirrors Node's activity.service.ts byProject() and reuses the
// shared cutoffDays() window so the date boundary matches overview/projects.
func ActivityByProject(db *sql.DB, rng string) (*ActivityResponse, error) {
	days, ok := activityRangeDays[rng]
	if !ok {
		days = 30 // caller validates; defensive default
	}

	out, err := queryRows(db, `
		SELECT date(created_at) AS day,
		       COALESCE(NULLIF(project, ''), ?) AS project,
		       COUNT(*) AS count
		  FROM observations
		 WHERE deleted_at IS NULL AND created_at >= ?
		 GROUP BY day, project
		 ORDER BY day ASC, project ASC`, []any{orphansLabel, cutoffDays(days)}, func(s scanner) (ActivityRow, error) {
		var r ActivityRow
		err := s.Scan(&r.Day, &r.Project, &r.Count)
		return r, err
	})
	if err != nil {
		return nil, fmt.Errorf("activity byProject: %w", err)
	}

	return &ActivityResponse{
		Range:       rng,
		GeneratedAt: time.Now().UTC().Format("2006-01-02T15:04:05.000Z"),
		Rows:        out,
	}, nil
}
