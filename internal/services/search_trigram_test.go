package services_test

import (
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/services"
)

// Under trigram, a term shorter than 3 runes matches no row, so "go auth"
// would AND to nothing. The short term is dropped from the MATCH instead; the
// default tokenizer keeps it.
func TestObservationsSearch_ShortTermsPerTokenizer(t *testing.T) {
	for name, tc := range map[string]struct {
		tokenize string
		want     int
	}{
		"default": {tokenizeDefault, 1}, // "go" AND "auth": only the row with both
		"trigram": {tokenizeTrigram, 2}, // "go" dropped: both auth rows
	} {
		t.Run(name, func(t *testing.T) {
			db := newSearchDB(t, tc.tokenize)
			addFiller(t, db, 4)
			addObs(t, db, "Auth bug in the go service", "token expired", "")
			addObs(t, db, "Auth flow review", "works", "")

			res, err := services.ObservationsSearch(db, "go auth", 10)
			if err != nil {
				t.Fatalf("search: %v", err)
			}
			if len(res.Items) != tc.want || res.MatchedAny {
				t.Errorf("got %d items MatchedAny=%v, want %d exact items", len(res.Items), res.MatchedAny, tc.want)
			}
		})
	}
}

// When every term is short under trigram, the search falls back to a
// case-insensitive substring match on the title, newest first.
func TestObservationsSearch_TrigramAllShortTermsMatchTitle(t *testing.T) {
	db := newSearchDB(t, tokenizeTrigram)
	ui := addObs(t, db, "UI polish", "spacing", "")
	addObs(t, db, "Release notes", "ui tweaks", "") // content only: not matched
	addObs(t, db, "Go modules", "deps", "")
	deleted := addObs(t, db, "Old UI", "gone", "")
	if _, err := db.Exec(`UPDATE observations SET deleted_at = '2026-02-01' WHERE id = ?`, deleted); err != nil {
		t.Fatalf("soft delete: %v", err)
	}

	res, err := services.ObservationsSearch(db, "ui", 10)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if got := searchIDs(res.Items); len(got) != 1 || got[0] != ui || res.MatchedAny {
		t.Fatalf("got %v MatchedAny=%v, want [%d]", got, res.MatchedAny, ui)
	}
	if res.Items[0].Snippet != "UI polish" {
		t.Errorf("snippet: got %q, want the plain title", res.Items[0].Snippet)
	}
}

func TestObservationsSearch_DefaultTokenizerKeepsShortTerms(t *testing.T) {
	db := newSearchDB(t, tokenizeDefault)
	addObs(t, db, "UI polish", "spacing", "")
	addObs(t, db, "Release notes", "ui tweaks", "")
	res, err := services.ObservationsSearch(db, "ui", 10)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(res.Items) != 2 {
		t.Errorf("got %v, want both FTS matches (title and content)", searchIDs(res.Items))
	}
}

// LIKE wildcards in a short term are literals.
func TestObservationsSearch_TrigramShortTermEscapesLike(t *testing.T) {
	db := newSearchDB(t, tokenizeTrigram)
	pct := addObs(t, db, "Rollout 50% done", "x", "")
	addObs(t, db, "Rollout 50 done", "x", "")
	addObs(t, db, "a_b naming", "x", "")
	res, err := services.ObservationsSearch(db, "0%", 10)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if got := searchIDs(res.Items); len(got) != 1 || got[0] != pct {
		t.Errorf(`"0%%": got %v, want [%d]`, got, pct)
	}
	res, err = services.ObservationsSearch(db, "_", 10)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(res.Items) != 1 {
		t.Errorf(`"_": got %v, want only the a_b row`, searchIDs(res.Items))
	}
}

func TestObservationsListPage_TrigramAllShortTermsMatchTitle(t *testing.T) {
	db := newSearchDB(t, tokenizeTrigram)
	ui := addObs(t, db, "UI polish", "spacing", "")
	addObs(t, db, "Release notes", "ui tweaks", "")
	page, err := services.ObservationsListPage(db, services.ObservationListParams{Q: "ui", Limit: 10})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if got := rowIDs(page.Items); len(got) != 1 || got[0] != ui || page.MatchedAny {
		t.Errorf("got %v MatchedAny=%v, want [%d]", got, page.MatchedAny, ui)
	}
}

// Mixed short and long terms under trigram keep the AND→OR fallback for the
// long terms: "go auth timeout" drops "go", finds no row with both "auth" and
// "timeout", and retries with either.
func TestObservationsSearch_TrigramMixedTermsKeepFallback(t *testing.T) {
	db := newSearchDB(t, tokenizeTrigram)
	addFiller(t, db, 4)
	auth := addObs(t, db, "Auth flow review", "works", "")
	res, err := services.ObservationsSearch(db, "go auth timeout", 10)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if got := searchIDs(res.Items); len(got) != 1 || got[0] != auth || !res.MatchedAny {
		t.Errorf("got %v MatchedAny=%v, want [%d] from the fallback", got, res.MatchedAny, auth)
	}
}

// Several short terms under trigram require every term in the title first,
// then fall back to any term and say so.
func TestObservationsSearch_TrigramAllShortTermsFallback(t *testing.T) {
	db := newSearchDB(t, tokenizeTrigram)
	both := addObs(t, db, "UI for Go tools", "x", "")
	ui := addObs(t, db, "UI polish", "x", "")
	addObs(t, db, "Release notes", "x", "")

	res, err := services.ObservationsSearch(db, "go ui", 10)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if got := searchIDs(res.Items); len(got) != 1 || got[0] != both || res.MatchedAny {
		t.Errorf("AND: got %v MatchedAny=%v, want [%d]", got, res.MatchedAny, both)
	}

	res, err = services.ObservationsSearch(db, "ui zz", 10)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if got := searchIDs(res.Items); len(got) != 2 || got[0] != ui || got[1] != both || !res.MatchedAny {
		t.Errorf("fallback: got %v MatchedAny=%v, want [%d %d] newest first", got, res.MatchedAny, ui, both)
	}

	page, err := services.ObservationsListPage(db, services.ObservationListParams{Q: "ui zz", Limit: 10})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if got := rowIDs(page.Items); len(got) != 2 || !page.MatchedAny {
		t.Errorf("list fallback: got %v MatchedAny=%v, want 2 rows from the fallback", got, page.MatchedAny)
	}
}

func TestPromptsSearch_TrigramAllShortTermsMatchContent(t *testing.T) {
	db := newSearchDB(t, tokenizeTrigram)
	if _, err := db.Exec(`INSERT INTO user_prompts (id, content, project, created_at) VALUES
		(1, 'fix the UI spacing', 'web', '2026-01-01'),
		(2, 'bump deps', 'web', '2026-01-02')`); err != nil {
		t.Fatalf("seed prompts: %v", err)
	}
	res, err := services.PromptsSearch(db, "ui", 10)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(res) != 1 || res[0].ID != 1 {
		t.Errorf("got %d results, want prompt 1 only", len(res))
	}
}
