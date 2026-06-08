# Engram UI — Architecture & Design

> **Version:** 2.0
> **Status:** Current implementation
> **Integration strategy:** Option D — direct SQLite read-only + subprocess to the official `engram` CLI + daemon HTTP telemetry. (Options A/B/C were evaluated and discarded; see §2.4.)

---

## Table of contents

1. [Executive summary](#1-executive-summary)
2. [Context, constraints, and strategy decision](#2-context-constraints-and-strategy-decision)
3. [Database schema and illustrative data characteristics](#3-database-schema-and-illustrative-data-characteristics)
4. [Data model (quick reference)](#4-data-model-quick-reference)
5. [Architecture](#5-architecture)
6. [Technology stack and decisions](#6-technology-stack-and-decisions)
7. [Dashboard views (detailed catalog)](#7-dashboard-views-detailed-catalog)
8. [HTTP API (endpoint catalog)](#8-http-api-endpoint-catalog)
9. [Issue detection rules](#9-issue-detection-rules)
10. [Risks and mitigations](#10-risks-and-mitigations)
11. [Code conventions and quality](#11-code-conventions-and-quality)
12. [Appendices](#12-appendices)

---

## 1. Executive summary

**Engram UI** is a local web dashboard that reads the Engram database (`~/.engram/engram.db`) and the daemon sync state to provide:

- **Deep exploration** of observations, sessions, and user prompts (with filters, full-text search, virtualization).
- **Temporal visualization** of work (timeline per project, type distribution, session activity).
- **Cloud sync diagnostics** (enrolled vs. non-enrolled projects, pending mutations, errors, leases).
- **Controlled cloud governance** (enroll/unenroll projects, executed via the official CLI — never by direct SQL writes to sync tables).

**What it is not:** a replacement for the official cloud, nor a fork of the Engram binary. It is a **complementary local diagnostic and exploration tool** — a visibility layer over a system that today only exposes a TUI, CLI, and minimal HTTP endpoints.

**Key design goal:** surface inconsistencies that the official TUI/CLI do not show visually — examples include projects with memory that are not enrolled in cloud sync, projects with silently broken sync (mutations enqueued but never acknowledged), and observations captured with an empty project key.

---

## 2. Context, constraints, and strategy decision

### 2.1 What Engram is

`engram` is a persistent memory system for AI agents. It stores classified observations (`architecture`, `bugfix`, `decision`, `discovery`, …), sessions, and user prompts, and optionally syncs them to a central cloud instance.

**Active processes:**

| Process | Command | Role |
|---------|---------|------|
| Local HTTP daemon | `engram serve` (port 7437) | Health, stats, sync/status, writing observations via POST |
| Per-agent MCP server | `engram mcp --tools=…` (stdio) | Launched by each AI agent when a session starts |
| Administrative CLI | `engram cloud enroll <project>`, `engram tui`, `engram stats`, etc. | Direct human operation |
| Local DB | `~/.engram/engram.db` + WAL | Canonical store |

### 2.2 Hard constraints (design)

| Constraint | Source | Implication |
|------------|--------|-------------|
| The daemon holds the DB open with WAL active | Running `engram` process, WAL ~several MB | Concurrent reads are fine; writes require `busy_timeout` and are avoided unless justified |
| The official binary is the only stable surface | Marketplace plugin may lag behind the daemon version | Do NOT fork, do NOT recompile, do NOT depend on unexposed internals |
| The official HTTP API is minimal | Direct inspection of available endpoints | Only exposes `/health`, `/stats`, `/sync/status`, `POST /observations` — not suitable for rich listings |
| MCP is pure stdio | `cmd/engram/main.go` confirms `ServeStdio` | Not consumable from a web dashboard without a stdio↔HTTP proxy (discarded) |
| `cloud.json` may have no token | `auth status: token not configured` | UI must degrade cleanly: local read-only mode works without a token |
| The binary may be updated | Updates are released periodically | Schema and CLI must be treated as versioned contracts; smoke-test at boot |

### 2.3 What this project is NOT

- NOT a competitor to the official cloud.
- Does NOT write to **data** tables (`observations`, `sessions`, `user_prompts`).
- Does NOT implement sync logic, retry, queuing, or conflict resolution — that is the binary's responsibility.
- Does NOT require cloud connectivity to operate; all reads are local.
- Does NOT generate memories in Engram (the dashboard does not call `mem_save` on its own behalf).

### 2.4 Strategy decision: why D and not A/B/C

| Option | Reading | Cloud mutations | Maintenance cost | Verdict |
|--------|---------|-----------------|------------------|---------|
| A — Read-only | SQLite read-only | Manual CLI command | Minimal | Functional but poor UX |
| B — Read + controlled write | SQLite read-only | `INSERT/DELETE` directly on `sync_enrolled_projects` | Medium (lock risk) | Couples with internal schema |
| C — Via official MCP | MCP does not expose listings | Tools do not exist (would require adding them) | **High**: requires forking the binary, recompiling Go, maintaining a fork in sync, stdio↔HTTP proxy | **Not viable** |
| **D — SQLite read + CLI subprocess** | **SQLite read-only direct (FTS5 included)** | **`exec("engram cloud enroll <p>")`** | **Minimal** | **Chosen** |

**Justification for D:**

- **SQLite read-only direct**: the DB is the richest surface available (native FTS5 via `observations_fts` and `prompts_fts`, all columns, indexes already created on `project`, `type`, `created_at`, `topic_key`). WAL ensures consistency for concurrent readers.
- **CLI subprocess**: the official CLI (`engram cloud enroll`) is the human API of the system — zero fork, zero binary changes.
- **HTTP telemetry**: `GET /health` and `GET /sync/status` complement reads to detect a downed daemon and live sync errors.

**Known risk**: `engram cloud unenroll` is not present in all published versions. Mitigation: the backend performs a direct `DELETE` from `sync_enrolled_projects` via the write pool. See §10.

---

## 3. Database schema and illustrative data characteristics

> This section describes the shape and characteristics of the data the dashboard is designed to handle. Numeric examples below are illustrative.

### 3.1 Typical data profile

A typical Engram database may contain:

- Dozens of **sessions** spread across multiple projects.
- Hundreds to thousands of **observations** across many distinct projects.
- A handful of **user prompts** captured during AI sessions.
- Multiple **observation types**: `architecture`, `bugfix`, `decision`, `discovery`, `session_summary`, `config`, `ui-fix`, `pattern`, `preference`, `insight`, `project`, `learning`, `manual`, `feature`, `command`, and others.

### 3.2 Sync state patterns the dashboard surfaces

The dashboard is designed to detect and surface the following classes of issues automatically:

- **Not enrolled**: projects with observations that do not appear in `sync_enrolled_projects`.
- **Silently broken sync**: projects where `lifecycle = 'pending'`, `last_acked_seq = 0`, and `last_enqueued_seq > 0` — many mutations enqueued with zero acknowledgement. The daemon's `last_error` may be empty, making this invisible without the dashboard.
- **Orphan enrolled projects**: projects in `sync_enrolled_projects` that have no observations (e.g. a false-positive enroll from running a command outside a project directory).
- **Empty project key**: observations or sessions where `project = ''` — a capture bug in the agent or integration.
- **Pending global mutation queue**: a large backlog of `observation upsert` or `session upsert` mutations waiting for acknowledgement.

### 3.3 Sync diagnostics refinement

The "sync broken" detection rule uses a **5-minute grace period** from `sync_state.updated_at`. Projects in the natural initial sync state (newly enrolled, first daemon cycle not yet complete) are shown as `idle` within that window and only promoted to `broken` outside it. Explicit errors (`last_error` or `reason_code` populated) are surfaced as `degraded` regardless of sequence numbers.

---

## 4. Data model (quick reference)

### 4.1 Core tables

```
sessions
├── id (TEXT PK)
├── project (TEXT)
├── directory (TEXT)
├── started_at, ended_at, summary

observations  ← core of the system
├── id (INTEGER PK)
├── session_id (FK sessions)
├── type (architecture | bugfix | decision | discovery | session_summary | …)
├── title, content, tool_name
├── project, scope (project|personal), topic_key
├── normalized_hash, revision_count, duplicate_count   ← dedup
├── last_seen_at, created_at, updated_at, deleted_at   ← soft delete
└── sync_id

user_prompts
├── id (INTEGER PK)
├── session_id (FK sessions)
├── content, project, created_at, sync_id

observations_fts  ← VIRTUAL FTS5 over observations
prompts_fts       ← VIRTUAL FTS5 over user_prompts
```

### 4.2 Sync tables (cloud)

```
sync_enrolled_projects   ← whitelist: if project is here, it is uploaded to the cloud
├── project (PK)
└── enrolled_at

sync_state               ← one row per target_key (cloud, cloud:<project>)
├── target_key (PK)
├── lifecycle (idle | pending | healthy | failed)
├── last_enqueued_seq, last_acked_seq, last_pulled_seq
├── consecutive_failures, backoff_until
├── lease_owner, lease_until
└── last_error, reason_code, reason_message

sync_mutations           ← append-only mutation log
├── seq (PK autoinc)
├── target_key (FK sync_state)
├── entity (observation | session | prompt)
├── entity_key, op (upsert|delete), payload (JSON), source (local|remote)
├── occurred_at, acked_at
└── project

sync_chunks              ← already-imported chunks (idempotency)
cloud_upgrade_state      ← per-project migrations (stage, repair_class, findings_json)
prompt_tombstones        ← soft-deleted prompt sync markers
```

### 4.3 Relevant indexes (pre-existing — do not add)

- `idx_obs_topic` on `(topic_key, project, scope, updated_at DESC)` → topic upsert
- `idx_obs_dedupe` on `(normalized_hash, project, scope, type, title, created_at DESC)` → fast dedup
- `idx_obs_created`, `idx_obs_project`, `idx_obs_type`, `idx_obs_scope`, `idx_obs_deleted`
- `idx_sync_mutations_pending` on `(target_key, acked_at, seq)` → pending queue

---

## 5. Architecture

### 5.1 Logical diagram

```
                  ┌──────────────────────────────┐
                  │   Browser (React SPA)        │
                  │   Vite + Tailwind +          │
                  │   shadcn/ui + TanStack       │
                  └──────────────┬───────────────┘
                                 │ HTTP/JSON (REST)
                                 ▼
                  ┌──────────────────────────────┐
                  │   Go single binary           │
                  │   ┌────────────────────────┐ │
                  │   │ Read layer             │ │
                  │   │ modernc.org/sqlite RO  │─┼──► ~/.engram/engram.db
                  │   │ (incl. FTS5 queries)   │ │     (read-only pool)
                  │   └────────────────────────┘ │
                  │   ┌────────────────────────┐ │
                  │   │ Cloud mutation layer   │ │
                  │   │ os/exec                │─┼──► engram cloud enroll <p>
                  │   └────────────────────────┘ │
                  │   ┌────────────────────────┐ │
                  │   │ Telemetry layer        │ │
                  │   │ net/http :7437/health, │─┼──► engram serve daemon
                  │   │ /stats, /sync/status   │ │
                  │   └────────────────────────┘ │
                  │   ┌────────────────────────┐ │
                  │   │ SPA embed              │ │
                  │   │ go:embed dist/         │─┼──► React build (build-time)
                  │   └────────────────────────┘ │
                  └──────────────────────────────┘
```

### 5.2 Application layers (backend)

| Layer | Responsibility | Packages |
|-------|----------------|----------|
| **HTTP API** | Input validation, serialization, status codes | `internal/httpapi` (Go stdlib `net/http`) |
| **Services** | Business logic (filters, aggregations, diagnostics) | `internal/services` |
| **SQLite adapter** | DB access — read-only pool + read-write pool | `internal/sqlite` (`modernc.org/sqlite`, no CGo) |
| **Daemon adapter** | HTTP client to `:7437` with short timeout and fallback | `internal/daemon` |
| **CLI adapter** | `os/exec` subprocess to `engram` binary | `internal/cloud` |
| **Config** | Path resolution (`ENGRAM_DATA_DIR`, port, etc.) | `internal/config` |
| **Web embed** | Serve the React SPA from the binary | `internal/web` (`go:embed`) |
| **Cursor / FTS** | Pagination cursor encoding; FTS5 query sanitization | `internal/cursor`, `internal/fts` |
| **Logging** | Structured log output | `internal/logging` (`log/slog`) |

### 5.3 Repository structure

```
engram-explorer/
├── cmd/
│   ├── engram-explorer/        ← binary entry point
│   │   └── main.go
│   └── seed-demo/        ← demo DB seeder
│       └── main.go
├── internal/
│   ├── config/           ← env + path resolution
│   ├── sqlite/           ← read-only + read-write pools (modernc.org/sqlite)
│   ├── services/         ← observations, sessions, sync diagnostics, cloud control
│   ├── httpapi/          ← route handlers (Go stdlib net/http)
│   ├── daemon/           ← HTTP client to engram serve
│   ├── cloud/            ← subprocess adapter (os/exec)
│   ├── web/              ← go:embed of apps/frontend/dist
│   ├── cursor/           ← cursor-based pagination encoding
│   ├── fts/              ← FTS5 query sanitization
│   └── logging/          ← slog setup
├── apps/
│   └── frontend/         ← React 19 + Vite (build-time only; embedded into binary)
│       └── src/
│           ├── components/
│           ├── pages/
│           ├── lib/
│           └── router.tsx
├── go.mod                ← module github.com/AlvaroQ/engram-explorer
├── go.sum
├── Makefile              ← make build, make test, make seed-demo
├── .goreleaser.yaml      ← cross-platform release + Homebrew tap
└── docs/
    ├── TUI-VS-DASHBOARD.md
    └── SCHEMA-NOTES.md
```

---

## 6. Technology stack and decisions

### 6.1 Backend (Go binary)

| Dimension | Choice | Alternatives considered | Reason |
|-----------|--------|------------------------|--------|
| Language | **Go 1.23+** | Node.js, Rust | Single static binary, no runtime dependency, trivial cross-compilation |
| SQLite driver | **modernc.org/sqlite** | mattn/go-sqlite3 (CGo) | Pure Go — no CGo, no C toolchain required; cross-compiles cleanly |
| HTTP framework | **Go stdlib `net/http`** | Gin, Echo, Chi | Zero external dependency for routing; sufficient for this API surface |
| Logging | **`log/slog`** | zap, zerolog | Standard library since Go 1.21; structured JSON output |
| CLI subprocess | **`os/exec`** | shell invocation | No shell injection (args as slice) |
| DB pool strategy | **Read-only + read-write** pools over the live WAL | Single connection | WAL MVCC allows concurrent readers; write pool used only for the `unenroll` fallback |
| Distribution | **GoReleaser + Homebrew tap** | Manual builds | Single command cross-platform release |

### 6.2 Frontend

| Dimension | Choice | Reason |
|-----------|--------|--------|
| Bundler | **Vite** | Instant dev server, reliable HMR |
| Framework | **React 19** | Richest ecosystem for data dashboards |
| Styles | **Tailwind CSS v3** | Fast iteration, no orphaned CSS |
| Components | **shadcn/ui** | Copy-in (no opaque dependency), WAI-ARIA accessibility, theming |
| Routing | **TanStack Router** | Typed routes, search-params as state |
| Data fetching | **TanStack Query** | Cache, invalidation, retry, suspension |
| Tables | **TanStack Table** + **react-virtual** | Virtualization for 1000+ rows |
| Charts | **Recharts** | Composable, responsive |
| Icons | **lucide-react** | Matches shadcn, tree-shakeable |

> **Note:** Node.js and pnpm are **build-time only**. The production artifact is the Go binary with the React SPA embedded via `go:embed`. There is no Node runtime in production.

### 6.3 Key architectural decisions (ADR summary)

1. **Single Go binary**: the binary embeds the React SPA via `go:embed`. `make build` produces one portable executable. Distribution is via GoReleaser and Homebrew; no installer, no runtime, no sidecar.
2. **Read-only DB handle (persistent)**: open the SQLite read-only pool once at boot. Close on SIGTERM. Do NOT open/close per request. The WAL provides MVCC — concurrent reads with the live daemon are safe.
3. **Zero ORM**: SQL queries are written by hand with prepared statements. The schema changes rarely and its shape is known. An ORM would add an unnecessary abstraction layer.
4. **Cloud mutations via CLI subprocess (with one exception)**: `enroll` is executed via `engram cloud enroll <project>` — if the binary updates its contract (validations, events, side-effects), the dashboard does not break. **Documented exception**: `unenroll` is performed by a direct `DELETE FROM sync_enrolled_projects` via the read-write pool, because the CLI does not expose `cloud unenroll` in any published version to date. Risk accepted: if upstream adds that subcommand with side-effects (e.g. cleaning `sync_state`, tombstones, cloud notification), `cloud-control` must be migrated to invoke the CLI. See §10.
5. **Full-text search = SQLite FTS5**: already created by the Engram daemon. Zero additional effort. Native `snippet()`/`highlight()` support.
6. **Cursor-based pagination**: for large tables (`observations` can grow unboundedly). Cursor = `(updated_at, id)` DESC order.
7. **Monolithic SPA**: no micro-frontends. This is an internal tool, not a multi-team product.
8. **No auth in standard operation**: bind to `127.0.0.1` only. If ever exposed to a LAN, add a basic token (configurable via env var).

---

## 7. Dashboard views (detailed catalog)

### 7.1 `/` — Overview

| Block | Content | Data source |
|-------|---------|-------------|
| Header KPIs | Sessions, Observations, Prompts, Projects (with delta vs. 7 days ago) | Aggregate SQL query |
| Activity chart | Obs created per day (last 30 days) | `SELECT date(created_at), count(*) FROM observations GROUP BY 1` |
| Type distribution | Pie/donut of obs by type | `GROUP BY type` |
| Sync Health summary | 3 cards: enrolled, healthy, broken | Joins `sync_state` + `sync_enrolled_projects` |
| Recent activity | Last 10 sessions or observations (linked) | `ORDER BY created_at DESC LIMIT 10` |
| Daemon status | Green/red + daemon version | `GET /health` from the daemon |

### 7.2 `/observations` — Observations

- Toolbar: filters, FTS search, "save view" (URL → bookmark)
- Virtualized table with configurable columns
- Detail drawer: full content + metadata + revisions + jump to session

### 7.3 `/sessions` — Sessions list

- Table with `id`, `project`, `started_at`, `duration`, `obs_count`, `prompts_count`, `summary?`
- Click → `/sessions/:id` (timeline)

### 7.4 `/sessions/:id` — Session detail

- Horizontal visual timeline
- Combined event list (prompts + observations) ordered by `created_at`
- Stats: types breakdown, `tool_name` distribution

### 7.5 `/sync` — Sync Health

- Project table with computed status
- "Issues detected" banner at the top (always visible when issues exist)
- Drill-down `/sync/:project`: mutations, errors, sequence graph, `cloud_upgrade_state`
- Enroll/Unenroll buttons

### 7.6 `/prompts` — User Prompts

- Table + FTS5 search
- Filters: project, date range
- Drawer: full content, link to session

### 7.7 `/topics` — Topic explorer

- List of `topic_key` with obs count and associated projects
- Click → observations filtered by that topic, ordered by `revision_count DESC`

### 7.8 `/settings` — Settings

- DB path (read + reconnect handle)
- Daemon HTTP URL
- Light/dark/system theme
- Installed binary info (`engram --version` via subprocess)

---

## 8. HTTP API (endpoint catalog)

> Base prefix: `/api`. Bind: `127.0.0.1:8787`. All responses are JSON.

### 8.1 Health & meta

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/health` | Dashboard health (DB connected, daemon reachable) |
| GET | `/api/meta` | Dashboard version + engram binary version |
| GET | `/api/daemon/health` | Proxy to `engram serve /health` |
| GET | `/api/daemon/sync-status` | Proxy to `engram serve /sync/status` |
| GET | `/api/daemon/stats` | Proxy to `engram serve /stats` |

### 8.2 SQLite reads

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/overview` | KPIs + activity chart data + sync health summary |
| GET | `/api/observations` | Filtered and paginated list (cursor) |
| GET | `/api/observations/:id` | Detail (includes topic revisions) |
| GET | `/api/observations/search?q=…` | FTS5 search (snippet + highlight) |
| GET | `/api/sessions` | Paginated list |
| GET | `/api/sessions/:id` | Detail |
| GET | `/api/sessions/:id/events` | Observations + prompts ordered by timestamp (timeline) |
| GET | `/api/prompts` | Paginated list |
| GET | `/api/prompts/search?q=…` | FTS5 |
| GET | `/api/projects` | Project list with counts (obs, sessions, prompts) |
| GET | `/api/topics` | Topic key list with stats |
| GET | `/api/sync/projects` | Unified table (project + enrolled? + sync_state + counts) |
| GET | `/api/sync/projects/:project` | Drill-down (mutations, errors, upgrade_state) |
| GET | `/api/sync/issues` | Computed list of detected problems |

### 8.3 Cloud mutations

| Method | Path | Description | Body |
|--------|------|-------------|------|
| POST | `/api/cloud/enroll` | Enroll a project | `{project: string, confirm: true}` |
| POST | `/api/cloud/unenroll` | Unenroll (direct DB delete; returns 501 if CLI support detected) | `{project: string, confirm: true}` |
| GET | `/api/cloud/status` | Proxy to `engram cloud status` |
| GET | `/api/cloud/audit` | Reads `logs/cloud-mutations.jsonl` |

---

## 9. Issue detection rules

> **Refinement note**: The original "sync broken" rule generated false positives by marking any newly enrolled project as `broken` (natural state: `lifecycle='pending', ack=0, enq>0` during the first daemon cycle). The effective rule in `sync-diagnostics` adds:
> - **5-minute grace period** from `sync_state.updated_at`. Within the window → `idle`. Outside → `broken`.
> - **Explicit errors first**: if `last_error` or `reason_code` are set, status is `degraded` (previously reported as `healthy` when `enq === ack` even with a 401 in `last_error`).
> - **`SYNC_BROKEN` issue only if `status === 'broken'`**: aligns the issue feed with the table badge.

| Issue | Detection rule | Severity | Suggested UI action |
|-------|---------------|----------|---------------------|
| Sync broken | `enrolled AND lifecycle='pending' AND last_acked_seq=0 AND last_enqueued_seq>0 AND (now - sync_state.updated_at) >= 5 min` | HIGH | Show `last_error` or `reason_message`, button "view queued mutations" |
| Non-enrolled project with data | `obs_count > 0 AND project NOT IN sync_enrolled_projects` | MEDIUM | "Enroll" button |
| Extended backoff | `backoff_until > now()+1h` | MEDIUM | Show remaining time, button "view last error" |
| High consecutive retries | `consecutive_failures > 3` | MEDIUM | Show count + last error |
| Stuck lease | `lease_until > now() AND last_acked_seq stale > 30 min` | HIGH | Suggest daemon restart |
| Empty project in data | `project = '' OR project IS NULL` (in obs/sessions) | HIGH | Mark as capture bug, suggest investigation |
| Enrolled project with no data | `project IN enrolled AND obs_count = 0` | LOW | Suggest unenroll |
| Large pending queue on global target | `count(sync_mutations.acked_at IS NULL) > 100` | LOW | Show count + age of oldest mutation |
| Daemon down | `GET /health` timeout or failure | HIGH | Yellow banner "telemetry unavailable" |
| Schema migrations in progress | `cloud_upgrade_state.stage NOT IN ('completed', 'planned')` | INFO | Show in project panel |

---

## 10. Risks and mitigations

| Risk | Probability | Impact | Mitigation |
|------|------------|--------|-----------|
| `engram cloud unenroll` not present in published CLI versions | Confirmed | Medium | **Resolved**: backend performs `DELETE FROM sync_enrolled_projects` directly via the write pool. Intentionally breaks ADR §6.3 #4. If upstream adds `cloud unenroll` with side-effects, migrate `cloud-control::UnenrollViaDB()` to invoke the CLI. |
| Schema changes between versions | Medium | High | Smoke-test against real DB at binary boot; warn in UI if schema is unrecognized |
| Daemon writes while dashboard reads | High (constant) | Low | WAL provides MVCC; reads are snapshot-consistent. Frontend polling (TanStack Query refetch) brings updates |
| `busy_timeout` insufficient for direct writes | Low | Medium | By design, subprocess CLI is used for enroll. Direct write (`unenroll`) uses `busy_timeout=5000` + retry with backoff |
| Cloud token not configured | Likely for some users | Low | Detect and degrade gracefully: full local reads available, disable actions requiring cloud |
| `engram` subprocess hangs | Low | Medium | `exec.CommandContext` with 10-second timeout; kill process if exceeded; report to UI |
| Injection via `project` name in CLI | Low | High | Strict whitelist regex (`^[a-zA-Z0-9_-]+$`) in backend before passing to exec; args as slice (no shell) |
| Dashboard exposed on LAN | Low | High | Bind to `127.0.0.1`. Documented and enforced in config |
| Malicious FTS5 query (DoS) | Low | Low | Limit `q` length; escape special FTS5 characters; SQLite query timeout via context cancellation |
| Read-only handle becomes stale after daemon restart | Medium | Low | Detect I/O errors; reopen pool automatically |

---

## 11. Code conventions and quality

- **Strict typing**: Go with `go vet`, `staticcheck`, and `golangci-lint`. No untyped interface{} where a concrete type is known.
- **No raw string concatenation for SQL**: all queries use prepared statements or parameterized queries.
- **Naming**: Go conventions (`camelCase` unexported, `PascalCase` exported). React: `PascalCase` for components, `kebab-case` for page files.
- **Comments**: only where the "why" is not obvious. Never describe the "what".
- **Tests**: every public service has at least one happy path + one edge case. FTS5 and SQL aggregation tests are mandatory (regression).
- **SQL**: prepared statements (cached at boot). No string concatenation with user input.
- **HTTP errors**: shape `{error: {code: string, message: string, details?: any}}`. Codes: `BAD_INPUT`, `NOT_FOUND`, `DAEMON_UNAVAILABLE`, `CLI_ERROR`, `INTERNAL`.
- **Conventional commits**: `feat:`, `fix:`, `chore:`, `docs:`, `refactor:`, `test:`. No AI attribution.
- **Build**: `make build` produces a single binary. `go test ./...` runs all tests. `make lint` runs `golangci-lint`.

---

## 12. Appendices

### 12.1 Relevant `engram` CLI commands (reference)

| Command | Use in the dashboard |
|---------|---------------------|
| `engram --version` | Display version in footer/settings |
| `engram cloud status` | Proxied at `/api/cloud/status` |
| `engram cloud enroll <project>` | Subprocess in `POST /api/cloud/enroll` |
| `engram serve` | HTTP daemon on `:7437` (consumed for telemetry) |
| `engram stats` | Possible alternative to direct SQL (not used in current implementation) |

### 12.2 Environment variables

| Variable | Default | Use |
|----------|---------|-----|
| `ENGRAM_DATA_DIR` | `~/.engram` | Path to the DB directory |
| `ENGRAM_PORT` | `7437` | Daemon port |
| `ENGRAM_CLOUD_TOKEN` | (empty) | Only needed if the dashboard executes cloud operations requiring auth |
| `DASHBOARD_PORT` | `8787` | Dashboard server port |
| `DASHBOARD_HOST` | `127.0.0.1` | Bind address |

### 12.3 Future considerations (out of current scope)

- Optional LAN auth (basic token, OIDC).
- "Diff" mode between DB snapshots (compare two points in time).
- Alerting (e.g. notify when a project accumulates more than N unacknowledged mutations).
- Obsidian plugin (export → vault).
- Observation editor (write path). Deferred — violates separation of concerns; if added, must go via the official MCP/API.
- Upstream request: add `engram cloud unenroll <project>` to the CLI.
- Upstream request: richer HTTP API (at least `GET /observations`, `GET /sessions`, `GET /sync/projects`) — would allow future versions of the dashboard to move away from direct SQLite reads.
