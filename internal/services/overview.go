package services

import (
	"database/sql"
	"fmt"
)

// OverviewKPIs holds global aggregate counts.
type OverviewKPIs struct {
	Sessions     int64 `json:"sessions"`
	Observations int64 `json:"observations"`
	Prompts      int64 `json:"prompts"`
	Projects     int64 `json:"projects"`
}

// SyncSummary holds the sync health summary embedded in the overview.
type SyncSummary struct {
	EnrolledCount         int64 `json:"enrolled_count"`
	HealthyCount          int64 `json:"healthy_count"`
	BrokenCount           int64 `json:"broken_count"`
	PendingMutationsTotal int64 `json:"pending_mutations_total"`
}

// OverviewRecentObs is a condensed observation for the overview recent list.
type OverviewRecentObs struct {
	ID        int64   `json:"id"`
	Type      string  `json:"type"`
	Title     *string `json:"title"`
	Project   *string `json:"project"`
	CreatedAt *string `json:"created_at"`
}

// OverviewResponse is the full /api/overview response shape.
type OverviewResponse struct {
	KPIs               OverviewKPIs        `json:"kpis"`
	Activity30d        []ActivityDay       `json:"activity_30d"`
	ByType             []TypeCount         `json:"by_type"`
	RecentObservations []OverviewRecentObs `json:"recent_observations"`
	SyncSummary        SyncSummary         `json:"sync_summary"`
}

// OverviewBuild computes the global overview response.
func OverviewBuild(db *sql.DB) (*OverviewResponse, error) {
	var kpis OverviewKPIs
	if err := db.QueryRow(`
		SELECT
		  (SELECT COUNT(*) FROM sessions) AS sessions,
		  (SELECT COUNT(*) FROM observations WHERE deleted_at IS NULL) AS observations,
		  (SELECT COUNT(*) FROM user_prompts) AS prompts,
		  (SELECT COUNT(DISTINCT project) FROM observations WHERE deleted_at IS NULL) AS projects`,
	).Scan(&kpis.Sessions, &kpis.Observations, &kpis.Prompts, &kpis.Projects); err != nil {
		return nil, fmt.Errorf("overview kpis: %w", err)
	}

	cutoff := cutoffDays(30)
	act30dRows, err := db.Query(`
		SELECT date(created_at) AS day, COUNT(*) AS count
		  FROM observations
		 WHERE deleted_at IS NULL AND created_at >= ?
		 GROUP BY day ORDER BY day ASC`, cutoff)
	if err != nil {
		return nil, fmt.Errorf("overview activity30d: %w", err)
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
		SELECT type, COUNT(*) AS count
		  FROM observations
		 WHERE deleted_at IS NULL
		 GROUP BY type ORDER BY count DESC`)
	if err != nil {
		return nil, fmt.Errorf("overview by_type: %w", err)
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

	recentRows, err := db.Query(`
		SELECT id, type, title, project, created_at
		  FROM observations
		 WHERE deleted_at IS NULL
		 ORDER BY COALESCE(updated_at, created_at) DESC, id DESC
		 LIMIT 10`)
	if err != nil {
		return nil, fmt.Errorf("overview recent: %w", err)
	}
	defer recentRows.Close()
	recentObs := []OverviewRecentObs{}
	for recentRows.Next() {
		var r OverviewRecentObs
		if err := recentRows.Scan(&r.ID, &r.Type, &r.Title, &r.Project, &r.CreatedAt); err != nil {
			return nil, err
		}
		recentObs = append(recentObs, r)
	}
	if err := recentRows.Err(); err != nil {
		return nil, err
	}

	// Sync summary: compute from enrolled/state tables.
	syncSummary, err := computeSyncSummary(db)
	if err != nil {
		return nil, fmt.Errorf("overview sync_summary: %w", err)
	}

	return &OverviewResponse{
		KPIs:               kpis,
		Activity30d:        activity30d,
		ByType:             byType,
		RecentObservations: recentObs,
		SyncSummary:        syncSummary,
	}, nil
}

// computeSyncSummary replicates the sync_summary block from the Node overview service.
// Node's overview service calls sync.listProjects() and derives the summary from the
// resulting project rows — so we do the same by calling SyncListProjects and summing.
func computeSyncSummary(db *sql.DB) (SyncSummary, error) {
	syncData, err := SyncListProjects(db)
	if err != nil {
		// sync tables may not exist on old schemas — return zeros.
		return SyncSummary{}, nil
	}

	var enrolledCount, healthyCount, brokenCount, pendingTotal int64
	for _, p := range syncData.Projects {
		if p.Enrolled {
			enrolledCount++
			switch p.Status {
			case SyncStatusHealthy:
				healthyCount++
			case SyncStatusBroken:
				brokenCount++
			}
		}
		pendingTotal += p.PendingMutations
	}

	return SyncSummary{
		EnrolledCount:         enrolledCount,
		HealthyCount:          healthyCount,
		BrokenCount:           brokenCount,
		PendingMutationsTotal: pendingTotal,
	}, nil
}
