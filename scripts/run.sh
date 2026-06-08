#!/usr/bin/env bash
# run.sh — One command to build the frontend, embed it, compile the single
# binary, and launch it against your real ~/.engram/engram.db.
#
# Usage:
#   ./scripts/run.sh                        # run against ~/.engram
#   ENGRAM_DASH_READONLY=true ./scripts/run.sh   # pure read-only viewer
#   ENGRAM_DATA_DIR=./demo    ./scripts/run.sh   # use the demo database
#
# After this runs once, ./engram-explorer is left on disk — re-launch it
# directly (instant, no rebuild) until you change the code again.
# Stop with Ctrl+C.
set -euo pipefail
cd "$(dirname "$0")/.."

echo "-> Building frontend (Vite)..."
RELEASE=1 pnpm -F @engram-explorer/frontend build

echo "-> Embedding frontend into the Go binary..."
rm -rf internal/web/dist
mkdir -p internal/web/dist
cp -r apps/frontend/dist/. internal/web/dist/

echo "-> Compiling engram-explorer..."
go build -o engram-explorer ./cmd/engram-explorer

echo "-> Starting at http://127.0.0.1:8787  (Ctrl+C to stop)"
exec ./engram-explorer "$@"
