# Contributing to engram-explorer

Thanks for your interest in improving **engram-explorer** — a local-first dashboard for the [Engram](https://github.com/Gentleman-Programming/engram) memory database.

## Prerequisites

- **Go 1.25+** (the backend + single binary; matches the `go` directive in `go.mod`)
- **Node.js 20+** and **pnpm 10+** (only to build the frontend that gets embedded)
- An Engram database at `~/.engram/engram.db` — or generate a demo one with `go run ./cmd/seed-demo` (see below)

## Project layout

| Path                   | What                                                                                                               |
| ---------------------- | ------------------------------------------------------------------------------------------------------------------ |
| `cmd/engram-explorer/` | Single-binary entrypoint — serves the templ+HTMX UI **and** the JSON API on one port                               |
| `cmd/seed-demo/`       | Generates a self-contained demo database so you can try the app without Engram                                     |
| `internal/ui/`         | Primary UI: templ+HTMX server-rendered pages, i18n (en/es), layout, static assets (Tailwind CSS)                   |
| `internal/doctor/`     | Diagnostic module: `/doctor/sync`, `/doctor/orphans` (templ+HTMX, reuses services layer)                           |
| `internal/`            | Go backend — `config`, `sqlite` (read-only + read-write pools), `services`, `httpapi`, `web` (embedding), `daemon` |
| `apps/frontend/`       | React islands (Vite multi-entry): brain (R3F), charts (recharts). Each island exports `mount(el, props)`           |
| `testdata/schema/`     | Authoritative SQLite schema used by tests and the demo seeder                                                      |

## Development

```bash
make build          # templ generate + vite build (islands) + build:ui-css (Tailwind) + embed + go build
./engram-explorer   # run the single binary
```

…or run directly (requires frontend already built):

```bash
make frontend embed           # build islands + embed
go run ./cmd/engram-explorer  # run backend
```

For island hot-reload during development:

```bash
# Terminal 1 — Go server
go run ./cmd/engram-explorer

# Terminal 2 — Vite dev server (proxies /api/* to :8787)
pnpm -F @engram-explorer/frontend dev
```

After editing `.templ` files, regenerate Go code:

```bash
templ generate ./internal/ui/...
templ generate ./internal/doctor/...
```

Open <http://127.0.0.1:8787>. The `engram serve` daemon is optional — the dashboard reads the SQLite file directly and degrades gracefully when the daemon is offline.

## Before opening a PR

- `go build ./...`, `go vet ./...`, and `go test ./...` must all pass.
- Use [Conventional Commits](https://www.conventionalcommits.org/) (`feat:`, `fix:`, `docs:`, `refactor:`, `chore:`).
- Keep **one logical change per PR**.
- **Do not** add AI attribution or `Co-Authored-By` trailers to commits.

## Parity contract

The dashboard opens the Engram SQLite database with a **read-only** handle so it never blocks the daemon's writes. When you touch data queries, keep responses in parity with the existing API shape — the templ templates and React islands depend on exact field names, nullability, and ordering.
