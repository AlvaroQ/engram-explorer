package services

import (
	"context"
	"database/sql"
	"errors"

	"github.com/AlvaroQ/engram-explorer/internal/sqlite"
)

// ObservationVersion is one earlier state of an observation, as kept by
// upstream Engram in observation_versions when a topic_key upsert or an edit
// replaces the title or content. Nullable columns serialize as JSON null.
type ObservationVersion struct {
	Version   int64   `json:"version"`
	Title     *string `json:"title"`
	Content   *string `json:"content"`
	CreatedAt *string `json:"created_at"`
}

// ObservationVersionsResult is the read-only revision history of one
// observation. Available is false when the database has no history table
// (older Engram schemas); Versions is then empty, never null.
type ObservationVersionsResult struct {
	Available bool                 `json:"available"`
	Versions  []ObservationVersion `json:"versions"`
}

// ObservationVersions returns the stored versions of observation id, newest
// first. found is false when the observation does not exist (soft-deleted
// observations are found). A missing observation_versions table is not an
// error: it reports Available=false. The query is read-only.
func ObservationVersions(ctx context.Context, db sqlite.Querier, id int64) (res ObservationVersionsResult, found bool, err error) {
	res.Versions = []ObservationVersion{}
	var one int
	err = db.QueryRowContext(ctx, `SELECT 1 FROM observations WHERE id = ?`, id).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return res, false, nil
	} else if err != nil {
		return res, false, err
	}

	var n int
	if err := db.QueryRowContext(ctx,
		`SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = 'observation_versions'`).Scan(&n); err != nil {
		return res, true, err
	}
	if n == 0 {
		return res, true, nil
	}
	res.Available = true

	rows, err := db.QueryContext(ctx, `SELECT version, title, content, created_at
		FROM observation_versions
		WHERE observation_id = ?
		ORDER BY version DESC, id DESC`, id)
	if err != nil {
		return res, true, err
	}
	defer rows.Close()
	for rows.Next() {
		var v ObservationVersion
		if err := rows.Scan(&v.Version, &v.Title, &v.Content, &v.CreatedAt); err != nil {
			return res, true, err
		}
		res.Versions = append(res.Versions, v)
	}
	return res, true, rows.Err()
}
