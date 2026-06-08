package services_test

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/AlvaroQ/engram-explorer/internal/services"
)

// openSyncTestDB creates a minimal DB with the sync-related tables.
func openSyncTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "engram_sync_test.db")
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	for _, s := range []string{
		`CREATE TABLE sessions (id TEXT PRIMARY KEY, project TEXT, directory TEXT, started_at TEXT, ended_at TEXT, summary TEXT)`,
		`CREATE TABLE observations (
			id INTEGER PRIMARY KEY AUTOINCREMENT, session_id TEXT, type TEXT NOT NULL DEFAULT 'note',
			title TEXT, content TEXT NOT NULL DEFAULT '', project TEXT, scope TEXT NOT NULL DEFAULT 'project',
			topic_key TEXT, normalized_hash TEXT, revision_count INTEGER NOT NULL DEFAULT 1,
			duplicate_count INTEGER NOT NULL DEFAULT 1, created_at TEXT, updated_at TEXT,
			deleted_at TEXT, sync_id TEXT)`,
		`CREATE TABLE user_prompts (id INTEGER PRIMARY KEY AUTOINCREMENT, session_id TEXT, content TEXT NOT NULL DEFAULT '', project TEXT, created_at TEXT, sync_id TEXT)`,
		`CREATE TABLE sync_enrolled_projects (project TEXT PRIMARY KEY, enrolled_at TEXT NOT NULL DEFAULT (datetime('now')))`,
		`CREATE TABLE sync_state (
			target_key           TEXT PRIMARY KEY,
			lifecycle            TEXT NOT NULL DEFAULT 'idle',
			last_enqueued_seq    INTEGER NOT NULL DEFAULT 0,
			last_acked_seq       INTEGER NOT NULL DEFAULT 0,
			last_pulled_seq      INTEGER NOT NULL DEFAULT 0,
			consecutive_failures INTEGER NOT NULL DEFAULT 0,
			backoff_until        TEXT,
			lease_owner          TEXT,
			lease_until          TEXT,
			last_error           TEXT,
			updated_at           TEXT NOT NULL DEFAULT (datetime('now')),
			reason_code          TEXT,
			reason_message       TEXT
		)`,
		`CREATE TABLE sync_mutations (
			seq INTEGER PRIMARY KEY AUTOINCREMENT, target_key TEXT NOT NULL,
			entity TEXT NOT NULL, entity_key TEXT NOT NULL, op TEXT NOT NULL,
			payload TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'local',
			occurred_at TEXT NOT NULL DEFAULT (datetime('now')), acked_at TEXT,
			project TEXT NOT NULL DEFAULT '')`,
	} {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("schema: %v", err)
		}
	}
	return db
}

// seedSyncProject inserts a session + observation for a project so it appears
// in the aggregates query.
func seedSyncProject(t *testing.T, db *sql.DB, project string) {
	t.Helper()
	sessID := "sess-" + project
	if _, err := db.Exec(`INSERT INTO sessions (id, project, directory, started_at) VALUES (?, ?, '/', datetime('now'))`, sessID, project); err != nil {
		t.Fatalf("seed session: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO observations (session_id, type, content, project, created_at, updated_at) VALUES (?, 'note', 'c', ?, datetime('now'), datetime('now'))`, sessID, project); err != nil {
		t.Fatalf("seed obs: %v", err)
	}
}

// TestSyncIssues_DaemonDown verifies DAEMON_DOWN is emitted when daemon is unavailable.
func TestSyncIssues_DaemonDown(t *testing.T) {
	db := openSyncTestDB(t)
	result, err := services.SyncComputeIssues(db, false /* daemonAvailable */)
	if err != nil {
		t.Fatalf("ComputeIssues: %v", err)
	}
	found := false
	for _, issue := range result.Issues {
		if issue.Code == "DAEMON_DOWN" {
			found = true
			if issue.Severity != services.SyncIssueSeverityHigh {
				t.Errorf("DAEMON_DOWN severity: got %v, want HIGH", issue.Severity)
			}
		}
	}
	if !found {
		t.Error("expected DAEMON_DOWN issue, not found")
	}
}

// TestSyncIssues_NotEnrolledHasData verifies NOT_ENROLLED_HAS_DATA issue.
func TestSyncIssues_NotEnrolledHasData(t *testing.T) {
	db := openSyncTestDB(t)
	seedSyncProject(t, db, "unenrolled-project")
	// Do NOT enroll it.

	result, err := services.SyncComputeIssues(db, true)
	if err != nil {
		t.Fatalf("ComputeIssues: %v", err)
	}
	found := false
	for _, issue := range result.Issues {
		if issue.Code == "NOT_ENROLLED_HAS_DATA" {
			found = true
			if issue.Project == nil || *issue.Project != "unenrolled-project" {
				t.Errorf("project: got %v, want unenrolled-project", issue.Project)
			}
		}
	}
	if !found {
		t.Error("expected NOT_ENROLLED_HAS_DATA issue")
	}
}

// TestSyncIssues_OrphanEnrolled verifies ORPHAN_ENROLLED issue.
func TestSyncIssues_OrphanEnrolled(t *testing.T) {
	db := openSyncTestDB(t)
	// Enroll a project that has no data.
	if _, err := db.Exec(`INSERT INTO sync_enrolled_projects (project, enrolled_at) VALUES ('ghost-proj', datetime('now'))`); err != nil {
		t.Fatalf("enroll: %v", err)
	}

	result, err := services.SyncComputeIssues(db, true)
	if err != nil {
		t.Fatalf("ComputeIssues: %v", err)
	}
	found := false
	for _, issue := range result.Issues {
		if issue.Code == "ORPHAN_ENROLLED" {
			found = true
			if issue.Project == nil || *issue.Project != "ghost-proj" {
				t.Errorf("project: got %v, want ghost-proj", issue.Project)
			}
		}
	}
	if !found {
		t.Error("expected ORPHAN_ENROLLED issue")
	}
}

// TestSyncIssues_SyncBroken_AfterGrace verifies SYNC_BROKEN after the grace period.
func TestSyncIssues_SyncBroken_AfterGrace(t *testing.T) {
	db := openSyncTestDB(t)
	seedSyncProject(t, db, "stuck-proj")
	if _, err := db.Exec(`INSERT INTO sync_enrolled_projects (project, enrolled_at) VALUES ('stuck-proj', datetime('now', '-1 hour'))`); err != nil {
		t.Fatalf("enroll: %v", err)
	}
	// Insert a stale sync_state: lifecycle=pending, enqueued>0, acked=0, updated_at 10 min ago (> 5 min grace).
	staleUpdatedAt := time.Now().UTC().Add(-10 * time.Minute).Format("2006-01-02 15:04:05")
	if _, err := db.Exec(
		`INSERT INTO sync_state (target_key, lifecycle, last_enqueued_seq, last_acked_seq, updated_at) VALUES ('cloud:stuck-proj', 'pending', 5, 0, ?)`,
		staleUpdatedAt,
	); err != nil {
		t.Fatalf("insert sync_state: %v", err)
	}

	result, err := services.SyncComputeIssues(db, true)
	if err != nil {
		t.Fatalf("ComputeIssues: %v", err)
	}
	found := false
	for _, issue := range result.Issues {
		if issue.Code == "SYNC_BROKEN" {
			found = true
		}
	}
	if !found {
		t.Error("expected SYNC_BROKEN issue after grace period")
	}
}

// TestSyncIssues_SyncBroken_WithinGrace verifies no SYNC_BROKEN within the grace period.
func TestSyncIssues_SyncBroken_WithinGrace(t *testing.T) {
	db := openSyncTestDB(t)
	seedSyncProject(t, db, "fresh-proj")
	if _, err := db.Exec(`INSERT INTO sync_enrolled_projects (project, enrolled_at) VALUES ('fresh-proj', datetime('now'))`); err != nil {
		t.Fatalf("enroll: %v", err)
	}
	// Insert sync_state: lifecycle=pending, enqueued>0, acked=0, updated_at just now (within grace).
	freshUpdatedAt := time.Now().UTC().Add(-30 * time.Second).Format("2006-01-02 15:04:05")
	if _, err := db.Exec(
		`INSERT INTO sync_state (target_key, lifecycle, last_enqueued_seq, last_acked_seq, updated_at) VALUES ('cloud:fresh-proj', 'pending', 3, 0, ?)`,
		freshUpdatedAt,
	); err != nil {
		t.Fatalf("insert sync_state: %v", err)
	}

	result, err := services.SyncComputeIssues(db, true)
	if err != nil {
		t.Fatalf("ComputeIssues: %v", err)
	}
	for _, issue := range result.Issues {
		if issue.Code == "SYNC_BROKEN" && issue.Project != nil && *issue.Project == "fresh-proj" {
			t.Error("expected NO SYNC_BROKEN within grace period, but got one")
		}
	}
}

// TestSyncIssues_SyncBroken_LifecycleFailed verifies SYNC_BROKEN is emitted for
// an explicit lifecycle='failed' state (not only the silent pending/0-acked case).
func TestSyncIssues_SyncBroken_LifecycleFailed(t *testing.T) {
	db := openSyncTestDB(t)
	seedSyncProject(t, db, "failed-proj")
	if _, err := db.Exec(`INSERT INTO sync_enrolled_projects (project, enrolled_at) VALUES ('failed-proj', datetime('now', '-1 hour'))`); err != nil {
		t.Fatalf("enroll: %v", err)
	}
	// lifecycle='failed' with mutations acked — NOT the pending/0-acked path.
	if _, err := db.Exec(
		`INSERT INTO sync_state (target_key, lifecycle, last_enqueued_seq, last_acked_seq, updated_at) VALUES ('cloud:failed-proj', 'failed', 2, 1, datetime('now'))`,
	); err != nil {
		t.Fatalf("insert sync_state: %v", err)
	}

	result, err := services.SyncComputeIssues(db, true)
	if err != nil {
		t.Fatalf("ComputeIssues: %v", err)
	}
	found := false
	for _, issue := range result.Issues {
		if issue.Code == "SYNC_BROKEN" && issue.Project != nil && *issue.Project == "failed-proj" {
			found = true
		}
	}
	if !found {
		t.Error("expected SYNC_BROKEN issue for lifecycle='failed'")
	}
}

// TestSyncIssues_HighFailures verifies HIGH_FAILURES issue.
func TestSyncIssues_HighFailures(t *testing.T) {
	db := openSyncTestDB(t)
	seedSyncProject(t, db, "failing-proj")
	if _, err := db.Exec(`INSERT INTO sync_enrolled_projects (project, enrolled_at) VALUES ('failing-proj', datetime('now'))`); err != nil {
		t.Fatalf("enroll: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO sync_state (target_key, lifecycle, consecutive_failures, updated_at) VALUES ('cloud:failing-proj', 'idle', 5, datetime('now'))`,
	); err != nil {
		t.Fatalf("insert sync_state: %v", err)
	}

	result, err := services.SyncComputeIssues(db, true)
	if err != nil {
		t.Fatalf("ComputeIssues: %v", err)
	}
	found := false
	for _, issue := range result.Issues {
		if issue.Code == "HIGH_FAILURES" {
			found = true
		}
	}
	if !found {
		t.Error("expected HIGH_FAILURES issue")
	}
}

// TestSyncIssues_LongBackoff verifies LONG_BACKOFF issue.
func TestSyncIssues_LongBackoff(t *testing.T) {
	db := openSyncTestDB(t)
	seedSyncProject(t, db, "backoff-proj")
	if _, err := db.Exec(`INSERT INTO sync_enrolled_projects (project, enrolled_at) VALUES ('backoff-proj', datetime('now'))`); err != nil {
		t.Fatalf("enroll: %v", err)
	}
	// backoff_until is stored in LOCAL time — set it to 2 hours from now.
	backoffUntil := time.Now().In(time.Local).Add(2 * time.Hour).Format("2006-01-02 15:04:05")
	if _, err := db.Exec(
		`INSERT INTO sync_state (target_key, lifecycle, backoff_until, updated_at) VALUES ('cloud:backoff-proj', 'idle', ?, datetime('now'))`,
		backoffUntil,
	); err != nil {
		t.Fatalf("insert sync_state: %v", err)
	}

	result, err := services.SyncComputeIssues(db, true)
	if err != nil {
		t.Fatalf("ComputeIssues: %v", err)
	}
	found := false
	for _, issue := range result.Issues {
		if issue.Code == "LONG_BACKOFF" {
			found = true
		}
	}
	if !found {
		t.Error("expected LONG_BACKOFF issue")
	}
}

// TestSyncIssues_StuckLease verifies STUCK_LEASE issue.
func TestSyncIssues_StuckLease(t *testing.T) {
	db := openSyncTestDB(t)
	seedSyncProject(t, db, "lease-proj")
	if _, err := db.Exec(`INSERT INTO sync_enrolled_projects (project, enrolled_at) VALUES ('lease-proj', datetime('now'))`); err != nil {
		t.Fatalf("enroll: %v", err)
	}
	// lease_until 2 hours from now (LOCAL time), with pending mutations.
	leaseUntil := time.Now().In(time.Local).Add(2 * time.Hour).Format("2006-01-02 15:04:05")
	if _, err := db.Exec(
		`INSERT INTO sync_state (target_key, lifecycle, last_acked_seq, lease_owner, lease_until, updated_at) VALUES ('cloud:lease-proj', 'idle', 1, 'worker-abc', ?, datetime('now'))`,
		leaseUntil,
	); err != nil {
		t.Fatalf("insert sync_state: %v", err)
	}
	// Insert a pending mutation.
	if _, err := db.Exec(
		`INSERT INTO sync_mutations (target_key, entity, entity_key, op, payload, project) VALUES ('cloud', 'observation', 'key1', 'upsert', '{}', 'lease-proj')`,
	); err != nil {
		t.Fatalf("insert mutation: %v", err)
	}

	result, err := services.SyncComputeIssues(db, true)
	if err != nil {
		t.Fatalf("ComputeIssues: %v", err)
	}
	found := false
	for _, issue := range result.Issues {
		if issue.Code == "STUCK_LEASE" {
			found = true
		}
	}
	if !found {
		t.Error("expected STUCK_LEASE issue")
	}
}

// TestSyncIssues_GlobalQueueLarge verifies GLOBAL_QUEUE_LARGE issue.
func TestSyncIssues_GlobalQueueLarge(t *testing.T) {
	db := openSyncTestDB(t)
	// Insert 101 pending mutations in the 'cloud' global target.
	for i := 0; i < 101; i++ {
		if _, err := db.Exec(
			`INSERT INTO sync_mutations (target_key, entity, entity_key, op, payload, project) VALUES ('cloud', 'observation', 'key', 'upsert', '{}', '')`,
		); err != nil {
			t.Fatalf("insert mutation %d: %v", i, err)
		}
	}
	if _, err := db.Exec(`INSERT INTO sync_state (target_key, lifecycle, updated_at) VALUES ('cloud', 'idle', datetime('now'))`); err != nil {
		t.Fatalf("insert global sync_state: %v", err)
	}

	result, err := services.SyncComputeIssues(db, true)
	if err != nil {
		t.Fatalf("ComputeIssues: %v", err)
	}
	found := false
	for _, issue := range result.Issues {
		if issue.Code == "GLOBAL_QUEUE_LARGE" {
			found = true
		}
	}
	if !found {
		t.Error("expected GLOBAL_QUEUE_LARGE issue")
	}
}

// TestSyncListProjects_Empty verifies that empty DB returns empty projects list.
func TestSyncListProjects_Empty(t *testing.T) {
	db := openSyncTestDB(t)
	result, err := services.SyncListProjects(db)
	if err != nil {
		t.Fatalf("SyncListProjects: %v", err)
	}
	if len(result.Projects) != 0 {
		t.Errorf("expected empty projects, got %d", len(result.Projects))
	}
}

// TestSyncProjectDetail_NotFound verifies nil return for missing project.
func TestSyncProjectDetail_NotFound(t *testing.T) {
	db := openSyncTestDB(t)
	detail, err := services.SyncGetProjectDetail(db, "no-such-project")
	if err != nil {
		t.Fatalf("SyncGetProjectDetail: %v", err)
	}
	if detail != nil {
		t.Error("expected nil detail for non-existent project")
	}
}
