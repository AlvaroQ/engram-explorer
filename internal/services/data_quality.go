package services

import (
	"fmt"

	"github.com/AlvaroQ/engram-explorer/internal/sqlite"
)

// Data quality (Maintenance → "Data quality").
//
// Two exact, read-only checks over live (not soft-deleted) memories:
//   - empty titles: the title is NULL, empty or whitespace only;
//   - exact duplicates: two or more memories share a non-empty
//     normalized_hash (Engram's content hash). Memories without a hash are
//     never grouped. Semantic near-duplicates are out of scope.
//
// The checks never delete or change anything: each finding links to the
// memory so the user decides what to do with it.

// DataQualityDefaultLimit bounds every list of the report.
const DataQualityDefaultLimit = 50

const (
	dataQualityEmptyTitleSQL = `deleted_at IS NULL AND TRIM(COALESCE(title, '')) = ''`
	dataQualityHashSQL       = `deleted_at IS NULL AND normalized_hash IS NOT NULL AND normalized_hash != ''`
)

// DataQualityMemory is one memory listed by a check.
type DataQualityMemory struct {
	ID        int64  `json:"id"`
	Title     string `json:"title"`
	Project   string `json:"project"`
	Type      string `json:"type"`
	CreatedAt string `json:"createdAt"`
}

// DuplicateGroup is a set of live memories with the same normalized_hash.
type DuplicateGroup struct {
	Hash string `json:"hash"`
	// Title is the title of the oldest copy.
	Title string `json:"title"`
	// Count is the number of live copies (Memories may list fewer).
	Count    int                 `json:"count"`
	Memories []DataQualityMemory `json:"memories"`
}

// DataQualityReport is the result of both checks.
type DataQualityReport struct {
	// Limit caps EmptyTitles, Duplicates and each group's Memories.
	Limit           int                 `json:"limit"`
	EmptyTitleTotal int                 `json:"emptyTitleTotal"`
	EmptyTitles     []DataQualityMemory `json:"emptyTitles"`
	// DuplicateGroupTotal counts every group; DuplicateMemoryTotal counts the
	// memories in them.
	DuplicateGroupTotal  int              `json:"duplicateGroupTotal"`
	DuplicateMemoryTotal int              `json:"duplicateMemoryTotal"`
	Duplicates           []DuplicateGroup `json:"duplicates"`
}

func scanDataQualityMemory(sc scanner) (DataQualityMemory, error) {
	var m DataQualityMemory
	err := sc.Scan(&m.ID, &m.Title, &m.Project, &m.Type, &m.CreatedAt)
	return m, err
}

const dataQualityColumns = `id, COALESCE(title, ''), COALESCE(project, ''), COALESCE(type, ''), COALESCE(created_at, '')`

// CheckDataQuality runs both checks. Lists are bounded by limit (a
// non-positive limit uses DataQualityDefaultLimit); totals count everything.
// Empty titles are listed oldest first; duplicate groups largest first, each
// group's memories oldest first.
func CheckDataQuality(db sqlite.Querier, limit int) (DataQualityReport, error) {
	if limit <= 0 {
		limit = DataQualityDefaultLimit
	}
	rep := DataQualityReport{Limit: limit, EmptyTitles: []DataQualityMemory{}, Duplicates: []DuplicateGroup{}}

	if err := db.QueryRow(`SELECT COUNT(*) FROM observations WHERE ` + dataQualityEmptyTitleSQL).Scan(&rep.EmptyTitleTotal); err != nil {
		return DataQualityReport{}, fmt.Errorf("count empty titles: %w", err)
	}
	empty, err := queryRows(db,
		`SELECT `+dataQualityColumns+` FROM observations WHERE `+dataQualityEmptyTitleSQL+` ORDER BY id LIMIT ?`,
		[]any{limit}, scanDataQualityMemory)
	if err != nil {
		return DataQualityReport{}, fmt.Errorf("list empty titles: %w", err)
	}
	rep.EmptyTitles = empty

	if err := db.QueryRow(`
		SELECT COUNT(*), COALESCE(SUM(n), 0) FROM (
		  SELECT COUNT(*) AS n FROM observations WHERE `+dataQualityHashSQL+`
		  GROUP BY normalized_hash HAVING COUNT(*) > 1
		)`).Scan(&rep.DuplicateGroupTotal, &rep.DuplicateMemoryTotal); err != nil {
		return DataQualityReport{}, fmt.Errorf("count duplicates: %w", err)
	}
	if rep.DuplicateGroupTotal == 0 {
		return rep, nil
	}

	// The listed groups and, per group, its first limit memories.
	type dupRow struct {
		hash  string
		count int
		mem   DataQualityMemory
	}
	rows, err := queryRows(db, `
		WITH groups AS (
		  SELECT normalized_hash AS hash, COUNT(*) AS n, MIN(id) AS first_id
		  FROM observations WHERE `+dataQualityHashSQL+`
		  GROUP BY normalized_hash HAVING COUNT(*) > 1
		  ORDER BY n DESC, first_id ASC
		  LIMIT ?
		), members AS (
		  SELECT o.*, g.n AS group_n, g.first_id AS group_first,
		         ROW_NUMBER() OVER (PARTITION BY o.normalized_hash ORDER BY o.id) AS rn
		  FROM observations o JOIN groups g ON g.hash = o.normalized_hash
		  WHERE o.deleted_at IS NULL
		)
		SELECT normalized_hash, group_n, `+dataQualityColumns+`
		FROM members WHERE rn <= ?
		ORDER BY group_n DESC, group_first ASC, id ASC`,
		[]any{limit, limit},
		func(sc scanner) (dupRow, error) {
			var r dupRow
			err := sc.Scan(&r.hash, &r.count, &r.mem.ID, &r.mem.Title, &r.mem.Project, &r.mem.Type, &r.mem.CreatedAt)
			return r, err
		})
	if err != nil {
		return DataQualityReport{}, fmt.Errorf("list duplicates: %w", err)
	}
	for _, r := range rows {
		last := len(rep.Duplicates) - 1
		if last < 0 || rep.Duplicates[last].Hash != r.hash {
			rep.Duplicates = append(rep.Duplicates, DuplicateGroup{Hash: r.hash, Title: r.mem.Title, Count: r.count, Memories: []DataQualityMemory{}})
			last++
		}
		rep.Duplicates[last].Memories = append(rep.Duplicates[last].Memories, r.mem)
	}
	return rep, nil
}
