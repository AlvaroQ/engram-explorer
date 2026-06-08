# Contributing to engram-explorer

Thanks for your interest in improving **engram-explorer** — a local-first dashboard for the [Engram](https://github.com/Gentleman-Programming/engram) memory database.

## Prerequisites

- **Go 1.23+** (the backend + single binary)
- **Node.js 20+** and **pnpm 10+** (only to build the frontend that gets embedded)
- An Engram database at `~/.engram/engram.db` — or generate a demo one with `go run ./cmd/seed-demo` (see below)

## Project layout

| Path | What |
| --- | --- |
| `cmd/engram-explorer/` | Single-binary entrypoint — serves the embedded SPA **and** the JSON API on one port |
| `cmd/seed-demo/` | Generates a self-contained demo database so you can try the app without Engram |
| `internal/` | Go backend — `config`, `sqlite` (read-only + read-write pools), `services`, `httpapi`, `web` (embedding), `daemon`, `cloud` |
| `apps/frontend/` | React 19 + Vite + TanStack + React-Three-Fiber UI (built and embedded via `go:embed`) |
| `testdata/schema/` | Authoritative SQLite schema used by tests and the demo seeder |

## Development

```bash
pnpm install                                   # frontend deps
pnpm -F @engram-explorer/frontend build              # build the SPA
cp -r apps/frontend/dist/* internal/web/dist/  # embed it
go run ./cmd/engram-explorer                          # run the single binary
```

…or simply `make build && ./engram-explorer` once GNU Make is available.

Open <http://127.0.0.1:8787>. The `engram serve` daemon is optional — the dashboard reads the SQLite file directly and degrades gracefully when the daemon is offline.

## Before opening a PR

- `go build ./...`, `go vet ./...`, and `go test ./...` must all pass.
- Use [Conventional Commits](https://www.conventionalcommits.org/) (`feat:`, `fix:`, `docs:`, `refactor:`, `chore:`).
- Keep **one logical change per PR**.
- **Do not** add AI attribution or `Co-Authored-By` trailers to commits.

## Parity contract

The dashboard opens the Engram SQLite database with a **read-only** handle so it never blocks the daemon's writes. When you touch data queries, keep responses in parity with the existing API shape — the React frontend depends on exact field names, nullability, and ordering.
