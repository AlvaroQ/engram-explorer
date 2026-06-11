# Engram Explorer

> See your agent's brain — a local-first dashboard companion for [Engram](https://github.com/Gentleman-Programming/engram).

![Engram Explorer Dashboard overview](docs/engram-explorer.png)

Engram stores your agents' long-term memory in a local SQLite database (`~/.engram/engram.db`). The official TUI/CLI is great for capture and recall — but it can't _show_ you the shape of that memory, nor the operational problems hiding inside it: orphan projects, broken cloud sync, thousands of pending mutations, observations captured without a project. This dashboard reads that database **directly** — read-only by default, so it never blocks the daemon's writes — and surfaces all of it across 13 views — projects, sessions, observations, prompts, topics, sync health, and diagnostics — in both **Spanish and English**, light or dark. It can also apply targeted, **user-confirmed** fixes (reassign a stray observation, rename a project, prune junk) through a guarded write path you can switch off entirely with `ENGRAM_DASH_READONLY=true` — see [Editing your memory](#editing-your-memory).

The distribution philosophy matches Engram itself: **one binary, one SQLite file.** No Node.js runtime, no WebView2, no sidecar. Download the binary for your platform, point it at your database, and open your browser.

---

## Relationship to Engram

Engram Explorer is a **community companion** — a read-only viewer and operational dashboard built on top of [Engram](https://github.com/Gentleman-Programming/engram). It is not an official plugin, not a fork, and not affiliated with Gentleman Programming.

The official Engram plugin ecosystem (MCP plugins and integrations) communicates with the daemon over HTTP at `127.0.0.1:7437`. Engram Explorer takes a different approach: it reads the SQLite file directly in WAL read-only mode, which means it can run whether or not the daemon is active, and it **never competes with `engram serve` as the authoritative owner of the database**. When the daemon is running, the dashboard respects it — it only proxies the daemon's own telemetry endpoints (`/health`, `/sync/status`, `/stats`) and, for user-confirmed cloud operations, shells out to the `engram` CLI the same way you would from a terminal.

The "one binary, one SQLite file" distribution philosophy is deliberate: it mirrors the simplicity Engram itself ships with.

Think of it this way — the CLI captures and recalls; this shows you the _shape_ of what your agent has remembered. It complements the official tooling, it doesn't replace it.

---

## The Brain — your memory as a 3D neural map

![The Brain — 3D force-directed graph of all observations](docs/brain_3d.png)

This is the centerpiece. **The Brain renders every observation in your database as a node in a live, force-directed 3D graph** — and the way it organizes them is by **project**.

- **Each project becomes a lobe.** Observations from the same project cluster together in space, forming the colored constellations you see above — one vivid region per project, dozens of them, each its own part of the brain.
- **Each sphere is a single memory.** Its size reflects weight (and how many duplicates were deduped into it); its color encodes either the **project** or the **observation type** (a toggle you control).
- **Synapses connect related memories.** Edges between observations carry GPU-driven particle flow — the brain literally "breathes" — with relationship and confidence baked into how each connection looks.
- **Hover any neuron to read it.** A detail card surfaces the memory's title, tags, project, and date right in the 3D scene (see the floating card above); click through for the full markdown content and `[[wikilinks]]`.
- **It's fast by design.** The force-directed layout runs off-main-thread in a Web Worker, rendering uses a single instanced mesh on a demand frameloop, and particle flow is GPU-driven — so the whole brain stays smooth even with hundreds of nodes on screen.

> The Brain will keep evolving toward a **deeper, drillable neural architecture** — immersive navigation into the "neurons" and clusters your memory forms as it grows.

---

## What it surfaces

| View               | What you get                                                                                                                  |
| ------------------ | ----------------------------------------------------------------------------------------------------------------------------- |
| **Overview**       | At-a-glance counts (projects, sessions, observations, prompts), activity-by-project chart, type distribution, and sync health |
| **Brain**          | The 3D neural map above                                                                                                       |
| **Projects**       | Per-project memory, activity, and sync state                                                                                  |
| **Project detail** | Full memory and session timeline for a single project                                                                         |
| **Observations**   | Every memory, filterable, with full markdown + wikilinks                                                                      |
| **Sessions**       | Session timeline and per-session detail                                                                                       |
| **Prompts**        | Captured user prompts, searchable                                                                                             |
| **Topics**         | Memory grouped by stable `topic_key`                                                                                          |
| **Sync Health**    | Cloud enrollment status: enrolled / healthy / broken / pending mutations                                                      |
| **Sync project**   | Per-project cloud sync detail and mutation queue                                                                              |
| **Orphans**        | Observations captured without a project — the capture bugs the CLI can't show                                                 |
| **Diagnostics**    | Orphan projects, broken sync — a diagnostic engine across all 13 views                                                        |
| **Settings**       | UI preferences, language, theme, Modules (provider toggles + path entry), and Accounts (profiles)                             |

---

## Requirements

### To run a pre-built binary

- The `engram-explorer` binary for your platform (macOS, Linux, Windows) — download from [GitHub Releases](https://github.com/AlvaroQ/engram-explorer/releases)
- No Node.js, no runtime dependencies
- An Engram or Claude Code install is **optional** — the dashboard starts without any data source and walks you through setup on first run (see [First-run onboarding](#first-run-onboarding))

### To build from source

- Go 1.25 or later — matches the `go` directive in `go.mod`, which is the minimum required by the `modernc.org/sqlite` and `golang.org/x/sync` dependencies
- Node.js 20+ and pnpm 10+ (to build the React island bundles; not needed at runtime)
- `templ` CLI (optional; generated `*_templ.go` files are committed, so `go build ./...` works without it)

### Engram daemon (optional)

The `engram serve` daemon is **not required** to run the dashboard. The dashboard reads the SQLite file directly and degrades gracefully when the daemon is offline: all read views stay fully functional; only the live `/health`, `/sync/status`, and cloud mutation endpoints rely on the daemon.

Install Engram if you don't have it yet:

```bash
brew install gentleman-programming/tap/engram
engram serve   # starts the daemon and creates ~/.engram/engram.db on first run
```

---

## Quick start

### Path A — Try it with demo data (no Engram required)

```bash
git clone https://github.com/AlvaroQ/engram-explorer.git
cd engram-explorer

# Build the frontend and the Go binary
make build

# Seed a realistic demo database
go run ./cmd/seed-demo

# Run the dashboard against the demo data
ENGRAM_DATA_DIR=./demo ./engram-explorer

# Open your browser
open http://127.0.0.1:8787
```

> `cmd/seed-demo` generates `./demo/engram.db` with realistic sample observations, sessions, projects, topics, and sync state so you can explore every view without needing a live Engram install.

### Path B — Use your real Engram database

```bash
git clone https://github.com/AlvaroQ/engram-explorer.git
cd engram-explorer

make build          # frontend → embed → go build
./engram-explorer         # reads ~/.engram/engram.db by default

open http://127.0.0.1:8787
```

Health check:

```bash
curl http://127.0.0.1:8787/api/health
```

### Run without building (Go installed)

```bash
# Frontend must be built first so the Go embed has content
make frontend embed
go run ./cmd/engram-explorer
```

---

## Architecture

Engram Explorer is a **single Go binary** (`cmd/engram-explorer`) that serves all views on one port (default `127.0.0.1:8787`). The backend opens the Engram SQLite database with two `modernc.org/sqlite` handles (pure-Go, no CGo): a **read-only** pool (`mode=ro` + `query_only`) that serves every view and coexists safely with the live WAL written by the daemon, and a single-connection **read-write** pool that powers a small set of explicit, user-confirmed mutations (see [Editing your memory](#editing-your-memory)). API routes (`/api/*`) are handled by an `internal/httpapi` mux built on Go's standard `net/http`. The backend proxies the daemon's HTTP telemetry endpoints (`/health`, `/sync/status`, `/stats`) and — when the user explicitly confirms a cloud enroll/sync — shells out to the `engram` CLI as a subprocess. Set `ENGRAM_DASH_READONLY=true` to disable the write path entirely.

**The primary UI is server-rendered** using [templ](https://templ.guide) and [HTMX](https://htmx.org) in the `internal/ui` package. Go handlers load data via the services layer → render templ templates (full-page on direct navigation, partial fragment on HTMX requests via the `HX-Request` header). Interactivity (filters, pagination, detail drawers) is driven by `hx-get`/`hx-post`/`hx-swap`. Long lists (observations, sessions) use server-side cursor-based pagination with `hx-trigger="revealed"` — no client-side virtualization. UI routes live at the root: `/` (overview), `/brain`, `/observations`, `/sessions`, `/projects`, `/prompts`, `/topics`, `/settings`, plus the diagnostic module at `/doctor/sync` and `/doctor/orphans`.

**React islands** handle the two UI surfaces that are not portable to HTMX: the 3D brain graph (React Three Fiber + Three.js + a Web Worker for force-directed layout) and the activity/type charts (recharts). Islands live in `apps/frontend/src/islands/` and each exports a `mount(el, props)` function. They are built by Vite with a multi-entry configuration (one entry per island) to stable paths (`/islands/<name>.js`) and mounted in the browser by a generic `loader.ts` that scans `<div data-island data-props>` elements rendered by templ. Island bundles are embedded via `go:embed` in `internal/web/` alongside the Tailwind-generated CSS; `internal/web/web.go` delegates `/islands/` and `/assets/` to the embedded file server and everything else to the main mux.

i18n is server-side: a Go loader over `internal/ui/locales/{en,es}.json` with a `T(lang, key, args...)` helper; language resolved from the `lang` cookie → `Accept-Language` header → `"en"`. Dark/light theme is applied via a `theme` cookie read at render time in `layout.templ`.

The `*_templ.go` generated files are committed so `go build ./...` works without the `templ` CLI. To regenerate after editing `.templ` files: `go install github.com/a-h/templ/cmd/templ@latest && templ generate ./internal/ui/...`.

---

## First-run onboarding

When you start Engram Explorer for the first time — or any time no data source is active — the dashboard shows an **onboarding screen** instead of an error. It does not exit and it does not require Engram to be installed.

The onboarding screen:

- Lists the Tier-1 providers that were auto-detected (Engram database, Claude Code projects directory).
- Offers a manual path entry field for each provider that was not detected automatically.
- Activates the first provider you configure and transitions directly to the main overview — no restart required.

Once at least one provider is active, the onboarding screen is no longer shown on subsequent starts.

---

## Modules

**Modules** are the data-source providers that Engram Explorer knows how to connect to. You manage them in **Settings → Modules**.

### Tier-1 providers (shipped)

| Provider                 | ID            | What it reads                                    | Default auto-detect path                         |
| ------------------------ | ------------- | ------------------------------------------------ | ------------------------------------------------ |
| **Engram**               | `engram`      | SQLite database written by the Engram daemon     | `~/.engram/engram.db` (or `$ENGRAM_DATA_DIR`)    |
| **Claude Code Sessions** | `cc-sessions` | JSONL session transcripts written by Claude Code | `~/.claude/projects` (or `$CLAUDE_PROJECTS_DIR`) |

Both are Tier-1: if they are not found at their default paths the UI keeps them visible in Settings with a path entry field, so you can point the dashboard at a non-default location without restarting.

### Tier-2 providers (planned)

Tier-2 providers are silent auto-detect only — shown only when detected, no manual path affordance. None ship in the current release; the framework seam is in place for future extensions.

### Provider states

Each provider row in **Settings → Modules** displays one of these state badges:

| Badge          | Meaning                                                                   |
| -------------- | ------------------------------------------------------------------------- |
| **enabled**    | Provider is open and serving requests; its sidebar section is visible     |
| **detected**   | Source was found but the provider has not been activated yet              |
| **disabled**   | User toggled the provider off; handles are closed, sidebar section hidden |
| **errored**    | Provider failed to open; check the path and data source health            |
| **registered** | Provider type is known but detection has not been attempted yet           |

### Activating a provider manually (Tier-1)

1. Open **Settings → Modules**.
2. Find the provider row showing **detected** (not configured) or **registered**.
3. Enter the path to the data source in the path field.
4. Click **Validate & Activate** — the dashboard validates the path before opening it.
5. On success the provider becomes **enabled** and its sidebar section appears immediately; no restart is needed.
6. The path is persisted to `~/.engram/config.json` and survives restarts.

---

## Accounts

**Accounts** are named configuration profiles. Each profile stores a discrete set of provider paths and activation states, so you can maintain separate configurations for (for example) a personal Engram database and a work one.

You manage profiles in **Settings → Accounts**.

### Creating a profile

1. Open **Settings → Accounts**.
2. Enter a name for the new profile and click **Create**.
3. The profile is created with empty provider settings. Go to **Settings → Modules** to configure providers for it.

### Switching profiles

1. Open **Settings → Accounts** (or use the account switcher dropdown in the sidebar header when two or more profiles exist).
2. Click **Switch** next to the profile you want to activate.

Profile switches are **hot-swap** — no restart required. The server closes the current profile's provider handles and opens the new profile's handles. In-flight requests complete against the previous profile before the switch takes effect.

If a profile references a path that no longer exists, the affected provider is marked **unavailable** and an inline warning is shown; other providers in the profile still load normally.

### Default profile

A **default** profile is created automatically on first run. It is seeded from environment variables and the legacy `~/.engram/explorer-settings.json` file if one exists, so existing users see no behavior change after upgrading.

### Persistence

The active profile name and all profile settings are stored in `~/.engram/config.json`. The active profile is restored on restart.

---

## Configuration

### config.json (persistent settings)

Provider paths and account profiles are stored in a JSON file written atomically on every change.

**Location:** `$ENGRAM_DATA_DIR/config.json` (default: `~/.engram/config.json`)

**Schema:**

```json
{
  "version": 1,
  "activeProfile": "default",
  "profiles": {
    "default": {
      "providers": {
        "engram": {
          "enabled": true,
          "path": "/Users/you/.engram/engram.db",
          "daemonUrl": "http://127.0.0.1:7437"
        },
        "cc-sessions": {
          "enabled": true,
          "path": "/Users/you/.claude/projects"
        }
      }
    }
  }
}
```

You do not need to edit this file manually — the Settings UI writes it for you.

### Precedence

When resolving a data-source path or daemon URL, the server applies this order (highest wins):

1. **Environment variable** — `ENGRAM_DATA_DIR`, `CLAUDE_PROJECTS_DIR`, `ENGRAM_DAEMON_URL`, etc.
2. **config.json** — the active profile's per-provider settings.
3. **Legacy `explorer-settings.json`** — read once on first run to seed the default profile; not re-read on subsequent starts.
4. **Built-in default** — `~/.engram/engram.db`, `~/.claude/projects`, port 7437, etc.

Existing users who rely on environment variables see no change — env vars continue to take the highest precedence.

### Environment variables

| Env var                    | Default                 | Purpose                                                                            |
| -------------------------- | ----------------------- | ---------------------------------------------------------------------------------- |
| `ENGRAM_DATA_DIR`          | `~/.engram`             | Directory containing `engram.db`; also the location of `config.json`               |
| `CLAUDE_PROJECTS_DIR`      | `~/.claude/projects`    | Directory where Claude Code stores session transcripts                             |
| `DASHBOARD_HOST`           | `127.0.0.1`             | HTTP bind address                                                                  |
| `DASHBOARD_PORT`           | `8787`                  | HTTP listen port                                                                   |
| `ENGRAM_PORT`              | `7437`                  | Port of the local `engram serve` daemon (used when `ENGRAM_DAEMON_URL` is not set) |
| `ENGRAM_DAEMON_URL`        | `http://127.0.0.1:7437` | Full base URL of the Engram daemon (overrides `ENGRAM_PORT`)                       |
| `ENGRAM_DAEMON_TIMEOUT_MS` | `1500`                  | HTTP timeout for daemon proxy calls (milliseconds)                                 |
| `LOG_LEVEL`                | `info` / `debug` in dev | Logging verbosity: `debug`, `info`, `warn`, `error`                                |
| `ENGRAM_DASH_ENV`          | `production`            | Set to `development` to expose internal error details in API responses             |
| `ENGRAM_DASH_READONLY`     | `false`                 | Set to `true` to disable all mutating routes (returns `503`)                       |

---

## Editing your memory

Engram Explorer is **read-first**: every view works against the read-only pool and never touches your data. On top of that it offers a few **explicit, user-confirmed** edits so you can fix the operational problems it surfaces without dropping to the CLI:

- **Edit an observation** — fix its title, type, `topic_key`, or content.
- **Reassign project** — move a stray observation/session/prompt to the right project.
- **Delete** — soft-delete an observation, or remove an empty session/prompt.
- **Rename / merge a project** — re-tag every row from one project name to another.
- **Import / export** — download a SQLite snapshot, or merge another Engram DB in (a safety backup is written next to your DB first).

Every mutation is confirmed in the UI, goes through a guarded write path, and runs inside a transaction. Because the `engram serve` daemon is the authoritative owner of the database, the rule of thumb is:

> The read path is always safe alongside a running daemon. For **destructive or bulk** edits (project rename/merge, DB import) prefer stopping the daemon first, and keep a backup.

If you'd rather run a strict viewer with **no** write capability at all, start the binary with `ENGRAM_DASH_READONLY=true` — the read-write pool is never opened and every mutating endpoint returns `503`. See [SECURITY.md](./SECURITY.md) for the full concurrency contract.

---

## Tech stack

| Layer             | Stack                                                                                                                      |
| ----------------- | -------------------------------------------------------------------------------------------------------------------------- |
| **Backend**       | Go · `net/http` · `modernc.org/sqlite` (read-only, pure-Go, no CGo) · `log/slog`                                           |
| **UI (primary)**  | templ + HTMX · server-rendered · cursor-based server-side pagination · i18n server-side (en/es) · Tailwind v3 (HSL tokens) |
| **React islands** | React 19 · Vite multi-entry · each island exports `mount(el,props)` · loaded by `loader.ts`                                |
| **Brain island**  | React Three Fiber · Three.js · drei · Web Worker force layout · GPU particle synapses                                      |
| **Charts island** | Recharts · activity-by-project, project-activity, type-breakdown                                                           |
| **Distribution**  | Single binary · `go:embed` · GoReleaser · Homebrew tap                                                                     |

---

## Layout

```
engram-explorer/
├── Makefile                        # Build targets: build (templ+vite+css+embed+go), dev, test, clean
├── go.mod                          # module github.com/AlvaroQ/engram-explorer
├── cmd/
│   ├── engram-explorer/            # Binary entry point — config → container → serve
│   └── seed-demo/                  # Demo database seeder (generates ./demo/engram.db)
├── internal/
│   ├── config/                     # Env-var config, ProfileStore, EnsureConfig (config.json read/write)
│   ├── httpapi/                    # net/http mux, routes_*.go, middleware, container (JSON API /api/*)
│   ├── providers/                  # Provider port + Registry (ordered, state-machine, hot-swap)
│   │   ├── engram/                 # Engram provider adapter (SQLite, daemon proxy, health)
│   │   ├── ccsessions/             # Claude Code Sessions provider adapter (JSONL reader)
│   │   └── providertest/           # FakeProvider + test helpers
│   ├── ui/                         # templ+HTMX server-rendered UI (all pages): handlers_*.go, *.templ, i18n.go
│   │   ├── locales/                # en.json, es.json
│   │   └── static/                 # app.css (Tailwind CLI output), styles.css, htmx.min.js
│   ├── doctor/                     # Diagnostic module: /doctor/sync, /doctor/orphans (templ+HTMX)
│   ├── services/                   # SQL service layer (observations, sessions, sync, …)
│   ├── sqlite/                     # Read-only + read-write connection pools (modernc.org/sqlite)
│   ├── daemon/                     # HTTP client for the engram serve daemon
│   ├── cursor/                     # Opaque cursor-based pagination
│   ├── fts/                        # FTS5 query sanitizer
│   ├── logging/                    # log/slog setup
│   └── web/                        # go:embed of built islands+assets; routes /islands/, /assets/ to embedded FS
├── apps/
│   └── frontend/                   # React islands via Vite multi-entry (build-time only)
│       └── src/
│           ├── islands/            # brain.tsx, *-chart.tsx, brain-preview-card.tsx (each exports mount(el,props))
│           ├── components/brain/   # R3F 3D neural graph (used by brain island)
│           ├── components/charts/  # recharts wrappers (used by chart islands)
│           └── loader.ts           # mounts islands from <div data-island data-props>
└── testdata/
    └── schema/engram-schema.sql    # Reference schema used by Go integration tests
```

---

## Build targets

| Target          | What it does                                                                                           |
| --------------- | ------------------------------------------------------------------------------------------------------ |
| `make build`    | Full build: `templ generate` → `vite build` (islands) → `build:ui-css` (Tailwind) → embed → `go build` |
| `make frontend` | Build React island bundles only (`pnpm -F @engram-explorer/frontend build`)                            |
| `make embed`    | Copy `apps/frontend/dist/` into `internal/web/dist/` for `go:embed`                                    |
| `make dev`      | Run the Go binary in dev mode (`go run ./cmd/engram-explorer`)                                         |
| `make test`     | Run all Go tests (`go test ./...`)                                                                     |
| `make clean`    | Remove the binary and the embedded dist copy                                                           |

Development workflow:

```bash
# Rebuild everything and run
make build && ./engram-explorer

# Or run directly (frontend must be built first)
make frontend embed
go run ./cmd/engram-explorer

# Hot-reload island changes only (Vite dev server, proxies to :8787)
# Terminal 1: make dev (Go server)
# Terminal 2: pnpm -F @engram-explorer/frontend dev
```

---

## Troubleshooting

### No data sources detected on first run

The dashboard starts without any data source and shows the **onboarding screen** instead of crashing. From there you can activate Engram or Claude Code Sessions (see [First-run onboarding](#first-run-onboarding)).

If you have Engram installed but the dashboard does not detect it automatically:

1. Go to **Settings → Modules** and enter the path to your `engram.db` file manually.
2. Or set `ENGRAM_DATA_DIR=/path/to/dir` before starting the binary.

To get a real Engram database: `brew install gentleman-programming/tap/engram && engram serve`.

To try the dashboard with demo data: `go run ./cmd/seed-demo`, then `ENGRAM_DATA_DIR=./demo ./engram-explorer`.

### Daemon offline

**Dashboard shows "daemon offline"** — the local `engram serve` daemon is not reachable. The dashboard degrades gracefully: all read views remain fully functional against the SQLite file. Start the daemon with `engram serve` to recover the live `/health`, `/sync/status`, and `/stats` panels.

**`/api/sync/issues` shows `DAEMON_DOWN HIGH`** — same root cause. The diagnostic engine surfaces this as a blocking issue because cloud sync operations require the daemon.

### Port already in use

**`address already in use`** — another process is bound to port 8787.

- macOS / Linux: `lsof -ti :8787 | xargs kill -9`
- Windows: `netstat -ano | findstr :8787`, then `taskkill /F /PID <id>`

To use a different port: `DASHBOARD_PORT=9090 ./engram-explorer`.

### Sync diagnostics

**Project shows `SYNC_BROKEN HIGH`** — `lifecycle='pending'` with `last_acked_seq=0` while `last_enqueued_seq>0`. Mutations are queued but never acknowledged. Common causes: cloud token not set, network partition, or cloud-side rejection. Check `last_error` / `reason_message` on `/sync/<project>`.

**Project with empty name (`""`) flagged** — observations were captured without a project context. This is an upstream capture bug in the agent or MCP that emitted them; the dashboard cannot fix it. Inspect the `session_id` of the affected rows and patch the captor.

### Cloud mutations

**`POST /api/cloud/enroll` returns `CLI_NOT_FOUND`** — `engram` is not on `$PATH` for the user running the dashboard. Install the binary system-wide or run the dashboard from a shell where `engram --version` resolves.

**`429 Rate limit exceeded`** — cloud mutation endpoints are rate-limited. The `Retry-After` header indicates how long to wait.

---

## Releases & changelog

Versioned binaries and per-release changelogs are published on [GitHub Releases](https://github.com/AlvaroQ/engram-explorer/releases). Releases are built with GoReleaser from `v*` tags and include macOS, Linux, and Windows artifacts plus a Homebrew tap.

---

## License

MIT — see [LICENSE](./LICENSE).

Built on top of [Engram](https://github.com/Gentleman-Programming/engram) by Gentleman Programming.
