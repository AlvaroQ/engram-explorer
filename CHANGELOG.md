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
- **Cloud is read-only** — the dashboard no longer enrolls, unenrolls or syncs projects and never spawns the
  `engram` CLI. Removed `POST /api/cloud/{enroll,unenroll,sync,sync-all}`, `GET /api/cloud/capabilities`,
  `POST /doctor/sync/{project}/{enroll,unenroll,sync}`, `POST /projects/{project}/{enroll,unenroll}` and
  `POST /partials/sync-cloud`, with their buttons. Sync diagnostics and the daemon GET proxy remain; use the
  `engram` CLI to enroll or sync. This also closes the gap where cloud actions ignored `ENGRAM_DASH_READONLY`.
- **JSON body limit** — JSON request bodies are capped at 1 MiB (`http.MaxBytesReader`); larger bodies get
  `413` with `PAYLOAD_TOO_LARGE`.
- **Write route rate limits** — every write route is rate limited per client IP and route pattern (no longer
  by raw path alone, and `X-Forwarded-For` is ignored): database import 5/min, export 10/min, other write
  routes 120/min. Expired buckets are pruned.
- **Settings path validation** — changing the Engram database path or the Claude projects directory in
  Settings now validates the target first (an SQLite file with the Engram `observations` and `sessions`
  tables; a directory with project folders, the same checks the providers apply). On rejection the active
  database/directory and saved settings stay unchanged, and the message only echoes the typed path.
- **No internal errors in HTML** — UI and doctor pages no longer show raw error text (SQLite errors,
  filesystem paths). Messages written for the user are still shown; any other error shows a generic
  localized message and the detail goes to the server log.
- **Stricter Content Security Policy** — `script-src` no longer allows `'unsafe-inline'`: inline scripts,
  event-handler attributes and `hx-on` moved to a static `ui-actions.js`, and htmx attribute evaluation is
  disabled (`allowEval: false`). `style-src` still allows `'unsafe-inline'` (inline style attributes and htmx
  indicator styles); removing it is planned for Phase 2.

### Changed

- **Go toolchain** — releases build with Go 1.26.10 (`toolchain` directive in `go.mod`); the minimum
  supported Go version stays at 1.25. Go 1.25.0 shipped standard library vulnerabilities reachable from the
  HTTP server.
- **Release gating** — the release workflow only publishes after the full CI suite passes for the tag.
- **Version format** — `make build` reports the version without the leading `v` (e.g. `0.2.0`), matching
  GoReleaser builds.

### Removed

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
