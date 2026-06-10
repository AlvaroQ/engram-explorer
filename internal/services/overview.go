package services

import (
	"fmt"

	"golang.org/x/sync/errgroup"

	"github.com/AlvaroQ/engram-explorer/internal/sqlite"
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

// OverviewBuild computes the global overview response. The five independent
// queries run concurrently via errgroup; the read pool (4 conns) bounds actual
// parallelism, and Wait() provides the happens-before barrier before the
// results are assembled, so the per-section variables are race-free.
func OverviewBuild(db sqlite.Querier) (*OverviewResponse, error) {
	var (
		kpis        OverviewKPIs
		activity30d = []ActivityDay{}
		byType      = []TypeCount{}
		recentObs   = []OverviewRecentObs{}
		syncSummary SyncSummary
	)

	var g errgroup.Group
	g.Go(func() error {
		v, err := overviewKPIs(db)
		kpis = v
		return err
	})
	g.Go(func() error {
		v, err := overviewActivity30d(db)
		activity30d = v
		return err
	})
	g.Go(func() error {
		v, err := overviewByType(db)
		byType = v
		return err
	})
	g.Go(func() error {
		v, err := overviewRecent(db)
		recentObs = v
		return err
	})
	g.Go(func() error {
		v, err := computeSyncSummary(db)
		if err != nil {
			return fmt.Errorf("overview sync_summary: %w", err)
		}
		syncSummary = v
		return nil
	})

	if err := g.Wait(); err != nil {
		return nil, err
	}

	return &OverviewResponse{
		KPIs:               kpis,
		Activity30d:        activity30d,
		ByType:             byType,
		RecentObservations: recentObs,
		SyncSummary:        syncSummary,
	}, nil
}

func overviewKPIs(db sqlite.Querier) (OverviewKPIs, error) {
	var kpis OverviewKPIs
	if err := db.QueryRow(`
		SELECT
		  (SELECT COUNT(*) FROM sessions) AS sessions,
		  (SELECT COUNT(*) FROM observations WHERE deleted_at IS NULL) AS observations,
		  (SELECT COUNT(*) FROM user_prompts) AS prompts,
		  (SELECT COUNT(DISTINCT project) FROM observations WHERE deleted_at IS NULL) AS projects`,
	).Scan(&kpis.Sessions, &kpis.Observations, &kpis.Prompts, &kpis.Projects); err != nil {
		return OverviewKPIs{}, fmt.Errorf("overview kpis: %w", err)
	}
	return kpis, nil
}

func overviewActivity30d(db sqlite.Querier) ([]ActivityDay, error) {
	cutoff := cutoffDays(30)
	activity30d, err := queryRows(db, `
		SELECT date(created_at) AS day, COUNT(*) AS count
		  FROM observations
		 WHERE deleted_at IS NULL AND created_at >= ?
		 GROUP BY day ORDER BY day ASC`, []any{cutoff}, scanActivityDay)
	if err != nil {
		return nil, fmt.Errorf("overview activity30d: %w", err)
	}
	return activity30d, nil
}

func overviewByType(db sqlite.Querier) ([]TypeCount, error) {
	byType, err := queryRows(db, `
		SELECT type, COUNT(*) AS count
		  FROM observations
		 WHERE deleted_at IS NULL
		 GROUP BY type ORDER BY count DESC`, nil, scanTypeCount)
	if err != nil {
		return nil, fmt.Errorf("overview by_type: %w", err)
	}
	return byType, nil
}

func overviewRecent(db sqlite.Querier) ([]OverviewRecentObs, error) {
	recentObs, err := queryRows(db, `
		SELECT id, type, title, project, created_at
		  FROM observations
		 WHERE deleted_at IS NULL
		 ORDER BY COALESCE(updated_at, created_at) DESC, id DESC
		 LIMIT 10`, nil, func(s scanner) (OverviewRecentObs, error) {
		var r OverviewRecentObs
		err := s.Scan(&r.ID, &r.Type, &r.Title, &r.Project, &r.CreatedAt)
		return r, err
	})
	if err != nil {
		return nil, fmt.Errorf("overview recent: %w", err)
	}
	return recentObs, nil
}

// computeSyncSummary replicates the sync_summary block from the Node overview service.
// Node's overview service calls sync.listProjects() and derives the summary from the
// resulting project rows — so we do the same by calling SyncListProjects and summing.
func computeSyncSummary(db sqlite.Querier) (SyncSummary, error) {
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
