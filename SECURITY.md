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
  renaming/merging a project, importing/exporting a database snapshot, and the
  local cloud `unenroll`. Every write route is guarded; if the writable handle
  is unavailable the route returns `503`.

> **Read-only mode.** Set `ENGRAM_DASH_READONLY=true` to run as a pure viewer:
> the read-write pool is never opened and every mutating route returns `503`.
> Use this when pointing the dashboard at a database you must not modify.

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

- The local backend binds to `127.0.0.1` only — it is not meant to be reachable over a network.
- Cloud `enroll` / `sync` are delegated to the official `engram` CLI as a subprocess (args passed as a slice, project names whitelisted) — the dashboard never writes to cloud storage directly. The one local SQL exception is `unenroll`, which removes the local enrollment/state rows for that project.
- Cloud mutation endpoints are rate-limited and require explicit user confirmation.
- In production (`ENGRAM_DASH_ENV` unset) internal error details are never returned to the client.
