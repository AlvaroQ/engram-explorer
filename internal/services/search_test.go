package services_test

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/cursor"
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

func rowIDs(items []services.ObservationRow) []int64 {
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
		got := searchIDs(res.Items)
		if len(got) != 2 || got[0] != titleID || got[1] != contentID {
			t.Errorf("order: got %v, want [%d %d] (title match first)", got, titleID, contentID)
		}
		if res.MatchedAny {
			t.Error("MatchedAny must be false for a single-term query")
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
		got := searchIDs(res.Items)
		if len(got) != 2 || got[0] != topicID || got[1] != contentID {
			t.Errorf("order: got %v, want [%d %d] (topic_key match first)", got, topicID, contentID)
		}
	})
}

func TestObservationsSearch_ORFallbackWhenANDFindsNothing(t *testing.T) {
	forEachTokenizer(t, func(t *testing.T, db *sql.DB) {
		a := addObs(t, db, "Auth bug in login", "token expired", "")
		b := addObs(t, db, "Request timeout", "gateway slow", "")
		addObs(t, db, "Unrelated", "nothing here", "")

		res, err := services.ObservationsSearch(db, "auth timeout", 10)
		if err != nil {
			t.Fatalf("search: %v", err)
		}
		if !res.MatchedAny {
			t.Error("MatchedAny: got false, want true after OR fallback")
		}
		got := searchIDs(res.Items)
		if len(got) != 2 || !(contains(got, a) && contains(got, b)) {
			t.Errorf("items: got %v, want ids %d and %d", got, a, b)
		}
	})
}

func TestObservationsSearch_NoFallbackWhenANDMatches(t *testing.T) {
	forEachTokenizer(t, func(t *testing.T, db *sql.DB) {
		a := addObs(t, db, "Auth bug in login", "token expired", "")
		addObs(t, db, "Auth flow", "works", "")

		res, err := services.ObservationsSearch(db, "auth bug", 10)
		if err != nil {
			t.Fatalf("search: %v", err)
		}
		if res.MatchedAny {
			t.Error("MatchedAny: got true, want false when the AND query matches")
		}
		if got := searchIDs(res.Items); len(got) != 1 || got[0] != a {
			t.Errorf("items: got %v, want [%d]", got, a)
		}
	})
}

func TestObservationsSearch_NoFallbackForSingleTerm(t *testing.T) {
	db := newSearchDB(t, tokenizeDefault)
	addObs(t, db, "Auth bug", "x", "")
	res, err := services.ObservationsSearch(db, "missing", 10)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if res.MatchedAny || len(res.Items) != 0 {
		t.Errorf("got %d items, MatchedAny=%v; want 0 items and no fallback", len(res.Items), res.MatchedAny)
	}
}

func TestObservationsSearch_EmptyQueryReturnsEmpty(t *testing.T) {
	db := newSearchDB(t, tokenizeDefault)
	addObs(t, db, "Auth bug", "x", "")
	res, err := services.ObservationsSearch(db, `"" AND ()`, 10)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if res.Items == nil || len(res.Items) != 0 || res.MatchedAny {
		t.Errorf("got %#v, want non-nil empty items without fallback", res)
	}
}

func TestObservationsListPage_ORFallbackAndPagination(t *testing.T) {
	forEachTokenizer(t, func(t *testing.T, db *sql.DB) {
		a := addObs(t, db, "Auth bug in login", "token expired", "")
		b := addObs(t, db, "Request timeout", "gateway slow", "")

		page, err := services.ObservationsListPage(db, services.ObservationListParams{Q: "auth timeout", Limit: 1})
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if !page.MatchedAny {
			t.Fatal("MatchedAny: got false, want true")
		}
		if got := rowIDs(page.Items); len(got) != 1 || got[0] != b || page.NextCursor == nil {
			t.Fatalf("page 1: got %v cursor=%v, want [%d] with a cursor", got, page.NextCursor, b)
		}

		// The caller carries MatchAny to the next page.
		page2, err := services.ObservationsListPage(db, services.ObservationListParams{
			Q: "auth timeout", Limit: 1, Cursor: *page.NextCursor, MatchAny: true,
		})
		if err != nil {
			t.Fatalf("list page 2: %v", err)
		}
		if got := rowIDs(page2.Items); len(got) != 1 || got[0] != a || !page2.MatchedAny {
			t.Errorf("page 2: got %v MatchedAny=%v, want [%d] true", got, page2.MatchedAny, a)
		}
	})
}

// A later page never falls back on its own: an exhausted AND result set must
// not be topped up with OR matches.
func TestObservationsListPage_NoFallbackOnCursorPage(t *testing.T) {
	db := newSearchDB(t, tokenizeDefault)
	addObs(t, db, "Auth only", "one word", "")
	addObs(t, db, "Timeout only", "one word", "")
	addObs(t, db, "Auth timeout", "both words", "")

	page, err := services.ObservationsListPage(db, services.ObservationListParams{Q: "auth timeout", Limit: 1})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if page.MatchedAny || len(page.Items) != 1 || page.NextCursor != nil {
		t.Fatalf("page 1: got %d items MatchedAny=%v cursor=%v; want 1 exact item, no cursor", len(page.Items), page.MatchedAny, page.NextCursor)
	}
	// A cursor past the only (newest) exact match: the older OR-only rows
	// must not be appended.
	last := page.Items[0]
	items, _, err := services.ObservationsList(db, services.ObservationListParams{
		Q: "auth timeout", Limit: 5, Cursor: cursor.Encode(*last.UpdatedAt, last.ID),
	})
	if err != nil {
		t.Fatalf("list cursor: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("cursor page: got %v, want no rows (no fallback on later pages)", rowIDs(items))
	}
}

func TestObservationsList_WrapperMatchesPage(t *testing.T) {
	db := newSearchDB(t, tokenizeDefault)
	b := addObs(t, db, "Request timeout", "gateway slow", "")
	items, next, err := services.ObservationsList(db, services.ObservationListParams{Q: "auth timeout", Limit: 10})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if got := rowIDs(items); len(got) != 1 || got[0] != b || next != nil {
		t.Errorf("got %v next=%v, want [%d] nil (fallback applied)", got, next, b)
	}
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

func contains(ids []int64, id int64) bool {
	for _, v := range ids {
		if v == id {
			return true
		}
	}
	return false
}
