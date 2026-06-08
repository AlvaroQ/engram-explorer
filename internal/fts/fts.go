// Package fts provides FTS5 query sanitization for SQLite full-text search.
//
// The sanitizer splits on whitespace, drops FTS5 boolean operators (AND, OR,
// NOT, NEAR), strips parentheses and quotes, wraps each surviving token in
// double quotes, and AND-joins them. This makes "auth bug" behave like a
// Google search while preventing accidental FTS5 syntax errors.
package fts

import (
	"regexp"
	"strings"
)

var (
	// ftsOperatorRE matches FTS5 boolean keywords (case-insensitive).
	ftsOperatorRE = regexp.MustCompile(`(?i)^(AND|OR|NOT|NEAR)$`)

	// ftsDropCharsRE matches characters that must be stripped before tokenizing.
	ftsDropCharsRE = regexp.MustCompile(`["'()]`)
)

// SanitizeQuery sanitizes a raw user query for use as a SQLite FTS5 MATCH value.
// Returns an empty string if no valid tokens remain.
func SanitizeQuery(input string) string {
	// Strip parens and quotes, then trim.
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
		// Inner double-quotes already stripped; wrap token.
		tokens = append(tokens, `"`+t+`"`)
	}
	if len(tokens) == 0 {
		return ""
	}
	return strings.Join(tokens, " AND ")
}
