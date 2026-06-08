package services_test

import (
	"database/sql"
	"encoding/base64"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/AlvaroQ/engram-explorer/internal/cursor"
	"github.com/AlvaroQ/engram-explorer/internal/services"
)

// openSessionTestDB creates a temporary SQLite DB with a minimal sessions schema.
func openSessionTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "engram_sess_test.db")
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	for _, s := range []string{
		`CREATE TABLE sessions (
			id         TEXT PRIMARY KEY,
			project    TEXT,
			directory  TEXT,
			started_at TEXT,
			ended_at   TEXT,
			summary    TEXT
		)`,
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
		`CREATE TABLE user_prompts (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id TEXT,
			content    TEXT NOT NULL DEFAULT '',
			project    TEXT,
			created_at TEXT,
			sync_id    TEXT
		)`,
	} {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("schema: %v", err)
		}
	}
	return db
}

func seedSessionData(t *testing.T, db *sql.DB) {
	t.Helper()
	for _, stmt := range []string{
		`INSERT INTO sessions (id, project, directory, started_at) VALUES ('s1', 'proj-a', '/a', '2026-01-10 10:00:00')`,
		`INSERT INTO sessions (id, project, directory, started_at) VALUES ('s2', 'proj-a', '/a', '2026-01-11 10:00:00')`,
		`INSERT INTO sessions (id, project, directory, started_at) VALUES ('s3', 'proj-b', '/b', '2026-01-12 10:00:00')`,
		`INSERT INTO observations (session_id, type, title, content, project, topic_key, created_at, updated_at) VALUES ('s1', 'note', 'A1', 'content1', 'proj-a', 'k/a', '2026-01-10 11:00:00', '2026-01-10 11:00:00')`,
		`INSERT INTO observations (session_id, type, title, content, project, topic_key, created_at, updated_at) VALUES ('s2', 'note', 'A2', 'content2', 'proj-a', 'k/b', '2026-01-11 11:00:00', '2026-01-11 11:00:00')`,
		`INSERT INTO user_prompts (session_id, content, project, created_at) VALUES ('s1', 'prompt1', 'proj-a', '2026-01-10 10:30:00')`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
}

func TestSessionsList_BasicPagination(t *testing.T) {
	db := openSessionTestDB(t)
	seedSessionData(t, db)

	result, err := services.SessionsList(db, services.SessionListParams{Limit: 10})
	if err != nil {
		t.Fatalf("SessionsList: %v", err)
	}
	if len(result.Items) != 3 {
		t.Errorf("expected 3 sessions, got %d", len(result.Items))
	}
	if result.NextCursor != nil {
		t.Errorf("expected nil nextCursor for full page, got non-nil")
	}
}

func TestSessionsList_LimitAndCursor(t *testing.T) {
	db := openSessionTestDB(t)
	seedSessionData(t, db)

	// Page 1: limit=2.
	page1, err := services.SessionsList(db, services.SessionListParams{Limit: 2})
	if err != nil {
		t.Fatalf("page1: %v", err)
	}
	if len(page1.Items) != 2 {
		t.Errorf("page1: expected 2 items, got %d", len(page1.Items))
	}
	if page1.NextCursor == nil {
		t.Fatal("page1: expected nextCursor, got nil")
	}

	// Page 2: use cursor from page 1.
	page2, err := services.SessionsList(db, services.SessionListParams{
		Limit:  2,
		Cursor: *page1.NextCursor,
	})
	if err != nil {
		t.Fatalf("page2: %v", err)
	}
	if len(page2.Items) != 1 {
		t.Errorf("page2: expected 1 item, got %d", len(page2.Items))
	}
	if page2.NextCursor != nil {
		t.Error("page2: expected nil nextCursor")
	}

	// No overlap between pages.
	seen := make(map[string]bool)
	for _, item := range page1.Items {
		seen[item.ID] = true
	}
	for _, item := range page2.Items {
		if seen[item.ID] {
			t.Errorf("overlap: id %q in both pages", item.ID)
		}
	}
}

// TestSessionsCursorRoundTrip verifies that the composite cursor used for TEXT
// session ids encodes and decodes correctly via the cursor package.
func TestSessionsCursorRoundTrip(t *testing.T) {
	cases := []struct {
		name      string
		sessionID string
		orderKey  string
	}{
		{"simple", "sess-abc", "2026-01-10 10:00:00"},
		{"with-pipe", "sess|with|pipes", "2026-05-01 12:00:00"},
		{"empty-order", "some-id", ""},
		{"unicode", "sëssion-üñícödé", "2026-12-31 23:59:59"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			idHash := base64.RawURLEncoding.EncodeToString([]byte(tc.sessionID))
			composite := tc.orderKey + "|" + idHash
			// cursor.Encode uses orderKey|id; for TEXT ids we embed in orderKey with id=0.
			encoded := cursor.Encode(composite, 0)

			decoded := cursor.Decode(encoded)
			if decoded == nil {
				t.Fatal("cursor.Decode returned nil")
			}
			if decoded.ID != 0 {
				t.Errorf("id: got %d, want 0", decoded.ID)
			}

			// Split on last '|' to extract real orderKey and id hash.
			combined := decoded.OrderKey
			sep := strings.LastIndex(combined, "|")
			if sep < 0 {
				t.Fatal("no '|' in decoded orderKey")
			}
			gotOrderKey := combined[:sep]
			gotIDHash := combined[sep+1:]

			if gotOrderKey != tc.orderKey {
				t.Errorf("orderKey: got %q, want %q", gotOrderKey, tc.orderKey)
			}

			gotIDBytes, err := base64.RawURLEncoding.DecodeString(gotIDHash)
			if err != nil {
				t.Fatalf("base64url decode: %v", err)
			}
			if string(gotIDBytes) != tc.sessionID {
				t.Errorf("sessionID: got %q, want %q", string(gotIDBytes), tc.sessionID)
			}
		})
	}
}

func TestSessionsList_SortRecentActivity(t *testing.T) {
	db := openSessionTestDB(t)
	seedSessionData(t, db)

	result, err := services.SessionsList(db, services.SessionListParams{
		Limit: 10,
		Sort:  "recent_activity",
	})
	if err != nil {
		t.Fatalf("recent_activity: %v", err)
	}
	if len(result.Items) != 3 {
		t.Errorf("expected 3 sessions, got %d", len(result.Items))
	}
	// s3 has no observations; its coalesce falls back to started_at=2026-01-12 which is the latest.
	if result.Items[0].ID != "s3" {
		t.Errorf("first item: got %q, want s3 (latest started_at)", result.Items[0].ID)
	}
}

func TestSessionsGetDetail_EventMergeAndSort(t *testing.T) {
	db := openSessionTestDB(t)
	for _, s := range []string{
		`INSERT INTO sessions (id, project, directory, started_at) VALUES ('s-evts', 'p', '/p', '2026-01-01 09:00:00')`,
		`INSERT INTO observations (session_id, type, content, created_at, updated_at) VALUES ('s-evts', 'note', 'obs1', '2026-01-01 10:00:00', '2026-01-01 10:00:00')`,
		`INSERT INTO user_prompts (session_id, content, created_at) VALUES ('s-evts', 'prompt1', '2026-01-01 09:30:00')`,
		`INSERT INTO observations (session_id, type, content, created_at, updated_at) VALUES ('s-evts', 'note', 'obs2', '2026-01-01 11:00:00', '2026-01-01 11:00:00')`,
		`INSERT INTO user_prompts (session_id, content, created_at) VALUES ('s-evts', 'prompt2', '2026-01-01 10:30:00')`,
	} {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	detail, err := services.SessionsGetDetail(db, "s-evts")
	if err != nil {
		t.Fatalf("getDetail: %v", err)
	}
	if detail == nil {
		t.Fatal("detail is nil")
	}
	if len(detail.Events) != 4 {
		t.Fatalf("expected 4 events, got %d", len(detail.Events))
	}

	// Should be sorted by `at`: prompt1(09:30) obs1(10:00) prompt2(10:30) obs2(11:00).
	expectedKinds := []string{"prompt", "observation", "prompt", "observation"}
	for i, ev := range detail.Events {
		if ev.Kind != expectedKinds[i] {
			t.Errorf("events[%d].kind: got %q, want %q", i, ev.Kind, expectedKinds[i])
		}
	}

	if detail.Stats.ObsTotal != 2 {
		t.Errorf("obs_total: got %d, want 2", detail.Stats.ObsTotal)
	}
	if detail.Stats.PromptsTotal != 2 {
		t.Errorf("prompts_total: got %d, want 2", detail.Stats.PromptsTotal)
	}
}

func TestSessionsGetDetail_NotFound(t *testing.T) {
	db := openSessionTestDB(t)
	detail, err := services.SessionsGetDetail(db, "non-existent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if detail != nil {
		t.Error("expected nil detail for non-existent session")
	}
}
