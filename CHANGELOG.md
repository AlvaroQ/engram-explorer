# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

---

## [Unreleased]

### Added

- **Weighted search** — memory and conversation search rank results with column-weighted BM25; a several-word
  search that finds nothing retries once matching any word, and the Memory list says when it did.
- **Trigram-aware search** — on Engram databases indexed with the trigram tokenizer, words under 3 characters no
  longer empty an all-words search, and a query of only short words matches memory titles.
- **Review due filter** — the Memory list shows a "Review due (N)" toggle that lists only memories whose review
  date has passed, counted with the other active filters; `GET /api/observations?review=due` does the same.
- **Revision history API** — `GET /api/observations/{id}/versions` exposes a memory's stored revisions (read-only;
  reports `available: false` on databases without history).
- **Revision history panel** — the memory detail has a "Revision history" section that loads the stored earlier
  versions when opened (newest first, capped at 50), and says when the database keeps no history.
- **Project backup** — `GET /api/projects/{project}/export` (and "Download project backup" on the project page and in
  Maintenance → Possibly fragmented projects) downloads a full-schema snapshot holding only that project's rows,
  which the existing import merges back. Sessions of other projects that the project's rows belong to are kept
  without their summary and directory.
- **ESLint** — a flat config (typescript-eslint, react-hooks) runs in CI via `pnpm lint` over the frontend islands
  and the UI browser scripts. Frontend tests now fail when no test files are found.
- **golangci-lint** — CI lints the Go backend with golangci-lint v2.15.0 (`.golangci.yml`: the standard linters plus
  bodyclose, errorlint and unconvert); the existing findings are fixed and unused code is removed.
- **Brain-first overview** — the overview opens on a full-width knowledge-graph hero with the page title, an
  "Open Knowledge Graph" button and a memory search (`/observations?q=`). On wide screens the hero collapses into
  a 72px bar as you scroll and the KPI cards fold into a 44px strip whose items stay links; under reduced motion
  the hero scrolls away and only the strip pins. The graph pauses while collapsed or off-screen. KPIs: memories
  saved (+ last 7 days), projects (most active in 30 days), AI conversations in the last 7 days per agent, and
  agent spend this calendar month per agent (`~` marks a lower bound). Agent figures load after the page, so a
  large transcript folder never delays it; when they cannot be loaded both cards say so instead of loading
  forever, and a note flags totals that miss a source that could not be read. With no agent connected the
  cards link to Settings → Data sources.
- **Error toasts** — failed htmx requests (4xx/5xx and network errors) show a toast in an `aria-live` region
  (errors are `role="alert"`), with the server's message when the response carries one and a localized
  fallback otherwise. Toasts close by button or after 8 s, pause while hovered or focused, and skip motion under
  reduced motion. The htmx `responseHandling` is pinned so error responses never replace page content; polled
  widgets (status pill, sync panels) opt out with `data-no-error-toast`.
- **Codex sessions provider** — a new `codex-sessions` module ("Codex") reads the local Codex CLI rollouts under
  `$CODEX_HOME/sessions` (default `~/.codex/sessions`), read-only. Token counts are normalized to disjoint
  buckets (cached tokens are a subset of Codex's `input_tokens`), usage is deduplicated by `response_id` (or by
  cumulative `token_count` totals), tool calls are paired with their output by `call_id`, and resumed or forked
  sessions do not double-count replayed usage. Models without a published OpenAI price count as unknown (cost 0,
  flagged with `~`). New installs get the module enabled at `~/.codex`; existing `config.json` files are not
  rewritten — the module runs on the built-in home until a path is saved in Settings → Modules, where the Codex
  row offers its path form also while the module is enabled.
- **Agent sessions** — Claude Code and Codex sessions share one vendor-neutral model and one set of pages:
  `/agent-sessions` (usage overview: totals, cost per agent / project / day, tokens per model, most expensive
  sessions), `/agent-sessions/list` (filters by agent, source, project, model, text and date range; sort and
  pages; one notice per source that cannot be read) and a detail page per session
  (`/agent-sessions/claude-code/<account>/<project>/<id>`, `/agent-sessions/codex/<rollout path>`) that shows
  text, collapsed reasoning, each tool call with its output and unpaired calls or outputs marked.
  `GET /api/agent-sessions/stats` returns the neutral aggregate (input, cache read, cache write and output
  tokens, reasoning apart, cost, unknown-model flag, per agent), cached by a source fingerprint. Enabling both
  modules shows a single "Agent sessions" sidebar group.
- **CI hardening** — CI now also runs on `develop`, uses least-privilege permissions and SHA-pinned actions,
  runs `go test -race` and `govulncheck`, and fails when generated templ code or the Tailwind CSS bundle is
  stale. Dependabot keeps Go modules, npm packages and GitHub Actions up to date.
- **Maintenance: empty sessions** — Settings → Maintenance lists sessions with no memories (soft-deleted ones
  count as content), no prompts, no summary (whitespace only counts as none) and no memory relation or prompt
  tombstone pointing at them, and deletes them in bulk after a confirmation. The delete takes
  no id list: it re-checks the rule inside one transaction, so a session that gained content in the meantime is
  kept. Sessions of cloud-enrolled projects and sessions started in the last 24 hours are never deleted; read-only
  mode hides the button. JSON: `GET /api/maintenance/empty-sessions`, `POST /api/maintenance/empty-sessions/delete`
  (`503 RW_UNAVAILABLE` in read-only mode).
- **Maintenance: possibly fragmented projects** — Settings → Maintenance suggests project names that probably
  mean the same project (letter case, surrounding whitespace, trailing separators, or a path vs its last folder;
  no generic prefix matching). Each group shows memory and session counts, a backup link and a confirmed
  "Combine into <target>" that reuses the project merge. Suggestions only: nothing is merged automatically,
  cloud-enrolled projects cannot be combined and read-only mode hides the action.
- **Maintenance: data quality** — Settings → Maintenance lists live memories with an empty title and groups of
  exact duplicates (same `normalized_hash`), capped at 50 entries with full counts. Each memory opens its detail;
  the checks are read-only and never delete anything.
- **Design specs** — `design/` (design system reference and overview shell directions) is now versioned.
- **Code of Conduct** — `CODE_OF_CONDUCT.md` (Contributor Covenant 2.1).

### Fixed

- **Agent cost on unpriced models** — usage on models without a known price (for example newer Codex models)
  read as `$0.00`, and the "Cost by project" and "Usage over time" charts drew no bars. Such costs now read
  "Not priced" on the agent sessions overview, list, detail and the overview spend card, with a visible note
  naming the models ("No known price for gpt-5.6-sol, gpt-5.6-terra"; a tooltip in list rows). A partially
  priced total notes that it excludes unpriced turns, and a scope with no priced cost plots tokens per
  project, agent and day with a short note. The overview drops the per-agent summary card, which repeated
  the sessions, tokens and cost cards.
- **Disabled-module page** — opening a disabled module's page directly shows the full app layout (sidebar,
  account switcher) with the notice and status `503`, instead of an unstyled fragment.
- **Module toggles and profile switches during requests** — a module disabled, re-opened or switched while
  pages and API calls are in flight no longer fails them with `500` ("database is closed"): each request keeps
  the data source it started with, and the old one is closed when those requests finish (at most 30 s later). A
  request still running after that 30 s drain timeout has its data source closed under it and fails. The HTTP
  write timeout (also 30 s) only stops the response, not the handler: a longer import can still complete on
  the server while the client sees an error (see "Limits" in the README).
- **Failed module re-open kept the old data source open** — when re-opening a module fails (it becomes `errored`),
  its previous database or directory handle is now closed once the requests using it finish, instead of staying
  open until the server stops. Disabling an `errored` module closes anything it still held.
- **Health-check log noise** — a failing provider health check (missing Claude Code projects directory, unusable
  Engram database) is logged once when it starts failing and once when it recovers, through the configured
  logger, instead of a warning on every `/api/health` poll.
- **Maintenance page sidebar** — Settings → Maintenance and the sync project page show the same sidebar as the
  other pages (enabled modules only, account switcher) instead of a fixed Engram + Claude Code layout.
- **Stale cloud locale strings** — removed leftover UI strings that claimed the `engram` CLI has no
  `cloud unenroll` and that the dashboard writes `sync_enrolled_projects` directly.
- **Module enable/disable without restart** — enabling a module in Settings serves its pages, API and
  maintenance routes right away (they used to 404 until a restart when the module was off at boot). While a
  module is disabled its API routes answer `503 MODULE_DISABLED` and its pages show a localized notice, instead
  of 404s or `500` errors on closed database handles. Switching the database with Engram disabled returns a user
  error instead of panicking.
- **`/api/health` after a database switch** — `db.path` reports the database in use after a Settings switch (it
  kept the path opened at startup).
- **Environment overrides ignored** — `ENGRAM_DATA_DIR`, `CLAUDE_PROJECTS_DIR` and `ENGRAM_DAEMON_URL` were ignored once
  `config.json` existed; they now win over the saved profile at startup and are never written to disk.
- **Profile store races** — concurrent Settings requests no longer race on the profile store (`config.Store`
  guards it with a lock), and module toggles after a profile switch are saved into the new active profile
  instead of the previous one. If saving a profile switch fails and restoring the previous profile fails too, both
  errors are reported.
- **Profile switch lifecycle** — switching profiles closes the providers the new profile disables and opens the
  ones it enables (it used to leave disabled providers open and never open newly enabled ones); a failed open
  rolls the switch back without leaking instances.
- **Database import confirmation** — choosing a file in Settings → Import now submits with
  `requestSubmit()`, so the confirmation dialog runs before anything is uploaded (`form.submit()` skipped it).
- **Concurrent database exports** — each export snapshot is written to its own private temp directory instead
  of a second-resolution file name, so two exports in the same second no longer fail with `500` (and one no
  longer deletes the other's file).
- **`HEAD /api/db/export`** — answers the download headers without building a snapshot (it used to run a full
  `VACUUM INTO` and discard it).
- **Fresh clone build** — `go build ./...` compiles on a fresh clone again: the `internal/web/dist`
  placeholder is committed (it was excluded by the generic `dist/` ignore rule).
- **Settings DB hot-swap** — changing the Engram database path from Settings now swaps the pool on the
  provider registry path as well.

### Security

- **Stored XSS in prompt search** — FTS snippets are no longer rendered as raw HTML. Snippets are built with
  private-use sentinel delimiters, rendered as escaped text, and only the match is wrapped in `<mark>`. The
  JSON `snippet` field keeps its `<mark>` delimiters and must be rendered as text by API consumers.
- **Import identifiers** — column names read from an uploaded database are quoted in the merge statements
  (defense in depth: validation already requires them to match the local schema).
- **Loopback-only bind** — the server refuses to start when `DASHBOARD_HOST` is not a loopback address
  (`127.0.0.1`, `::1`, `localhost`), because the dashboard has no authentication. Set
  `DASHBOARD_ALLOW_REMOTE=true` to bind another address at your own risk.
- **DNS rebinding and CSRF protection** — every request (including the embedded `/islands/` and `/assets/`
  bundles) must carry a `Host` that names the dashboard (`localhost`, `*.localhost`, an IP literal or the
  configured `DASHBOARD_HOST`); unsafe methods must be same-origin (`Sec-Fetch-Site` / `Origin`), and
  `GET /api/db/export` refuses cross-site requests and navigations. Blocked requests get `403` with
  `HOST_NOT_ALLOWED` or `CROSS_ORIGIN_REQUEST`. Security headers now also apply to the embedded bundles.
- **Vite dev origins only in development** — `http://localhost:5173` and `http://127.0.0.1:5173` are trusted
  for CORS, cross-origin protection and `GET /api/db/export` only when `ENGRAM_DASH_ENV=development`
  (`make dev`). In production they are treated like any other foreign origin.
- **Cloud is read-only** — the dashboard no longer enrolls, unenrolls or syncs projects and never spawns the
  `engram` CLI. Removed `POST /api/cloud/{enroll,unenroll,sync,sync-all}`, `GET /api/cloud/capabilities`,
  `POST /doctor/sync/{project}/{enroll,unenroll,sync}`, `POST /projects/{project}/{enroll,unenroll}` and
  `POST /partials/sync-cloud`, with their buttons. Sync diagnostics and the daemon GET proxy remain; use the
  `engram` CLI to enroll or sync. This also closes the gap where cloud actions ignored `ENGRAM_DASH_READONLY`.
  The README documents `engram cloud enroll|unenroll <project>`, and the unused island strings for the removed
  sync button are gone.
- **JSON body limit** — JSON request bodies are capped at 1 MiB (`http.MaxBytesReader`); larger bodies get
  `413` with `PAYLOAD_TOO_LARGE`. Bodies with data after the JSON value (for example `{"a":1}{"b":2}`) are
  rejected with `400 BAD_INPUT`; trailing whitespace is still accepted.
- **Write route rate limits** — every JSON API write route is rate limited per client IP and route pattern (no longer
  by raw path alone, and `X-Forwarded-For` is ignored): database import 5/min, export 10/min, other write
  routes 120/min. Expired buckets are pruned. Rate-limited requests answer `429` with error code `RATE_LIMITED`
  (was `BAD_INPUT`) and a `Retry-After` header.
- **Health endpoint without daemon error text** — when the daemon is unreachable, `/api/health` reports
  `"error": "daemon unavailable"` plus a `code` (`NETWORK`, `TIMEOUT`, `HTTP`, `PARSE`); the raw client error (URL,
  dial error) is only shown with `ENGRAM_DASH_ENV=development`.
- **Health provider errors without OS text** — `/api/health` `providers[].error` shows a generic message
  (`projects directory unavailable`, `database unavailable`) outside development; the OS detail (paths) goes to
  the server log.
- **Daemon proxy without client error text** — `/api/daemon/*` errors answer `DAEMON_UNAVAILABLE` with
  `daemon <path> unavailable (<CODE>)` in production; the raw client error (URL, dial error) goes to the log and is
  only returned with `ENGRAM_DASH_ENV=development`.
- **UI and doctor rate limit** — form posts and doctor actions (`POST`/`PUT`/`PATCH`/`DELETE` outside `/api/`) share a
  limiter of 120 requests per minute per client, method and route pattern (path parameters share one budget). API
  write routes keep their own limits and are not limited twice.
- **Settings path validation** — changing the Engram database path or the Claude projects directory in
  Settings now validates the target first (an SQLite file with the Engram `observations` and `sessions`
  tables; a directory with project folders, the same checks the providers apply). On rejection the active
  database/directory and saved settings stay unchanged, and the message only echoes the typed path.
- **No internal errors in HTML** — UI and doctor pages no longer show raw error text (SQLite errors,
  filesystem paths), including the Settings daemon and module hints. Messages written for the user are still
  shown; any other error shows a generic localized message and the detail goes to the server log.
- **No raw module or sync errors in HTML** — the Settings module row error is unexported and only built from
  the provider error through the safe error mapper, so raw error text cannot reach the page by construction,
  and Maintenance → Cloud project detail replaces the daemon's `last_error` text with a generic localized
  message and no longer shows its `reason_message` (the reason code is still shown).
- **Stricter Content Security Policy** — the policy is now `script-src 'self'; style-src 'self'` with no
  `'unsafe-inline'`: inline scripts, event-handler attributes and `hx-on` moved to a static `ui-actions.js`,
  htmx attribute evaluation is disabled (`allowEval: false`), templates carry no `style` attributes or
  `<style>` elements (view-transition and htmx indicator rules live in `/static/styles.css`), and htmx runs
  with `includeIndicatorStyles: false`.

### Changed

- **Faster knowledge-graph layout** — large graphs (up to the 5000-node cap) lay out about 4× faster: repulsion
  above 255 nodes uses a Barnes–Hut approximation instead of comparing every pair. Every graph size keeps the full
  200 simulation steps, so project clusters stay as clearly separated as before.
- **Icon rail shell** — the sidebar starts as a 64px icon rail (labels show as chips on hover and keyboard
  focus) and expands to 236px; the choice is kept in the `sidebar` cookie, so the first paint is right. Every page
  starts with a "Skip to content" link, the current entry has `aria-current="page"`, and the agent pages are an
  "Agents" group with one entry per live agent (`/agent-sessions?agent=claude-code`, `?agent=codex`) instead of
  the neutral Overview / Sessions pair, which stays reachable as the page's "All agents" tab.
- **Accessible dialogs and account menu** — the orphan detail dialog opens with `showModal()` (focus stays inside,
  Esc closes it, focus returns to the row that opened it); the sidebar account switcher is a keyboard menu button
  (Enter, Space or arrows open it; arrows, Home and End move; Esc closes it and returns focus).
- **Maintenance pages in Spanish** — the orphans list, its detail dialog and the cloud sync panels no longer
  hard-code English; every user-visible string in the templ sources goes through the locale catalogs (guarded by
  `TestTemplSources_NoEnglishLiterals`). Inline errors built in Go handlers (orphan detail and assign errors,
  read-only writes, invalid ids, the "Failed to load …" prefixes) and the project-combine confirmation follow
  the language too (guarded by `TestGoSources_NoEnglishErrorLiterals`).
- **Design tokens meet WCAG AA** — colors live only in `internal/ui/static/tokens.css` (linked before `app.css`
  and `styles.css`; the doctor stylesheet now uses them instead of its own hex palette), every text/background
  pair passes AA in light and dark (guarded by `tokens_contrast_test.go`), and new `--accent-fg`, `--info` and
  `--brand-codex` tokens cover button text, info badges and the Codex agent.
- **Claude Code URLs moved** — `/cc-overview` now answers `301` to `/agent-sessions`; `/cc-sessions`,
  `/cc-sessions/list` and `/cc-overview?tab=sessions` answer `301` to `/agent-sessions/list?agent=claude-code`
  (the project filter is kept and `account=<id>` becomes `source=claude-code/<id>`); `/cc-sessions/<project>/<id>`
  answers `301` to `/agent-sessions/claude-code/<account>/<project>/<id>`. The sidebar section visibility cookie
  for these pages is now `nav_agent-sessions` (a previously hidden Claude Code section shows again once).
- **Provider capabilities (agent sessions)** — session providers expose `AgentSessionSource`; the host resolves
  every live one per request (`Registry.FindAll` / `Lease.FindAll`), and each provider mounts the detail route of
  its own sources. The UI no longer switches on provider IDs either (sidebar icons, section toggles and Settings
  rows come from the registry's nav groups).
- **Provider capabilities** — provider wiring goes through capability interfaces (`Provider.Routes` with a host
  `Router`, `Writable`, `Searcher`, `StatsSource`) instead of a switch on provider IDs: search
  (`GET /api/observations/search`) and overview stats (`GET /api/overview`) are served through `Searcher` and
  `StatsSource`, and write routes and read-write handles are gated by `Writable`.
- **Unreadable `config.json`** — a `config.json` that cannot be parsed is left untouched; the explorer runs on
  defaults and saves nothing.
- **Single saved configuration** — `config.json` is the only file the explorer writes: Settings database and
  Claude directory changes are saved in the active profile (a Claude directory change now survives a
  restart), and `explorer-settings.json` is read once to create `config.json` and never written again.
- **Go toolchain** — releases build with Go 1.26.10 (`toolchain` directive in `go.mod`); the minimum
  supported Go version stays at 1.25. Go 1.25.0 shipped standard library vulnerabilities reachable from the
  HTTP server.
- **Release gating** — the release workflow only publishes after the full CI suite passes for the tag.
- **Version format** — `make build` reports the version without the leading `v` (e.g. `0.2.0`), matching
  GoReleaser builds.
- **Pinned templ generator** — `make templ` (and `make build`) runs `templ@v0.3.1001` via `go run`, the same
  version CI uses for its drift check, instead of whatever `templ` binary is on `PATH`.

### Deprecated

- **`GET /api/cc-sessions/stats`** — still answers with its previous JSON shape over the Claude Code accounts only,
  now with `Deprecation: true` and `Link: </api/agent-sessions/stats>; rel="successor-version"` headers. Use
  `GET /api/agent-sessions/stats`; the old endpoint will be removed in a future release.

### Removed

- **BREAKING: demo mode** — `--demo`, `--demo-db`, `ENGRAM_DEMO`, `ENGRAM_DEMO_DB`, the demo banner and the
  `403 DEMO_MODE` answer are gone. The binary now refuses to start when one of those flags or variables is set
  (instead of serving your real data) and names the replacement. `make demo` seeds `./demo` — `engram.db` plus
  sample Claude Code sessions in `./demo/claude` and sample Codex rollouts in `./demo/codex` — and runs the normal
  binary with `ENGRAM_DATA_DIR=./demo`, `CLAUDE_PROJECTS_DIR=./demo/claude`, `CODEX_HOME=./demo/codex` and
  `ENGRAM_DASH_READONLY=true`: write routes answer
  `503 RW_UNAVAILABLE`, Settings changes persist in `./demo/config.json`, and `~/.claude/projects` is no longer
  read by the demo. Scripts that ran `engram-explorer --demo` must switch to `make demo` or those variables.
- `doctor.IsHTMX`, `doctor.ErrorPartial` and `doctor.EmptyState` (internal): the doctor module reuses the UI
  shell helpers (`ui.Render`, `ui.RenderError`, `ui.IsHTMX`, `ui.RequireRW`, `ui.ErrorPartial`,
  `ui.EmptyState`) instead of keeping copies.
- Unused Pico CSS stylesheet from the doctor module.
- Unused locale strings `projects.toast.enrollSuccess`, `projects.toast.enrollError` and `settings.row.cloudEnroll`,
  and the unused helpers `engram.EngramDeps`, the Engram instance's `DoctorDeps` and `ccsessions.CCRuntimePaths`
  (internal).
- `scripts/update-engram.*`, a personal helper unrelated to this project.

## [0.2.0] - 2026-06-12

### Added

- **Demo mode** — `--demo` (or `ENGRAM_DEMO=1`) serves a bundled sample database with writes blocked;
  `--demo-db=<path>` (or `ENGRAM_DEMO_DB`) points it at another demo file.
- **Brain WebGL fallback** — the 3D Brain detects missing WebGL support and shows an actionable fallback.
- **Review due badge** — memories past their review date are flagged in the Memory views.
- **Combine projects** — a "Combine with another project" action on the project detail page.
- **Advanced view toggle** — per-profile switch that hides technical columns.
- **Modular provider framework** — a `Provider` / `Instance` port and an ordered `Registry` replace the
  hard-wired database open in `container.go`. Providers boot independently; a failure in one never prevents
  others from loading.
- **Engram provider** (`internal/providers/engram`) — wraps the existing SQLite read-only pool, daemon proxy,
  and schema-validity health check as a first-class provider.
- **Claude Code Sessions provider** (`internal/providers/ccsessions`) — wraps the JSONL session reader and
  stats cache as a Tier-1 provider.
- **Persistent config** (`~/.engram/config.json`) — provider paths and activation states are now saved
  atomically across restarts. Legacy `explorer-settings.json` overrides are migrated automatically into the
  default profile on first run; the old file is left in place.
- **Account profiles** — named profiles in `config.json` let you maintain separate data-source configurations
  (e.g. personal vs. work). Create, switch, and delete profiles from **Settings → Accounts**. Profile
  switches are hot-swap — no restart required.
- **Account switcher** — a dropdown in the sidebar header appears when two or more profiles exist.
- **Settings → Modules** — new settings pane listing all registered providers with detection state, activation
  toggle, and (for Tier-1 providers) a path entry field with validation before activation.
- **Settings → Accounts** — new settings pane for profile management (create, switch, delete).
- **Onboarding zero-state** — when no provider is active, `GET /` renders a guided onboarding screen instead
  of an error. The onboarding screen lists auto-detected sources and offers Tier-1 path entry; once a provider
  is activated it transitions directly to the main overview without a restart.
- **Per-provider health aggregation** — `GET /api/health` now returns a `providers[]` array alongside the
  existing `db` back-compat alias. The top-level `ok` field reflects the aggregate of all active providers.
- **Schema-validity health check** — the Engram provider's health check now verifies that the expected schema
  tables exist, not just that the SQLite file is openable. An empty or schemaless database reports
  `ok: false` instead of a false positive.
- **Data-driven sidebar** — sidebar navigation is built from provider `NavGroup` contributions; adding or
  removing a provider updates the sidebar without template edits.
- **View Transitions + HTMX smooth navigation** — CSS `@view-transition` on the main content region and
  `hx-swap="innerHTML transition:true"` on partial swaps eliminate full-page flashes. Sidebar scroll position
  is preserved across swaps via `hx-preserve`.
- **i18n coverage** — all new UI strings (Modules, Accounts, onboarding, validation errors, state labels) are
  present in both `locales/en.json` and `locales/es.json`.

### Fixed

- **Non-fatal startup** — the server no longer exits with code 1 when no Engram database is found. Provider
  open failures are isolated; the server starts and serves the onboarding UI instead.
- **Health false positive** — an empty/zero-byte SQLite file previously caused `db.ok=true`. The schema
  check (`sqlite_master` query) now catches this case.

### Changed

- **Navigation** — simplified primary nav; Topics and Prompts fold into Memory as tabs, Claude Code Overview
  and Sessions merge into one tabbed page, and diagnostics move to **Settings → Maintenance**.
- **Palette** — the UI moves to a Rose Pine based palette with a pink primary color.
- **Home page** — simplified for first-glance comprehension.
- **Settings page** — extended with **Modules** and **Accounts** sections; existing path-override and
  daemon-status fields are unchanged.
- **`GET /`** — routes to the onboarding screen when no provider is active; routes to the existing overview
  when at least one provider is active.

### Migration notes

- No breaking changes. All existing environment variables (`ENGRAM_DATA_DIR`, `CLAUDE_PROJECTS_DIR`,
  `ENGRAM_PORT`, `ENGRAM_DAEMON_URL`, `ENGRAM_DASH_READONLY`, `ENGRAM_DASH_ENV`, `DASHBOARD_HOST`,
  `DASHBOARD_PORT`, `LOG_LEVEL`, `ENGRAM_DAEMON_TIMEOUT_MS`) continue to work identically.
- On first start after upgrading, a `default` profile is seeded automatically from your current environment
  variables and any `explorer-settings.json` overrides. You will see no behavior change.
- The `config.json` file is created in `$ENGRAM_DATA_DIR` (default `~/.engram/config.json`) on first
  activation via Settings or on first start if env vars are set. You do not need to create it manually.

## [0.1.0] - 2026-06-10

### Added

- **Single-binary dashboard** — one Go binary serves the templ + HTMX UI, the JSON API and the embedded
  React islands for exploring the Engram memory database.
- **Claude Code sessions** — overview and sessions views with usage charts, a conversation filter,
  attachment chips and a redesigned session detail.
- **Editable data paths** — the Engram database and Claude Code directories can be changed and hot-swapped
  from Settings.

[Unreleased]: https://github.com/AlvaroQ/engram-explorer/compare/v0.2.0...HEAD
[0.2.0]: https://github.com/AlvaroQ/engram-explorer/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/AlvaroQ/engram-explorer/releases/tag/v0.1.0
