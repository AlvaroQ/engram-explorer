#!/usr/bin/env pwsh
# run.ps1 — One command to build the frontend, embed it, compile the single
# binary, and launch it against your real ~/.engram/engram.db.
#
# Usage:
#   ./scripts/run.ps1                      # run against ~/.engram
#   $env:ENGRAM_DASH_READONLY="true"; ./scripts/run.ps1   # pure read-only viewer
#   $env:ENGRAM_DATA_DIR="./demo";    ./scripts/run.ps1   # use the demo database
#
# After this runs once, ./engram-explorer.exe is left on disk — re-launch it
# directly (instant, no rebuild) until you change the code again.
# Stop with Ctrl+C.

$ErrorActionPreference = 'Stop'
$repo = Split-Path -Parent $PSScriptRoot
Push-Location $repo
try {
    Write-Host '-> Generating templ Go code (version pinned by go.mod)...' -ForegroundColor Cyan
    go run github.com/a-h/templ/cmd/templ generate ./internal/doctor/...
    go run github.com/a-h/templ/cmd/templ generate ./internal/ui/...

    Write-Host '-> Building frontend (Vite) + island bundles...' -ForegroundColor Cyan
    pnpm -F '@engram-explorer/frontend' build

    Write-Host '-> Building Tailwind CSS for the templ UI...' -ForegroundColor Cyan
    pnpm -F '@engram-explorer/frontend' build:ui-css

    Write-Host '-> Embedding frontend into the Go binary...' -ForegroundColor Cyan
    Remove-Item -Recurse -Force internal/web/dist -ErrorAction SilentlyContinue
    Copy-Item -Recurse apps/frontend/dist internal/web/dist

    Write-Host '-> Compiling engram-explorer.exe...' -ForegroundColor Cyan
    go build -o engram-explorer.exe ./cmd/engram-explorer

    Write-Host '-> Starting at http://127.0.0.1:8787  (Ctrl+C to stop)' -ForegroundColor Green
    & ./engram-explorer.exe @args
}
finally {
    Pop-Location
}
