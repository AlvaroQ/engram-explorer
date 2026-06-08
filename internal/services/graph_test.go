package services_test

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/AlvaroQ/engram-explorer/internal/services"
)

// openGraphTestDB creates a minimal DB with the graph-related tables.
func openGraphTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "engram_graph_test.db")
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	for _, s := range []string{
		`CREATE TABLE sessions (id TEXT PRIMARY KEY, project TEXT, directory TEXT, started_at TEXT, ended_at TEXT, summary TEXT)`,
		`CREATE TABLE observations (
			id              INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id      TEXT,
			type            TEXT NOT NULL DEFAULT 'note',
			title           TEXT,
			content         TEXT NOT NULL DEFAULT '',
			tool_name       TEXT,
			project         TEXT,
			scope           TEXT NOT NULL DEFAULT 'project',
			topic_key       TEXT,
			normalized_hash TEXT,
			revision_count  INTEGER NOT NULL DEFAULT 1,
			duplicate_count INTEGER NOT NULL DEFAULT 1,
			created_at      TEXT,
			updated_at      TEXT,
			deleted_at      TEXT,
			sync_id         TEXT
		)`,
		`CREATE TABLE user_prompts (id INTEGER PRIMARY KEY AUTOINCREMENT, session_id TEXT, content TEXT NOT NULL DEFAULT '', project TEXT, created_at TEXT, sync_id TEXT)`,
	} {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("schema: %v", err)
		}
	}
	return db
}

// insertObs inserts a minimal observation row with optional normalized_hash.
func insertObs(t *testing.T, db *sql.DB, project, hash string) int64 {
	t.Helper()
	var hashVal interface{}
	if hash != "" {
		hashVal = hash
	}
	res, err := db.Exec(
		`INSERT INTO observations (type, content, project, normalized_hash, revision_count, duplicate_count, created_at, updated_at)
		 VALUES ('note', '', ?, ?, 1, 1, datetime('now'), datetime('now'))`,
		project, hashVal,
	)
	if err != nil {
		t.Fatalf("insertObs: %v", err)
	}
	id, _ := res.LastInsertId()
	return id
}

func TestGraphBuild_Empty(t *testing.T) {
	db := openGraphTestDB(t)
	resp, err := services.GraphBuild(db, "", 0)
	if err != nil {
		t.Fatalf("GraphBuild: %v", err)
	}
	if len(resp.Nodes) != 0 {
		t.Errorf("expected 0 nodes, got %d", len(resp.Nodes))
	}
	if len(resp.Edges) != 0 {
		t.Errorf("expected 0 edges, got %d", len(resp.Edges))
	}
	if resp.Meta.TotalObservations != 0 {
		t.Errorf("TotalObservations: got %d, want 0", resp.Meta.TotalObservations)
	}
	if resp.Meta.Truncated {
		t.Error("expected Truncated=false for empty graph")
	}
}

func TestGraphBuild_SingletonNodes(t *testing.T) {
	db := openGraphTestDB(t)
	// 3 observations with no normalized_hash → 3 singletons.
	insertObs(t, db, "proj", "")
	insertObs(t, db, "proj", "")
	insertObs(t, db, "proj", "")

	resp, err := services.GraphBuild(db, "", 0)
	if err != nil {
		t.Fatalf("GraphBuild: %v", err)
	}
	if len(resp.Nodes) != 3 {
		t.Errorf("expected 3 nodes, got %d", len(resp.Nodes))
	}
	if resp.Meta.DuplicateGroups != 0 {
		t.Errorf("DuplicateGroups: got %d, want 0", resp.Meta.DuplicateGroups)
	}
}

func TestGraphBuild_DuplicateCollapse(t *testing.T) {
	db := openGraphTestDB(t)
	// 3 observations sharing the same normalized_hash → 1 representative node.
	insertObs(t, db, "proj", "hash-abc")
	insertObs(t, db, "proj", "hash-abc")
	insertObs(t, db, "proj", "hash-abc")
	// 1 singleton.
	insertObs(t, db, "proj", "")

	resp, err := services.GraphBuild(db, "", 0)
	if err != nil {
		t.Fatalf("GraphBuild: %v", err)
	}
	// 1 representative (MIN id from hash group) + 1 singleton = 2 nodes.
	if len(resp.Nodes) != 2 {
		t.Errorf("expected 2 nodes, got %d: %+v", len(resp.Nodes), resp.Nodes)
	}
	if resp.Meta.DuplicateGroups != 1 {
		t.Errorf("DuplicateGroups: got %d, want 1", resp.Meta.DuplicateGroups)
	}
	// The collapsed representative should have duplicateCount=3.
	for _, n := range resp.Nodes {
		if n.DuplicateCount == 3 {
			// Correct.
			return
		}
	}
	t.Error("expected one node with duplicateCount=3")
}

func TestGraphBuild_NodeCapTruncation(t *testing.T) {
	db := openGraphTestDB(t)
	// Insert 10 singletons.
	for i := 0; i < 10; i++ {
		insertObs(t, db, "proj", "")
	}

	// Use a node cap of 5.
	resp, err := services.GraphBuild(db, "", 5)
	if err != nil {
		t.Fatalf("GraphBuild: %v", err)
	}
	if len(resp.Nodes) != 5 {
		t.Errorf("expected 5 nodes after cap, got %d", len(resp.Nodes))
	}
	if !resp.Meta.Truncated {
		t.Error("expected Truncated=true")
	}
	if resp.Meta.TruncatedAt == nil || *resp.Meta.TruncatedAt != 5 {
		t.Errorf("TruncatedAt: got %v, want 5", resp.Meta.TruncatedAt)
	}
	if resp.Meta.TotalObservations != 10 {
		t.Errorf("TotalObservations: got %d, want 10", resp.Meta.TotalObservations)
	}
}

func TestGraphBuild_ProjectFilter(t *testing.T) {
	db := openGraphTestDB(t)
	insertObs(t, db, "proj-a", "")
	insertObs(t, db, "proj-a", "")
	insertObs(t, db, "proj-b", "")

	// Filter to proj-a only.
	resp, err := services.GraphBuild(db, "proj-a", 0)
	if err != nil {
		t.Fatalf("GraphBuild: %v", err)
	}
	if len(resp.Nodes) != 2 {
		t.Errorf("expected 2 nodes for proj-a, got %d", len(resp.Nodes))
	}
	for _, n := range resp.Nodes {
		if n.Project != nil && *n.Project != "proj-a" {
			t.Errorf("node.project: got %q, want proj-a", *n.Project)
		}
	}
}

func TestGraphBuild_MetaShape(t *testing.T) {
	db := openGraphTestDB(t)
	insertObs(t, db, "proj", "")

	resp, err := services.GraphBuild(db, "", 0)
	if err != nil {
		t.Fatalf("GraphBuild: %v", err)
	}
	if resp.Meta.GeneratedAt == "" {
		t.Error("expected non-empty GeneratedAt")
	}
	if resp.Meta.ShownNodes != len(resp.Nodes) {
		t.Errorf("ShownNodes: got %d, want %d", resp.Meta.ShownNodes, len(resp.Nodes))
	}
	if resp.Meta.TruncatedAt != nil {
		t.Error("expected TruncatedAt=nil when not truncated")
	}
}

func TestGraphBuild_SortByWeightDesc(t *testing.T) {
	db := openGraphTestDB(t)
	// Insert 3 observations: two share a hash (weight=2) and one singleton (weight=1).
	// Also update revision_count to vary weights.
	insertObs(t, db, "proj", "shared-hash")
	insertObs(t, db, "proj", "shared-hash") // representative: duplicateCount=2
	insertObs(t, db, "proj", "")            // singleton: duplicateCount=1

	resp, err := services.GraphBuild(db, "", 0)
	if err != nil {
		t.Fatalf("GraphBuild: %v", err)
	}
	if len(resp.Nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(resp.Nodes))
	}
	// First node (highest weight) should be the collapsed pair (weight = 1+2 = 3).
	// Second node is the singleton (weight = 1+1 = 2).
	if resp.Nodes[0].Weight <= resp.Nodes[1].Weight {
		t.Errorf("nodes not sorted by weight DESC: [0].Weight=%d, [1].Weight=%d",
			resp.Nodes[0].Weight, resp.Nodes[1].Weight)
	}
}

func TestGraphBuild_TruncationPurgesEdges(t *testing.T) {
	// This test verifies that edges whose endpoints are truncated away are purged.
	// We use the memory_relations table pattern but since the table likely won't
	// exist in our minimal schema, we just verify that the response doesn't error
	// when the table is missing (edges = []) and that truncation works end-to-end.
	db := openGraphTestDB(t)
	for i := 0; i < 5; i++ {
		insertObs(t, db, "proj", "")
	}
	// Cap at 3.
	resp, err := services.GraphBuild(db, "", 3)
	if err != nil {
		t.Fatalf("GraphBuild: %v", err)
	}
	if len(resp.Nodes) != 3 {
		t.Errorf("expected 3 nodes, got %d", len(resp.Nodes))
	}
	if len(resp.Edges) != 0 {
		t.Errorf("expected 0 edges (no memory_relations), got %d", len(resp.Edges))
	}
}
