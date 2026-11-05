package services

import "github.com/AlvaroQ/engram-explorer/internal/fts"

// BM25 column weights. SQLite's bm25() returns lower-is-better scores, so the
// searches ORDER BY the expression ascending, then id DESC as a stable
// tie-break. The weights follow the FTS column order of each table:
//
//   - observations_fts(title, content, tool_name, type, project, topic_key):
//     title 5 — the author-written one-line summary, the strongest signal of
//     what a memory is about; topic_key 3 — a curated slug ("arch/auth") that
//     names the subject; content 1 — the long body, already length-normalized
//     by BM25; tool_name, type and project 0.2 — low-cardinality labels that
//     match many rows, so they may break ties but must not dominate.
//   - prompts_fts(content, project): content 1, project 0.2 for the same
//     label-versus-text reason.
//
// Labels are one or two tokens long, so BM25's length normalization already
// gives a label hit the maximum term-frequency score; at 0.5 a project-name
// hit still outranked an ordinary sentence that contains the word
// (TestPromptsSearch_ContentOutranksProject), at 0.2 it does not.
const (
	observationsBM25 = `bm25(observations_fts, 5.0, 1.0, 0.2, 0.2, 0.2, 3.0)`
	promptsBM25      = `bm25(prompts_fts, 1.0, 0.2)`
)

// ftsQuery is the MATCH plan for one raw user query.
type ftsQuery struct {
	// all requires every term (the default, precise search). "" means the
	// query has no usable term.
	all string
	// any requires at least one term. It is set only for 2+ terms, because a
	// single term's OR query equals its AND query; "" means no fallback.
	any string
}

// newFTSQuery sanitizes rawQ once and prepares both MATCH expressions.
//
// Fallback rule (shared by every search entry point): run all first; only
// when it returns no rows on the first page and any is set, run any once. The
// result then reports MatchedAny so callers can say the results match any word.
func newFTSQuery(rawQ string) ftsQuery {
	terms := fts.Terms(rawQ)
	q := ftsQuery{all: fts.MatchAll(terms)}
	if len(terms) >= 2 {
		q.any = fts.MatchAny(terms)
	}
	return q
}
