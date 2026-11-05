package services_test

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/services"
	_ "modernc.org/sqlite"
)

// Tokenizer fixtures: the explorer's own seed (cmd/seed-demo) uses the FTS5
// default tokenizer, upstream Engram uses trigram with diacritics removal.
const (
	tokenizeDefault = ""
	tokenizeTrigram = "trigram remove_diacritics 1"
)

// newSearchDB builds a minimal Engram-shaped schema whose FTS tables use the
// given tokenizer and are kept in sync by insert triggers, like upstream.
func newSearchDB(t *testing.T, tokenize string) *sql.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "search.db")
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })

	tok := ""
	if tokenize != "" {
		tok = fmt.Sprintf(", tokenize='%s'", tokenize)
	}
	stmts := []string{
		`CREATE TABLE observations (
			id INTEGER PRIMARY KEY AUTOINCREMENT, session_id TEXT, type TEXT NOT NULL DEFAULT 'note',
			title TEXT, content TEXT, tool_name TEXT, project TEXT, scope TEXT DEFAULT 'project',
			topic_key TEXT, normalized_hash TEXT, revision_count INTEGER DEFAULT 1,
			duplicate_count INTEGER DEFAULT 1, last_seen_at TEXT, created_at TEXT, updated_at TEXT,
			deleted_at TEXT, sync_id TEXT, review_after TEXT)`,
		`CREATE VIRTUAL TABLE observations_fts USING fts5(
			title, content, tool_name, type, project, topic_key,
			content='observations', content_rowid='id'` + tok + `)`,
		`CREATE TRIGGER obs_fts_insert AFTER INSERT ON observations BEGIN
			INSERT INTO observations_fts(rowid, title, content, tool_name, type, project, topic_key)
			VALUES (new.id, new.title, new.content, new.tool_name, new.type, new.project, new.topic_key);
		END`,
		`CREATE TABLE user_prompts (
			id INTEGER PRIMARY KEY AUTOINCREMENT, session_id TEXT, content TEXT,
			project TEXT, created_at TEXT, sync_id TEXT)`,
		`CREATE VIRTUAL TABLE prompts_fts USING fts5(
			content, project, content='user_prompts', content_rowid='id'` + tok + `)`,
		`CREATE TRIGGER prompt_fts_insert AFTER INSERT ON user_prompts BEGIN
			INSERT INTO prompts_fts(rowid, content, project) VALUES (new.id, new.content, new.project);
		END`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("schema: %v\n%s", err, s)
		}
	}
	return db
}

// addObs inserts an observation and returns its id. Later inserts get a
// larger id and a later updated_at, so the id DESC tie-break favours them.
func addObs(t *testing.T, db *sql.DB, title, content, topicKey string) int64 {
	t.Helper()
	res, err := db.Exec(`INSERT INTO observations (title, content, project, topic_key, created_at, updated_at)
		VALUES (?, ?, 'proj', NULLIF(?, ''), datetime('2026-01-01', '+' || (SELECT count(*) FROM observations) || ' minutes'),
		datetime('2026-01-01', '+' || (SELECT count(*) FROM observations) || ' minutes'))`, title, content, topicKey)
	if err != nil {
		t.Fatalf("insert observation: %v", err)
	}
	id, _ := res.LastInsertId()
	return id
}

// addFiller inserts n unrelated rows in both tables. BM25's IDF degenerates
// (SQLite clamps it to 1e-6) when most rows match a term, so ranking
// fixtures need a realistic share of non-matching rows.
func addFiller(t *testing.T, db *sql.DB, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		addObs(t, db, fmt.Sprintf("Filler memory %d", i), "Routine notes about meetings, lunch and calendars.", "")
		if _, err := db.Exec(`INSERT INTO user_prompts (content, project, created_at) VALUES (?, 'misc', '2025-12-31')`,
			fmt.Sprintf("filler prompt %d about meetings and calendars", i)); err != nil {
			t.Fatalf("insert filler prompt: %v", err)
		}
	}
}

func searchIDs(items []services.ObservationWithSnippet) []int64 {
	ids := make([]int64, len(items))
	for i, it := range items {
		ids[i] = it.ID
	}
	return ids
}

// forEachTokenizer runs fn against a fresh DB per tokenizer, pre-seeded with
// filler rows so BM25 has realistic document frequencies.
func forEachTokenizer(t *testing.T, fn func(t *testing.T, db *sql.DB)) {
	for name, tok := range map[string]string{"default": tokenizeDefault, "trigram": tokenizeTrigram} {
		t.Run(name, func(t *testing.T) {
			db := newSearchDB(t, tok)
			addFiller(t, db, 8)
			fn(t, db)
		})
	}
}

// A title match must outrank a content-only match, even when the content-only
// row is short (which unweighted BM25 rewards) and newer (id DESC tie-break).
func TestObservationsSearch_TitleOutranksContentOnly(t *testing.T) {
	forEachTokenizer(t, func(t *testing.T, db *sql.DB) {
		titleID := addObs(t, db, "Kubernetes rollout plan for the production cluster and staging",
			"We agreed on a phased approach with canaries, dashboards, alerts and a rollback runbook for every region.", "")
		contentID := addObs(t, db, "Deploy notes", "kubernetes", "")

		res, err := services.ObservationsSearch(db, "kubernetes", 10)
		if err != nil {
			t.Fatalf("search: %v", err)
		}
		got := searchIDs(res)
		if len(got) != 2 || got[0] != titleID || got[1] != contentID {
			t.Errorf("order: got %v, want [%d %d] (title match first)", got, titleID, contentID)
		}
	})
}

// A topic_key match outranks a content-only match.
func TestObservationsSearch_TopicKeyOutranksContentOnly(t *testing.T) {
	forEachTokenizer(t, func(t *testing.T, db *sql.DB) {
		topicID := addObs(t, db, "Decision record", "Long body text describing many unrelated considerations in detail.", "architecture/sqlite")
		contentID := addObs(t, db, "Notes", "We moved the cache from redis to sqlite last week.", "")

		res, err := services.ObservationsSearch(db, "sqlite", 10)
		if err != nil {
			t.Fatalf("search: %v", err)
		}
		got := searchIDs(res)
		if len(got) != 2 || got[0] != topicID || got[1] != contentID {
			t.Errorf("order: got %v, want [%d %d] (topic_key match first)", got, topicID, contentID)
		}
	})
}

func TestPromptsSearch_ContentOutranksProject(t *testing.T) {
	forEachTokenizer(t, func(t *testing.T, db *sql.DB) {
		if _, err := db.Exec(`INSERT INTO user_prompts (id, content, project, created_at) VALUES
			(101, 'please deploy the service to production after the review is approved by the team', 'web', '2026-01-01'),
			(102, 'fix lint', 'deploy', '2026-01-02')`); err != nil {
			t.Fatalf("seed prompts: %v", err)
		}
		res, err := services.PromptsSearch(db, "deploy", 10)
		if err != nil {
			t.Fatalf("search: %v", err)
		}
		if len(res) != 2 || res[0].ID != 101 {
			ids := []int64{}
			for _, r := range res {
				ids = append(ids, r.ID)
			}
			t.Errorf("order: got %v, want content match 101 first", ids)
		}
	})
}
