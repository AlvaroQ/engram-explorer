package services

import (
	"database/sql"
	"sort"
	"time"
)

// GraphNodeCap is the maximum number of nodes returned in a single graph response.
const GraphNodeCap = 5000

// GraphNode is one node in the 3D brain graph.
type GraphNode struct {
	ID             int64   `json:"id"`
	Project        *string `json:"project"`
	Type           *string `json:"type"`
	Scope          *string `json:"scope"`
	TopicKey       *string `json:"topicKey"`
	Label          *string `json:"label"`
	Weight         int64   `json:"weight"`
	DuplicateCount int64   `json:"duplicateCount"`
	CreatedAt      *string `json:"createdAt"`
}

// GraphEdge is one semantic edge in the graph.
type GraphEdge struct {
	Source     int64    `json:"source"`
	Target     int64    `json:"target"`
	Relation   string   `json:"relation"`
	Confidence *float64 `json:"confidence,omitempty"`
	Reason     *string  `json:"reason,omitempty"`
}

// GraphEdgeCounts holds per-relation edge counts.
type GraphEdgeCounts struct {
	Related    int `json:"related"`
	Scoped     int `json:"scoped"`
	Compatible int `json:"compatible"`
}

// GraphMeta holds the metadata block of the graph response.
type GraphMeta struct {
	GeneratedAt      string          `json:"generatedAt"`
	TotalObservations int64          `json:"totalObservations"`
	ShownNodes       int             `json:"shownNodes"`
	DuplicateGroups  int64           `json:"duplicateGroups"`
	EdgeCounts       GraphEdgeCounts `json:"edgeCounts"`
	Projects         int64           `json:"projects"`
	Truncated        bool            `json:"truncated"`
	TruncatedAt      *int            `json:"truncatedAt"`
}

// GraphResponse is the full graph response.
type GraphResponse struct {
	Nodes []GraphNode `json:"nodes"`
	Edges []GraphEdge `json:"edges"`
	Meta  GraphMeta   `json:"meta"`
}

// collapseRow is one row from the duplicate-collapse query.
type collapseRow struct {
	id             int64
	project        *string
	obsType        *string
	title          *string
	scope          *string
	topicKey       *string
	revisionCount  int64
	createdAt      *string
	duplicateCount int64
}

// relationRow is one row from the semantic-edge JOIN query.
type relationRow struct {
	sourceObsID int64
	targetObsID int64
	relation    string
	confidence  *float64
	reason      *string
}

// metaAggRow is the aggregate row from the meta subquery.
type metaAggRow struct {
	totalObs        int64
	duplicateGroups int64
	projects        int64
}

// memberRow maps a non-representative observation id to its representative.
type memberRow struct {
	id    int64
	repID int64
}

// GraphBuild builds the 3D graph response with an optional project filter.
// nodeCap overrides GraphNodeCap when > 0 (useful in tests).
func GraphBuild(db *sql.DB, project string, nodeCap int) (GraphResponse, error) {
	if nodeCap <= 0 {
		nodeCap = GraphNodeCap
	}

	// ── 1. Duplicate-collapse node query ────────────────────────────────────────
	var collapseRows []collapseRow
	if project != "" {
		rows, err := db.Query(`
			SELECT
			  MIN(id)          AS id,
			  MAX(project)     AS project,
			  MAX(type)        AS type,
			  MAX(title)       AS title,
			  MAX(scope)       AS scope,
			  MAX(topic_key)   AS topic_key,
			  SUM(revision_count) AS revision_count,
			  MIN(created_at)  AS created_at,
			  COUNT(*)         AS duplicate_count
			FROM observations
			WHERE deleted_at IS NULL
			  AND project = ?
			  AND normalized_hash IS NOT NULL
			  AND normalized_hash != ''
			GROUP BY normalized_hash
			UNION ALL
			SELECT
			  id, project, type, title, scope, topic_key,
			  revision_count, created_at,
			  1 AS duplicate_count
			FROM observations
			WHERE deleted_at IS NULL
			  AND project = ?
			  AND (normalized_hash IS NULL OR normalized_hash = '')`,
			project, project,
		)
		if err != nil {
			return GraphResponse{}, err
		}
		defer rows.Close()
		if err := scanCollapseRows(rows, &collapseRows); err != nil {
			return GraphResponse{}, err
		}
	} else {
		rows, err := db.Query(`
			SELECT
			  MIN(id)          AS id,
			  MAX(project)     AS project,
			  MAX(type)        AS type,
			  MAX(title)       AS title,
			  MAX(scope)       AS scope,
			  MAX(topic_key)   AS topic_key,
			  SUM(revision_count) AS revision_count,
			  MIN(created_at)  AS created_at,
			  COUNT(*)         AS duplicate_count
			FROM observations
			WHERE deleted_at IS NULL
			  AND normalized_hash IS NOT NULL
			  AND normalized_hash != ''
			GROUP BY normalized_hash
			UNION ALL
			SELECT
			  id, project, type, title, scope, topic_key,
			  revision_count, created_at,
			  1 AS duplicate_count
			FROM observations
			WHERE deleted_at IS NULL
			  AND (normalized_hash IS NULL OR normalized_hash = '')`,
		)
		if err != nil {
			return GraphResponse{}, err
		}
		defer rows.Close()
		if err := scanCollapseRows(rows, &collapseRows); err != nil {
			return GraphResponse{}, err
		}
	}

	// Build live id set (representatives + singletons).
	liveIDSet := make(map[int64]struct{}, len(collapseRows))
	for _, r := range collapseRows {
		liveIDSet[r.id] = struct{}{}
	}

	// ── 2. Build rawId → representative id map ───────────────────────────────
	rawIDToRepID := make(map[int64]int64, len(collapseRows))
	for _, r := range collapseRows {
		rawIDToRepID[r.id] = r.id
	}

	// Fetch non-representative members only when there are dup groups.
	hasDupGroups := false
	for _, r := range collapseRows {
		if r.duplicateCount > 1 {
			hasDupGroups = true
			break
		}
	}
	if hasDupGroups {
		if project != "" {
			members, err := db.Query(`
				SELECT o.id, g.rep_id
				  FROM observations o
				  JOIN (
				    SELECT normalized_hash, MIN(id) AS rep_id
				      FROM observations
				     WHERE deleted_at IS NULL
				       AND project = ?
				       AND normalized_hash IS NOT NULL
				       AND normalized_hash != ''
				     GROUP BY normalized_hash
				    HAVING COUNT(*) > 1
				  ) g ON o.normalized_hash = g.normalized_hash
				 WHERE o.deleted_at IS NULL
				   AND o.project = ?
				   AND o.id != g.rep_id`,
				project, project,
			)
			if err != nil {
				return GraphResponse{}, err
			}
			defer members.Close()
			if err := scanMemberRows(members, rawIDToRepID); err != nil {
				return GraphResponse{}, err
			}
		} else {
			members, err := db.Query(`
				SELECT o.id, g.rep_id
				  FROM observations o
				  JOIN (
				    SELECT normalized_hash, MIN(id) AS rep_id
				      FROM observations
				     WHERE deleted_at IS NULL
				       AND normalized_hash IS NOT NULL
				       AND normalized_hash != ''
				     GROUP BY normalized_hash
				    HAVING COUNT(*) > 1
				  ) g ON o.normalized_hash = g.normalized_hash
				 WHERE o.deleted_at IS NULL
				   AND o.id != g.rep_id`,
			)
			if err != nil {
				return GraphResponse{}, err
			}
			defer members.Close()
			if err := scanMemberRows(members, rawIDToRepID); err != nil {
				return GraphResponse{}, err
			}
		}
	}

	// ── 3. Semantic-edge JOIN query ───────────────────────────────────────────
	var filteredEdges []GraphEdge
	relRows, err := db.Query(`
		SELECT
		  s.id           AS source_obs_id,
		  t.id           AS target_obs_id,
		  mr.relation    AS relation,
		  mr.confidence  AS confidence,
		  mr.reason      AS reason
		FROM memory_relations mr
		INNER JOIN observations s ON s.sync_id = mr.source_id AND s.deleted_at IS NULL
		INNER JOIN observations t ON t.sync_id = mr.target_id AND t.deleted_at IS NULL
		WHERE mr.judgment_status = 'judged'
		  AND mr.relation IN ('related', 'scoped', 'compatible')`,
	)
	if err != nil {
		// Table may not exist on older schemas — treat as no edges.
		if !isTableMissing(err) {
			return GraphResponse{}, err
		}
	} else {
		defer relRows.Close()
		for relRows.Next() {
			var rr relationRow
			if err := relRows.Scan(&rr.sourceObsID, &rr.targetObsID, &rr.relation, &rr.confidence, &rr.reason); err != nil {
				return GraphResponse{}, err
			}
			srcRep, ok := rawIDToRepID[rr.sourceObsID]
			if !ok {
				srcRep = rr.sourceObsID
			}
			tgtRep, ok := rawIDToRepID[rr.targetObsID]
			if !ok {
				tgtRep = rr.targetObsID
			}
			if _, ok := liveIDSet[srcRep]; !ok {
				continue
			}
			if _, ok := liveIDSet[tgtRep]; !ok {
				continue
			}
			if srcRep == tgtRep {
				continue
			}
			edge := GraphEdge{
				Source:     srcRep,
				Target:     tgtRep,
				Relation:   rr.relation,
				Confidence: rr.confidence,
				Reason:     rr.reason,
			}
			filteredEdges = append(filteredEdges, edge)
		}
		if err := relRows.Err(); err != nil {
			return GraphResponse{}, err
		}
	}

	// ── 4. Build nodes, sort by weight DESC, apply cap ───────────────────────
	nodes := make([]GraphNode, 0, len(collapseRows))
	for _, r := range collapseRows {
		nodes = append(nodes, GraphNode{
			ID:             r.id,
			Project:        r.project,
			Type:           r.obsType,
			Scope:          r.scope,
			TopicKey:       r.topicKey,
			Label:          r.title,
			Weight:         r.revisionCount + r.duplicateCount,
			DuplicateCount: r.duplicateCount,
			CreatedAt:      r.createdAt,
		})
	}

	sort.SliceStable(nodes, func(i, j int) bool {
		return nodes[i].Weight > nodes[j].Weight
	})

	truncated := len(nodes) > nodeCap
	var truncatedAt *int
	if truncated {
		cap := nodeCap
		truncatedAt = &cap
		keptIDs := make(map[int64]struct{}, nodeCap)
		for _, n := range nodes[:nodeCap] {
			keptIDs[n.ID] = struct{}{}
		}
		nodes = nodes[:nodeCap]
		// Purge edges whose endpoints were truncated away.
		kept := filteredEdges[:0]
		for _, e := range filteredEdges {
			if _, ok := keptIDs[e.Source]; !ok {
				continue
			}
			if _, ok := keptIDs[e.Target]; !ok {
				continue
			}
			kept = append(kept, e)
		}
		filteredEdges = kept
	}

	// ── 5. Meta aggregates ────────────────────────────────────────────────────
	var metaRow metaAggRow
	if project != "" {
		err = db.QueryRow(`
			SELECT
			  (SELECT COUNT(*) FROM observations WHERE deleted_at IS NULL AND project = ?) AS total_obs,
			  (SELECT COUNT(*) FROM (
			    SELECT 1 FROM observations
			     WHERE deleted_at IS NULL AND project = ?
			       AND normalized_hash IS NOT NULL AND normalized_hash != ''
			     GROUP BY normalized_hash HAVING COUNT(*) > 1
			  )) AS duplicate_groups,
			  (SELECT COUNT(DISTINCT project) FROM observations WHERE deleted_at IS NULL AND project = ?) AS projects`,
			project, project, project,
		).Scan(&metaRow.totalObs, &metaRow.duplicateGroups, &metaRow.projects)
	} else {
		err = db.QueryRow(`
			SELECT
			  (SELECT COUNT(*) FROM observations WHERE deleted_at IS NULL) AS total_obs,
			  (SELECT COUNT(*) FROM (
			    SELECT 1 FROM observations
			     WHERE deleted_at IS NULL
			       AND normalized_hash IS NOT NULL AND normalized_hash != ''
			     GROUP BY normalized_hash HAVING COUNT(*) > 1
			  )) AS duplicate_groups,
			  (SELECT COUNT(DISTINCT project) FROM observations WHERE deleted_at IS NULL) AS projects`,
		).Scan(&metaRow.totalObs, &metaRow.duplicateGroups, &metaRow.projects)
	}
	if err != nil {
		return GraphResponse{}, err
	}

	// Count edges by relation type.
	var ecRelated, ecScoped, ecCompatible int
	for _, e := range filteredEdges {
		switch e.Relation {
		case "related":
			ecRelated++
		case "scoped":
			ecScoped++
		case "compatible":
			ecCompatible++
		}
	}

	if nodes == nil {
		nodes = []GraphNode{}
	}
	if filteredEdges == nil {
		filteredEdges = []GraphEdge{}
	}

	return GraphResponse{
		Nodes: nodes,
		Edges: filteredEdges,
		Meta: GraphMeta{
			GeneratedAt:       time.Now().UTC().Format(time.RFC3339),
			TotalObservations: metaRow.totalObs,
			ShownNodes:        len(nodes),
			DuplicateGroups:   metaRow.duplicateGroups,
			EdgeCounts: GraphEdgeCounts{
				Related:    ecRelated,
				Scoped:     ecScoped,
				Compatible: ecCompatible,
			},
			Projects:    metaRow.projects,
			Truncated:   truncated,
			TruncatedAt: truncatedAt,
		},
	}, nil
}

// scanCollapseRows reads the collapse query result into a slice.
func scanCollapseRows(rows *sql.Rows, out *[]collapseRow) error {
	for rows.Next() {
		var r collapseRow
		if err := rows.Scan(
			&r.id, &r.project, &r.obsType, &r.title, &r.scope, &r.topicKey,
			&r.revisionCount, &r.createdAt, &r.duplicateCount,
		); err != nil {
			return err
		}
		*out = append(*out, r)
	}
	return rows.Err()
}

// scanMemberRows reads the member-map query result into rawIDToRepID.
func scanMemberRows(rows *sql.Rows, rawIDToRepID map[int64]int64) error {
	for rows.Next() {
		var mr memberRow
		if err := rows.Scan(&mr.id, &mr.repID); err != nil {
			return err
		}
		rawIDToRepID[mr.id] = mr.repID
	}
	return rows.Err()
}

// isTableMissing returns true for "no such table" SQLite errors.
func isTableMissing(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return len(msg) >= 13 && containsSubstr(msg, "no such table")
}

// containsSubstr is a simple substring check that avoids importing strings
// at the package level (strings is already imported in sync.go).
func containsSubstr(s, sub string) bool {
	if len(sub) == 0 {
		return true
	}
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
