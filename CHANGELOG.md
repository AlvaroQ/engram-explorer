# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

---

## [Unreleased]

### Added

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
