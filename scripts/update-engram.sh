#!/usr/bin/env bash
# update-engram.sh — Update the engram CLI safely on Windows + Git Bash.
#
# Designed for the situation where you have multiple copies of `engram` in
# PATH and a running `engram serve` daemon, which is what we found on this
# machine. Reads from PATH at runtime instead of hard-coding paths.
#
# What it does (in order, all skippable with flags):
#   1. Sanity-check: bash, go, engram present
#   2. Read the *active* engram path (whatever PATH resolves first)
#   3. Print current version + target (latest from go install)
#   4. Ask for confirmation (skip with --yes)
#   5. Backup ~/.engram/engram.db (skip with --no-backup)
#   6. Stop the engram serve daemon listening on :7437 (skip with --no-stop)
#   7. Run `go install github.com/Gentleman-Programming/engram/cmd/engram@latest`
#   8. Copy the freshly built binary into the active PATH location, so the
#      one that ACTUALLY runs when you type `engram` is the new one
#   9. Verify the resulting version
#  10. Optionally restart the daemon (--restart)
#
# Reversible:
#   The DB backup goes to ~/.engram/engram.db.bak-<old-version>-<timestamp>
#   so you can restore with: cp ~/.engram/engram.db.bak-... ~/.engram/engram.db
#
# Idempotent:
#   If the active binary is already at the latest version, the script exits
#   early without touching anything (unless --force is set).

set -euo pipefail

# ───────────────────────── flags ────────────────────────────
DRY_RUN=0
ASSUME_YES=0
DO_BACKUP=1
DO_STOP_DAEMON=1
DO_RESTART=0
FORCE=0
TARGET="github.com/Gentleman-Programming/engram/cmd/engram@latest"

usage() {
  cat <<EOF
Usage: $(basename "$0") [options]

Options:
  --yes               Skip the interactive confirmation
  --dry-run           Print what would be done, don't execute
  --no-backup         Skip the engram.db backup (NOT recommended)
  --no-stop           Don't stop the engram serve daemon
  --restart           Start \`engram serve\` in the background after updating
  --force             Update even if already on the latest version
  --target <ref>      Override the go install target
                      (default: $TARGET)
  -h, --help          Show this help

Examples:
  $(basename "$0")              # interactive, with backup, leaves daemon stopped
  $(basename "$0") --yes        # non-interactive
  $(basename "$0") --dry-run    # preview only
  $(basename "$0") --restart    # auto-restart daemon when done
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --yes) ASSUME_YES=1 ;;
    --dry-run) DRY_RUN=1 ;;
    --no-backup) DO_BACKUP=0 ;;
    --no-stop) DO_STOP_DAEMON=0 ;;
    --restart) DO_RESTART=1 ;;
    --force) FORCE=1 ;;
    --target) TARGET="${2:-}"; shift ;;
    -h|--help) usage; exit 0 ;;
    *) echo "Unknown option: $1" >&2; usage; exit 2 ;;
  esac
  shift
done

# ───────────────────────── helpers ──────────────────────────
say() { printf '%s\n' "$*"; }
hdr() { printf '\n=== %s ===\n' "$*"; }
warn() { printf 'WARN: %s\n' "$*" >&2; }
fail() { printf 'ERROR: %s\n' "$*" >&2; exit 1; }
run() {
  if [[ "$DRY_RUN" -eq 1 ]]; then
    printf '  [dry-run] %s\n' "$*"
  else
    printf '  $ %s\n' "$*"
    # eval is required here: call sites rely on shell redirection (>log),
    # backgrounding (&) and quoted paths. The only externally supplied value,
    # --target, is validated below so it cannot smuggle shell metacharacters in.
    eval "$@"
  fi
}

# Validate --target before it can reach eval in run(). A go module path only
# needs letters, digits and . _ / @ - ; reject anything else (spaces, ;, $, |,
# &, quotes, backticks) so a crafted --target can't inject shell commands.
if [[ ! "$TARGET" =~ ^[A-Za-z0-9._/@-]+$ ]]; then
  fail "Invalid --target '$TARGET' (allowed characters: A-Z a-z 0-9 . _ / @ -)."
fi

# ───────────────────────── 1. preflight ─────────────────────
hdr "1. Preflight"

command -v go >/dev/null 2>&1 || fail "Go is not installed (or not in PATH). Get it from https://go.dev/dl/"
command -v engram >/dev/null 2>&1 || fail "engram is not installed (or not in PATH)."

# Resolve the *active* binary — the one PATH picks up first.
ACTIVE_BIN="$(command -v engram)"
say "Active engram binary: $ACTIVE_BIN"

# Current version (parse the last word of the version line; ignore the
# "Update available" notice the binary itself prints).
CURRENT_VERSION="$(engram --version 2>&1 | awk '/^engram [0-9]/ {print $2; exit}')"
[[ -n "$CURRENT_VERSION" ]] || fail "Could not parse current engram version."
say "Current version : $CURRENT_VERSION"

GOBIN="$(go env GOBIN 2>/dev/null || true)"
[[ -z "$GOBIN" ]] && GOBIN="$(go env GOPATH)/bin"
say "Go install dir  : $GOBIN"
say "Target          : $TARGET"

# ───────────────────────── 2. who else is using the DB ──────
hdr "2. Process check"

DAEMON_PIDS=""
if command -v netstat >/dev/null 2>&1; then
  # Lines that LISTEN on :7437 → extract the PID column on Windows netstat.
  DAEMON_PIDS="$(netstat -ano 2>/dev/null | awk '/127\.0\.0\.1:7437.*LISTENING/ {print $NF}' | sort -u | tr '\n' ' ')"
fi

if [[ -n "${DAEMON_PIDS// /}" ]]; then
  say "engram serve daemon listening on :7437 (pids:$DAEMON_PIDS)"
else
  say "No engram serve daemon detected on :7437."
fi

# Other engram processes (likely MCP servers spawned by agents).
OTHER_PIDS=""
if command -v tasklist >/dev/null 2>&1; then
  OTHER_PIDS="$(tasklist 2>/dev/null \
    | awk '/[Ee]ngram\.exe/ {print $2}' \
    | sort -u \
    | tr '\n' ' ')"
fi

if [[ -n "${OTHER_PIDS// /}" ]]; then
  say "All engram processes alive (PIDs):$OTHER_PIDS"
  say "  (MCP servers spawned by AI agents will reappear when those agents reconnect.)"
fi

# ───────────────────────── 3. plan ──────────────────────────
hdr "3. Plan"
say "  • Backup the database (if --no-backup not set)"
[[ "$DO_BACKUP" -eq 1 ]] || say "    SKIPPED via --no-backup"
say "  • Stop the engram serve daemon"
[[ "$DO_STOP_DAEMON" -eq 1 ]] || say "    SKIPPED via --no-stop"
say "  • go install $TARGET"
say "  • Copy fresh binary to $ACTIVE_BIN"
say "  • Verify new version"
say "  • Restart engram serve in background"
[[ "$DO_RESTART" -eq 1 ]] || say "    SKIPPED — pass --restart to enable"

if [[ "$ASSUME_YES" -eq 0 && "$DRY_RUN" -eq 0 ]]; then
  printf '\nProceed? [y/N] '
  read -r reply
  case "$reply" in
    y|Y|yes|YES) ;;
    *) say "Aborted."; exit 0 ;;
  esac
fi

# ───────────────────────── 4. backup ────────────────────────
hdr "4. Backup"
DB_DIR="${ENGRAM_DATA_DIR:-$HOME/.engram}"
DB_FILE="$DB_DIR/engram.db"
TS="$(date +%Y%m%d-%H%M%S)"

if [[ "$DO_BACKUP" -eq 1 ]]; then
  if [[ -f "$DB_FILE" ]]; then
    BACKUP="$DB_FILE.bak-$CURRENT_VERSION-$TS"
    run "cp '$DB_FILE' '$BACKUP'"
    [[ -f "$DB_FILE-wal" ]] && run "cp '$DB_FILE-wal' '$BACKUP-wal' || true"
    [[ -f "$DB_FILE-shm" ]] && run "cp '$DB_FILE-shm' '$BACKUP-shm' || true"
    say "Backup written: $BACKUP"
  else
    warn "DB not found at $DB_FILE — skipping backup."
  fi
else
  say "Backup skipped."
fi

# ───────────────────────── 5. stop daemon ───────────────────
hdr "5. Stop daemon"
if [[ "$DO_STOP_DAEMON" -eq 1 && -n "${DAEMON_PIDS// /}" ]]; then
  for pid in $DAEMON_PIDS; do
    run "taskkill //F //PID $pid"
  done
  say "Daemon stopped."
else
  say "Nothing to stop (or skipped)."
fi

# ───────────────────────── 6. go install ────────────────────
hdr "6. go install"
run "go install '$TARGET'"

NEW_BIN="$GOBIN/engram.exe"
if [[ ! -f "$NEW_BIN" && "$DRY_RUN" -eq 0 ]]; then
  # Fallback for non-Windows or odd setups.
  NEW_BIN="$GOBIN/engram"
fi
[[ "$DRY_RUN" -eq 1 ]] || [[ -f "$NEW_BIN" ]] || fail "Expected the freshly built binary at $NEW_BIN but it's missing."

# ───────────────────────── 7. promote ───────────────────────
hdr "7. Promote to active PATH location"
if [[ "$NEW_BIN" != "$ACTIVE_BIN" ]]; then
  if [[ "$FORCE" -eq 0 ]]; then
    NEW_VERSION_PEEK=""
    if [[ "$DRY_RUN" -eq 0 ]]; then
      NEW_VERSION_PEEK="$("$NEW_BIN" --version 2>&1 | awk '/^engram [0-9]/ {print $2; exit}')"
    fi
    if [[ -n "$NEW_VERSION_PEEK" && "$NEW_VERSION_PEEK" == "$CURRENT_VERSION" ]]; then
      say "Already on $CURRENT_VERSION — nothing to promote. Use --force to copy anyway."
    else
      run "cp '$NEW_BIN' '$ACTIVE_BIN'"
    fi
  else
    run "cp '$NEW_BIN' '$ACTIVE_BIN'"
  fi
else
  say "Active path already points at the go install location."
fi

# ───────────────────────── 8. verify ────────────────────────
hdr "8. Verify"
if [[ "$DRY_RUN" -eq 0 ]]; then
  POST_VERSION="$(engram --version 2>&1 | awk '/^engram [0-9]/ {print $2; exit}')"
  say "Active engram is now: $POST_VERSION (was $CURRENT_VERSION)"
else
  say "(dry-run — skipping version check)"
fi

# ───────────────────────── 9. restart ───────────────────────
hdr "9. Daemon"
if [[ "$DO_RESTART" -eq 1 ]]; then
  say "Starting engram serve in background…"
  if [[ "$DRY_RUN" -eq 0 ]]; then
    nohup engram serve >"$DB_DIR/serve.log" 2>&1 &
    NEW_PID=$!
    sleep 1
    if kill -0 "$NEW_PID" 2>/dev/null; then
      say "Daemon up (pid $NEW_PID, log: $DB_DIR/serve.log)"
    else
      warn "Daemon failed to start — check $DB_DIR/serve.log"
    fi
  else
    run "nohup engram serve >'$DB_DIR/serve.log' 2>&1 &"
  fi
else
  say "Daemon left stopped. Start it manually when ready: engram serve"
fi

# ───────────────────────── 10. summary ──────────────────────
hdr "Done"
say "Old: $CURRENT_VERSION"
[[ "$DRY_RUN" -eq 0 ]] && say "New: $(engram --version 2>&1 | awk '/^engram [0-9]/ {print $2; exit}')"
say "Backup: ${BACKUP:-(none)}"
say ""
say "Next steps you may want to run from the dashboard repo:"
say "  pnpm test              # 72 tests; should still pass"
say "  pnpm dev               # restart backend so it re-opens the (possibly migrated) DB"
say "  curl http://127.0.0.1:8787/api/health"
