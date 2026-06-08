# Engram Explorer

> A local-first web dashboard that turns your Engram memory database into something you can **see** — including a 3D neural map of every memory your agents have ever stored.

<p align="center">
  <img src="https://img.shields.io/badge/Go-1.23+-00ADD8?logo=go&logoColor=white" alt="Go 1.23+" />
  <img src="https://img.shields.io/badge/React-19-61DAFB?logo=react&logoColor=black" alt="React 19" />
  <img src="https://img.shields.io/badge/Three.js-r184-000000?logo=threedotjs&logoColor=white" alt="Three.js" />
  <img src="https://img.shields.io/badge/SQLite-read--only-003B57?logo=sqlite&logoColor=white" alt="SQLite read-only" />
  <img src="https://img.shields.io/badge/single--binary-distribution-4A90E2" alt="single-binary" />
  <img src="https://img.shields.io/badge/license-MIT-green" alt="MIT license" />
</p>

![Engram Explorer Dashboard overview](docs/engram-explorer.png)

Engram stores your agents' long-term memory in a local SQLite database (`~/.engram/engram.db`). The official TUI/CLI is great for capture and recall — but it can't *show* you the shape of that memory, nor the operational problems hiding inside it: orphan projects, broken cloud sync, thousands of pending mutations, observations captured without a project. This dashboard reads that database **directly and read-only** (so it never blocks the daemon's writes) and surfaces all of it across 13 views — projects, sessions, observations, prompts, topics, sync health, and diagnostics — in both **Spanish and English**, light or dark.

The distribution philosophy matches Engram itself: **one binary, one SQLite file.** No Node.js runtime, no WebView2, no sidecar. Download the binary for your platform, point it at your database, and open your browser.

---

## The Brain — your memory as a 3D neural map

![The Brain — 3D force-directed graph of all observations](docs/brain_3d.png)

This is the centerpiece. **The Brain renders every observation in your database as a node in a live, force-directed 3D graph** — and the way it organizes them is by **project**.

- **Each project becomes a lobe.** Observations from the same project cluster together in space, forming the colored constellations you see above — one vivid region per project, dozens of them, each its own part of the brain.
- **Each sphere is a single memory.** Its size reflects weight (and how many duplicates were deduped into it); its color encodes either the **project** or the **observation type** (a toggle you control).
- **Synapses connect related memories.** Edges between observations carry GPU-driven particle flow — the brain literally "breathes" — with relationship and confidence baked into how each connection looks.
- **Hover any neuron to read it.** A detail card surfaces the memory's title, tags, project, and date right in the 3D scene (see the floating card above); click through for the full markdown content and `[[wikilinks]]`.
- **It's fast by design.** The force-directed layout runs off-main-thread in a Web Worker, rendering uses a single instanced mesh on a demand frameloop, and particle flow is GPU-driven — so the whole brain stays smooth even with hundreds of nodes on screen.

> The Brain is evolving from this flat project-clustered view into a **deep, drillable neural architecture** — switchable views (Lobes / Topics / Organic), immersive zoom into "neurons" that store clusters of memory, and stackable segmentation facets.

---

## What it surfaces

| View | What you get |
| --- | --- |
| **Overview** | At-a-glance counts (projects, sessions, observations, prompts), activity-by-project chart, type distribution, and sync health |
| **Brain** | The 3D neural map above |
| **Projects** | Per-project memory, activity, and sync state |
| **Project detail** | Full memory and session timeline for a single project |
| **Observations** | Every memory, filterable, with full markdown + wikilinks |
| **Sessions** | Session timeline and per-session detail |
| **Prompts** | Captured user prompts, searchable |
| **Topics** | Memory grouped by stable `topic_key` |
| **Sync Health** | Cloud enrollment status: enrolled / healthy / broken / pending mutations |
| **Sync project** | Per-project cloud sync detail and mutation queue |
| **Orphans** | Observations captured without a project — the capture bugs the CLI can't show |
| **Diagnostics** | Orphan projects, broken sync — a diagnostic engine across all 13 views |
| **Settings** | UI preferences, language, theme |

---

## Requirements

### To run a pre-built binary

- The `engram-explorer` binary for your platform (macOS, Linux, Windows)
- An existing Engram install: `~/.engram/engram.db` — **or** use the demo seeder (see Quick Start)
- No Node.js, no runtime dependencies

### To build from source

- Go 1.23 or later (`go.mod` requires `go 1.26.1` — the toolchain directive; the minimum supported release is 1.23)
- Node.js 20+ and pnpm 10+ (to build the React frontend; not needed at runtime)

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

Engram UI is a **single Go binary** (`cmd/engram-explorer`) that embeds the compiled React SPA via `go:embed` and serves both the JSON API and the frontend assets on one port (default `127.0.0.1:8787`). The backend opens the Engram SQLite database with a **read-only** handle using `modernc.org/sqlite` — a pure-Go driver with no CGo — so it coexists safely with the live WAL written by the daemon. API routes (`/api/*`) are handled by an `internal/httpapi` mux built on Go's standard `net/http`; every other path is served from the embedded `dist/` filesystem with SPA deep-link fallback. The backend proxies the daemon's HTTP telemetry endpoints (`/health`, `/sync/status`, `/stats`) and — when the user explicitly confirms a cloud mutation — shells out to the `engram` CLI as a subprocess. It **never** writes to data tables.

---

## Tech stack

| Layer | Stack |
| --- | --- |
| **Backend** | Go · `net/http` · `modernc.org/sqlite` (read-only, pure-Go, no CGo) · `log/slog` |
| **Frontend** | React 19 · Vite · TanStack Query / Router / Table / Virtual · Tailwind v3 (HSL tokens) · Recharts · i18next (ES/EN) |
| **Brain** | React Three Fiber 9 · Three.js r184 · drei · Web Worker force layout · GPU particle synapses |
| **Distribution** | Single binary · `go:embed` · GoReleaser · Homebrew tap |

---

## Layout

```
engram-explorer/
├── Makefile                        # Build targets: frontend, embed, build, dev, test, clean
├── go.mod                          # module github.com/AlvaroQ/engram-explorer
├── cmd/
│   ├── engram-explorer/            # Binary entry point — config → container → serve
│   └── seed-demo/                  # Demo database seeder (generates ./demo/engram.db)
├── internal/
│   ├── config/                     # Env-var config (Config struct + Load())
│   ├── httpapi/                    # net/http mux, routes, middleware, container
│   ├── services/                   # SQL service layer (observations, sessions, sync, …)
│   ├── sqlite/                     # Read-only connection pool (modernc.org/sqlite)
│   ├── daemon/                     # HTTP client for the engram serve daemon
│   ├── cursor/                     # Opaque cursor-based pagination
│   ├── fts/                        # FTS5 query sanitizer
│   ├── logging/                    # log/slog setup
│   └── web/                        # go:embed wrapper — serves dist/ + SPA fallback
├── apps/
│   └── frontend/                   # React 19 + Vite + TanStack + Tailwind + R3F (Brain)
│       └── src/
│           ├── components/brain/   # R3F 3D neural graph
│           └── pages/              # 13 views (overview, brain, projects, sessions, …)
└── testdata/
    └── schema/engram-schema.sql    # Reference schema used by Go integration tests
```

---

## Configuration

All configuration is via environment variables. Defaults work out of the box for a standard Engram install.

| Env var | Default | Purpose |
| --- | --- | --- |
| `ENGRAM_DATA_DIR` | `~/.engram` | Directory where `engram.db` lives |
| `DASHBOARD_PORT` | `8787` | HTTP listen port |
| `DASHBOARD_HOST` | `127.0.0.1` | HTTP bind address (LAN exposure not recommended) |
| `ENGRAM_PORT` | `7437` | Port of the local `engram serve` daemon |
| `ENGRAM_DAEMON_URL` | `http://127.0.0.1:7437` | Full base URL of the daemon (overrides `ENGRAM_PORT`) |
| `LOG_LEVEL` | `info` (prod) / `debug` (dev) | Logging verbosity: `debug`, `info`, `warn`, `error` |
| `ENGRAM_DASH_ENV` | `production` | Set to `development` to expose internal error details in API responses |

---

## Build targets

| Target | What it does |
| --- | --- |
| `make build` | Full build: React frontend → embed into Go → compile binary |
| `make frontend` | Compile the React app only (`pnpm -F @engram-explorer/frontend build`) |
| `make embed` | Copy `apps/frontend/dist/` into `internal/web/dist/` for `go:embed` |
| `make dev` | Run the Go binary in dev mode (`go run ./cmd/engram-explorer`) |
| `make test` | Run all Go tests (`go test ./...`) |
| `make clean` | Remove the binary and the embedded dist copy |

Frontend dev workflow (hot reload):

```bash
# Terminal 1 — Go API server
make dev

# Terminal 2 — Vite dev server with proxy to :8787
pnpm -F @engram-explorer/frontend dev
```

---

## Troubleshooting

### Database not found

**`failed to open Engram database: ...`** — the binary looks for `engram.db` in `ENGRAM_DATA_DIR` (default `~/.engram`). Two options:

1. **No Engram yet?** Run `go run ./cmd/seed-demo` to generate `./demo/engram.db`, then `ENGRAM_DATA_DIR=./demo ./engram-explorer`.
2. **Engram installed elsewhere?** Set `ENGRAM_DATA_DIR=/path/to/dir`.

The binary does **not** create the database. To get a real one: `brew install gentleman-programming/tap/engram && engram serve`.

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

## License

MIT — see [LICENSE](./LICENSE).

Built on top of [Engram](https://github.com/gentleman-programming/engram) by Gentleman Programming.
