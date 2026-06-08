// Package fts provides FTS5 query sanitization for SQLite full-text search.
//
// The sanitizer caps the input length, splits on whitespace, drops FTS5 boolean
// operators (AND, OR, NOT, NEAR), strips characters that carry FTS5 query
// semantics (quotes, parentheses, the '*' prefix marker, the ':' column filter,
// and the '^' initial-token anchor), wraps each surviving token in double
// quotes, and AND-joins them. This makes "auth bug" behave like a Google search
// while preventing FTS5 syntax errors and unbounded-input DoS.
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
// Returns an empty string if no valid tokens remain.
func SanitizeQuery(input string) string {
	// Cap length first (rune-safe) to bound work on pathological input.
	if r := []rune(input); len(r) > maxQueryRunes {
		input = string(r[:maxQueryRunes])
	}

	// Strip FTS5-significant chars, then trim.
	cleaned := strings.TrimSpace(ftsDropCharsRE.ReplaceAllString(input, " "))
	if cleaned == "" {
		return ""
	}

	fields := strings.Fields(cleaned)
	tokens := make([]string, 0, len(fields))
	for _, t := range fields {
		if t == "" || ftsOperatorRE.MatchString(t) {
			continue
		}
		// Inner quotes/operators already stripped; wrap token as a literal term.
		tokens = append(tokens, `"`+t+`"`)
		if len(tokens) >= maxTokens {
			break
		}
	}
	if len(tokens) == 0 {
		return ""
	}
	return strings.Join(tokens, " AND ")
}
