<div align="center">

# 🧠 Engram Explorer

### See your agent's brain.

A local-first dashboard for [**Engram**](https://github.com/Gentleman-Programming/engram) — explore your AI agent's long-term memory as a live 3D neural map, diagnose sync problems, and fix capture bugs the CLI can't even show you.

[![Release](https://img.shields.io/github/v/release/AlvaroQ/engram-explorer?style=for-the-badge&logo=github&color=6d28d9)](https://github.com/AlvaroQ/engram-explorer/releases/latest)
[![Downloads](https://img.shields.io/github/downloads/AlvaroQ/engram-explorer/total?style=for-the-badge&color=059669)](https://github.com/AlvaroQ/engram-explorer/releases)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue?style=for-the-badge)](./LICENSE)
[![Go 1.25+](https://img.shields.io/badge/Go-1.25+-00ADD8?style=for-the-badge&logo=go&logoColor=white)](https://go.dev)

<br />

[**⬇️ Download**](#-download--run-90-seconds) · [**🧠 The Brain**](#-the-brain--your-memory-as-a-3d-neural-map) · [**📊 What it shows**](#-what-it-surfaces) · [**🛠️ Build from source**](#-build-from-source)

<br />

![Engram Explorer Dashboard overview](docs/engram-explorer.png)

</div>

---

## ⚡ Why this exists

Engram stores your agents' long-term memory in a local SQLite database (`~/.engram/engram.db`). The official TUI/CLI is great for **capture and recall** — but it can't _show_ you the **shape** of that memory, nor the operational problems hiding inside it: orphan projects, broken cloud sync, thousands of pending mutations, observations captured without a project.

Engram Explorer reads that database **directly** — read-only by default, so it never blocks the daemon's writes — and surfaces all of it across **13 views** in **Spanish and English**, light or dark. It can also apply targeted, **user-confirmed** fixes (reassign a stray observation, rename a project, prune junk) through a guarded write path you can switch off entirely.

> **One binary, one SQLite file.** No Node.js runtime, no WebView2, no sidecar. Download the binary for your platform, point it at your database, open your browser.

---

## 📥 Download & run (90 seconds)

You **don't need to clone or compile anything**. Grab the binary for your OS from the [latest release](https://github.com/AlvaroQ/engram-explorer/releases/latest) and run it.

| Platform       | Download                                                                                                                                                                                                                                 | Run it                                                                        |
| -------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------- |
| 🍎 **macOS**   | [`engram-explorer_*_darwin_arm64.tar.gz`](https://github.com/AlvaroQ/engram-explorer/releases/latest) (Apple Silicon)<br />[`engram-explorer_*_darwin_amd64.tar.gz`](https://github.com/AlvaroQ/engram-explorer/releases/latest) (Intel) | `tar -xzf engram-explorer_*.tar.gz`<br />`./engram-explorer`                  |
| 🐧 **Linux**   | [`engram-explorer_*_linux_amd64.tar.gz`](https://github.com/AlvaroQ/engram-explorer/releases/latest) (or `arm64`)                                                                                                                        | `tar -xzf engram-explorer_*.tar.gz`<br />`./engram-explorer`                  |
| 🪟 **Windows** | [`engram-explorer_*_windows_amd64.zip`](https://github.com/AlvaroQ/engram-explorer/releases/latest)                                                                                                                                      | Unzip → double-click `engram-explorer.exe`<br />_(or run it from PowerShell)_ |

Then open **<http://127.0.0.1:8787>** in your browser. That's it. 🎉

### 🍺 One-liner for macOS / Linux (Homebrew)

If you have [Homebrew](https://brew.sh):

```bash
brew install AlvaroQ/tap/engram-explorer
engram-explorer
```

> **On Windows?** There is no Homebrew step — and you don't need one. Homebrew on Windows requires WSL2 and adds friction for zero benefit. Just download the `.zip` from the table above, unzip it, and run the `.exe`. A single native binary, exactly like macOS and Linux.

### 🧩 Do I need Engram installed?

**No — it's optional.** The dashboard starts even with no data source and walks you through setup on first run (see [First-run onboarding](#first-run-onboarding)). But to see _your own_ memory, install Engram:

```bash
# macOS / Linux
brew install gentleman-programming/tap/engram
engram serve   # creates ~/.engram/engram.db on first run
```

> The `engram serve` daemon is **not required** to run the dashboard — it reads the SQLite file directly and degrades gracefully when the daemon is offline. Only the live sync/health panels need it.

### 👀 Just want to look around first? (demo data)

No Engram, no real data needed — seed a realistic demo dataset and explore every view. Requires [building from source](#-build-from-source):

```bash
make demo   # seeds ./demo (engram.db + sample Claude Code and Codex sessions) and runs read-only against it
```

`make demo` is a normal run with four environment variables — `ENGRAM_DATA_DIR=./demo`,
`CLAUDE_PROJECTS_DIR=./demo/claude`, `CODEX_HOME=./demo/codex` and `ENGRAM_DASH_READONLY=true`. The seeder
writes sample Claude Code transcripts under `./demo/claude` and sample Codex rollouts under `./demo/codex`.
Engram write actions answer `503`, and Settings changes are saved in `./demo/config.json`. That file may still
record the default `~/.claude/projects` and `~/.codex` paths: the `CLAUDE_PROJECTS_DIR` and `CODEX_HOME`
overrides are what keep the demo away from your real sessions (and `ENGRAM_DATA_DIR` from `~/.engram`), so
running the binary against `./demo` without them reads your real `~/.claude/projects` and `~/.codex`.

---

## 🧠 The Brain — your memory as a 3D neural map

![The Brain — 3D force-directed graph of all observations](docs/brain_3d.png)

This is the centerpiece. **The Brain renders every observation in your database as a node in a live, force-directed 3D graph** — organized by **project**.

- 🌐 **Each project becomes a lobe.** Observations from the same project cluster together in space — one vivid colored region per project, dozens of them, each its own part of the brain.
- 🔵 **Each sphere is a single memory.** Its size reflects weight (and how many duplicates were deduped into it); its color encodes either the **project** or the **observation type** — a toggle you control.
- ⚡ **Synapses connect related memories.** Edges carry GPU-driven particle flow — the brain literally "breathes" — with relationship and confidence baked into how each connection looks.
- 🔎 **Hover any neuron to read it.** A detail card surfaces the memory's title, tags, project, and date right in the 3D scene; click through for the full markdown and `[[wikilinks]]`.
- 🚀 **Fast by design.** Force layout runs off-main-thread in a Web Worker, rendering uses a single instanced mesh on a demand frameloop, particles are GPU-driven — smooth even with hundreds of nodes on screen.

> The Brain keeps evolving toward a **deeper, drillable neural architecture** — immersive navigation into the clusters your memory forms as it grows.

---

## 📊 What it surfaces

14 views, all server-rendered, all in EN/ES + light/dark:

| View                  | What you get                                                                                                                                                                                           |
| --------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| 🏠 **Overview**       | Knowledge-graph hero with memory search that collapses into a bar as you scroll; KPIs (memories, projects, AI conversations and agent spend this month per agent); activity, types and recent memories |
| 🧠 **Brain**          | The 3D neural map above                                                                                                                                                                                |
| 📁 **Projects**       | Per-project memory, activity, and sync state                                                                                                                                                           |
| 📄 **Project detail** | Full memory and session timeline for a single project                                                                                                                                                  |
| 💭 **Observations**   | Every memory, filterable, with full markdown + wikilinks                                                                                                                                               |
| 🕑 **Sessions**       | Session timeline and per-session detail                                                                                                                                                                |
| ✍️ **Prompts**        | Captured user prompts, searchable                                                                                                                                                                      |
| 🏷️ **Topics**         | Memory grouped by stable `topic_key`                                                                                                                                                                   |
| ☁️ **Sync Health**    | Read-only cloud sync diagnostics: enrolled / healthy / broken / pending mutations                                                                                                                      |
| 🔄 **Sync project**   | Per-project cloud sync detail and mutation queue (read-only)                                                                                                                                           |
| 🧹 **Orphans**        | Observations captured without a project — the capture bugs the CLI can't show                                                                                                                          |
| 🩺 **Diagnostics**    | Orphan projects, broken sync — a diagnostic engine across all views                                                                                                                                    |
| 🤖 **Agent sessions** | Claude Code and Codex sessions: usage overview, filterable list, transcript with tool calls                                                                                                            |
| ⚙️ **Settings**       | UI preferences, language, theme, Modules (provider toggles), Accounts (profiles)                                                                                                                       |

---

## 🔗 Relationship to Engram

Engram Explorer is a **community companion** — a read-only viewer and operational dashboard built on top of [Engram](https://github.com/Gentleman-Programming/engram). It is **not** an official plugin, not a fork, and not affiliated with Gentleman Programming.

The official plugin ecosystem talks to the daemon over HTTP at `127.0.0.1:7437`. Engram Explorer takes a different approach: it reads the SQLite file directly in **WAL read-only mode**, so it runs whether or not the daemon is active and **never competes with `engram serve` as the authoritative owner of the database**. When the daemon is running, the dashboard respects it — it only proxies the daemon's own telemetry endpoints (`/health`, `/sync/status`, `/stats`). Cloud sync is **read-only**: the dashboard diagnoses enrollment and sync state but never enrolls, unenrolls or syncs a project — do that with the `engram` CLI directly.

> The CLI captures and recalls; this shows you the _shape_ of what your agent has remembered. It complements the official tooling — it doesn't replace it.

---

## 🛠️ Build from source

> Only needed if you want to contribute, run the demo seeder, or build a custom binary. **Most users should just [download a release](#-download--run-90-seconds).**

### Requirements

- **Go 1.25+** — matches the `go` directive in `go.mod` (minimum required by `modernc.org/sqlite` and `golang.org/x/sync`)
- **Node.js 20+ and pnpm 10+** — to build the React island bundles (not needed at runtime)
- **`templ` CLI** — optional; generated `*_templ.go` files are committed, so `go build ./...` works without it

### Path A — Try it with demo data (no Engram required)

```bash
git clone https://github.com/AlvaroQ/engram-explorer.git
cd engram-explorer

make build                                  # frontend → embed → go build
go run ./cmd/seed-demo --out ./demo --force # engram.db + sample Claude Code and Codex sessions in ./demo
ENGRAM_DATA_DIR=./demo CLAUDE_PROJECTS_DIR=./demo/claude CODEX_HOME=./demo/codex ENGRAM_DASH_READONLY=true ./engram-explorer
# open http://127.0.0.1:8787   (`make demo` does the last two steps with `go run`)
```

### Path B — Use your real Engram database

```bash
git clone https://github.com/AlvaroQ/engram-explorer.git
cd engram-explorer

make build            # frontend → embed → go build
./engram-explorer     # reads ~/.engram/engram.db by default
# open http://127.0.0.1:8787
```

Health check: `curl http://127.0.0.1:8787/api/health`

### Run without building the binary (Go installed)

```bash
make frontend embed          # frontend must be built first for the Go embed
go run ./cmd/engram-explorer
```

### Build targets

| Target          | What it does                                                                              |
| --------------- | ----------------------------------------------------------------------------------------- |
| `make build`    | Full build: `templ generate` → `vite build` (islands) → Tailwind CSS → embed → `go build` |
| `make frontend` | Build React island bundles only                                                           |
| `make embed`    | Copy `apps/frontend/dist/` into `internal/web/dist/` for `go:embed`                       |
| `make dev`      | Run the Go binary in dev mode                                                             |
| `make test`     | Run all Go tests                                                                          |
| `make clean`    | Remove the binary and the embedded dist copy                                              |

---

## 🏗️ Architecture

Engram Explorer is a **single Go binary** (`cmd/engram-explorer`) serving all views on one port (default `127.0.0.1:8787`). It opens the Engram SQLite database with two `modernc.org/sqlite` handles (pure-Go, no CGo): a **read-only** pool (`mode=ro` + `query_only`) that serves every view and coexists safely with the live WAL written by the daemon, and a single-connection **read-write** pool that powers a small set of explicit, user-confirmed mutations. API routes (`/api/*`) run on an `internal/httpapi` mux built on Go's standard `net/http`. The backend proxies the daemon's GET telemetry endpoints (`/health`, `/sync/status`, `/stats`); cloud sync is read-only and the dashboard never spawns the `engram` CLI. Set `ENGRAM_DASH_READONLY=true` to disable the write path entirely.

**The primary UI is server-rendered** using [templ](https://templ.guide) and [HTMX](https://htmx.org) in `internal/ui`. Go handlers load data via the services layer → render templ templates (full-page on navigation, partial fragment on HTMX requests via the `HX-Request` header). Interactivity (filters, pagination, drawers) is driven by `hx-get`/`hx-post`/`hx-swap`. A 4xx/5xx response never replaces page content: `static/toasts.js` shows it as a toast instead (the `.inline-error-message` of an `ErrorPartial`, or `error.message` of a JSON error). Long lists use server-side cursor-based pagination with `hx-trigger="revealed"` — no client-side virtualization.

**React islands** handle the two surfaces not portable to HTMX: the 3D brain graph (React Three Fiber + Three.js + a Web Worker for force layout) and the activity/type charts (Recharts). Islands live in `apps/frontend/src/islands/`, each exporting a `mount(el, props)` function, built by Vite (multi-entry) to stable paths (`/islands/<name>.js`) and embedded via `go:embed` in `internal/web/`.

i18n is server-side: a Go loader over `internal/ui/locales/{en,es}.json` with `T(lang, key, args...)`; language resolved from the `lang` cookie → `Accept-Language` header → `"en"`. Theme is applied via a `theme` cookie read at render time.

### Tech stack

| Layer             | Stack                                                                                 |
| ----------------- | ------------------------------------------------------------------------------------- |
| **Backend**       | Go · `net/http` · `modernc.org/sqlite` (read-only, pure-Go, no CGo) · `log/slog`      |
| **UI (primary)**  | templ + HTMX · server-rendered · cursor pagination · i18n (en/es) · Tailwind v3       |
| **React islands** | React 19 · Vite multi-entry · each island exports `mount(el,props)`                   |
| **Brain island**  | React Three Fiber · Three.js · drei · Web Worker force layout · GPU particle synapses |
| **Charts island** | Recharts · activity-by-project, project-activity, type-breakdown                      |
| **Distribution**  | Single binary · `go:embed` · GoReleaser · Homebrew tap                                |

---

## 🚦 First-run onboarding

When you start Engram Explorer for the first time — or any time no data source is active — the dashboard shows an **onboarding screen** instead of an error. It does not exit and does not require Engram to be installed.

It lists the auto-detected Tier-1 providers (Engram database, Claude Code projects directory), offers a manual path field for any not detected, and activates the first provider you configure — transitioning straight to the overview, no restart required.

---

## 🧩 Modules

**Modules** are the data-source providers Engram Explorer can connect to. Manage them in **Settings → Modules**.

| Provider                 | ID               | What it reads                                                    | Default auto-detect path                         |
| ------------------------ | ---------------- | ---------------------------------------------------------------- | ------------------------------------------------ |
| **Engram**               | `engram`         | SQLite database written by the Engram daemon                     | `~/.engram/engram.db` (or `$ENGRAM_DATA_DIR`)    |
| **Claude Code Sessions** | `cc-sessions`    | JSONL session transcripts written by Claude Code                 | `~/.claude/projects` (or `$CLAUDE_PROJECTS_DIR`) |
| **Codex**                | `codex-sessions` | Local Codex CLI rollouts (`sessions/YYYY/MM/DD/rollout-*.jsonl`) | `~/.codex` (or `$CODEX_HOME`)                    |

All three are **Tier-1**: if not found at their default paths the UI keeps them visible with a path entry field, so you can point the dashboard at a non-default location without restarting. **Tier-2 providers** (planned) are silent auto-detect only; none ship in the current release.

**Provider states:** `enabled` (open and serving) · `detected` (found, not yet activated) · `disabled` (toggled off) · `errored` (failed to open — check the path) · `registered` (known, detection not attempted).

**Disabled modules:** routes are mounted once at startup and follow the module's live state. While a module is disabled its API routes answer `503` with code `MODULE_DISABLED` and its pages show "This module is disabled. Enable it in Settings → Modules."; enabling it serves them again without a restart. A request that is already running when a module is disabled or the profile changes finishes on the data source it started with.

**Limits:** that grace is bounded. A superseded data source waits at most **30 s** (the drain timeout) for the requests still using it, then it is closed and a request still running on it fails. The HTTP server's write timeout is also **30 s**, but it only stops the response: the handler is not cancelled, so a database import that needs more time can still complete on the server while the client sees a connection error. Reload to check the result before retrying.

**Agent sessions (Claude Code + Codex).** Both session modules feed one set of vendor-neutral pages, under a
"Agents" sidebar group with one entry per live agent (`/agent-sessions?agent=<kind>`):

| Route                                                    | What it shows                                                                                                                                                                        |
| -------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `/agent-sessions`                                        | Usage overview: totals, cost per agent / project / day, tokens per model, most expensive sessions; `?agent=<kind>` (tabs: All agents, one per live agent) scopes it to one agent     |
| `/agent-sessions/list`                                   | Every session; filters `agent`, `source`, `project`, `model`, `q`, `from`/`to` (days, UTC), `sort`/`order`; 50 rows per page with **Load more** (`page` still links a numbered page) |
| `/agent-sessions/claude-code/<account>/<project>/<id>`   | One Claude Code session (served by the Claude Code module, one source per enabled account)                                                                                           |
| `/agent-sessions/codex/<YYYY>/<MM>/<DD>/<rollout>.jsonl` | One Codex session (served by the Codex module)                                                                                                                                       |
| `GET /api/agent-sessions/stats`                          | The same aggregate as JSON, cached until a session file changes                                                                                                                      |

Token counts use **disjoint buckets**: uncached input, cache read, cache write and output add up to the total;
reasoning tokens are part of output and shown apart. Codex reports cached tokens inside `input_tokens`, so the
explorer subtracts them; usage is deduplicated by `response_id` (or by the cumulative `token_count` totals of
older files), and a resumed or forked session never counts replayed usage twice. Costs use public list prices;
a model without a published price counts as unknown (cost `0`, shown with `~`). The Codex module only reads the
rollout files under `<CODEX_HOME>/sessions`: never your Codex configuration, credentials or history, and it
never runs the `codex` CLI. Change its home in Settings → Modules (Codex row → path → Apply); `CODEX_HOME`, when
set, still wins at startup. The old Claude Code URLs (`/cc-overview`, `/cc-sessions…`) redirect to these pages,
and `GET /api/cc-sessions/stats` remains as a deprecated Claude-only endpoint with its previous shape.

**To activate a Tier-1 provider manually:** Settings → Modules → find the `detected`/`registered` row → enter the path → **Validate & Activate**. On success it becomes `enabled` and its sidebar section appears immediately; the path is persisted to `~/.engram/config.json`.

---

## 👤 Accounts

**Accounts** are named configuration profiles. Each stores a discrete set of provider paths and activation states, so you can keep separate setups for (say) a personal Engram database and a work one. Manage them in **Settings → Accounts**.

- **Create:** Settings → Accounts → name it → **Create** → configure providers in Settings → Modules.
- **Switch:** click **Switch** next to the profile (or use the sidebar account switcher when ≥2 profiles exist). Switches are **hot-swap** — no restart; in-flight requests complete against the previous profile first.
- **Default profile:** created on first run from the legacy `~/.engram/explorer-settings.json` if present, otherwise from the built-in defaults. Environment variables are applied on top at startup and are not written into it.
- **Persistence:** active profile name and all settings live in `~/.engram/config.json` and are restored on restart.

If a profile references a path that no longer exists, that provider is marked **unavailable** with an inline warning; other providers still load normally.

---

## ⚙️ Configuration

### Command-line flags

| Flag                         | Purpose                                     |
| ---------------------------- | ------------------------------------------- |
| `--version`, `-v`, `version` | Print `engram-explorer <version>` and exit. |

The former demo mode (`--demo`, `--demo-db`, `ENGRAM_DEMO`, `ENGRAM_DEMO_DB`) was removed: the binary refuses to
start when one of them is set and points to `make demo`. The demo dataset is not shipped with release binaries —
generate it with `go run ./cmd/seed-demo` (or `make seed-demo`).

### Environment variables

| Env var                    | Default                 | Purpose                                                                                                                                           |
| -------------------------- | ----------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------- |
| `ENGRAM_DATA_DIR`          | `~/.engram`             | Directory containing `engram.db`; also the location of `config.json`                                                                              |
| `CLAUDE_PROJECTS_DIR`      | `~/.claude/projects`    | Directory where Claude Code stores session transcripts                                                                                            |
| `CODEX_HOME`               | `~/.codex`              | Codex CLI home; the Codex module reads the rollouts under `$CODEX_HOME/sessions` (in memory only, never saved)                                    |
| `DASHBOARD_HOST`           | `127.0.0.1`             | HTTP bind address; must be loopback unless `DASHBOARD_ALLOW_REMOTE=true`                                                                          |
| `DASHBOARD_ALLOW_REMOTE`   | `false`                 | Set to `true` to allow a non-loopback `DASHBOARD_HOST` (no authentication!)                                                                       |
| `DASHBOARD_PORT`           | `8787`                  | HTTP listen port                                                                                                                                  |
| `ENGRAM_PORT`              | `7437`                  | Port of the local `engram serve` daemon (used when `ENGRAM_DAEMON_URL` is not set)                                                                |
| `ENGRAM_DAEMON_URL`        | `http://127.0.0.1:7437` | Full base URL of the Engram daemon (overrides `ENGRAM_PORT`)                                                                                      |
| `ENGRAM_DAEMON_TIMEOUT_MS` | `1500`                  | HTTP timeout for daemon proxy calls (milliseconds)                                                                                                |
| `LOG_LEVEL`                | `info` / `debug` in dev | Logging verbosity: `debug`, `info`, `warn`, `error`                                                                                               |
| `ENGRAM_DASH_ENV`          | `production`            | Set to `development` to expose internal error details in API responses and trust the Vite dev server origins (`localhost:5173`, `127.0.0.1:5173`) |
| `ENGRAM_DASH_READONLY`     | `false`                 | Set to `true` to disable all mutating routes (returns `503`)                                                                                      |

### config.json (persistent settings)

Provider paths and account profiles are stored at `$ENGRAM_DATA_DIR/config.json` (default `~/.engram/config.json`), written atomically on every change. **You don't need to edit it manually — the Settings UI writes it for you.**

**Precedence** (highest wins): **1.** environment variable (`ENGRAM_DATA_DIR`, `CLAUDE_PROJECTS_DIR`, `CODEX_HOME`, `ENGRAM_DAEMON_URL`), applied in memory at startup and never saved → **2.** the active profile in `config.json`, the only file the explorer writes → **3.** built-in default. The legacy `explorer-settings.json` is read once, only to create `config.json` on first run, and is never written. While an environment variable is set it wins over a path saved in Settings; the saved value applies again once the variable is unset. `CLAUDE_PROJECTS_DIR` applies to the `default` Claude Code account. A profile without a saved Codex path uses the built-in `~/.codex`.

### 🔎 Search and revision history

- **Ranking** — memory search orders results with BM25 weighted by column (title and `topic_key` count more than the body), newest first on ties. Conversation search weights prompt text over the project name.
- **Any-word fallback** — when a search with several words finds nothing that contains all of them, it runs once more matching any word, and the Memory list says so ("No exact matches. Showing memories that match any of the words.").
- **Trigram indexes** — on databases indexed with Engram's trigram tokenizer, words shorter than 3 characters are ignored by full-text search; if every word is that short, Memory search and the Memory list match titles containing the words instead (all words first, then any word).
- **Revision history** — `GET /api/observations/{id}/versions` returns a memory's stored revisions, newest first: `{"available": true, "versions": [{"version", "title", "content", "created_at"}]}`. `available` is `false` (with an empty list) when the database has no `observation_versions` table (older Engram versions); 404 for an unknown memory, 400 for a non-integer id. Read-only.

---

## ✏️ Editing your memory

Engram Explorer is **read-first**: every view works against the read-only pool and never touches your data. On top of that it offers a few **explicit, user-confirmed** edits so you can fix the problems it surfaces without dropping to the CLI:

- **Edit an observation** — fix its title, type, `topic_key`, or content.
- **Reassign project** — move a stray observation/session/prompt to the right project.
- **Delete** — soft-delete an observation, or remove an empty session/prompt.
- **Rename / merge a project** — re-tag every row from one project name to another.
- **Import / export** — download a SQLite snapshot, or merge another Engram DB in (a safety backup is written next to your DB first).

Every mutation is confirmed in the UI, goes through a guarded write path, and runs inside a transaction. Because `engram serve` is the authoritative owner:

> The read path is always safe alongside a running daemon. For **destructive or bulk** edits (project rename/merge, DB import) prefer stopping the daemon first, and keep a backup.

For a strict viewer with **no** write capability, start with `ENGRAM_DASH_READONLY=true` — the read-write pool is never opened and every mutating endpoint returns `503`. Cloud sync is read-only in every mode, so no cloud operation bypasses this switch. See [SECURITY.md](./SECURITY.md) for the full concurrency contract.

### 🧹 Maintenance checks

**Settings → Maintenance** collects data-cleanup checks. Each one explains what it found; nothing runs on its own.

- **Empty sessions** — sessions with no memories, no prompts and no summary (a session whose only memories are soft-deleted is _not_ empty). "Delete all N empty sessions" asks for confirmation, then deletes, in one transaction, only the sessions that are still empty at that moment. Sessions of cloud-enrolled projects and sessions started in the last 24 hours (they may belong to an agent that is still running) are always kept. Also available as `GET /api/maintenance/empty-sessions` and `POST /api/maintenance/empty-sessions/delete`.
- **Possibly fragmented projects** — project names that probably mean the same project because Engram derived them from different working directories: the same name in a different letter case, with extra spaces or trailing slashes, or a full folder path whose last folder is the other name (`/Users/me/code/foo` and `foo`). Nothing broader is matched: `api` and `API-docs`, or two different paths that only share their last folder, are never suggested. Each group lists every name with its memory and session counts and a suggested target (a plain name before a path, then the one with most memories). "Combine into <target>" asks for confirmation and runs the same merge as the project page; the server re-checks that the pair is still related. Cloud-enrolled projects are listed but cannot be combined (unenroll them with the engram CLI first).
- **Data quality** — two exact, read-only checks over live memories: memories whose title is empty or whitespace only, and groups of memories stored more than once with exactly the same content (same `normalized_hash`; memories without a hash are never grouped, and near-duplicates are out of scope). Lists are capped at 50 entries, with the full count shown. Each memory opens its Memory detail in place; nothing is changed or deleted from this section, so it works the same in read-only mode.

Cleanup is permanent: download a backup first (Settings → export, `GET /api/db/export`). In read-only mode (`ENGRAM_DASH_READONLY=true`) the checks still show their findings, but every cleanup action is hidden and its route answers `503`.

---

## 🩹 Troubleshooting

<details>
<summary><strong>No data sources detected on first run</strong></summary>

The dashboard shows the **onboarding screen** instead of crashing. From there you can activate Engram or Claude Code Sessions. If you have Engram but it isn't auto-detected: go to **Settings → Modules** and enter the path to `engram.db`, or set `ENGRAM_DATA_DIR=/path/to/dir` before starting.

To get a real DB: `brew install gentleman-programming/tap/engram && engram serve`. To try demo data: `make demo`.

</details>

<details>
<summary><strong>Dashboard shows "daemon offline"</strong></summary>

The local `engram serve` daemon isn't reachable. The dashboard degrades gracefully — all read views stay functional against the SQLite file. Start the daemon with `engram serve` to recover the live `/health`, `/sync/status`, and `/stats` panels. (`/api/sync/issues` showing `DAEMON_DOWN HIGH` is the same root cause.)

</details>

<details>
<summary><strong>Port already in use (<code>address already in use</code>)</strong></summary>

Another process is bound to port 8787.

- **macOS / Linux:** `lsof -ti :8787 | xargs kill -9`
- **Windows:** `netstat -ano | findstr :8787`, then `taskkill /F /PID <id>`

Or use a different port: `DASHBOARD_PORT=9090 ./engram-explorer`.

</details>

<details>
<summary><strong>Sync diagnostics (<code>SYNC_BROKEN HIGH</code>, empty project name)</strong></summary>

**`SYNC_BROKEN HIGH`** — `lifecycle='pending'` with `last_acked_seq=0` while `last_enqueued_seq>0`: mutations queued but never acknowledged. Common causes: cloud token not set, network partition, cloud-side rejection. Check `last_error` / `reason_message` on `/sync/<project>`.

**Project with empty name (`""`)** — observations captured without a project context. This is an upstream capture bug in the agent or MCP that emitted them; the dashboard can't fix it. Inspect the `session_id` of the affected rows and patch the captor.

</details>

<details>
<summary><strong>Enrolling, unenrolling or syncing a project</strong></summary>

The dashboard is **read-only** for cloud sync: it has no enroll, unenroll or sync buttons or endpoints. Run those operations with the `engram` CLI directly; the Sync Health views pick up the new state on their next refresh.

```bash
engram cloud enroll <project>     # start replicating a project to the cloud
engram cloud unenroll <project>   # stop future replication (local and remote data are kept)
engram cloud status               # check the cloud connection
```

Use the project name exactly as the dashboard shows it. Run `engram cloud <subcommand> --help` for the options your
installed version supports.

</details>

---

## 📦 Releases & changelog

Versioned binaries and per-release changelogs are published on [GitHub Releases](https://github.com/AlvaroQ/engram-explorer/releases). Built with GoReleaser from `v*` tags — macOS, Linux, and Windows artifacts plus a Homebrew tap.

---

## 📂 Project layout

<details>
<summary>Expand the full directory tree</summary>

```
engram-explorer/
├── Makefile                        # Build targets: build, dev, test, clean
├── go.mod                          # module github.com/AlvaroQ/engram-explorer
├── cmd/
│   ├── engram-explorer/            # Binary entry point — config → container → serve
│   └── seed-demo/                  # Demo dataset seeder (./demo/engram.db + ./demo/claude)
├── internal/
│   ├── config/                     # Env-var config, ProfileStore, EnsureConfig
│   ├── agentsessions/              # Vendor-neutral agent-session model (+ codex/ rollout reader)
│   ├── httpapi/                    # net/http mux, routes, middleware, container (/api/*)
│   ├── providers/                  # Provider port + Registry (state-machine, hot-swap)
│   │   ├── engram/                 # Engram provider adapter (SQLite, daemon proxy, health)
│   │   ├── ccsessions/             # Claude Code Sessions provider adapter (JSONL reader)
│   │   ├── codexsessions/          # Codex provider adapter (local rollouts, read-only)
│   │   └── providertest/           # FakeProvider + test helpers
│   ├── ui/                         # templ+HTMX server-rendered UI (all pages)
│   │   ├── locales/                # en.json, es.json
│   │   └── static/                 # app.css (Tailwind), styles.css, htmx.min.js
│   ├── doctor/                     # Diagnostic module: /doctor/sync, /doctor/orphans
│   ├── services/                   # SQL service layer (observations, sessions, sync, …)
│   ├── sqlite/                     # Read-only + read-write connection pools
│   ├── daemon/                     # HTTP client for the engram serve daemon
│   ├── cursor/                     # Opaque cursor-based pagination
│   ├── fts/                        # FTS5 query sanitizer
│   ├── logging/                    # log/slog setup
│   └── web/                        # go:embed of built islands+assets
├── apps/
│   └── frontend/                   # React islands via Vite multi-entry (build-time only)
│       └── src/
│           ├── islands/            # brain.tsx, *-chart.tsx (each exports mount(el,props))
│           ├── components/brain/   # R3F 3D neural graph
│           ├── components/charts/  # recharts wrappers
│           └── loader.ts           # mounts islands from <div data-island data-props>
└── testdata/
    └── schema/engram-schema.sql    # Reference schema for Go integration tests
```

</details>

---

## 🤝 Contributing

See [CONTRIBUTING.md](./CONTRIBUTING.md). This project follows the [Contributor Covenant Code of Conduct](./CODE_OF_CONDUCT.md).

---

## 📄 License

MIT — see [LICENSE](./LICENSE).
