# Security Policy

**engram-explorer** is a **local-first** tool. It binds to `127.0.0.1` only, opens your local Engram SQLite database with a **read-only** handle, and never transmits your memory data anywhere. LAN exposure is not supported by design.

## Reporting a vulnerability

Please report security issues **privately** — do not open a public issue.

- **Preferred:** use GitHub's [private vulnerability reporting](https://github.com/AlvaroQ/engram-explorer/security/advisories/new) — the **"Report a vulnerability"** button under the repository's **Security** tab.
- **Alternative:** email **alvaroquintanapalacios@gmail.com**.

Include a description and steps to reproduce. Please allow a reasonable time for a fix before any public disclosure.

## Scope notes

- The local backend binds to `127.0.0.1` only — it is not meant to be reachable over a network.
- The Engram database is opened **read-only**; the dashboard never writes to your data tables.
- Cloud mutations (`enroll` / `sync`) are delegated to the official `engram` CLI as a subprocess; the dashboard never writes to cloud storage directly. The one local exception is `unenroll`, which removes the local enrollment rows via SQL.
- Cloud mutation endpoints are rate-limited and require explicit user confirmation.
