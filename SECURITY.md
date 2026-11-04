# Security Policy

**engram-explorer** is a **local-first** tool. It binds to `127.0.0.1` only and never transmits your memory data anywhere. LAN exposure is not supported by design.

## Reporting a vulnerability

Please report security issues **privately** — do not open a public issue.

- **Preferred:** use GitHub's [private vulnerability reporting](https://github.com/AlvaroQ/engram-explorer/security/advisories/new) — the **"Report a vulnerability"** button under the repository's **Security** tab.
- **Alternative:** email **alvaroquintanapalacios@gmail.com**.

Include a description and steps to reproduce. Please allow a reasonable time for a fix before any public disclosure.

## How the dashboard touches your database

Engram Explorer opens your local `engram.db` with **two** SQLite handles:

- **Read path (always on):** a read-only pool (`mode=ro` + `query_only`) serves
  every view. It cannot mutate the database and coexists safely with the live
  WAL written by the `engram serve` daemon.
- **Write path (opt-out):** a single-connection read-write pool powers a small
  set of **explicit, user-confirmed** mutations — editing an observation,
  reassigning an item's project, deleting an observation/session/prompt,
  renaming/merging a project, and importing/exporting a database snapshot.
  Every write route is guarded; if the writable handle is unavailable the
  route returns `503`.

> **Read-only mode.** Set `ENGRAM_DASH_READONLY=true` to run as a pure viewer:
> the read-write pool is never opened and every mutating route returns `503`.
> Use this when pointing the dashboard at a database you must not modify.
> Cloud sync is read-only in every mode, so no cloud operation bypasses it.

### Concurrency with the daemon

The `engram serve` daemon is the **authoritative owner** of the database. When
the daemon is running, the read path is always safe (WAL gives readers a
consistent snapshot). The write path uses `busy_timeout` and wraps every
mutation in a transaction, but a dashboard write still races the daemon's own
writes and bypasses its in-process dedup/FTS/sync bookkeeping. For destructive
or bulk operations (project rename/merge, database import) prefer running them
while the daemon is **stopped**, and keep a backup — the import path writes a
safety backup next to the DB before merging.

## Scope notes

- The local backend binds to `127.0.0.1` only — it is not meant to be reachable over a network. It refuses to
  start when `DASHBOARD_HOST` is not a loopback address (`127.0.0.1`, `::1`, `localhost`); the
  `DASHBOARD_ALLOW_REMOTE=true` override exists for isolated setups only, since there is no authentication.
- Browser protections: every request must carry a `Host` that names the dashboard (`localhost`, `*.localhost`,
  an IP literal or the configured `DASHBOARD_HOST`), which defeats DNS rebinding. Unsafe methods (`POST`,
  `PUT`, `PATCH`, `DELETE`) must be same-origin according to `Sec-Fetch-Site` / `Origin`, and
  `GET /api/db/export` refuses cross-site requests and navigations. Non-browser clients that send neither
  header (curl, scripts) are unaffected. Blocked requests get `403` (`HOST_NOT_ALLOWED` /
  `CROSS_ORIGIN_REQUEST`). The Vite dev server origins (`http://localhost:5173`, `http://127.0.0.1:5173`) are
  trusted for CORS, CSRF and the export route only when `ENGRAM_DASH_ENV=development`.
- Rate limits: JSON API write routes allow 120 requests/min (export 10/min, import 5/min) and unsafe requests
  outside `/api/` (UI form posts, doctor actions) share one 120/min limiter, each per client IP, method and routed
  pattern (path parameters share a bucket; `X-Forwarded-For` is ignored). Limited requests get `429` with
  `Retry-After` and error code `RATE_LIMITED`; htmx does not swap 4xx responses, so a limited UI action leaves the
  page unchanged.
- Cloud sync is **read-only**: the dashboard shows sync diagnostics and proxies the daemon's GET telemetry endpoints (`/health`, `/sync/status`, `/stats`). It never enrolls, unenrolls or syncs a project and never spawns the `engram` CLI — do those with the `engram` CLI directly (`engram cloud enroll <project>`, `engram cloud unenroll <project>`).
- There is no demo mode: `make demo` is a normal read-only run (`ENGRAM_DASH_READONLY=true`) against the
  seeded `./demo` dataset (`ENGRAM_DATA_DIR=./demo`, `CLAUDE_PROJECTS_DIR=./demo/claude`,
  `CODEX_HOME=./demo/codex`; the seeder writes sample Claude Code transcripts and Codex rollouts there), so it
  never reads `~/.engram`, `~/.claude/projects` or `~/.codex`. Read-only mode blocks Engram writes only;
  Settings changes persist in `./demo/config.json`, which may still record the default `~/.claude/projects` and
  `~/.codex` paths — the `CLAUDE_PROJECTS_DIR` and `CODEX_HOME` overrides are what keep the demo away from
  your real sessions, so do not run against `./demo` without them. The removed `--demo` / `--demo-db` flags and `ENGRAM_DEMO` / `ENGRAM_DEMO_DB` variables
  make the binary refuse to start instead of silently serving your real data.
  Environment overrides (`ENGRAM_DATA_DIR`, `CLAUDE_PROJECTS_DIR`, `CODEX_HOME`, `ENGRAM_DAEMON_URL`) are never
  written to disk.
- Agent sessions are **read-only**. The Claude Code and Codex modules only read session files: Claude Code
  transcripts under each account's projects directory and Codex rollouts under `$CODEX_HOME/sessions`
  (default `~/.codex/sessions`) — never Codex's configuration, credentials or history files. They never write,
  move or delete anything there and never execute a program (no `codex` or `claude` CLI). Session keys taken
  from the URL are validated (clean relative paths only, no `..`, no absolute paths, no backslashes or NUL; a
  Codex key must also be a rollout the listing found) and an invalid key answers `404`. Symlinks are not
  followed when walking the Codex sessions directory, and a detail page only opens regular files.
  `CODEX_HOME` is an in-memory override and is never written to disk.
- In production (`ENGRAM_DASH_ENV` unset) internal error details are never returned to the client. HTML views
  never show raw provider or daemon error text: Settings module errors go through the same safe error mapper,
  and the sync daemon's `last_error` / `reason_message` texts are replaced by a generic localized message plus
  the reason code.

## Content Security Policy

Every response carries:

```text
default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'
```

- **Scripts:** `script-src 'self'` only. The templates contain no inline `<script>` blocks or `on*=` handlers;
  their behaviours live in `/static/ui-actions.js` (event delegation on `data-*` attributes). htmx runs with
  `allowEval: false`, so `hx-on`, `hx-trigger` `[filters]` and `js:` values are never evaluated. A test scans
  every `.templ` file and fails on any of these constructs.
- **Styles:** `style-src 'self'` only. Templates must not use `style=` attributes or `<style>` elements (a
  test scans every `.templ` file and the rendered pages); rules live in `/static/styles.css` or Tailwind
  classes. htmx is configured with `includeIndicatorStyles: false`, so it never injects a `<style>` element.
  Scripts that need dynamic styling use the CSSOM (`element.style`), which CSP allows, never
  `setAttribute('style', …)` or HTML strings.
