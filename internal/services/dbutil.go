package services

import "database/sql"

// scanner is the minimal interface shared by *sql.Row and *sql.Rows, so a
// single scan helper can be reused for both single-row and multi-row queries.
type scanner interface{ Scan(dest ...any) error }

// rowsQuerier is implemented by both *sql.DB and *sql.Tx, so queryRows can run
// standalone or inside a transaction (mirrors rowQuerier in write_shared.go).
type rowsQuerier interface {
	Query(query string, args ...any) (*sql.Rows, error)
}

// queryRows runs query with args, scans every row via scan, and ALWAYS returns
// a non-nil slice (JSON contract: empty lists serialize as [], never null).
//
// The scan callback takes a scanner (not *sql.Rows) so row-scan helpers like
// scanObservation can be passed directly and reused with single-row QueryRow.
//
// It centralizes the repeated "db.Query → defer rows.Close → for rows.Next →
// rows.Err → nil guard" pattern. Callers that need to wrap the error with extra
// context keep that wrapping at the call site.
func queryRows[T any](db rowsQuerier, query string, args []any, scan func(scanner) (T, error)) ([]T, error) {
	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []T{}
	for rows.Next() {
		v, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
