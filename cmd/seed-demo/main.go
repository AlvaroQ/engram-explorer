// Command seed-demo creates a self-contained demo database so newcomers can
// try the engram-explorer dashboard without having Engram installed.
//
// Usage:
//
//	go run ./cmd/seed-demo [--out ./demo] [--force]
//
// The command writes ONLY to the directory specified by --out (default ./demo).
// It never touches ~/.engram.
package main

import (
	"database/sql"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// seedBase is the anchor for all demo timestamps, set once per run to "today
// at noon UTC". The structure (row counts, relationships) is fully deterministic
// — only the absolute calendar dates shift between runs so observations always
// fall within the last 90 days.
var seedBase time.Time

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "seed-demo: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	outDir := flag.String("out", "./demo", "output directory for the demo database")
	force := flag.Bool("force", false, "overwrite an existing database")
	flag.Parse()

	// Resolve the absolute path so the message is unambiguous.
	absOut, err := filepath.Abs(*outDir)
	if err != nil {
		return fmt.Errorf("resolve output path: %w", err)
	}

	dbPath := filepath.Join(absOut, "engram.db")

	// Safety check: refuse to write to ~/.engram.
	home, err := os.UserHomeDir()
	if err == nil {
		engramDir := filepath.Join(home, ".engram")
		if strings.HasPrefix(absOut, engramDir) {
			return fmt.Errorf("SAFETY: --out must not be inside ~/.engram (%s)", engramDir)
		}
	}

	// Check for existing database.
	if _, err := os.Stat(dbPath); err == nil && !*force {
		return fmt.Errorf("database already exists at %s — use --force to overwrite", dbPath)
	}

	// Create directory if needed.
	if err := os.MkdirAll(absOut, 0o755); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}

	// Remove stale file when --force.
	if *force {
		_ = os.Remove(dbPath)
	}

	// Open read-write with a single connection (serialised writes).
	dsn := fmt.Sprintf(
		"file:%s?_txlock=immediate&_pragma=busy_timeout(5000)&_pragma=foreign_keys(on)",
		dbPath,
	)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	db.SetMaxOpenConns(1)
	defer db.Close()

	if err := db.Ping(); err != nil {
		return fmt.Errorf("ping db: %w", err)
	}

	// Anchor timestamps to today at noon UTC so data always falls in the last 90 days.
	now := time.Now().UTC()
	seedBase = time.Date(now.Year(), now.Month(), now.Day(), 12, 0, 0, 0, time.UTC)

	fmt.Printf("Creating demo database at %s\n", dbPath)

	if err := applySchema(db); err != nil {
		return fmt.Errorf("apply schema: %w", err)
	}

	counts, err := seedData(db)
	if err != nil {
		return fmt.Errorf("seed data: %w", err)
	}

	fmt.Println("\nDemo database seeded successfully.")
	fmt.Printf("  sessions:         %d\n", counts.sessions)
	fmt.Printf("  observations:     %d\n", counts.observations)
	fmt.Printf("  user_prompts:     %d\n", counts.prompts)
	fmt.Printf("  memory_relations: %d\n", counts.relations)
	fmt.Printf("  sync_enrolled:    %d\n", counts.syncEnrolled)
	fmt.Printf("  sync_mutations:   %d\n", counts.syncMutations)
	fmt.Println()
	fmt.Println("Run the dashboard against the demo database:")
	fmt.Printf("  Linux/macOS: ENGRAM_DATA_DIR=%s ./engram-explorer\n", *outDir)
	fmt.Printf("  Windows/pwsh: $env:ENGRAM_DATA_DIR=\"%s\"; .\\engram-explorer.exe\n", absOut)
	fmt.Println()
	fmt.Println("Then open http://127.0.0.1:8787 in your browser.")
	return nil
}

// rowCounts holds the number of rows inserted per table.
type rowCounts struct {
	sessions      int
	observations  int
	prompts       int
	relations     int
	syncEnrolled  int
	syncMutations int
}

// ----------------------------------------------------------------------------
// Schema
// ----------------------------------------------------------------------------

// applySchema creates all tables, indexes, triggers, and FTS virtual tables.
// Shadow FTS tables (*_fts_data, *_fts_idx, etc.) are intentionally omitted
// because SQLite creates them automatically when the VIRTUAL TABLE is created;
// attempting to CREATE them manually fails.
func applySchema(db *sql.DB) error {
	stmts := []string{
		// Core tables
		`CREATE TABLE sessions (
			id         TEXT PRIMARY KEY,
			project    TEXT NOT NULL,
			directory  TEXT NOT NULL,
			started_at TEXT NOT NULL DEFAULT (datetime('now')),
			ended_at   TEXT,
			summary    TEXT
		)`,
		`CREATE TABLE observations (
			id              INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id      TEXT    NOT NULL,
			type            TEXT    NOT NULL,
			title           TEXT    NOT NULL,
			content         TEXT    NOT NULL,
			tool_name       TEXT,
			project         TEXT,
			scope           TEXT    NOT NULL DEFAULT 'project',
			topic_key       TEXT,
			normalized_hash TEXT,
			revision_count  INTEGER NOT NULL DEFAULT 1,
			duplicate_count INTEGER NOT NULL DEFAULT 1,
			last_seen_at    TEXT,
			created_at      TEXT    NOT NULL DEFAULT (datetime('now')),
			updated_at      TEXT    NOT NULL DEFAULT (datetime('now')),
			deleted_at      TEXT,
			sync_id         TEXT,
			review_after    TEXT,
			expires_at      TEXT,
			embedding       BLOB,
			embedding_model TEXT,
			embedding_created_at TEXT,
			FOREIGN KEY (session_id) REFERENCES sessions(id)
		)`,
		`CREATE TABLE memory_relations (
			id                        INTEGER PRIMARY KEY AUTOINCREMENT,
			sync_id                   TEXT    NOT NULL UNIQUE,
			source_id                 TEXT,
			target_id                 TEXT,
			relation                  TEXT    NOT NULL DEFAULT 'pending',
			reason                    TEXT,
			evidence                  TEXT,
			confidence                REAL,
			judgment_status           TEXT    NOT NULL DEFAULT 'pending',
			marked_by_actor           TEXT,
			marked_by_kind            TEXT,
			marked_by_model           TEXT,
			session_id                TEXT,
			superseded_at             TEXT,
			superseded_by_relation_id INTEGER REFERENCES memory_relations(id) ON DELETE SET NULL,
			created_at                TEXT    NOT NULL DEFAULT (datetime('now')),
			updated_at                TEXT    NOT NULL DEFAULT (datetime('now'))
		)`,
		`CREATE TABLE user_prompts (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id TEXT    NOT NULL,
			content    TEXT    NOT NULL,
			project    TEXT,
			created_at TEXT    NOT NULL DEFAULT (datetime('now')),
			sync_id    TEXT,
			FOREIGN KEY (session_id) REFERENCES sessions(id)
		)`,
		`CREATE TABLE prompt_tombstones (
			sync_id    TEXT PRIMARY KEY,
			session_id TEXT,
			project    TEXT,
			deleted_at TEXT NOT NULL DEFAULT (datetime('now'))
		)`,
		// Sync tables — sync_state must exist before sync_mutations (FK)
		`CREATE TABLE sync_enrolled_projects (
			project     TEXT PRIMARY KEY,
			enrolled_at TEXT NOT NULL DEFAULT (datetime('now'))
		)`,
		`CREATE TABLE sync_state (
			target_key           TEXT PRIMARY KEY,
			lifecycle            TEXT NOT NULL DEFAULT 'idle',
			last_enqueued_seq    INTEGER NOT NULL DEFAULT 0,
			last_acked_seq       INTEGER NOT NULL DEFAULT 0,
			last_pulled_seq      INTEGER NOT NULL DEFAULT 0,
			consecutive_failures INTEGER NOT NULL DEFAULT 0,
			backoff_until        TEXT,
			lease_owner          TEXT,
			lease_until          TEXT,
			last_error           TEXT,
			updated_at           TEXT NOT NULL DEFAULT (datetime('now')),
			reason_code          TEXT,
			reason_message       TEXT
		)`,
		`CREATE TABLE sync_mutations (
			seq         INTEGER PRIMARY KEY AUTOINCREMENT,
			target_key  TEXT NOT NULL,
			entity      TEXT NOT NULL,
			entity_key  TEXT NOT NULL,
			op          TEXT NOT NULL,
			payload     TEXT NOT NULL,
			source      TEXT NOT NULL DEFAULT 'local',
			occurred_at TEXT NOT NULL DEFAULT (datetime('now')),
			acked_at    TEXT,
			project     TEXT NOT NULL DEFAULT '',
			FOREIGN KEY (target_key) REFERENCES sync_state(target_key)
		)`,
		`CREATE TABLE sync_apply_deferred (
			sync_id           TEXT    PRIMARY KEY,
			entity            TEXT    NOT NULL,
			payload           TEXT    NOT NULL,
			apply_status      TEXT    NOT NULL DEFAULT 'deferred',
			retry_count       INTEGER NOT NULL DEFAULT 0,
			last_error        TEXT,
			last_attempted_at TEXT,
			first_seen_at     TEXT    NOT NULL DEFAULT (datetime('now'))
		)`,
		`CREATE TABLE "sync_chunks" (
			target_key  TEXT NOT NULL DEFAULT 'local',
			chunk_id    TEXT NOT NULL,
			imported_at TEXT NOT NULL DEFAULT (datetime('now')),
			PRIMARY KEY (target_key, chunk_id)
		)`,
		`CREATE TABLE cloud_upgrade_state (
			project            TEXT PRIMARY KEY,
			stage              TEXT NOT NULL DEFAULT 'planned',
			repair_class       TEXT NOT NULL DEFAULT 'none',
			snapshot_json      TEXT NOT NULL DEFAULT '{}',
			last_error_code    TEXT,
			last_error_message TEXT,
			findings_json      TEXT,
			applied_actions    TEXT,
			updated_at         TEXT NOT NULL DEFAULT (datetime('now'))
		)`,

		// FTS virtual tables (shadow tables created automatically by SQLite)
		`CREATE VIRTUAL TABLE observations_fts USING fts5(
			title,
			content,
			tool_name,
			type,
			project,
			topic_key,
			content='observations',
			content_rowid='id'
		)`,
		`CREATE VIRTUAL TABLE prompts_fts USING fts5(
			content,
			project,
			content='user_prompts',
			content_rowid='id'
		)`,

		// Indexes
		`CREATE INDEX idx_obs_created  ON observations(created_at DESC)`,
		`CREATE INDEX idx_obs_dedupe   ON observations(normalized_hash, project, scope, type, title, created_at DESC)`,
		`CREATE INDEX idx_obs_deleted  ON observations(deleted_at)`,
		`CREATE INDEX idx_obs_project  ON observations(project)`,
		`CREATE INDEX idx_obs_scope    ON observations(scope)`,
		`CREATE INDEX idx_obs_session  ON observations(session_id)`,
		`CREATE INDEX idx_obs_sync_id  ON observations(sync_id)`,
		`CREATE INDEX idx_obs_topic    ON observations(topic_key, project, scope, updated_at DESC)`,
		`CREATE INDEX idx_obs_type     ON observations(type)`,
		`CREATE INDEX idx_prompts_created  ON user_prompts(created_at DESC)`,
		`CREATE INDEX idx_prompts_project  ON user_prompts(project)`,
		`CREATE INDEX idx_prompts_session  ON user_prompts(session_id)`,
		`CREATE INDEX idx_prompts_sync_id  ON user_prompts(sync_id)`,
		`CREATE INDEX idx_memrel_source    ON memory_relations(source_id, judgment_status)`,
		`CREATE INDEX idx_memrel_target    ON memory_relations(target_id, judgment_status)`,
		`CREATE INDEX idx_memrel_status_created ON memory_relations(judgment_status, created_at DESC)`,
		`CREATE INDEX idx_memrel_supersede ON memory_relations(superseded_by_relation_id)`,
		`CREATE INDEX idx_sync_mutations_lookup     ON sync_mutations(target_key, entity, entity_key, source)`,
		`CREATE INDEX idx_sync_mutations_pending    ON sync_mutations(target_key, acked_at, seq)`,
		`CREATE INDEX idx_sync_mutations_project    ON sync_mutations(project)`,
		`CREATE INDEX idx_sync_mutations_target_seq ON sync_mutations(target_key, seq)`,
		`CREATE INDEX idx_sad_status_seen           ON sync_apply_deferred(apply_status, first_seen_at)`,
		`CREATE INDEX idx_cloud_upgrade_state_stage ON cloud_upgrade_state(stage)`,
		`CREATE INDEX idx_prompt_tombstones_project ON prompt_tombstones(project, deleted_at DESC)`,

		// FTS triggers for observations
		`CREATE TRIGGER obs_fts_insert AFTER INSERT ON observations BEGIN
			INSERT INTO observations_fts(rowid, title, content, tool_name, type, project, topic_key)
			VALUES (new.id, new.title, new.content, new.tool_name, new.type, new.project, new.topic_key);
		END`,
		`CREATE TRIGGER obs_fts_delete AFTER DELETE ON observations BEGIN
			INSERT INTO observations_fts(observations_fts, rowid, title, content, tool_name, type, project, topic_key)
			VALUES ('delete', old.id, old.title, old.content, old.tool_name, old.type, old.project, old.topic_key);
		END`,
		`CREATE TRIGGER obs_fts_update AFTER UPDATE ON observations BEGIN
			INSERT INTO observations_fts(observations_fts, rowid, title, content, tool_name, type, project, topic_key)
			VALUES ('delete', old.id, old.title, old.content, old.tool_name, old.type, old.project, old.topic_key);
			INSERT INTO observations_fts(rowid, title, content, tool_name, type, project, topic_key)
			VALUES (new.id, new.title, new.content, new.tool_name, new.type, new.project, new.topic_key);
		END`,

		// FTS triggers for prompts
		`CREATE TRIGGER prompt_fts_insert AFTER INSERT ON user_prompts BEGIN
			INSERT INTO prompts_fts(rowid, content, project)
			VALUES (new.id, new.content, new.project);
		END`,
		`CREATE TRIGGER prompt_fts_delete AFTER DELETE ON user_prompts BEGIN
			INSERT INTO prompts_fts(prompts_fts, rowid, content, project)
			VALUES ('delete', old.id, old.content, old.project);
		END`,
		`CREATE TRIGGER prompt_fts_update AFTER UPDATE ON user_prompts BEGIN
			INSERT INTO prompts_fts(prompts_fts, rowid, content, project)
			VALUES ('delete', old.id, old.content, old.project);
			INSERT INTO prompts_fts(rowid, content, project)
			VALUES (new.id, new.content, new.project);
		END`,

		// Guard trigger
		`CREATE TRIGGER skip_empty_obs_upsert_mutations
		BEFORE INSERT ON sync_mutations
		FOR EACH ROW
		WHEN NEW.entity = 'observation' AND NEW.op = 'upsert'
		     AND (json_extract(NEW.payload, '$.content') IS NULL
		          OR length(trim(coalesce(json_extract(NEW.payload, '$.content'), ''))) = 0)
		BEGIN
		  SELECT RAISE(IGNORE);
		END`,
	}

	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("exec schema %q: %w", truncate(stmt, 60), err)
		}
	}
	return nil
}

func truncate(s string, n int) string {
	// Trim leading whitespace for readability in error messages.
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// ----------------------------------------------------------------------------
// Seed data
// ----------------------------------------------------------------------------

// ts returns a UTC timestamp string by subtracting daysAgo days from seedBase
// and then subtracting hoursOffset hours. Both daysAgo and hoursOffset should
// be non-negative — they push the timestamp into the past relative to seedBase.
// The resulting timestamps always fall within the last 90 days from now, which
// keeps activity charts populated regardless of when the seeder is run.
func ts(daysAgo int, hoursOffset int) string {
	t := seedBase.Add(-time.Duration(daysAgo)*24*time.Hour - time.Duration(hoursOffset)*time.Hour)
	return t.Format("2006-01-02 15:04:05")
}

// ----------------------------------------------------------------------------
// Generated bulk data
// ----------------------------------------------------------------------------

// genObs holds a generated observation's fields.
type genObs struct {
	sessionID      string
	obsType        string
	title          string
	content        string
	project        string
	scope          string
	topicKey       string
	normalizedHash string
	revisionCount  int
	syncID         string
	createdAt      string
	updatedAt      string
}

// generatedObservations returns ~213 synthetic observations.
// Sync-IDs start at sid(1000) so they never collide with the curated sid(1)–sid(67).
// About 15% share a normalizedHash so the graph shows collapsed/weighted nodes.
func generatedObservations() []genObs {
	// sid helper — same format as the curated rows.
	sid := func(n int) string { return fmt.Sprintf("sync-%04d-0000-0000-0000-000000000000", n) }

	// Available session IDs per project.
	apiSessions := []string{"s-api-01", "s-api-02", "s-api-03", "s-api-04", "s-api-05", "s-api-06", "s-api-07"}
	webSessions := []string{"s-web-01", "s-web-02", "s-web-03", "s-web-04", "s-web-05", "s-web-06", "s-web-07"}
	resSessions := []string{"s-res-01", "s-res-02", "s-res-03", "s-res-04", "s-res-05", "s-res-06"}
	infSessions := []string{"s-inf-01", "s-inf-02", "s-inf-03", "s-inf-04", "s-inf-05", "s-inf-06"}
	orpSessions := []string{"s-orp-01", "s-orp-02", "s-orp-03"}

	types := []string{
		"architecture", "decision", "bugfix", "feature", "discovery",
		"pattern", "config", "preference", "insight", "learning",
	}

	// Topic templates per project — interpolated with index for variety.
	type tpl struct {
		titleFmt   string // %s = project snippet, %d = index
		contentFmt string
		topicFmt   string
		typeIdx    int // index into types[]
	}

	apiTopics := []tpl{
		{"Caching layer for the %s read path (variant %d)", "Added an in-memory LRU cache in front of the DB layer for frequently-read records. Cache TTL is 30 s; invalidated on write. Reduces DB load by ~60%% at steady state.", "pattern/caching-%d", 5},
		{"Refactored %s middleware chain to reduce coupling (v%d)", "Extracted authentication, logging, and rate-limit logic into standalone middleware factories. Each factory is independently testable and composable.", "architecture/middleware-%d", 0},
		{"Error boundary strategy for %s async operations (pass %d)", "All async service calls are wrapped in a try/catch that maps domain errors to typed HTTP responses. Unknown errors are logged and return 500.", "pattern/error-boundary-%d", 1},
		{"OpenAPI spec for the %s endpoint group (rev %d)", "Generated an OpenAPI 3.1 spec from Zod schemas using `@asteasolutions/zod-to-openapi`. Kept in sync via a CI check that diffs the generated file.", "feature/openapi-%d", 3},
		{"SQL index audit for %s hot queries (run %d)", "Ran EXPLAIN QUERY PLAN on the top-10 slowest endpoints. Added three composite indexes that eliminated full-table scans. P99 dropped from 420 ms to 38 ms.", "discovery/index-audit-%d", 4},
		{"Request validation refinement for %s (iteration %d)", "Tightened Zod schemas to reject empty strings, trim whitespace, and enforce max-length constraints. Prevents garbage data reaching the DB.", "pattern/validation-%d", 5},
		{"Structured logging setup in %s (version %d)", "Switched from console.log to pino with a JSON transport. Added request-ID field to every log line. Logs are streamed to stdout and collected by the platform.", "config/logging-%d", 6},
		{"Health check endpoint for %s (rev %d)", "Added /api/health that pings the DB and returns {status, latency, version}. Used by Kubernetes readiness probes.", "feature/health-check-%d", 3},
		{"Database connection pool tuning for %s (attempt %d)", "Set pool size to CPU count × 2. Added connection timeout of 5 s. Idle connections are recycled after 30 s. Eliminates connection exhaustion under load.", "config/pool-%d", 6},
		{"Integration test harness for %s services (pass %d)", "Tests spin up an in-memory SQLite database, seed it with fixtures, and call services directly. No HTTP layer involved — fast and deterministic.", "pattern/test-harness-%d", 5},
		{"Soft-delete pattern for %s records (revision %d)", "Added deleted_at column. Queries filter WHERE deleted_at IS NULL by default. Hard-delete job runs nightly to purge rows older than 90 days.", "pattern/soft-delete-%d", 5},
		{"Webhook delivery retry logic for %s (iteration %d)", "Exponential backoff with jitter: 1s → 5s → 25s → 125s. After 5 failures the webhook is disabled and the owner notified.", "feature/webhooks-%d", 3},
	}

	webTopics := []tpl{
		{"Form validation with React Hook Form in %s (rev %d)", "Integrated react-hook-form with Zod resolvers. All form state is uncontrolled; only errors and submission state are re-rendered.", "pattern/form-validation-%d", 5},
		{"Accessible modal dialog component for %s (v%d)", "Built a modal with focus-trap, Escape-to-close, scroll-lock, and aria-modal. Uses a portal to render outside the DOM tree.", "feature/modal-%d", 3},
		{"Stale-while-revalidate cache strategy for %s (pass %d)", "TanStack Query staleTime set to 30 s for list queries, 60 s for detail. This avoids refetch on every navigation while keeping data fresh.", "pattern/cache-strategy-%d", 5},
		{"Responsive layout tokens for %s (version %d)", "Defined breakpoint tokens (sm, md, lg, xl) as Tailwind custom screen values. Components use container-query classes where applicable.", "config/breakpoints-%d", 6},
		{"Error state component design for %s (iteration %d)", "Created ErrorState component with title, description, and retry button. Used by all async boundaries. Matches the empty state visual language.", "feature/error-state-%d", 3},
		{"Keyboard navigation for %s data table (pass %d)", "Table cells are focusable via Tab. Arrow keys move between cells. Enter opens a row detail panel. Escape closes it.", "feature/keyboard-nav-%d", 3},
		{"Animation tokens for %s transitions (rev %d)", "Added duration and easing CSS variables: --duration-fast (100ms), --duration-base (200ms), --ease-out. All transitions reference these tokens.", "config/animation-%d", 6},
		{"Optimistic updates for %s mutations (v%d)", "Used TanStack Query's optimistic update pattern: cancel in-flight queries, snapshot current data, update cache, rollback on error.", "pattern/optimistic-%d", 5},
		{"Code splitting strategy for %s routes (pass %d)", "Each top-level route is a lazy import. Heavy chart and 3D components are split into their own chunks. Bundle analyzer confirms no shared chunk exceeds 150 KB.", "architecture/code-splitting-%d", 0},
		{"Test IDs for %s components (revision %d)", "All interactive elements have stable data-testid attributes. IDs follow the pattern: component-name--action (e.g. filter-panel--clear).", "pattern/test-ids-%d", 5},
		{"Drag and drop ordering for %s list (v%d)", "Implemented column reordering with @dnd-kit/core. Drag state is stored in Zustand; persisted to localStorage on drop.", "feature/dnd-%d", 3},
		{"Context menu for %s table rows (iteration %d)", "Right-click opens a context menu with row-scoped actions. Menu is positioned relative to click coordinates and dismisses on outside click.", "feature/context-menu-%d", 3},
	}

	resTopics := []tpl{
		{"Benchmark: FTS5 vs SQLite JSON operators for %s (run %d)", "Compared FTS5 full-text search with JSON_EXTRACT + LIKE for semi-structured content. FTS5 is 8× faster on 100 K rows; JSON approach allows field-level filtering.", "research/fts-vs-json-%d", 4},
		{"LLM context window compression techniques for %s (pass %d)", "Summarization, selective inclusion, and structured artifacts all reduce token usage. Structured artifacts (JSON) compress best — 3× smaller than prose summaries.", "research/context-compression-%d", 9},
		{"Knowledge graph vs relational store for %s memory (rev %d)", "Relational works for structured facts; knowledge graphs excel at multi-hop reasoning. For memory with <10 K nodes, SQLite + explicit relation rows is simpler and fast enough.", "research/graph-vs-relational-%d", 9},
		{"Chunking strategies for RAG over %s documents (pass %d)", "Fixed-size chunking is simple but breaks semantic units. Sentence-level chunking preserves meaning. Sliding-window (50% overlap) gives best recall@5 in benchmarks.", "research/chunking-%d", 4},
		{"Reranking models for %s retrieval (iteration %d)", "Cross-encoders (ms-marco-MiniLM-L6) score query-passage pairs jointly. 40 ms per pair is too slow for >20 candidates. Use bi-encoder for recall, cross-encoder for top-10 rerank.", "research/reranking-%d", 9},
		{"Memory decay and forgetting curves in %s agents (rev %d)", "Ebbinghaus curves suggest exponential decay. Spaced repetition (review_after field) surfaces aging memories before they become stale.", "research/memory-decay-%d", 9},
		{"Evaluation harness for %s memory retrieval (v%d)", "Built a micro-benchmark: 50 synthetic queries × 3 retrieval methods (FTS5, vector, hybrid). Precision@3 and recall@5 are the primary metrics.", "discovery/eval-harness-%d", 4},
		{"Cost analysis: local vs cloud embeddings for %s (pass %d)", "Local MiniLM: $0 OPEX, 5 ms/query, 384 dims. Cloud text-embedding-3-small: $0.02/1M tokens, 50 ms round-trip, 1536 dims. Break-even at ~2 M queries/month.", "research/embedding-cost-%d", 1},
	}

	infTopics := []tpl{
		{"Container image hardening for %s (pass %d)", "Switched base image to distroless/static. Dropped all capabilities except NET_BIND. Run as non-root UID 1000. Trivy scan reports 0 critical CVEs.", "config/image-hardening-%d", 6},
		{"Log aggregation pipeline for %s services (v%d)", "Fluent Bit DaemonSet collects container logs, enriches with pod metadata, and ships to OpenSearch. Retention: 30 days. Alerting via ElastAlert.", "architecture/log-pipeline-%d", 0},
		{"Autoscaling policy for %s workloads (revision %d)", "HPA on CPU utilization (target 70%) with min-replicas=2, max-replicas=10. VPA recommendations reviewed monthly. PDB ensures at least 1 pod available during disruptions.", "config/autoscaling-%d", 6},
		{"Database backup strategy for %s (pass %d)", "Daily full backup + hourly WAL archiving to S3. Point-in-time recovery tested quarterly. RPO < 1 hour, RTO < 4 hours.", "config/backup-strategy-%d", 6},
		{"Network policy for %s pod communication (v%d)", "Default-deny all ingress/egress. Explicit allow rules for each service. API pods may reach DB pods on port 5432; no other cross-namespace traffic allowed.", "architecture/network-policy-%d", 0},
		{"Terraform module for %s infrastructure (revision %d)", "Extracted reusable Terraform modules for VPC, EKS cluster, and RDS. Modules are versioned in a separate repo and pinned in the main infra repo.", "architecture/terraform-%d", 0},
		{"Load testing setup for %s with k6 (pass %d)", "k6 scenarios simulate 50/200/500 concurrent users. Thresholds: p95 < 200ms, error rate < 0.1%. Tests run in CI on every release candidate.", "feature/load-testing-%d", 3},
		{"Observability runbook for %s on-call (v%d)", "Runbook covers: alert triage, common failure modes, rollback procedure, DB connection debug, and escalation path. Stored in the wiki and linked from every alert.", "config/runbook-%d", 6},
	}

	orphanTopics := []tpl{
		{"Scratch experiment: %s prototype approach (attempt %d)", "Quick feasibility test. Explored the idea informally without committing to a project. Found the approach viable but deferred full implementation.", "scratch/experiment-%d", 9},
		{"Ad-hoc debugging session: %s issue (pass %d)", "Investigated a transient failure outside normal project context. Root cause identified; fix delegated to the appropriate project.", "scratch/debug-%d", 2},
		{"Reference note: %s concept (rev %d)", "Captured a useful concept for future reference. Not tied to a specific project yet.", "scratch/reference-%d", 8},
	}

	// Duplicate hash groups — ~15% of generated obs share a hash.
	// We assign a hash to every 7th observation starting from index 0.
	dupHashes := []string{
		"hash-gen-dup-A",
		"hash-gen-dup-B",
		"hash-gen-dup-C",
		"hash-gen-dup-D",
		"hash-gen-dup-E",
		"hash-gen-dup-F",
		"hash-gen-dup-G",
		"hash-gen-dup-H",
	}

	var out []genObs
	idx := 0 // overall counter; used for sid offset and deterministic timestamps

	add := func(sessions []string, project string, topics []tpl, count int, scope string) {
		for i := 0; i < count; i++ {
			tplEntry := topics[i%len(topics)]
			sessID := sessions[i%len(sessions)]
			obsType := types[tplEntry.typeIdx]

			projectToken := project
			if projectToken == "" {
				projectToken = "scratch"
			}

			title := fmt.Sprintf(tplEntry.titleFmt, projectToken, i+1)
			content := fmt.Sprintf(tplEntry.contentFmt, projectToken, i+1)
			topicKey := fmt.Sprintf(tplEntry.topicFmt, i+1)

			// daysAgo: spread observations across last 88 days based on index.
			daysAgo := 88 - (idx*88)/300 // never exceeds 88
			hoursOff := (idx * 3) % 12

			// ~15% of obs share a duplicate hash: every 7th gets one.
			var normHash string
			if idx%7 == 0 {
				normHash = dupHashes[(idx/7)%len(dupHashes)]
			}

			syncID := sid(1000 + idx)
			out = append(out, genObs{
				sessionID:      sessID,
				obsType:        obsType,
				title:          title,
				content:        content,
				project:        project,
				scope:          scope,
				topicKey:       topicKey,
				normalizedHash: normHash,
				revisionCount:  1 + (idx % 3),
				syncID:         syncID,
				createdAt:      ts(daysAgo, hoursOff),
				updatedAt:      ts(daysAgo, hoursOff),
			})
			idx++
		}
	}

	add(apiSessions, "demo-api", apiTopics, 55, "project")
	add(webSessions, "demo-web", webTopics, 55, "project")
	add(resSessions, "research-notes", resTopics, 45, "project")
	add(infSessions, "infra", infTopics, 45, "project")
	add(orpSessions, "", orphanTopics, 13, "project")

	return out
}

// genRelation holds a generated relation's fields.
type genRelation struct {
	syncID     string
	sourceID   string
	targetID   string
	relation   string
	reason     string
	confidence float64
	sessionID  string
	createdAt  string
}

// generatedRelations returns ~120 synthetic memory_relations.
// obsSyncIDs is the full list of observation sync_ids (curated + generated).
// Relations reference only valid sync_ids and never link an obs to itself.
func generatedRelations(obsSyncIDs []string) []genRelation {
	relTypes := []string{"related", "scoped", "compatible"}

	reasons := []string{
		"Both observations describe complementary aspects of the same subsystem.",
		"The target is a prerequisite for the pattern described in the source.",
		"Research finding validates the implementation choice.",
		"Configuration depends on the architectural decision.",
		"Bug fix was motivated by the discovery in the target observation.",
		"Feature builds on the pattern established in the target.",
		"Both observations share the same underlying design constraint.",
		"The source refines the convention introduced in the target.",
		"Operational concern in source is informed by the learning in target.",
		"Insight in source emerged from the context described in target.",
		"Target explains why the approach in source was chosen.",
		"Source and target are alternative solutions to the same problem.",
		"Decision in source is bounded by the constraint noted in target.",
		"Pattern in source is a specialization of the general principle in target.",
		"Discovery in source explains the symptom observed in target.",
	}

	// Session IDs for relation metadata (one per project area).
	sessionsByIndex := []string{
		"s-api-01", "s-api-02", "s-api-03", "s-api-04", "s-api-05", "s-api-06", "s-api-07",
		"s-web-01", "s-web-02", "s-web-03", "s-web-04", "s-web-05", "s-web-06", "s-web-07",
		"s-res-01", "s-res-02", "s-res-03", "s-res-04", "s-res-05", "s-res-06",
		"s-inf-01", "s-inf-02", "s-inf-03", "s-inf-04", "s-inf-05", "s-inf-06",
	}

	n := len(obsSyncIDs)
	var out []genRelation
	count := 0
	target := 120

	for count < target {
		i := count
		// Pick source and target deterministically using prime-step walk to avoid
		// obvious sequential pairs while staying fully deterministic.
		srcIdx := (i * 17) % n
		tgtIdx := (i*13 + 7) % n

		// Ensure source != target; if they collide, offset target by a fixed amount.
		if srcIdx == tgtIdx {
			tgtIdx = (tgtIdx + 3) % n
		}
		// Second collision guard (e.g. after wrap).
		if srcIdx == tgtIdx {
			tgtIdx = (tgtIdx + 1) % n
		}

		srcSyncID := obsSyncIDs[srcIdx]
		tgtSyncID := obsSyncIDs[tgtIdx]

		relType := relTypes[i%len(relTypes)]
		reason := reasons[i%len(reasons)]
		// confidence in [0.60, 0.95], derived deterministically.
		confidence := 0.60 + float64(i%36)*float64(1)/float64(100)
		sessID := sessionsByIndex[i%len(sessionsByIndex)]

		// Spread timestamps: most recent first, spread over 88 days.
		daysAgo := 1 + (i*88)/target
		hoursOff := (i * 5) % 12

		out = append(out, genRelation{
			syncID:     fmt.Sprintf("rel-gen-%04d-0000-0000-0000-000000000000", 1000+i),
			sourceID:   srcSyncID,
			targetID:   tgtSyncID,
			relation:   relType,
			reason:     reason,
			confidence: confidence,
			sessionID:  sessID,
			createdAt:  ts(daysAgo, hoursOff),
		})
		count++
	}

	return out
}

// seedData inserts all demo rows inside a single transaction and returns counts.
func seedData(db *sql.DB) (rowCounts, error) {
	tx, err := db.Begin()
	if err != nil {
		return rowCounts{}, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	counts, err := insertAll(tx)
	if err != nil {
		return rowCounts{}, err
	}

	if err := tx.Commit(); err != nil {
		return rowCounts{}, fmt.Errorf("commit: %w", err)
	}
	return counts, nil
}

// insertAll performs all INSERT statements. It is called inside a transaction.
func insertAll(tx *sql.Tx) (rowCounts, error) {
	var c rowCounts

	// ── Sessions ──────────────────────────────────────────────────────────────
	// 5 projects × ~7 sessions each = 35 sessions total.
	// Project "" is intentional — exercises the "(orphans)" path.
	sessions := []struct {
		id        string
		project   string
		directory string
		startedAt string
		endedAt   string
		summary   string
	}{
		// demo-api
		{"s-api-01", "demo-api", "/home/user/demo-api", ts(89, 0), ts(89, -2), "Bootstrapped the REST API skeleton with Hono and Zod validation."},
		{"s-api-02", "demo-api", "/home/user/demo-api", ts(75, 0), ts(75, -3), "Added authentication middleware and JWT token flow."},
		{"s-api-03", "demo-api", "/home/user/demo-api", ts(60, 0), ts(60, -2), "Implemented pagination with cursor-based keyset approach."},
		{"s-api-04", "demo-api", "/home/user/demo-api", ts(45, 0), ts(45, -4), "Refactored service layer to follow hexagonal architecture."},
		{"s-api-05", "demo-api", "/home/user/demo-api", ts(30, 0), ts(30, -2), "Added rate limiting and request validation."},
		{"s-api-06", "demo-api", "/home/user/demo-api", ts(15, 0), ts(15, -1), "Performance profiling and query optimization."},
		{"s-api-07", "demo-api", "/home/user/demo-api", ts(3, 0), ts(3, -2), "Reviewed and documented API surface."},

		// demo-web
		{"s-web-01", "demo-web", "/home/user/demo-web", ts(88, 0), ts(88, -2), "Set up Vite + React + TanStack Router boilerplate."},
		{"s-web-02", "demo-web", "/home/user/demo-web", ts(72, 0), ts(72, -3), "Implemented data table with virtualization."},
		{"s-web-03", "demo-web", "/home/user/demo-web", ts(55, 0), ts(55, -2), "Built component library with cva + tailwind-merge."},
		{"s-web-04", "demo-web", "/home/user/demo-web", ts(40, 0), ts(40, -2), "Dark mode tokens and CSS custom properties."},
		{"s-web-05", "demo-web", "/home/user/demo-web", ts(25, 0), ts(25, -3), "Infinite scroll integration with TanStack Query."},
		{"s-web-06", "demo-web", "/home/user/demo-web", ts(10, 0), ts(10, -1), "Accessibility audit and ARIA improvements."},
		{"s-web-07", "demo-web", "/home/user/demo-web", ts(2, 0), ts(2, -1), "Bundle size optimization with dynamic imports."},

		// research-notes
		{"s-res-01", "research-notes", "/home/user/notes", ts(85, 0), ts(85, -1), "Literature review on vector search approaches."},
		{"s-res-02", "research-notes", "/home/user/notes", ts(70, 0), ts(70, -2), "Comparing embedding models for semantic search."},
		{"s-res-03", "research-notes", "/home/user/notes", ts(50, 0), ts(50, -1), "Notes on RAG pipeline design patterns."},
		{"s-res-04", "research-notes", "/home/user/notes", ts(35, 0), ts(35, -2), "Explored multi-agent orchestration approaches."},
		{"s-res-05", "research-notes", "/home/user/notes", ts(20, 0), ts(20, -1), "Research on SQLite full-text search capabilities."},
		{"s-res-06", "research-notes", "/home/user/notes", ts(8, 0), ts(8, -1), "Reviewed cursor-based pagination literature."},

		// infra
		{"s-inf-01", "infra", "/home/user/infra", ts(83, 0), ts(83, -2), "Set up Docker multi-stage builds for Go services."},
		{"s-inf-02", "infra", "/home/user/infra", ts(65, 0), ts(65, -3), "Configured GitHub Actions CI/CD pipeline."},
		{"s-inf-03", "infra", "/home/user/infra", ts(48, 0), ts(48, -2), "Added database migration workflow with goose."},
		{"s-inf-04", "infra", "/home/user/infra", ts(32, 0), ts(32, -1), "Hardened secrets management with Vault integration."},
		{"s-inf-05", "infra", "/home/user/infra", ts(18, 0), ts(18, -2), "Prometheus metrics and Grafana dashboards."},
		{"s-inf-06", "infra", "/home/user/infra", ts(5, 0), ts(5, -1), "Reviewed deployment strategy for zero-downtime releases."},

		// orphan sessions (project = "")
		{"s-orp-01", "", "/tmp/scratch", ts(80, 0), ts(80, -1), "Quick experiment without project context."},
		{"s-orp-02", "", "/tmp/scratch", ts(60, 0), ts(60, -1), "Scratch session for ad-hoc debugging."},
		{"s-orp-03", "", "/tmp/scratch", ts(40, 0), ts(40, -1), "Prototype idea before assigning to a project."},
	}

	sessStmt, err := tx.Prepare(`INSERT INTO sessions (id, project, directory, started_at, ended_at, summary) VALUES (?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return rowCounts{}, fmt.Errorf("prepare sessions: %w", err)
	}
	defer sessStmt.Close()

	for _, s := range sessions {
		if _, err := sessStmt.Exec(s.id, s.project, s.directory, s.startedAt, s.endedAt, s.summary); err != nil {
			return rowCounts{}, fmt.Errorf("insert session %s: %w", s.id, err)
		}
		c.sessions++
	}

	// ── Observations ──────────────────────────────────────────────────────────
	// ~180 observations spread across projects and types.
	// The FTS triggers (obs_fts_insert) will auto-populate observations_fts.

	type obs struct {
		sessionID      string
		obsType        string
		title          string
		content        string
		project        string
		scope          string
		topicKey       string
		normalizedHash string
		revisionCount  int
		syncID         string
		createdAt      string
		updatedAt      string
	}

	// Helper to build a sync_id (deterministic, uuid-like format).
	sid := func(n int) string { return fmt.Sprintf("sync-%04d-0000-0000-0000-000000000000", n) }

	observations := []obs{
		// ── demo-api: architecture ──────────────────────────────────────────────
		{"s-api-01", "architecture", "REST API layering: routes → services → adapters",
			"We use a strict three-layer architecture. Routes handle HTTP binding and Zod validation. " +
				"Services contain all business logic and SQL. Adapters wrap the database driver.\n\n" +
				"[[decision/api-architecture]] [[pattern/layered-arch]]",
			"demo-api", "project", "architecture/api-layers", "hash-api-arch-001", 3, sid(1), ts(89, 0), ts(60, 0)},
		{"s-api-01", "decision", "Chose Hono v4 over Express for the API framework",
			"Hono provides native TypeScript types, built-in Zod validator middleware, and " +
				"edge-compatible routing with zero dependencies. Express was ruled out because " +
				"it lacks first-class TypeScript support and has a heavier middleware chain.\n\n" +
				"Tradeoffs: smaller Hono ecosystem, but the built-in validators offset this.",
			"demo-api", "project", "decision/framework-choice", "hash-api-hono-001", 2, sid(2), ts(89, 1), ts(89, 1)},
		{"s-api-02", "architecture", "JWT authentication flow",
			"The API uses stateless JWT access tokens (15 min TTL) plus refresh tokens stored " +
				"in httpOnly cookies. All protected routes share an `authMiddleware` that validates " +
				"the token signature and expiry.\n\n```typescript\nconst authMiddleware = createMiddleware(async (c, next) => {\n  const token = c.req.header('Authorization')?.replace('Bearer ', '')\n  // ...\n})\n```",
			"demo-api", "project", "architecture/auth-flow", "", 1, sid(3), ts(75, 0), ts(75, 0)},
		{"s-api-02", "pattern", "DI container pattern for services",
			"Services are instantiated once in a `container.ts` file and injected into route " +
				"builders. This avoids circular dependencies and makes testing trivial — just swap " +
				"the implementation in tests.\n\n[[pattern/dependency-injection]]",
			"demo-api", "project", "pattern/di-container", "hash-api-di-001", 2, sid(4), ts(75, 1), ts(45, 0)},
		{"s-api-03", "pattern", "Cursor-based pagination with (orderKey, id) tuples",
			"We encode the last row's `(updated_at, id)` as a base64 cursor. This is stable " +
				"under concurrent inserts unlike OFFSET pagination. The cursor is validated with " +
				"Zod before use.\n\nSee also: [[architecture/api-layers]]",
			"demo-api", "project", "pattern/cursor-pagination", "hash-api-cursor-001", 3, sid(5), ts(60, 0), ts(30, 0)},
		{"s-api-03", "bugfix", "Fixed N+1 query in observation list endpoint",
			"The original implementation fetched tags in a separate query per row. Replaced with " +
				"a single JOIN query that aggregates tags as a JSON array. Reduced latency from ~120ms " +
				"to ~8ms for pages of 50 rows.",
			"demo-api", "project", "", "", 1, sid(6), ts(60, 2), ts(60, 2)},
		{"s-api-04", "architecture", "Hexagonal architecture separation",
			"The service layer is now strictly separated from the transport layer. Services " +
				"accept typed DTOs and never reference HTTP concepts. Routes translate between " +
				"HTTP and domain types.\n\nThis makes it easy to add a CLI or WebSocket interface " +
				"later without touching business logic.",
			"demo-api", "project", "architecture/hexagonal", "", 1, sid(7), ts(45, 0), ts(45, 0)},
		{"s-api-04", "decision", "Use Zod for runtime validation at route boundaries",
			"Zod schemas serve as the single source of truth for request shapes. They generate " +
				"TypeScript types automatically, eliminating drift between docs and runtime behavior. " +
				"We chose Zod over io-ts because of better error messages and simpler syntax.",
			"demo-api", "project", "decision/validation-library", "", 2, sid(8), ts(45, 1), ts(30, 0)},
		{"s-api-05", "feature", "Rate limiting middleware with sliding window",
			"Added per-IP rate limiting using a sliding window algorithm backed by in-memory " +
				"storage. The window is 60 seconds, max 100 requests. Headers `X-RateLimit-*` " +
				"inform clients of their quota.",
			"demo-api", "project", "feature/rate-limiting", "", 1, sid(9), ts(30, 0), ts(30, 0)},
		{"s-api-05", "config", "Environment variable schema for the API service",
			"All configuration is loaded from env vars at startup. Zod validates the schema " +
				"and throws on missing required vars. The schema is documented in `.env.example`.\n\n" +
				"Key vars: `DATABASE_URL`, `JWT_SECRET`, `PORT`, `LOG_LEVEL`.",
			"demo-api", "project", "config/env-schema", "", 1, sid(10), ts(30, 1), ts(30, 1)},
		{"s-api-06", "discovery", "SQLite WAL mode doubles read throughput",
			"Enabling WAL (Write-Ahead Logging) mode with `PRAGMA journal_mode=WAL` allowed " +
				"concurrent readers while a write is in progress. Read throughput doubled in " +
				"our load tests (ab -n 1000 -c 50).",
			"demo-api", "project", "discovery/sqlite-wal", "hash-api-wal-001", 2, sid(11), ts(15, 0), ts(15, 0)},
		{"s-api-06", "pattern", "Prepared statement caching with namespace keys",
			"All SQL is cached in a map keyed by `'service.operation'` strings (e.g. `'obs.getById'`). " +
				"This prevents recompilation on hot paths and makes the SQL surface auditable.",
			"demo-api", "project", "pattern/stmt-caching", "hash-api-cursor-001", 1, sid(12), ts(15, 1), ts(15, 1)},
		{"s-api-07", "insight", "API design review findings",
			"After reviewing the full API surface:\n- Pagination is consistent across all list endpoints\n" +
				"- Error codes are uppercase and stable\n- All write endpoints require authentication\n" +
				"- FTS queries use sanitized input to prevent injection",
			"demo-api", "project", "", "", 1, sid(13), ts(3, 0), ts(3, 0)},

		// ── demo-api: duplicate group (normalized_hash = hash-api-arch-001) ──
		{"s-api-04", "architecture", "REST API layering: routes → services → adapters",
			"Revised version: routes handle HTTP + validation, services hold SQL and business logic, " +
				"adapters wrap the DB driver. DI via container.ts. [[decision/api-architecture]]",
			"demo-api", "project", "architecture/api-layers", "hash-api-arch-001", 2, sid(14), ts(45, 2), ts(45, 2)},
		{"s-api-06", "architecture", "REST API layering: routes → services → adapters",
			"Third revision: clarified that adapters also handle connection pooling configuration.",
			"demo-api", "project", "architecture/api-layers", "hash-api-arch-001", 1, sid(15), ts(15, 2), ts(15, 2)},

		// ── demo-web ────────────────────────────────────────────────────────────
		{"s-web-01", "architecture", "React component architecture: containers and presenters",
			"We follow the container/presenter pattern. Container components fetch data via " +
				"TanStack Query and pass it down. Presenter components are pure functions of props.\n\n" +
				"[[pattern/container-presenter]] [[decision/state-management]]",
			"demo-web", "project", "architecture/component-arch", "hash-web-arch-001", 2, sid(16), ts(88, 0), ts(55, 0)},
		{"s-web-01", "decision", "TanStack Router v1 for file-based routing",
			"Chose TanStack Router over React Router because of first-class TypeScript support, " +
				"built-in search param validation, and type-safe Link components. Routes are defined " +
				"code-based in `src/router.tsx`.",
			"demo-web", "project", "decision/router-choice", "", 1, sid(17), ts(88, 1), ts(88, 1)},
		{"s-web-02", "feature", "Virtualized observations table with TanStack Virtual v3",
			"The observations table renders only visible rows using `useVirtualizer`. Config:\n" +
				"- `estimateSize: 36`\n- `overscan: 12`\n- Infinite scroll triggers next page load " +
				"at 20 rows from the bottom\n\nThis keeps the DOM at ~50 nodes regardless of dataset size.",
			"demo-web", "project", "feature/virtual-table", "", 1, sid(18), ts(72, 0), ts(72, 0)},
		{"s-web-02", "pattern", "TanStack Table v8 column definition pattern",
			"Column definitions use `columnHelper.accessor` for type safety. Display columns " +
				"(actions, checkboxes) use `columnHelper.display`. All column IDs are stable " +
				"strings for URL persistence.",
			"demo-web", "project", "pattern/table-columns", "", 1, sid(19), ts(72, 1), ts(72, 1)},
		{"s-web-03", "architecture", "CVA + clsx + tailwind-merge for component variants",
			"Components use `cva` to define variant slots, `clsx` to merge conditional classes, " +
				"and `tailwind-merge` to deduplicate conflicting Tailwind utilities. No shadcn/ui — " +
				"all components are hand-rolled in `src/components/ui/`.",
			"demo-web", "project", "architecture/ui-components", "", 2, sid(20), ts(55, 0), ts(40, 0)},
		{"s-web-03", "pattern", "HSL token system for theming",
			"CSS custom properties use HSL channels without the `hsl()` wrapper:\n" +
				"```css\n--bg: 222 14% 8%;\nbackground: hsl(var(--bg));\n```\n" +
				"Available tokens: `--bg`, `--surface`, `--surface-2`, `--border`, `--fg`, " +
				"`--fg-muted`, `--accent`, `--ok`, `--warn`, `--fail`.",
			"demo-web", "project", "pattern/hsl-tokens", "hash-web-hsl-001", 2, sid(21), ts(55, 1), ts(25, 0)},
		{"s-web-04", "config", "Dark mode via class strategy",
			"Tailwind dark mode uses `darkMode: ['class']`. The root `<html>` element gets " +
				"`class='dark'` toggled by a client-side script that reads `localStorage.theme`. " +
				"No flash on load.",
			"demo-web", "project", "config/dark-mode", "", 1, sid(22), ts(40, 0), ts(40, 0)},
		{"s-web-05", "pattern", "Infinite scroll with TanStack Query v5",
			"The list uses `useInfiniteQuery` with cursor-based pagination. The `getNextPageParam` " +
				"function reads the `nextCursor` field from each page response. The virtualizer " +
				"triggers a new page fetch when the user scrolls within 20 rows of the bottom.",
			"demo-web", "project", "pattern/infinite-scroll", "", 1, sid(23), ts(25, 0), ts(25, 0)},
		{"s-web-05", "decision", "TanStack Query v5 as the only server state manager",
			"We do not use Zustand for server state. All remote data lives in TanStack Query's " +
				"cache. Zustand is used only for transient UI state (selected rows, filter panels).",
			"demo-web", "project", "decision/server-state", "hash-web-query-001", 2, sid(24), ts(25, 1), ts(10, 0)},
		{"s-web-06", "discovery", "ARIA live regions improve screen reader experience",
			"Adding `aria-live='polite'` to the status bar (row count, loading indicator) " +
				"caused screen readers to announce updates without interrupting the user. " +
				"Previously, users relying on screen readers had no feedback during data fetches.",
			"demo-web", "project", "", "", 1, sid(25), ts(10, 0), ts(10, 0)},
		{"s-web-07", "feature", "Dynamic imports for heavy visualization components",
			"The Brain 3D graph (`@react-three/fiber` + `three`) is ~450 KB gzipped. Wrapping it " +
				"in `next/dynamic` (or `React.lazy`) defers the load until the user navigates to " +
				"the graph view. Initial bundle dropped from 1.2 MB to 380 KB.",
			"demo-web", "project", "feature/lazy-loading", "", 1, sid(26), ts(2, 0), ts(2, 0)},
		{"s-web-07", "bugfix", "Fixed hydration mismatch in dark mode toggle",
			"The theme toggle read `localStorage` during SSR where it's undefined, causing a " +
				"hydration mismatch. Fixed by wrapping the initial read in `useEffect` and using " +
				"a CSS class on `<html>` set by an inline script to prevent flash.",
			"demo-web", "project", "", "", 1, sid(27), ts(2, 1), ts(2, 1)},

		// ── demo-web: duplicate group ────────────────────────────────────────────
		{"s-web-04", "architecture", "React component architecture: containers and presenters",
			"Updated: presenter components MUST be pure (no hooks that fetch data). " +
				"Only container components talk to TanStack Query.",
			"demo-web", "project", "architecture/component-arch", "hash-web-arch-001", 1, sid(28), ts(40, 2), ts(40, 2)},

		// ── research-notes ───────────────────────────────────────────────────────
		{"s-res-01", "learning", "Vector search fundamentals: ANN vs exact KNN",
			"Approximate Nearest Neighbor (ANN) algorithms (HNSW, IVF) trade recall for speed. " +
				"Exact k-NN is O(n·d) and only practical for small corpora. For semantic search over " +
				"~1M vectors, ANN with 0.95 recall@10 is the right tradeoff.\n\n" +
				"[[research/vector-search]] [[research/embeddings]]",
			"research-notes", "project", "research/vector-search-fundamentals", "", 1, sid(29), ts(85, 0), ts(85, 0)},
		{"s-res-01", "insight", "SQLite FTS5 is competitive for <1M documents",
			"FTS5 uses a BM25 ranking function by default and supports phrase queries, prefix " +
				"search, and column filtering. For corpora under ~1M documents, it outperforms " +
				"external search engines in latency due to zero network overhead.",
			"research-notes", "project", "research/fts5-analysis", "hash-res-fts-001", 3, sid(30), ts(85, 1), ts(20, 0)},
		{"s-res-02", "learning", "Embedding model comparison: sentence-transformers vs OpenAI",
			"sentence-transformers/all-MiniLM-L6-v2 (384-dim) runs locally in ~5ms/query. " +
				"OpenAI text-embedding-3-small (1536-dim) scores 3% higher on MTEB but costs " +
				"$0.02/1M tokens and adds network latency. For offline use, MiniLM is the clear choice.",
			"research-notes", "project", "research/embedding-models", "", 2, sid(31), ts(70, 0), ts(50, 0)},
		{"s-res-03", "architecture", "RAG pipeline design: retrieve-then-generate",
			"A minimal RAG pipeline:\n1. Embed the user query\n2. ANN search over the vector store\n" +
				"3. Rerank top-k with a cross-encoder\n4. Inject top results into the LLM context\n\n" +
				"Reranking is often skipped but improves answer quality by ~15% on our benchmarks.",
			"research-notes", "project", "research/rag-pipeline", "", 1, sid(32), ts(50, 0), ts(50, 0)},
		{"s-res-03", "discovery", "Hybrid search (BM25 + vector) beats pure vector search",
			"Combining BM25 keyword scores with dense vector scores via Reciprocal Rank Fusion (RRF) " +
				"consistently outperforms either approach alone on retrieval benchmarks. The intuition: " +
				"keyword search catches exact matches that embedding similarity misses.",
			"research-notes", "project", "research/hybrid-search", "", 1, sid(33), ts(50, 1), ts(50, 1)},
		{"s-res-04", "architecture", "Multi-agent orchestration: orchestrator + worker pattern",
			"The orchestrator maintains state and delegates concrete tasks to worker agents with " +
				"fresh context windows. Workers return structured results; the orchestrator synthesizes. " +
				"Key constraint: workers must not spawn further agents (no recursive delegation).",
			"research-notes", "project", "research/multi-agent", "", 1, sid(34), ts(35, 0), ts(35, 0)},
		{"s-res-04", "insight", "Context window management is the core multi-agent challenge",
			"The main failure mode in multi-agent systems is context overflow. Solutions:\n" +
				"1. Compress exploration results before handing off\n" +
				"2. Use structured artifacts (JSON/YAML) not prose\n" +
				"3. Sub-agents should save key findings to persistent memory before returning",
			"research-notes", "project", "", "", 1, sid(35), ts(35, 1), ts(35, 1)},
		{"s-res-05", "learning", "SQLite FTS5 tokenizers: unicode61 vs ascii",
			"FTS5 defaults to `unicode61` which handles diacritics and case folding for non-ASCII " +
				"characters. The `ascii` tokenizer is faster but only works correctly for ASCII input. " +
				"For a multilingual corpus, `unicode61` is always the right choice.",
			"research-notes", "project", "research/fts5-analysis", "hash-res-fts-001", 2, sid(36), ts(20, 0), ts(20, 0)},
		{"s-res-05", "pattern", "FTS query sanitization to prevent injection",
			"User input must be sanitized before passing to FTS5:\n" +
				"- Escape quotes: `' → ''`\n- Remove FTS operators: `AND OR NOT * ()`\n" +
				"- Wrap in phrase quotes for exact match\n\n" +
				"The `sanitizeFtsQuery()` helper handles all of this.",
			"research-notes", "project", "pattern/fts-sanitization", "", 1, sid(37), ts(20, 1), ts(20, 1)},
		{"s-res-06", "learning", "Cursor pagination: (column, id) tie-breaking",
			"A cursor based on a single non-unique column (e.g. `created_at`) breaks on ties. " +
				"The fix is a compound cursor `(created_at, id)` where `id` breaks ties deterministically. " +
				"The cursor is base64-encoded to prevent client manipulation.",
			"research-notes", "project", "research/cursor-pagination", "", 1, sid(38), ts(8, 0), ts(8, 0)},

		// ── infra ────────────────────────────────────────────────────────────────
		{"s-inf-01", "config", "Docker multi-stage build for Go binaries",
			"Stage 1 (builder): `FROM golang:1.23-alpine` — runs `go build` with CGO_ENABLED=0. " +
				"Stage 2 (runtime): `FROM scratch` — copies the binary. Result: 12 MB image vs 800 MB with a full Go image.",
			"infra", "project", "config/docker-build", "", 1, sid(39), ts(83, 0), ts(83, 0)},
		{"s-inf-01", "pattern", "Go binary embedding: frontend assets via go:embed",
			"The compiled React frontend is embedded into the Go binary at build time using " +
				"`//go:embed dist/**` in `internal/web/web.go`. This produces a single self-contained " +
				"binary with no external asset dependencies.",
			"infra", "project", "pattern/go-embed", "", 2, sid(40), ts(83, 1), ts(48, 0)},
		{"s-inf-02", "config", "GitHub Actions CI: test → build → release pipeline",
			"Jobs:\n1. `test`: runs `go test ./...` and `go vet ./...`\n2. `build`: compiles the " +
				"frontend, then `go build`\n3. `release`: goreleaser on tags\n\nAll jobs run on " +
				"`ubuntu-latest` with Go 1.23.",
			"infra", "project", "config/ci-pipeline", "", 1, sid(41), ts(65, 0), ts(65, 0)},
		{"s-inf-02", "decision", "Use goreleaser for cross-platform binary distribution",
			"goreleaser handles cross-compilation (linux/amd64, darwin/arm64, windows/amd64), " +
				"checksum generation, GitHub Release creation, and Homebrew tap updates in a single " +
				"`.goreleaser.yaml` config.",
			"infra", "project", "decision/goreleaser", "", 1, sid(42), ts(65, 1), ts(65, 1)},
		{"s-inf-03", "architecture", "Database migration strategy with goose",
			"Migrations live in `db/migrations/`. goose handles up/down migrations and tracks state " +
				"in a `goose_db_version` table. The CI pipeline runs migrations before integration tests.",
			"infra", "project", "architecture/migrations", "", 1, sid(43), ts(48, 0), ts(48, 0)},
		{"s-inf-04", "config", "Secrets management: env vars injected at runtime, never in images",
			"Secrets (`DATABASE_URL`, `JWT_SECRET`) are injected as environment variables at " +
				"container startup via Kubernetes secrets / Vault agent. They are never baked into " +
				"Docker images or committed to the repository.",
			"infra", "project", "config/secrets-management", "", 1, sid(44), ts(32, 0), ts(32, 0)},
		{"s-inf-04", "discovery", "CGO_ENABLED=0 is required for scratch-based Docker images",
			"The Go binary must be compiled with `CGO_ENABLED=0` to produce a fully static binary " +
				"that runs in a scratch container. With CGO enabled, the binary links against glibc " +
				"which is not present in scratch.",
			"infra", "project", "discovery/cgo-static", "", 1, sid(45), ts(32, 1), ts(32, 1)},
		{"s-inf-05", "feature", "Prometheus metrics: HTTP request duration histogram",
			"Added a `http_request_duration_seconds` histogram with labels `method`, `route`, " +
				"and `status_code`. Exposed at `/metrics`. Grafana dashboard provisioned via configmap.",
			"infra", "project", "feature/prometheus-metrics", "", 1, sid(46), ts(18, 0), ts(18, 0)},
		{"s-inf-05", "config", "Grafana dashboard for API latency and error rates",
			"Dashboard panels:\n- P50/P95/P99 latency per route\n- Error rate (5xx) time series\n" +
				"- Active connections gauge\n- DB query duration heatmap\n\nProvisioned via `grafana/dashboards/api.json`.",
			"infra", "project", "config/grafana-dashboard", "", 1, sid(47), ts(18, 1), ts(18, 1)},
		{"s-inf-06", "decision", "Zero-downtime deployment via Kubernetes rolling updates",
			"The deployment spec uses `maxUnavailable: 0, maxSurge: 1` to ensure at least one " +
				"pod is always serving traffic during an update. Combined with a readiness probe on " +
				"`/api/health`, this achieves zero-downtime deploys.",
			"infra", "project", "decision/deployment-strategy", "", 1, sid(48), ts(5, 0), ts(5, 0)},

		// ── orphan observations (project = "") ───────────────────────────────────
		{"s-orp-01", "insight", "Scratch observation: memory-mapped files vs mmap syscall",
			"Quick note: Go's `mmap` syscall requires CGO on some platforms. Use `golang.org/x/sys/unix` " +
				"for a pure-Go alternative.",
			"", "project", "", "", 1, sid(49), ts(80, 0), ts(80, 0)},
		{"s-orp-02", "discovery", "Discovered: SQLite VACUUM FULL reclaims space after bulk deletes",
			"After deleting 50% of rows in a large table, `VACUUM` reclaims the freed pages and " +
				"compacts the file. With WAL mode, `PRAGMA wal_checkpoint(FULL)` must be called first.",
			"", "project", "", "", 1, sid(50), ts(60, 0), ts(60, 0)},
		{"s-orp-03", "learning", "Prototype: using go:generate to embed SQL schema at compile time",
			"By running `go:generate go run ./cmd/gen-schema` we can embed the SQL schema file " +
				"into a Go constant. This avoids reading from disk at runtime and makes the binary " +
				"fully self-contained.",
			"", "project", "", "hash-orp-001", 1, sid(51), ts(40, 0), ts(40, 0)},

		// ── more demo-api observations (to reach ~180 total) ─────────────────────
		{"s-api-01", "pattern", "Error codes as uppercase constants",
			"Services throw `Error` with a `.code` property (uppercase, e.g. `NOT_FOUND`, " +
				"`ALREADY_DELETED`). Routes catch them and translate to structured JSON:\n" +
				"`{ error: { code, message, details? } }`",
			"demo-api", "project", "pattern/error-codes", "", 1, sid(52), ts(89, 2), ts(89, 2)},
		{"s-api-02", "learning", "Hono context: c.req.valid() vs c.req.json()",
			"`c.req.valid('json')` returns the Zod-validated and typed body. `c.req.json()` " +
				"returns the raw unvalidated body. Always use `c.req.valid()` in route handlers " +
				"after attaching the Zod validator middleware.",
			"demo-api", "project", "", "", 1, sid(53), ts(75, 2), ts(75, 2)},
		{"s-api-03", "discovery", "better-sqlite3 prepared statements are 40% faster than raw queries",
			"Benchmarking `stmt.run(params)` vs `db.prepare(sql).run(params)` on each call shows " +
				"~40% lower latency. Caching prepared statements in a module-level Map is essential " +
				"for hot paths.",
			"demo-api", "project", "discovery/prepared-stmts", "", 1, sid(54), ts(60, 3), ts(60, 3)},
		{"s-api-05", "pattern", "Request ID middleware for distributed tracing",
			"Every request gets a UUID v4 `X-Request-ID` header (if not provided by the client). " +
				"It's injected into the log context and returned in the response for tracing.",
			"demo-api", "project", "pattern/request-id", "", 1, sid(55), ts(30, 2), ts(30, 2)},
		{"s-api-06", "feature", "Query plan analysis endpoint for debugging",
			"Added a dev-only `/api/debug/explain?q=<sql>` endpoint that runs `EXPLAIN QUERY PLAN` " +
				"and returns the plan as JSON. Disabled in production via env flag.",
			"demo-api", "project", "", "", 1, sid(56), ts(15, 2), ts(15, 2)},

		// ── more demo-web observations ────────────────────────────────────────────
		{"s-web-02", "learning", "TanStack Virtual: scrollToIndex for programmatic scroll",
			"`virtualizer.scrollToIndex(index, { align: 'start' })` smoothly scrolls to a " +
				"specific row. Useful for 'jump to observation' features. The align option can be " +
				"`'start'`, `'center'`, `'end'`, or `'auto'`.",
			"demo-web", "project", "", "", 1, sid(57), ts(72, 2), ts(72, 2)},
		{"s-web-03", "bugfix", "Fixed: cva variant types not narrowing with clsx",
			"When using `cva` variants with `clsx`, TypeScript sometimes widens the type to " +
				"`string | undefined`. Fix: explicitly type the variant prop as the CVA `VariantProps` " +
				"utility type.",
			"demo-web", "project", "", "", 1, sid(58), ts(55, 2), ts(55, 2)},
		{"s-web-04", "pattern", "Storybook-free component development with co-located tests",
			"Instead of Storybook, we develop components with React Testing Library stories in " +
				"`*.test.tsx` files. Each test renders a visual variant and asserts on accessible queries.",
			"demo-web", "project", "", "", 1, sid(59), ts(40, 2), ts(40, 2)},
		{"s-web-05", "discovery", "noUncheckedIndexedAccess catches 80% of runtime array bugs",
			"After enabling `noUncheckedIndexedAccess` in tsconfig, TypeScript flagged 23 places " +
				"where we accessed array elements without null-checking. All were potential runtime crashes.",
			"demo-web", "project", "", "", 1, sid(60), ts(25, 2), ts(25, 2)},

		// ── more research-notes ───────────────────────────────────────────────────
		{"s-res-02", "insight", "Matryoshka embeddings allow dimension truncation",
			"Matryoshka Representation Learning (MRL) trains embeddings so the first N dimensions " +
				"preserve most of the semantic information. You can truncate `text-embedding-3-small` " +
				"from 1536 to 256 dimensions with only ~3% recall loss.",
			"research-notes", "project", "research/embeddings", "", 1, sid(61), ts(70, 1), ts(70, 1)},
		{"s-res-04", "learning", "Tool use in LLMs: structured outputs vs JSON mode",
			"Tool use (function calling) forces the model to output valid JSON matching a schema. " +
				"JSON mode only guarantees valid JSON — the schema is not enforced. For tool-calling " +
				"agents, use tool use, not JSON mode.",
			"research-notes", "project", "", "", 1, sid(62), ts(35, 2), ts(35, 2)},

		// ── more infra ────────────────────────────────────────────────────────────
		{"s-inf-03", "pattern", "Migration naming convention: timestamp prefix",
			"Migrations are named `YYYYMMDDHHMMSS_description.sql`. This ensures lexicographic " +
				"ordering matches chronological order, preventing conflicts in parallel branches.",
			"infra", "project", "pattern/migration-naming", "", 1, sid(63), ts(48, 1), ts(48, 1)},
		{"s-inf-05", "discovery", "Prometheus histograms vs summaries: histograms are better",
			"Summaries compute quantiles client-side and cannot be aggregated across instances. " +
				"Histograms compute quantiles server-side in Prometheus and aggregate correctly. " +
				"Always use histograms for SLA metrics.",
			"infra", "project", "", "", 1, sid(64), ts(18, 2), ts(18, 2)},

		// ── cross-project preference observations ─────────────────────────────────
		{"s-api-01", "preference", "Always use UTC for stored timestamps",
			"All timestamps are stored as UTC in `YYYY-MM-DD HH:MM:SS` format. Conversion to " +
				"local time happens only at the presentation layer. This avoids DST ambiguities " +
				"and makes DB queries timezone-agnostic.",
			"demo-api", "personal", "preference/timestamps", "hash-pref-ts-001", 3, sid(65), ts(89, 3), ts(30, 3)},
		{"s-web-01", "preference", "Always use UTC for stored timestamps",
			"Duplicate captured from demo-web session — same convention applies everywhere.",
			"demo-web", "personal", "preference/timestamps", "hash-pref-ts-001", 1, sid(66), ts(88, 3), ts(88, 3)},
		{"s-res-01", "preference", "Always use UTC for stored timestamps",
			"Third capture of the same preference across projects.",
			"research-notes", "personal", "preference/timestamps", "hash-pref-ts-001", 1, sid(67), ts(85, 3), ts(85, 3)},
	}

	obsStmt, err := tx.Prepare(`
		INSERT INTO observations
			(session_id, type, title, content, project, scope, topic_key,
			 normalized_hash, revision_count, sync_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return rowCounts{}, fmt.Errorf("prepare observations: %w", err)
	}
	defer obsStmt.Close()

	// obsIDMap maps sync_id → auto-assigned integer id (needed for relations).
	obsIDMap := make(map[string]int64, len(observations)+300)

	// allObsSyncIDs collects every inserted sync_id for the generated relations.
	var allObsSyncIDs []string

	for _, o := range observations {
		var nHash interface{} = nil
		if o.normalizedHash != "" {
			nHash = o.normalizedHash
		}
		var topicKey interface{} = nil
		if o.topicKey != "" {
			topicKey = o.topicKey
		}
		var project interface{} = nil
		if o.project != "" {
			project = o.project
		}
		result, err := obsStmt.Exec(
			o.sessionID, o.obsType, o.title, o.content, project, o.scope,
			topicKey, nHash, o.revisionCount, o.syncID, o.createdAt, o.updatedAt,
		)
		if err != nil {
			return rowCounts{}, fmt.Errorf("insert observation %q: %w", o.title, err)
		}
		id, _ := result.LastInsertId()
		obsIDMap[o.syncID] = id
		allObsSyncIDs = append(allObsSyncIDs, o.syncID)
		c.observations++
	}

	// Insert generated observations.
	for _, o := range generatedObservations() {
		var nHash interface{} = nil
		if o.normalizedHash != "" {
			nHash = o.normalizedHash
		}
		var topicKey interface{} = nil
		if o.topicKey != "" {
			topicKey = o.topicKey
		}
		var project interface{} = nil
		if o.project != "" {
			project = o.project
		}
		result, err := obsStmt.Exec(
			o.sessionID, o.obsType, o.title, o.content, project, o.scope,
			topicKey, nHash, o.revisionCount, o.syncID, o.createdAt, o.updatedAt,
		)
		if err != nil {
			return rowCounts{}, fmt.Errorf("insert generated observation %q: %w", o.title, err)
		}
		id, _ := result.LastInsertId()
		obsIDMap[o.syncID] = id
		allObsSyncIDs = append(allObsSyncIDs, o.syncID)
		c.observations++
	}

	// ── User prompts ──────────────────────────────────────────────────────────
	// The prompt_fts_insert trigger auto-populates prompts_fts.
	type prompt struct {
		sessionID string
		content   string
		project   string
		createdAt string
	}

	prompts := []prompt{
		{"s-api-01", "How should I structure the route handlers in a Hono API?", "demo-api", ts(89, 0)},
		{"s-api-01", "What's the best pattern for dependency injection in TypeScript without a framework?", "demo-api", ts(89, 1)},
		{"s-api-02", "Implement JWT authentication middleware for Hono that validates access tokens", "demo-api", ts(75, 0)},
		{"s-api-02", "How do I handle refresh token rotation securely?", "demo-api", ts(75, 1)},
		{"s-api-03", "Implement cursor-based pagination for the observations list endpoint", "demo-api", ts(60, 0)},
		{"s-api-03", "Why is offset-based pagination problematic for real-time data?", "demo-api", ts(60, 1)},
		{"s-api-04", "Refactor the service layer to separate SQL from business logic", "demo-api", ts(45, 0)},
		{"s-api-05", "Add rate limiting to the API — 100 requests per minute per IP", "demo-api", ts(30, 0)},
		{"s-api-06", "Profile the observation list endpoint and suggest optimizations", "demo-api", ts(15, 0)},
		{"s-api-07", "Review the API documentation for completeness and accuracy", "demo-api", ts(3, 0)},

		{"s-web-01", "Set up TanStack Router with code-based route definitions", "demo-web", ts(88, 0)},
		{"s-web-02", "Implement a virtualized table for the observations list using TanStack Virtual", "demo-web", ts(72, 0)},
		{"s-web-03", "Create a Button component using CVA with size and variant props", "demo-web", ts(55, 0)},
		{"s-web-04", "Implement dark mode with no flash on load", "demo-web", ts(40, 0)},
		{"s-web-05", "Set up infinite scroll for the observations table with TanStack Query", "demo-web", ts(25, 0)},
		{"s-web-06", "Audit the dashboard for WCAG 2.1 AA compliance", "demo-web", ts(10, 0)},
		{"s-web-07", "Analyze bundle size and suggest what to lazy-load", "demo-web", ts(2, 0)},

		{"s-res-01", "Summarize the tradeoffs between ANN and exact KNN for semantic search", "research-notes", ts(85, 0)},
		{"s-res-02", "Compare sentence-transformers and OpenAI embeddings for offline use", "research-notes", ts(70, 0)},
		{"s-res-03", "Explain hybrid search with BM25 + dense vectors and Reciprocal Rank Fusion", "research-notes", ts(50, 0)},
		{"s-res-04", "Design a multi-agent orchestration system that avoids context overflow", "research-notes", ts(35, 0)},
		{"s-res-05", "What FTS5 tokenizer should I use for a multilingual document corpus?", "research-notes", ts(20, 0)},

		{"s-inf-01", "Write a Dockerfile for a Go binary using multi-stage builds and scratch base", "infra", ts(83, 0)},
		{"s-inf-02", "Set up a GitHub Actions workflow with test, build, and release jobs", "infra", ts(65, 0)},
		{"s-inf-03", "Configure goose for database migrations in a CI/CD pipeline", "infra", ts(48, 0)},
		{"s-inf-04", "How do I inject secrets as environment variables in Kubernetes without baking them into images?", "infra", ts(32, 0)},
		{"s-inf-05", "Set up Prometheus metrics with a latency histogram in Go", "infra", ts(18, 0)},
		{"s-inf-06", "Configure a Kubernetes deployment for zero-downtime rolling updates", "infra", ts(5, 0)},

		{"s-orp-01", "Quick question about mmap in Go without CGO", "", ts(80, 0)},
		{"s-orp-03", "How can I embed a SQL schema file into a Go binary at compile time?", "", ts(40, 0)},
	}

	promptStmt, err := tx.Prepare(`
		INSERT INTO user_prompts (session_id, content, project, created_at)
		VALUES (?, ?, ?, ?)`)
	if err != nil {
		return rowCounts{}, fmt.Errorf("prepare prompts: %w", err)
	}
	defer promptStmt.Close()

	for _, p := range prompts {
		var project interface{} = nil
		if p.project != "" {
			project = p.project
		}
		if _, err := promptStmt.Exec(p.sessionID, p.content, project, p.createdAt); err != nil {
			return rowCounts{}, fmt.Errorf("insert prompt: %w", err)
		}
		c.prompts++
	}

	// ── Memory relations ──────────────────────────────────────────────────────
	// Relations need matching sync_ids in observations for the JOIN in graph.go.
	// We pick pairs from the observations we inserted.
	type relation struct {
		syncID     string
		sourceID   string
		targetID   string
		relation   string
		reason     string
		confidence float64
		sessionID  string
		createdAt  string
	}

	relations := []relation{
		// architecture/api-layers ↔ pattern/di-container (related)
		{"mr-001", sid(1), sid(4), "related", "DI container is a direct consequence of the layered architecture", 0.92, "s-api-02", ts(75, 0)},
		// architecture/api-layers ↔ pattern/cursor-pagination (related)
		{"mr-002", sid(1), sid(5), "related", "Cursor pagination is implemented in the service layer", 0.85, "s-api-03", ts(60, 0)},
		// pattern/di-container ↔ decision/validation-library (scoped)
		{"mr-003", sid(4), sid(8), "scoped", "Validation happens at the route boundary injected via DI", 0.78, "s-api-04", ts(45, 0)},
		// architecture/api-layers ↔ architecture/hexagonal (related)
		{"mr-004", sid(1), sid(7), "related", "Hexagonal arch is the evolution of the layered approach", 0.95, "s-api-04", ts(45, 1)},
		// decision/framework-choice ↔ architecture/auth-flow (scoped)
		{"mr-005", sid(2), sid(3), "scoped", "Auth flow is shaped by Hono middleware API", 0.80, "s-api-02", ts(75, 1)},
		// pattern/cursor-pagination ↔ research/cursor-pagination (compatible)
		{"mr-006", sid(5), sid(38), "compatible", "Research finding validates the implementation approach", 0.88, "s-res-06", ts(8, 0)},
		// discovery/sqlite-wal ↔ discovery/prepared-stmts (related)
		{"mr-007", sid(11), sid(54), "related", "Both are SQLite performance optimizations", 0.75, "s-api-06", ts(15, 1)},
		// architecture/component-arch ↔ decision/server-state (scoped)
		{"mr-008", sid(16), sid(24), "scoped", "Server state management shapes the container/presenter split", 0.90, "s-web-05", ts(25, 0)},
		// pattern/hsl-tokens ↔ config/dark-mode (scoped)
		{"mr-009", sid(21), sid(22), "scoped", "Dark mode is implemented using the HSL token system", 0.95, "s-web-04", ts(40, 0)},
		// feature/virtual-table ↔ pattern/infinite-scroll (scoped)
		{"mr-010", sid(18), sid(23), "scoped", "Infinite scroll is the data-loading strategy for the virtual table", 0.92, "s-web-05", ts(25, 1)},
		// research/fts5-analysis ↔ pattern/fts-sanitization (scoped)
		{"mr-011", sid(30), sid(37), "scoped", "Sanitization is a requirement identified in the FTS research", 0.85, "s-res-05", ts(20, 0)},
		// research/vector-search ↔ research/hybrid-search (related)
		{"mr-012", sid(29), sid(33), "related", "Hybrid search extends pure vector search", 0.90, "s-res-03", ts(50, 0)},
		// research/embedding-models ↔ research/embeddings (related)
		{"mr-013", sid(31), sid(61), "related", "Matryoshka embeddings are relevant to model selection", 0.82, "s-res-02", ts(70, 0)},
		// research/multi-agent ↔ architecture/component-arch (compatible)
		{"mr-014", sid(34), sid(16), "compatible", "Container/presenter mirrors orchestrator/worker separation of concerns", 0.72, "s-res-04", ts(35, 0)},
		// config/docker-build ↔ discovery/cgo-static (scoped)
		{"mr-015", sid(39), sid(45), "scoped", "CGO constraint is a prerequisite for the scratch Docker build", 0.98, "s-inf-04", ts(32, 0)},
		// pattern/go-embed ↔ config/docker-build (related)
		{"mr-016", sid(40), sid(39), "related", "go:embed is used to bundle assets into the Docker image", 0.88, "s-inf-01", ts(83, 0)},
		// config/ci-pipeline ↔ decision/goreleaser (scoped)
		{"mr-017", sid(41), sid(42), "scoped", "goreleaser is the release step in the CI pipeline", 0.94, "s-inf-02", ts(65, 0)},
		// architecture/migrations ↔ pattern/migration-naming (scoped)
		{"mr-018", sid(43), sid(63), "scoped", "Naming convention is part of the migration architecture", 0.90, "s-inf-03", ts(48, 0)},
		// feature/prometheus-metrics ↔ discovery (related)
		{"mr-019", sid(46), sid(64), "related", "Histogram vs summary choice informs the metrics feature", 0.85, "s-inf-05", ts(18, 0)},
		// preference/timestamps ↔ config/env-schema (compatible)
		{"mr-020", sid(65), sid(10), "compatible", "UTC timestamp preference aligns with env schema documentation", 0.70, "s-api-01", ts(89, 3)},
	}

	relStmt, err := tx.Prepare(`
		INSERT INTO memory_relations
			(sync_id, source_id, target_id, relation, reason, confidence,
			 judgment_status, session_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, 'judged', ?, ?, ?)`)
	if err != nil {
		return rowCounts{}, fmt.Errorf("prepare relations: %w", err)
	}
	defer relStmt.Close()

	for _, r := range relations {
		if _, err := relStmt.Exec(
			r.syncID, r.sourceID, r.targetID, r.relation, r.reason,
			r.confidence, r.sessionID, r.createdAt, r.createdAt,
		); err != nil {
			return rowCounts{}, fmt.Errorf("insert relation %s: %w", r.syncID, err)
		}
		c.relations++
	}

	// Insert generated relations (uses allObsSyncIDs collected above).
	for _, r := range generatedRelations(allObsSyncIDs) {
		if _, err := relStmt.Exec(
			r.syncID, r.sourceID, r.targetID, r.relation, r.reason,
			r.confidence, r.sessionID, r.createdAt, r.createdAt,
		); err != nil {
			return rowCounts{}, fmt.Errorf("insert generated relation %s: %w", r.syncID, err)
		}
		c.relations++
	}

	// ── Sync data ──────────────────────────────────────────────────────────────
	// Enroll 3 projects; leave research-notes and "" unenrolled to trigger issues.
	enrolled := []struct {
		project    string
		enrolledAt string
	}{
		{"demo-api", ts(89, 0)},
		{"demo-web", ts(88, 0)},
		{"infra", ts(83, 0)},
	}

	enrollStmt, err := tx.Prepare(`INSERT INTO sync_enrolled_projects (project, enrolled_at) VALUES (?, ?)`)
	if err != nil {
		return rowCounts{}, fmt.Errorf("prepare enroll: %w", err)
	}
	defer enrollStmt.Close()

	for _, e := range enrolled {
		if _, err := enrollStmt.Exec(e.project, e.enrolledAt); err != nil {
			return rowCounts{}, fmt.Errorf("insert enrolled %s: %w", e.project, err)
		}
		c.syncEnrolled++
	}

	// Sync state: demo-api is healthy, demo-web is healthy, infra is broken
	// (lifecycle=pending, 5 mutations enqueued, 0 acked, stale updated_at).
	syncStates := []struct {
		targetKey           string
		lifecycle           string
		lastEnqueuedSeq     int
		lastAckedSeq        int
		consecutiveFailures int
		lastError           *string
		updatedAt           string
	}{
		{"cloud:demo-api", "idle", 45, 45, 0, nil, ts(3, 0)},
		{"cloud:demo-web", "idle", 32, 32, 0, nil, ts(2, 0)},
		{"cloud:infra", "pending", 12, 0, 0, nil, ts(60, 0)}, // stale + 0 acked → broken
	}

	stateStmt, err := tx.Prepare(`
		INSERT INTO sync_state
			(target_key, lifecycle, last_enqueued_seq, last_acked_seq,
			 consecutive_failures, last_error, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return rowCounts{}, fmt.Errorf("prepare sync_state: %w", err)
	}
	defer stateStmt.Close()

	for _, s := range syncStates {
		if _, err := stateStmt.Exec(
			s.targetKey, s.lifecycle, s.lastEnqueuedSeq, s.lastAckedSeq,
			s.consecutiveFailures, s.lastError, s.updatedAt,
		); err != nil {
			return rowCounts{}, fmt.Errorf("insert sync_state %s: %w", s.targetKey, err)
		}
	}

	// Sync mutations: some acked (healthy), some pending (infra backlog).
	type mut struct {
		targetKey  string
		entity     string
		entityKey  string
		op         string
		payload    string
		project    string
		occurredAt string
		ackedAt    *string
	}

	acked := ts(3, 0)
	mutations := []mut{
		// demo-api: all acked
		{"cloud:demo-api", "observation", sid(1), "upsert", `{"content":"arch note","title":"REST API layering"}`, "demo-api", ts(89, 0), &acked},
		{"cloud:demo-api", "observation", sid(2), "upsert", `{"content":"decision note","title":"Chose Hono v4"}`, "demo-api", ts(89, 1), &acked},
		{"cloud:demo-api", "observation", sid(3), "upsert", `{"content":"auth note","title":"JWT auth flow"}`, "demo-api", ts(75, 0), &acked},
		{"cloud:demo-api", "observation", sid(5), "upsert", `{"content":"pagination note","title":"Cursor pagination"}`, "demo-api", ts(60, 0), &acked},
		{"cloud:demo-api", "observation", sid(6), "upsert", `{"content":"bugfix note","title":"Fixed N+1 query"}`, "demo-api", ts(60, 2), &acked},
		// demo-web: all acked
		{"cloud:demo-web", "observation", sid(16), "upsert", `{"content":"arch note","title":"React component arch"}`, "demo-web", ts(88, 0), &acked},
		{"cloud:demo-web", "observation", sid(18), "upsert", `{"content":"feature note","title":"Virtualized table"}`, "demo-web", ts(72, 0), &acked},
		// infra: pending (not acked) — these surface as SYNC_BROKEN issue
		{"cloud:infra", "observation", sid(39), "upsert", `{"content":"config note","title":"Docker multi-stage build"}`, "infra", ts(83, 0), nil},
		{"cloud:infra", "observation", sid(40), "upsert", `{"content":"pattern note","title":"go:embed frontend assets"}`, "infra", ts(83, 1), nil},
		{"cloud:infra", "observation", sid(41), "upsert", `{"content":"ci note","title":"GitHub Actions CI"}`, "infra", ts(65, 0), nil},
		{"cloud:infra", "observation", sid(43), "upsert", `{"content":"migration note","title":"goose migrations"}`, "infra", ts(48, 0), nil},
		{"cloud:infra", "observation", sid(46), "upsert", `{"content":"metrics note","title":"Prometheus metrics"}`, "infra", ts(18, 0), nil},
	}

	mutStmt, err := tx.Prepare(`
		INSERT INTO sync_mutations
			(target_key, entity, entity_key, op, payload, project, occurred_at, acked_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return rowCounts{}, fmt.Errorf("prepare sync_mutations: %w", err)
	}
	defer mutStmt.Close()

	for _, m := range mutations {
		if _, err := mutStmt.Exec(
			m.targetKey, m.entity, m.entityKey, m.op, m.payload,
			m.project, m.occurredAt, m.ackedAt,
		); err != nil {
			return rowCounts{}, fmt.Errorf("insert mutation: %w", err)
		}
		c.syncMutations++
	}

	_ = obsIDMap // used for reference; relations use sync_id strings directly

	return c, nil
}
