# Engram TUI vs. Local Dashboard — Comparison guide

> **Purpose:** explains the differences between the official Engram TUI (`engram tui`) and this local dashboard, what each tool is good at, and when to use which one.
>
> Tested against **engram v1.13.1+**. Menu labels may shift across minor versions; the concepts remain the same.

## Table of contents

1. [How to launch each tool](#1-how-to-launch-each-tool)
2. [TUI anatomy](#2-tui-anatomy)
3. [Dashboard anatomy](#3-dashboard-anatomy)
4. [Action-to-action mapping](#4-action-to-action-mapping)
5. [Three exercises to verify parity](#5-three-exercises-to-verify-parity)
6. [What only the TUI does](#6-what-only-the-tui-does)
7. [What only the dashboard does](#7-what-only-the-dashboard-does)
8. [Cheatsheet](#8-cheatsheet)

---

## 1. How to launch each tool

### Official TUI

```bash
engram tui
```

Starts an interactive terminal interface. Reads the same DB (`~/.engram/engram.db`) as the dashboard.

> The TUI **takes over the terminal**. To exit cleanly: press `q`.

### Local dashboard

From the project root:

```bash
make build        # produces the engram-explorer binary
./engram-explorer       # starts the server on 127.0.0.1:8787
```

Or run directly during development:

```bash
go run ./cmd/engram-explorer
```

Open `http://localhost:8787` in your browser. The top bar shows whether the `engram serve` daemon is active (the daemon is optional for the TUI as well; the dashboard works without it but shows no live telemetry).

> Tip: for quick read-only numbers without either tool, `engram stats` and `engram projects list` print counts to stdout.

---

## 2. TUI anatomy

When you run `engram tui` you see something like this:

```
╔══════════════════════════════════════════════════════════════════╗
║  🐘 SYSTEM ONLINE                                  MEM: OK 100%  ║
║                                                                  ║
║         ███████ ███    ██  ██████  ██████   █████  ███    ███    ║
║         (ASCII banner "ENGRAM")                                  ║
║                                                                  ║
║  > engram 1.13.1 — An elephant never forgets                     ║
╚══════════════════════════════════════════════════════════════════╝

┌───────────────────────────┐
│        26   sessions      │
│       747   observations  │
│         3   prompts       │
│        20   projects      │
└───────────────────────────┘

  Projects
  • demo-api
  • research-notes
  • my-project
    ...and N more projects

  Actions
 ▸ Search memories
   Recent observations
   Browse sessions
   Setup agent plugin
   Quit

  j/k navigate • enter select • s search • q quit
```

### Footer keybindings

| Key | Action |
|-----|--------|
| `j` / `k` | Move cursor up / down in the Actions list |
| `↑` / `↓` | Same as `j` / `k` |
| `Enter` | Activate highlighted action |
| `s` | Shortcut directly to "Search memories" |
| `q` | Exit the TUI |

### The 5 main menu actions

1. **Search memories** — text search against the FTS5 index over `observations` and `user_prompts`.
2. **Recent observations** — chronological list of the most recent observations across all projects.
3. **Browse sessions** — session list; select one to see its observations.
4. **Setup agent plugin** — wizard to integrate Engram with an AI agent (`opencode`, `claude-code`, `gemini-cli`, `codex`). Unrelated to the dashboard.
5. **Quit** — exit.

> Each sub-view has its own keybindings (typically `/` to search, `Esc` or `q` to go back, arrows to navigate). Press `?` inside a view if available, or read the on-screen footer.

---

## 3. Dashboard anatomy

When you open `http://localhost:8787/` you see:

- **Sidebar** on the left with 8 entries:
  1. Overview
  2. Projects
  3. Observations
  4. Sessions
  5. Sync Health
  6. Prompts
  7. Topics
  8. Settings
- **Topbar** with a health badge (green/yellow/red based on backend + DB + daemon reachability) and a sun/moon button for dark/light theme.
- **Content area** that changes per route.

### Routes and what they show

| Route | Content |
|-------|---------|
| `/` (Overview) | Clickable KPIs (Observations / Sessions / Projects / Prompts), "Activity (last 30 days)" chart, "By type" pie, Sync Health summary, "Issues detected" banner, "Recent observations" |
| `/projects` | Full project list with counts and sync badge. Filter by name. Click for drill-down |
| `/projects/:project` | Per-project KPIs + 30-day activity + type breakdown + tools used + top topics + recent sessions, observations, and prompts |
| `/observations` | Virtualized table (10 000+ rows without lag). URL-state multi-select filters: project, type, tool_name, scope, topic_key, deleted. FTS5 search with debounce. Detail drawer with topic revisions and JSON / SQL export |
| `/sessions` | Paginated list with filters |
| `/sessions/:id` | Horizontal visual timeline: prompts (blue) + observations colored by type. Click a marker for details |
| `/sync` | Auto-refreshing table (every 15 s) with computed status, "Issues detected" banner, Enroll / Unenroll buttons per project |
| `/sync/:project` | Operational drill-down: lifecycle, sequence numbers, last error, pending mutations, `cloud_upgrade_state` |
| `/prompts` | Table + FTS5 search with `<mark>` highlight |
| `/topics` | Topic key list with revision count. Click → observations filtered by that topic |
| `/settings` | DB path, daemon URL, CLI capabilities, runtime info, theme toggle |

---

## 4. Action-to-action mapping

### Common ground

| Task | TUI | Dashboard |
|------|-----|-----------|
| View global KPIs | Home screen (counts box) | `/` Overview KPIs |
| View project list | Home screen → "Projects" (top 5 + "and N more") | `/projects` (complete list with drill-down) |
| Search observations by keyword | Action **Search memories** (`s` or select + `Enter`) | `/observations` → "Full-text search…" box |
| View recent observations | Action **Recent observations** | `/` Overview → "Recent observations", or `/observations` sorted by `updated_at DESC` |
| Browse sessions | Action **Browse sessions** | `/sessions` |
| View session detail | Browse sessions → select | `/sessions/<id>` (with visual timeline, not just a list) |
| Exit | `q` | Close the browser tab |

### TUI only (see §6)

- **Setup agent plugin** (integration wizard for AI agents)

### Dashboard only (see §7)

- Per-project drill-down with filtered activity / by_type / tools / topics
- Sync Health table with automatic detection of SYNC_BROKEN, NOT_ENROLLED_HAS_DATA, etc.
- Enroll / Unenroll projects from the UI
- Virtualized table with URL-combinable filters
- Visual session timeline
- JSON / CSV export of filtered rows
- CLI capabilities detection

---

## 5. Three exercises to verify parity

These exercises produce **comparable** results between the TUI and the dashboard. If numbers diverge, there is likely a difference in how soft-deletes are counted (the dashboard excludes soft-deleted observations by default; toggle "Deleted rows: include" in `/observations` to match).

### Exercise A — Global counts

**TUI:**
```bash
engram tui
```
Note the counts in the top box (sessions, observations, prompts, projects).

**Dashboard:**
1. Open `http://localhost:8787/`
2. Check the 4 large KPIs at the top.

**Expected:** numbers match. Minor discrepancies may reflect soft-deleted observations (see note above).

### Exercise B — Inspect a specific project

**TUI:**
1. `engram tui`
2. Navigate to "Browse sessions" or "Recent observations"
3. Filter by project if the view allows it

**Dashboard:**
1. `http://localhost:8787/projects` → click on the project name
2. You see: observation count, session count, topic count, prompt count, last activity, 30-day activity chart, type distribution, tools used, top topics

**Expected:** counts match `engram projects list` (non-interactive CLI):

```bash
engram projects list | grep <project-name>
```

### Exercise C — Find silently broken sync targets

This is the primary use case the dashboard was built for.

**TUI:** the TUI does **not** show cloud sync state per project. From the CLI:
```bash
engram cloud status
```

**Dashboard:**
1. `http://localhost:8787/sync`
2. Projects with `lifecycle = pending`, many mutations enqueued, and zero acknowledgements appear with status `broken`
3. The "Issues detected" banner at the top lists exactly those cases as `SYNC_BROKEN HIGH`

**Expected:** you can see that pending mutations far exceed acknowledged ones — something that `engram cloud status` reports as a single summary but does not surface per-project visually.

---

## 6. What only the TUI does

| Capability | Why TUI yes, dashboard no |
|------------|--------------------------|
| **Setup agent plugin** | A wizard that writes config files for AI agents (e.g. `.claude/settings.json`, `.opencode/`). This is a filesystem operation outside the scope of an inspection dashboard |
| **Works without a browser** | Single Go binary; run it on a headless VPS with nothing else installed |
| **No dependency on the dashboard backend** | Reads the DB directly as a native Go program. The dashboard requires the Go binary to be running as an HTTP server |

---

## 7. What only the dashboard does

| Capability | Why |
|------------|-----|
| **Automatic sync problem detection** | The diagnostics engine in `internal/services` applies rules over `sync_state` + `sync_mutations` and emits typed issues with severity. The TUI does not touch those tables |
| **Per-project drill-down with 30-day activity and type breakdown** | New view (`/projects/:project`) — does not exist in TUI |
| **Virtualized table with URL-combinable filters** | `/observations` with cursor pagination and multi-select. State lives in the URL; views are bookmarkable |
| **Visual session timeline** | Horizontal SVG with markers colored by type. The TUI lists events one by one |
| **Enroll / Unenroll from the UI** | Backend executes `engram cloud enroll <project>` as a subprocess with a whitelist regex and audit log. Button shows confirmation dialog with the exact command |
| **JSON / CSV export of filtered view** | Useful for external analysis |
| **FTS5 with `<mark>` highlight** | Backend uses SQLite `snippet()` and the UI renders highlighted matches |
| **Automatic sync state refresh** | 15-second polling on `/sync`. The TUI shows a point-in-time snapshot |

---

## 8. Cheatsheet

### What to use for what

| Situation | Best tool |
|-----------|-----------|
| "How much is there in total?" | `engram stats` (CLI) or Overview in dashboard |
| "Find a memory by keyword" | TUI (`s`) or `/observations` with FTS5 |
| "See full content of an observation and its revisions" | Dashboard `/observations` → click → drawer |
| "What happened in project X?" | Dashboard `/projects/X` (charts, types, sessions, topics) |
| "Is any project's sync broken?" | Dashboard `/sync` (Issues banner + status badges) |
| "Enroll project Y in cloud sync" | CLI: `engram cloud enroll Y`. Dashboard: "Enroll" button in the project row. TUI: no such action |
| "I'm on a headless server with no browser" | TUI |
| "I'm on my workstation with a browser" | Dashboard |

### Quick TUI → URL equivalents

| TUI | Dashboard URL |
|-----|--------------|
| Home screen | `/` |
| Action: Search memories | `/observations` (FTS5 in the toolbar) |
| Action: Recent observations | `/` (section "Recent observations") or `/observations` sorted by `updated_at` |
| Action: Browse sessions → select | `/sessions` → click → `/sessions/:id` |
| (does not exist) | `/projects/:project` |
| (does not exist) | `/sync` |
| (does not exist) | `/topics` |

### CLI alternative (without TUI or dashboard)

```bash
engram --version            # binary version
engram stats                # global KPIs to stdout
engram projects list        # list projects with counts
engram context              # recent sessions + prompts in markdown
engram search <query>       # FTS5 from shell
engram cloud status         # cloud sync state (the dashboard also uses this as a subprocess)
```

---

If TUI and dashboard counts diverge, the most likely cause is soft-delete handling: the dashboard excludes soft-deleted observations by default. Enable "Deleted rows: include" in `/observations` to see the full count.
