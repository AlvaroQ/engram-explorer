package services

import (
	"database/sql"
	"encoding/json"
	"sort"
	"strings"
	"time"
)

// enrollGraceMs is the window in which a freshly-enrolled project is allowed
// to have lifecycle='pending', ack=0, enq>0 without being flagged broken.
const enrollGraceMs = 5 * 60 * 1000

// SyncStatus values.
type SyncStatus string

const (
	SyncStatusHealthy     SyncStatus = "healthy"
	SyncStatusDegraded    SyncStatus = "degraded"
	SyncStatusBroken      SyncStatus = "broken"
	SyncStatusNotEnrolled SyncStatus = "not_enrolled"
	SyncStatusIdle        SyncStatus = "idle"
)

// SyncProjectRow is one project's sync summary.
type SyncProjectRow struct {
	Project             string     `json:"project"`
	ObsCount            int64      `json:"obs_count"`
	SessionsCount       int64      `json:"sessions_count"`
	PromptsCount        int64      `json:"prompts_count"`
	LastActivity        *string    `json:"last_activity"`
	Enrolled            bool       `json:"enrolled"`
	EnrolledAt          *string    `json:"enrolled_at"`
	Lifecycle           *string    `json:"lifecycle"`
	LastEnqueuedSeq     *int64     `json:"last_enqueued_seq"`
	LastAckedSeq        *int64     `json:"last_acked_seq"`
	LastPulledSeq       *int64     `json:"last_pulled_seq"`
	ConsecutiveFailures *int64     `json:"consecutive_failures"`
	BackoffUntil        *string    `json:"backoff_until"`
	LeaseOwner          *string    `json:"lease_owner"`
	LeaseUntil          *string    `json:"lease_until"`
	LastError           *string    `json:"last_error"`
	ReasonCode          *string    `json:"reason_code"`
	ReasonMessage       *string    `json:"reason_message"`
	PendingMutations    int64      `json:"pending_mutations"`
	Status              SyncStatus `json:"status"`
}

// SyncGlobalTarget holds the 'cloud' global target aggregate.
type SyncGlobalTarget struct {
	TargetKey        string  `json:"target_key"`
	Lifecycle        *string `json:"lifecycle"`
	LastEnqueuedSeq  *int64  `json:"last_enqueued_seq"`
	LastAckedSeq     *int64  `json:"last_acked_seq"`
	PendingMutations int64   `json:"pending_mutations"`
}

// SyncProjectsResponse is the /api/sync/projects response.
type SyncProjectsResponse struct {
	GlobalTarget *SyncGlobalTarget `json:"global_target"`
	Projects     []SyncProjectRow  `json:"projects"`
}

// SyncMutationRow is one pending sync mutation.
type SyncMutationRow struct {
	Seq        int64   `json:"seq"`
	TargetKey  string  `json:"target_key"`
	Entity     string  `json:"entity"`
	EntityKey  *string `json:"entity_key"`
	Op         string  `json:"op"`
	Source     *string `json:"source"`
	OccurredAt *string `json:"occurred_at"`
	AckedAt    *string `json:"acked_at"`
	Project    *string `json:"project"`
}

// UpgradeState holds the cloud upgrade state for a project.
type UpgradeState struct {
	Project     string      `json:"project"`
	Stage       *string     `json:"stage"`
	RepairClass *string     `json:"repair_class"`
	Findings    interface{} `json:"findings"`
	UpdatedAt   *string     `json:"updated_at"`
}

// SyncProjectDetailResponse is the full detail for a single project.
type SyncProjectDetailResponse struct {
	Summary      SyncProjectRow    `json:"summary"`
	Pending      []SyncMutationRow `json:"pending"`
	UpgradeState *UpgradeState     `json:"upgrade_state"`
}

// SyncIssueSeverity represents the severity level of an issue.
type SyncIssueSeverity string

const (
	SyncIssueSeverityHigh   SyncIssueSeverity = "HIGH"
	SyncIssueSeverityMedium SyncIssueSeverity = "MEDIUM"
	SyncIssueSeverityLow    SyncIssueSeverity = "LOW"
	SyncIssueSeverityInfo   SyncIssueSeverity = "INFO"
)

// SyncIssue is a single diagnostic issue.
type SyncIssue struct {
	Code     string            `json:"code"`
	Severity SyncIssueSeverity `json:"severity"`
	Message  string            `json:"message"`
	Project  *string           `json:"project"`
	Hint     *string           `json:"hint"`
	Evidence map[string]any    `json:"evidence,omitempty"`
}

// SyncIssuesResponse is the /api/sync/issues response.
type SyncIssuesResponse struct {
	Issues      []SyncIssue `json:"issues"`
	GeneratedAt string      `json:"generated_at"`
}

// projectAggregate holds per-project aggregate counts from the entity union.
type projectAggregate struct {
	project       string
	obsCount      int64
	sessionsCount int64
	promptsCount  int64
	lastActivity  *string
}

// syncStateRow mirrors the sync_state table.
type syncStateRow struct {
	targetKey           string
	lifecycle           *string
	lastEnqueuedSeq     *int64
	lastAckedSeq        *int64
	lastPulledSeq       *int64
	consecutiveFailures *int64
	backoffUntil        *string
	leaseOwner          *string
	leaseUntil          *string
	lastError           *string
	reasonCode          *string
	reasonMessage       *string
	updatedAt           *string
}

// mutationCountRow holds pending mutation counts for a key.
type mutationCountRow struct {
	pending       int64
	oldestPending *string
}

// readProjectAggregates fetches the per-project aggregates. The entity UNION is
// shared with ProjectsList via projectEntitiesUnionSQL (defined in projects.go).
func readProjectAggregates(db *sql.DB) ([]projectAggregate, error) {
	query := `
		SELECT COALESCE(o.project, '') AS project,
		       SUM(o.obs_count) AS obs_count,
		       SUM(o.sessions_count) AS sessions_count,
		       SUM(o.prompts_count) AS prompts_count,
		       MAX(o.last_activity) AS last_activity
		  FROM (` + projectEntitiesUnionSQL + `) o
		 GROUP BY COALESCE(o.project, '')`

	return queryRows(db, query, nil, func(s scanner) (projectAggregate, error) {
		var a projectAggregate
		err := s.Scan(&a.project, &a.obsCount, &a.sessionsCount, &a.promptsCount, &a.lastActivity)
		return a, err
	})
}

// readEnrolledProjects returns a map of project → enrolled_at.
func readEnrolledProjects(db *sql.DB) (map[string]*string, error) {
	rows, err := db.Query(`SELECT project, enrolled_at FROM sync_enrolled_projects`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	m := make(map[string]*string)
	for rows.Next() {
		var project string
		var enrolledAt *string
		if err := rows.Scan(&project, &enrolledAt); err != nil {
			return nil, err
		}
		m[project] = enrolledAt
	}
	return m, rows.Err()
}

// readSyncStates returns all sync_state rows keyed by target_key.
func readSyncStates(db *sql.DB) (map[string]syncStateRow, error) {
	rows, err := db.Query(`
		SELECT target_key, lifecycle, last_enqueued_seq, last_acked_seq, last_pulled_seq,
		       consecutive_failures, backoff_until, lease_owner, lease_until,
		       last_error, reason_code, reason_message, updated_at
		  FROM sync_state`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	m := make(map[string]syncStateRow)
	for rows.Next() {
		var r syncStateRow
		if err := rows.Scan(
			&r.targetKey, &r.lifecycle, &r.lastEnqueuedSeq, &r.lastAckedSeq, &r.lastPulledSeq,
			&r.consecutiveFailures, &r.backoffUntil, &r.leaseOwner, &r.leaseUntil,
			&r.lastError, &r.reasonCode, &r.reasonMessage, &r.updatedAt,
		); err != nil {
			return nil, err
		}
		m[r.targetKey] = r
	}
	return m, rows.Err()
}

// readPendingCounts returns two maps: byTargetKey and byProject.
func readPendingCounts(db *sql.DB) (byTargetKey map[string]mutationCountRow, byProject map[string]mutationCountRow, err error) {
	byTargetKey = make(map[string]mutationCountRow)
	byProject = make(map[string]mutationCountRow)

	rows, err := db.Query(`
		SELECT target_key, COUNT(*) AS pending, MIN(occurred_at) AS oldest_pending
		  FROM sync_mutations
		 WHERE acked_at IS NULL
		 GROUP BY target_key`)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var key string
		var r mutationCountRow
		if err := rows.Scan(&key, &r.pending, &r.oldestPending); err != nil {
			return nil, nil, err
		}
		byTargetKey[key] = r
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}

	rows2, err := db.Query(`
		SELECT COALESCE(project, '') AS proj, COUNT(*) AS pending, MIN(occurred_at) AS oldest_pending
		  FROM sync_mutations
		 WHERE acked_at IS NULL AND project IS NOT NULL AND project != ''
		 GROUP BY project`)
	if err != nil {
		return nil, nil, err
	}
	defer rows2.Close()
	for rows2.Next() {
		var key string
		var r mutationCountRow
		if err := rows2.Scan(&key, &r.pending, &r.oldestPending); err != nil {
			return nil, nil, err
		}
		byProject[key] = r
	}
	return byTargetKey, byProject, rows2.Err()
}

// stateIsStale returns true when the sync_state row's updated_at is older than
// enrollGraceMs. The updated_at column is stored as UTC "YYYY-MM-DD HH:MM:SS"
// so we append 'Z' to parse it as UTC.
func stateIsStale(state syncStateRow, nowMs int64) bool {
	if state.updatedAt == nil || *state.updatedAt == "" {
		return true
	}
	// Append Z to treat as UTC, matching Date.parse(`${updated_at}Z`) in TS.
	ts, err := time.Parse("2006-01-02 15:04:05Z", *state.updatedAt+"Z")
	if err != nil {
		return true
	}
	return nowMs-ts.UnixMilli() >= enrollGraceMs
}

// deriveStatus mirrors the TS deriveStatus function exactly.
func deriveStatus(
	enrolled bool,
	state *syncStateRow,
	pending int64,
	nowMs int64,
) SyncStatus {
	if !enrolled {
		return SyncStatusNotEnrolled
	}
	if state == nil {
		return SyncStatusIdle
	}
	if state.lifecycle != nil && *state.lifecycle == "failed" {
		return SyncStatusBroken
	}
	if state.consecutiveFailures != nil && *state.consecutiveFailures > 3 {
		return SyncStatusBroken
	}
	if state.lastError != nil || state.reasonCode != nil {
		return SyncStatusDegraded
	}
	if state.lifecycle != nil && *state.lifecycle == "pending" {
		ackedSeq := int64(0)
		if state.lastAckedSeq != nil {
			ackedSeq = *state.lastAckedSeq
		}
		enqSeq := int64(0)
		if state.lastEnqueuedSeq != nil {
			enqSeq = *state.lastEnqueuedSeq
		}
		if ackedSeq == 0 && enqSeq > 0 {
			if stateIsStale(*state, nowMs) {
				return SyncStatusBroken
			}
			return SyncStatusIdle
		}
	}
	if pending > 50 {
		return SyncStatusDegraded
	}
	if state.lifecycle != nil && *state.lifecycle == "idle" {
		return SyncStatusIdle
	}
	return SyncStatusHealthy
}

// buildProjectRow assembles a SyncProjectRow from the resolved data.
func buildProjectRow(
	project string,
	agg *projectAggregate,
	enrolledMap map[string]*string,
	states map[string]syncStateRow,
	byTargetKey map[string]mutationCountRow,
	byProject map[string]mutationCountRow,
	nowMs int64,
) SyncProjectRow {
	enrolledAt, enrolled := enrolledMap[project]

	stateKey := "cloud:" + project
	state, hasState := states[stateKey]
	var statePtr *syncStateRow
	if hasState {
		s := state
		statePtr = &s
	}

	// Prefer per-project bucket; fall back to cloud:<project> target bucket.
	pending := int64(0)
	if r, ok := byProject[project]; ok {
		pending = r.pending
	} else if r, ok := byTargetKey["cloud:"+project]; ok {
		pending = r.pending
	}

	status := deriveStatus(enrolled, statePtr, pending, nowMs)

	row := SyncProjectRow{
		Project:          project,
		ObsCount:         0,
		SessionsCount:    0,
		PromptsCount:     0,
		LastActivity:     nil,
		Enrolled:         enrolled,
		EnrolledAt:       nil,
		PendingMutations: pending,
		Status:           status,
	}
	if agg != nil {
		row.ObsCount = agg.obsCount
		row.SessionsCount = agg.sessionsCount
		row.PromptsCount = agg.promptsCount
		row.LastActivity = agg.lastActivity
	}
	if enrolled && enrolledAt != nil {
		row.EnrolledAt = enrolledAt
	}
	if hasState {
		row.Lifecycle = state.lifecycle
		row.LastEnqueuedSeq = state.lastEnqueuedSeq
		row.LastAckedSeq = state.lastAckedSeq
		row.LastPulledSeq = state.lastPulledSeq
		row.ConsecutiveFailures = state.consecutiveFailures
		row.BackoffUntil = state.backoffUntil
		row.LeaseOwner = state.leaseOwner
		row.LeaseUntil = state.leaseUntil
		row.LastError = state.lastError
		row.ReasonCode = state.reasonCode
		row.ReasonMessage = state.reasonMessage
	}
	return row
}

// SyncListProjects returns the aggregated sync state for all projects.
func SyncListProjects(db *sql.DB) (SyncProjectsResponse, error) {
	aggregates, err := readProjectAggregates(db)
	if err != nil {
		return SyncProjectsResponse{}, err
	}
	enrolled, err := readEnrolledProjects(db)
	if err != nil {
		return SyncProjectsResponse{}, err
	}
	states, err := readSyncStates(db)
	if err != nil {
		return SyncProjectsResponse{}, err
	}
	byTargetKey, byProject, err := readPendingCounts(db)
	if err != nil {
		return SyncProjectsResponse{}, err
	}
	nowMs := time.Now().UnixMilli()

	// Union of project keys from aggregates and enrolled map.
	projectKeys := make(map[string]struct{})
	for _, a := range aggregates {
		projectKeys[a.project] = struct{}{}
	}
	for k := range enrolled {
		projectKeys[k] = struct{}{}
	}

	aggMap := make(map[string]*projectAggregate, len(aggregates))
	for i := range aggregates {
		a := aggregates[i]
		aggMap[a.project] = &a
	}

	projects := make([]SyncProjectRow, 0, len(projectKeys))
	for project := range projectKeys {
		row := buildProjectRow(project, aggMap[project], enrolled, states, byTargetKey, byProject, nowMs)
		projects = append(projects, row)
	}

	// Sort by obs_count DESC, then by project byte-order (stable, mirrors localeCompare on ASCII names).
	sort.SliceStable(projects, func(i, j int) bool {
		if projects[i].ObsCount != projects[j].ObsCount {
			return projects[i].ObsCount > projects[j].ObsCount
		}
		return strings.Compare(projects[i].Project, projects[j].Project) < 0
	})

	var globalTarget *SyncGlobalTarget
	if gs, ok := states["cloud"]; ok {
		globalPending := int64(0)
		if r, ok2 := byTargetKey["cloud"]; ok2 {
			globalPending = r.pending
		}
		globalTarget = &SyncGlobalTarget{
			TargetKey:        "cloud",
			Lifecycle:        gs.lifecycle,
			LastEnqueuedSeq:  gs.lastEnqueuedSeq,
			LastAckedSeq:     gs.lastAckedSeq,
			PendingMutations: globalPending,
		}
	}

	return SyncProjectsResponse{
		GlobalTarget: globalTarget,
		Projects:     projects,
	}, nil
}

// SyncGetProjectDetail returns the full sync detail for a single project.
// Returns nil, nil if the project is not found.
func SyncGetProjectDetail(db *sql.DB, project string) (*SyncProjectDetailResponse, error) {
	all, err := SyncListProjects(db)
	if err != nil {
		return nil, err
	}

	var summary *SyncProjectRow
	for i := range all.Projects {
		if all.Projects[i].Project == project {
			s := all.Projects[i]
			summary = &s
			break
		}
	}
	if summary == nil {
		return nil, nil
	}

	// Fetch pending mutations.
	pending, err := queryRows(db, `
		SELECT seq, target_key, entity, entity_key, op, source, occurred_at, acked_at, project
		  FROM sync_mutations
		 WHERE acked_at IS NULL AND (target_key = ? OR project = ?)
		 ORDER BY seq DESC LIMIT 200`,
		[]any{"cloud:" + project, project},
		func(s scanner) (SyncMutationRow, error) {
			var m SyncMutationRow
			err := s.Scan(&m.Seq, &m.TargetKey, &m.Entity, &m.EntityKey, &m.Op, &m.Source, &m.OccurredAt, &m.AckedAt, &m.Project)
			return m, err
		})
	if err != nil {
		return nil, err
	}

	// Fetch upgrade state — table may not exist on older schemas.
	var upgradeState *UpgradeState
	func() {
		row := db.QueryRow(
			`SELECT project, stage, repair_class, findings_json, updated_at FROM cloud_upgrade_state WHERE project = ?`,
			project,
		)
		var proj string
		var stage, repairClass, findingsJSON, updatedAt *string
		if err := row.Scan(&proj, &stage, &repairClass, &findingsJSON, &updatedAt); err != nil {
			// Not found or table missing — both are fine.
			return
		}
		var findings interface{}
		if findingsJSON != nil && *findingsJSON != "" {
			var parsed interface{}
			if json.Unmarshal([]byte(*findingsJSON), &parsed) == nil {
				findings = parsed
			} else {
				findings = *findingsJSON
			}
		}
		upgradeState = &UpgradeState{
			Project:     proj,
			Stage:       stage,
			RepairClass: repairClass,
			Findings:    findings,
			UpdatedAt:   updatedAt,
		}
	}()

	return &SyncProjectDetailResponse{
		Summary:      *summary,
		Pending:      pending,
		UpgradeState: upgradeState,
	}, nil
}

// SyncComputeIssues computes the diagnostic issues for all projects.
func SyncComputeIssues(db *sql.DB, daemonAvailable bool) (SyncIssuesResponse, error) {
	data, err := SyncListProjects(db)
	if err != nil {
		return SyncIssuesResponse{}, err
	}

	issues := []SyncIssue{}
	nowMs := time.Now().UnixMilli()
	oneHourMs := int64(60 * 60 * 1000)
	thirtyMinMs := int64(30 * 60 * 1000)

	if !daemonAvailable {
		hint := "Start the daemon with `engram serve` and reload"
		issues = append(issues, SyncIssue{
			Code:     "DAEMON_DOWN",
			Severity: SyncIssueSeverityHigh,
			Message:  "engram serve is not reachable — telemetry unavailable",
			Project:  nil,
			Hint:     &hint,
		})
	}

	for _, p := range data.Projects {
		pp := p // local copy for taking addresses
		if p.Project == "" {
			hint := "Find the agent/session capturing without project context and fix it upstream"
			issues = append(issues, SyncIssue{
				Code:     "EMPTY_PROJECT_NAME",
				Severity: SyncIssueSeverityHigh,
				Message:  itoa64(p.ObsCount) + " observations have an empty project name (capture bug)",
				Project:  &pp.Project,
				Hint:     &hint,
				Evidence: map[string]any{"obs_count": p.ObsCount},
			})
		}
		if p.ObsCount > 0 && !p.Enrolled && p.Project != "" {
			hint := "Run `engram cloud enroll " + p.Project + "` to start syncing"
			issues = append(issues, SyncIssue{
				Code:     "NOT_ENROLLED_HAS_DATA",
				Severity: SyncIssueSeverityMedium,
				Message:  p.Project + ": " + itoa64(p.ObsCount) + " observations but project is not enrolled",
				Project:  &pp.Project,
				Hint:     &hint,
				Evidence: map[string]any{"obs_count": p.ObsCount, "last_activity": p.LastActivity},
			})
		}
		if p.Enrolled && p.ObsCount == 0 {
			hint := "Consider unenrolling"
			issues = append(issues, SyncIssue{
				Code:     "ORPHAN_ENROLLED",
				Severity: SyncIssueSeverityLow,
				Message:  p.Project + ": enrolled but no data — likely a false positive enrollment",
				Project:  &pp.Project,
				Hint:     &hint,
			})
		}
		enq := int64(0)
		if p.LastEnqueuedSeq != nil {
			enq = *p.LastEnqueuedSeq
		}
		ack := int64(0)
		if p.LastAckedSeq != nil {
			ack = *p.LastAckedSeq
		}
		if p.Status == SyncStatusBroken {
			// Surface SYNC_BROKEN for EVERY broken state so the issue feed matches
			// the table badge: the silent pending/0-acked case, an explicit
			// lifecycle='failed', and consecutive_failures > 3.
			var msg, hint string
			if p.Lifecycle != nil && *p.Lifecycle == "pending" && ack == 0 && enq > 0 {
				msg = p.Project + ": " + itoa64(enq) + " mutations enqueued, 0 acked — sync is silently broken"
				hint = "Check `last_error` / `reason_message` and verify cloud token / connectivity"
			} else {
				msg = p.Project + ": sync is broken"
				if p.Lifecycle != nil && *p.Lifecycle != "" {
					msg = p.Project + ": sync is broken (lifecycle=" + *p.Lifecycle + ")"
				}
				hint = "Check `last_error` / `reason_message`; a daemon restart may be required"
			}
			issues = append(issues, SyncIssue{
				Code:     "SYNC_BROKEN",
				Severity: SyncIssueSeverityHigh,
				Message:  msg,
				Project:  &pp.Project,
				Hint:     &hint,
				Evidence: map[string]any{
					"last_enqueued_seq": enq,
					"last_acked_seq":    ack,
					"last_error":        p.LastError,
					"reason_message":    p.ReasonMessage,
				},
			})
		}
		if p.ConsecutiveFailures != nil && *p.ConsecutiveFailures > 3 {
			hint := "See last_error"
			issues = append(issues, SyncIssue{
				Code:     "HIGH_FAILURES",
				Severity: SyncIssueSeverityMedium,
				Message:  p.Project + ": " + itoa64(*p.ConsecutiveFailures) + " consecutive failures",
				Project:  &pp.Project,
				Hint:     &hint,
				Evidence: map[string]any{"last_error": p.LastError},
			})
		}
		if p.BackoffUntil != nil && *p.BackoffUntil != "" {
			// backoff_until is stored in LOCAL time (Date.parse without Z in TS),
			// so parse in time.Local to match the TS behavior.
			backoffMs := parseLocalTime(*p.BackoffUntil)
			if backoffMs > 0 && backoffMs-nowMs > oneHourMs {
				hint := "Sync is paused — investigate root cause"
				issues = append(issues, SyncIssue{
					Code:     "LONG_BACKOFF",
					Severity: SyncIssueSeverityMedium,
					Message:  p.Project + ": backoff active until " + *p.BackoffUntil,
					Project:  &pp.Project,
					Hint:     &hint,
				})
			}
		}
		if p.LeaseUntil != nil && *p.LeaseUntil != "" && p.LastAckedSeq != nil {
			// lease_until is also in LOCAL time.
			leaseMs := parseLocalTime(*p.LeaseUntil)
			if leaseMs > 0 && leaseMs > nowMs {
				if p.PendingMutations > 0 && leaseMs-nowMs > thirtyMinMs {
					owner := "?"
					if p.LeaseOwner != nil {
						owner = *p.LeaseOwner
					}
					hint := "Lease may be stuck. Restarting `engram serve` releases it."
					issues = append(issues, SyncIssue{
						Code:     "STUCK_LEASE",
						Severity: SyncIssueSeverityHigh,
						Message:  p.Project + ": lease held by " + owner + " until " + *p.LeaseUntil,
						Project:  &pp.Project,
						Hint:     &hint,
					})
				}
			}
		}
	}

	globalPending := int64(0)
	if data.GlobalTarget != nil {
		globalPending = data.GlobalTarget.PendingMutations
	}
	if globalPending > 100 {
		hint := "Daemon may be slow or paused — check sync status"
		issues = append(issues, SyncIssue{
			Code:     "GLOBAL_QUEUE_LARGE",
			Severity: SyncIssueSeverityLow,
			Message:  itoa64(globalPending) + " mutations pending in the global cloud target",
			Project:  nil,
			Hint:     &hint,
		})
	}

	return SyncIssuesResponse{
		Issues:      issues,
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
	}, nil
}

// parseLocalTime parses a datetime string stored in LOCAL time (no timezone suffix).
// This matches Date.parse(str) in JS when str has no timezone indicator — JS
// treats it as local time when there is no 'Z' or offset suffix.
func parseLocalTime(s string) int64 {
	// Try standard SQLite format "YYYY-MM-DD HH:MM:SS" in local time.
	t, err := time.ParseInLocation("2006-01-02 15:04:05", s, time.Local)
	if err != nil {
		// Fallback: try RFC3339 if the daemon stored it with a timezone.
		t2, err2 := time.Parse(time.RFC3339, s)
		if err2 != nil {
			return 0
		}
		return t2.UnixMilli()
	}
	return t.UnixMilli()
}

// itoa64 converts an int64 to a decimal string without importing fmt.
func itoa64(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	buf := make([]byte, 20)
	pos := len(buf)
	for n > 0 {
		pos--
		buf[pos] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}
