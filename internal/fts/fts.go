// Package fts provides FTS5 query sanitization for SQLite full-text search.
//
// The sanitizer caps the input length, splits on whitespace, drops FTS5 boolean
// operators (AND, OR, NOT, NEAR), strips characters that carry FTS5 query
// semantics (quotes, parentheses, the '*' prefix marker, the ':' column filter,
// and the '^' initial-token anchor), wraps each surviving token in double
// quotes, and AND-joins them. This makes "auth bug" behave like a Google search
// while preventing FTS5 syntax errors and unbounded-input DoS. Terms, MatchAll
// and MatchAny expose the same steps so callers can retry with OR-joined terms.
package fts

import (
	"regexp"
	"strings"
)

const (
	// maxQueryRunes caps the raw query length before tokenizing, bounding both
	// CPU and the size of the generated MATCH expression (DoS guard).
	maxQueryRunes = 256
	// maxTokens caps the number of AND-joined terms in the final expression.
	maxTokens = 32
)

var (
	// ftsOperatorRE matches FTS5 boolean keywords (case-insensitive).
	ftsOperatorRE = regexp.MustCompile(`(?i)^(AND|OR|NOT|NEAR)$`)

	// ftsDropCharsRE matches characters that must be stripped before tokenizing:
	// quotes, parens, the prefix '*', the column-filter ':', and the '^' anchor.
	ftsDropCharsRE = regexp.MustCompile(`["'()*^:]`)
)

// SanitizeQuery sanitizes a raw user query for use as a SQLite FTS5 MATCH value.
// Returns an empty string if no valid tokens remain. It is MatchAll(Terms(input)).
func SanitizeQuery(input string) string {
	return MatchAll(Terms(input))
}

// Terms returns the sanitized literal terms of a raw user query, unquoted and
// in input order: the input is capped to maxQueryRunes, FTS5-significant
// characters are stripped, boolean operators are dropped, and at most
// maxTokens terms are kept. Returns nil when no term survives.
func Terms(input string) []string {
	// Cap length first (rune-safe) to bound work on pathological input.
	if r := []rune(input); len(r) > maxQueryRunes {
		input = string(r[:maxQueryRunes])
	}

	// Strip FTS5-significant chars, then split on whitespace.
	fields := strings.Fields(ftsDropCharsRE.ReplaceAllString(input, " "))
	var terms []string
	for _, t := range fields {
		if ftsOperatorRE.MatchString(t) {
			continue
		}
		terms = append(terms, t)
		if len(terms) >= maxTokens {
			break
		}
	}
	return terms
}

// MatchAll builds a MATCH expression requiring every term ("a" AND "b").
// Terms must come from Terms. Returns "" for no terms.
func MatchAll(terms []string) string { return join(terms, " AND ") }

// MatchAny builds a MATCH expression requiring at least one term ("a" OR "b"),
// used as the fallback when MatchAll finds nothing. Returns "" for no terms.
func MatchAny(terms []string) string { return join(terms, " OR ") }

// join wraps each term as a quoted literal and joins them with sep. Inner
// quotes and operators were already stripped by Terms.
func join(terms []string, sep string) string {
	quoted := make([]string, len(terms))
	for i, t := range terms {
		quoted[i] = `"` + t + `"`
	}
	return strings.Join(quoted, sep)
}
