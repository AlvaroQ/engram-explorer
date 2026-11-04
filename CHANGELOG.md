# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

---

## [Unreleased]

### Added

- **CI hardening** — CI now also runs on `develop`, uses least-privilege permissions and SHA-pinned actions,
  runs `go test -race` and `govulncheck`, and fails when generated templ code or the Tailwind CSS bundle is
  stale. Dependabot keeps Go modules, npm packages and GitHub Actions up to date.
- **Design specs** — `design/` (design system reference and overview shell directions) is now versioned.
- **Code of Conduct** — `CODE_OF_CONDUCT.md` (Contributor Covenant 2.1).

### Fixed

- **Stale cloud locale strings** — removed leftover UI strings that claimed the `engram` CLI has no
  `cloud unenroll` and that the dashboard writes `sync_enrolled_projects` directly.
- **Demo mode data-source lock** — demo mode rejects every change of data source (Engram database path, Claude Code
  directory, module enable/disable and path, profile create/switch/delete, Claude Code accounts) with "Write
  operations are disabled in demo mode.", so a demo instance can never be repointed at a real database. Theme,
  language, sidebar and advanced-view preferences still work.
- **Module enable/disable without restart** — enabling a module in Settings serves its pages, API and
  maintenance routes right away (they used to 404 until a restart when the module was off at boot). While a
  module is disabled its API routes answer `503 MODULE_DISABLED` and its pages show a localized notice, instead
  of 404s or `500` errors on closed database handles. Switching the database with Engram disabled returns a user
  error instead of panicking.
- **`/api/health` after a database switch** — `db.path` reports the database in use after a Settings switch (it
  kept the path opened at startup).
- **Demo mode served the real database** — with an existing `config.json` profile, `--demo` served the profile's
  database. Demo mode now runs on an in-memory profile: it always serves the demo database and never reads or
  writes `config.json`.
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
- **`make demo`** — the target now starts the server in demo mode (`--demo` with the seeded database), so the
  demo banner is shown and writes are rejected.
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

### Removed

- `doctor.IsHTMX`, `doctor.ErrorPartial` and `doctor.EmptyState` (internal): the doctor module reuses the UI
  shell helpers (`ui.Render`, `ui.RenderError`, `ui.IsHTMX`, `ui.RequireRW`, `ui.ErrorPartial`,
  `ui.EmptyState`) instead of keeping copies.
- `doctor.IsHTMX`, `doctor.ErrorPartial` and `doctor.EmptyState` (internal): the doctor module reuses the UI
  shell helpers (`ui.Render`, `ui.RenderError`, `ui.IsHTMX`, `ui.RequireRW`, `ui.ErrorPartial`,
  `ui.EmptyState`) instead of keeping copies.
- Unused Pico CSS stylesheet from the doctor module.
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
