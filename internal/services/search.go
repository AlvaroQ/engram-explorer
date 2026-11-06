package services

import (
	"strings"

	"github.com/AlvaroQ/engram-explorer/internal/fts"
	"github.com/AlvaroQ/engram-explorer/internal/sqlite"
)

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

// ftsQuery is the search plan for one raw user query.
type ftsQuery struct {
	// all requires every term (the default, precise search). "" means no
	// MATCH: either the query has no usable term or like is set.
	all string
	// any requires at least one term. It is set only for 2+ terms, because a
	// single term's OR query equals its AND query; "" means no fallback.
	any string
	// like holds the terms for a substring LIKE search, used instead of MATCH
	// when the index is trigram and every term is shorter than
	// fts.TrigramMinRunes (see newFTSQuery).
	like []string
}

// textFilter is one executable step of an ftsQuery: an FTS5 MATCH expression
// or a LIKE condition with its arguments. The zero value filters nothing.
type textFilter struct {
	match    string
	like     string
	likeArgs []any
}

// none reports that the filter does not restrict rows.
func (f textFilter) none() bool { return f.match == "" && f.like == "" }

// newFTSQuery sanitizes rawQ once and prepares the search for the FTS5 table.
//
// Fallback rule (shared by every search entry point): run the strict filter
// first; only when it returns no rows on the first page and a fallback
// exists, run the fallback once. The result then reports MatchedAny so
// callers can say the results match any word.
//
// Trigram rule: upstream Engram indexes with the trigram tokenizer, where a
// term shorter than fts.TrigramMinRunes runes matches nothing and would empty
// an AND query. On a trigram table such terms are dropped from the MATCH;
// when every term is short, the search becomes a substring LIKE on one short
// column (the observation title), never on the long body, where a one- or
// two-letter substring would match almost every row. The LIKE search is
// bounded like MATCH: the terms capped by fts.Terms, the caller's row limit,
// and SQLite's LIKE is case-insensitive for ASCII only. Other tokenizers keep
// short terms in the MATCH.
//
// The tokenizer is read from sqlite_master on every call instead of being
// cached: the lookup is one indexed row of a tiny table, and the DB handle
// can be hot-swapped to another file at runtime.
func newFTSQuery(db sqlite.Querier, table, rawQ string) ftsQuery {
	terms := fts.Terms(rawQ)
	if len(terms) > 0 && ftsUsesTrigram(db, table) {
		long, short := fts.SplitShort(terms, fts.TrigramMinRunes)
		if len(long) == 0 {
			return ftsQuery{like: short}
		}
		terms = long
	}
	q := ftsQuery{all: fts.MatchAll(terms)}
	if len(terms) >= 2 {
		q.any = fts.MatchAny(terms)
	}
	return q
}

// filters returns the strict filter and its fallback (zero when there is
// none). likeColumn is the column searched in LIKE mode. The fallback of a
// LIKE search requires any term, mirroring the MATCH AND→OR fallback.
func (q ftsQuery) filters(likeColumn string) (strict, fallback textFilter) {
	if len(q.like) > 0 {
		strict = likeFilter(likeColumn, q.like, " AND ")
		if len(q.like) >= 2 {
			fallback = likeFilter(likeColumn, q.like, " OR ")
		}
		return strict, fallback
	}
	return textFilter{match: q.all}, textFilter{match: q.any}
}

// ftsUsesTrigram reports whether the FTS5 table was created with the trigram
// tokenizer, read from its CREATE statement in sqlite_master. A missing table
// or a lookup error reports false (plain MATCH behaviour).
func ftsUsesTrigram(db sqlite.Querier, table string) bool {
	var ddl string
	err := db.QueryRow(`SELECT sql FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&ddl)
	return err == nil && strings.Contains(strings.ToLower(ddl), "trigram")
}

// likeFilter builds one parenthesized "column LIKE ? ESCAPE '\'" condition
// per term joined by sep, with %term% arguments whose LIKE wildcards are
// escaped so they match literally.
func likeFilter(column string, terms []string, sep string) textFilter {
	esc := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	conds := make([]string, len(terms))
	args := make([]any, len(terms))
	for i, t := range terms {
		conds[i] = column + ` LIKE ? ESCAPE '\'`
		args[i] = "%" + esc.Replace(t) + "%"
	}
	return textFilter{like: "(" + strings.Join(conds, sep) + ")", likeArgs: args}
}
