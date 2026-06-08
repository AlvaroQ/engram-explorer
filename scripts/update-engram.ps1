<#
.SYNOPSIS
  Update the engram CLI safely on Windows. PowerShell-native sibling of
  scripts/update-engram.sh.

.DESCRIPTION
  Designed for the situation where you have multiple copies of `engram` in
  PATH and a running `engram serve` daemon. Reads from PATH at runtime
  instead of hard-coding paths.

  What it does (in order, all skippable with switches):
    1. Sanity-check: go and engram present
    2. Read the *active* engram path (whatever PATH resolves first)
    3. Print current version + target (latest from go install)
    4. Ask for confirmation (skip with -Yes)
    5. Backup ~/.engram/engram.db (skip with -NoBackup)
    6. Stop the engram serve daemon listening on :7437 (skip with -NoStop)
    7. Run `go install github.com/Gentleman-Programming/engram/cmd/engram@latest`
    8. Copy the freshly built binary into the active PATH location, so the
       one that ACTUALLY runs when you type `engram` is the new one
    9. Verify the resulting version
   10. Optionally restart the daemon (-Restart)

.PARAMETER Yes
  Skip the interactive confirmation.

.PARAMETER DryRun
  Print what would be done, don't execute.

.PARAMETER NoBackup
  Skip the engram.db backup (NOT recommended).

.PARAMETER NoStop
  Don't stop the engram serve daemon.

.PARAMETER Restart
  Start `engram serve` in the background after updating.

.PARAMETER Force
  Update even if already on the latest version.

.PARAMETER Target
  Override the go install target. Default:
  github.com/Gentleman-Programming/engram/cmd/engram@latest

.EXAMPLE
  .\scripts\update-engram.ps1 -DryRun
  .\scripts\update-engram.ps1 -Yes -Restart
  .\scripts\update-engram.ps1 -Target 'github.com/Gentleman-Programming/engram/cmd/engram@v1.14.5'
#>

[CmdletBinding()]
param(
  [switch]$Yes,
  [switch]$DryRun,
  [switch]$NoBackup,
  [switch]$NoStop,
  [switch]$Restart,
  [switch]$Force,
  [string]$Target = 'github.com/Gentleman-Programming/engram/cmd/engram@latest'
)

$ErrorActionPreference = 'Stop'

# ───────────────────────── helpers ──────────────────────────
function Write-Hdr([string]$Title) {
  Write-Host ""
  Write-Host "=== $Title ===" -ForegroundColor Cyan
}
function Write-Say([string]$Text) { Write-Host $Text }
function Write-Warn([string]$Text) { Write-Host "WARN: $Text" -ForegroundColor Yellow }
function Fail([string]$Text) { Write-Host "ERROR: $Text" -ForegroundColor Red; exit 1 }

function Invoke-Step([string]$Description, [scriptblock]$Action) {
  if ($DryRun) {
    Write-Host "  [dry-run] $Description"
  } else {
    Write-Host "  > $Description"
    & $Action
  }
}

function Get-EngramVersion([string]$Bin) {
  # The engram binary prints "Update available" on stderr; PowerShell would
  # format that as a NativeCommandError and pollute our parse. Use Start-Process
  # with a redirected stdout file so we get clean stdout and nothing else.
  $tmp = [System.IO.Path]::GetTempFileName()
  try {
    $null = Start-Process -FilePath $Bin -ArgumentList '--version' `
      -NoNewWindow -Wait -RedirectStandardOutput $tmp -RedirectStandardError ([System.IO.Path]::GetTempFileName())
    $raw = Get-Content $tmp -Raw
    if (-not $raw) { return $null }
    foreach ($line in ($raw -split "`r?`n")) {
      $trimmed = $line.Trim()
      if ($trimmed -match '^engram (\d+\.\d+\.\d+)') { return $Matches[1] }
    }
    return $null
  } catch {
    return $null
  } finally {
    Remove-Item $tmp -ErrorAction SilentlyContinue
  }
}

# ───────────────────────── 1. preflight ─────────────────────
Write-Hdr "1. Preflight"

$goCmd = Get-Command go -ErrorAction SilentlyContinue
if (-not $goCmd) { Fail "Go is not installed (or not in PATH). Get it from https://go.dev/dl/" }

$engramCmd = Get-Command engram -ErrorAction SilentlyContinue
if (-not $engramCmd) { Fail "engram is not installed (or not in PATH)." }

$activeBin = $engramCmd.Source
Write-Say "Active engram binary: $activeBin"

$currentVersion = Get-EngramVersion $activeBin
if (-not $currentVersion) { Fail "Could not parse current engram version." }
Write-Say "Current version : $currentVersion"

$gobin = (& go env GOBIN 2>$null)
if ([string]::IsNullOrWhiteSpace($gobin)) {
  $gobin = Join-Path (& go env GOPATH) 'bin'
}
Write-Say "Go install dir  : $gobin"
Write-Say "Target          : $Target"

# ───────────────────────── 2. process check ─────────────────
Write-Hdr "2. Process check"

$daemonPids = @()
try {
  $conns = Get-NetTCPConnection -LocalPort 7437 -State Listen -ErrorAction SilentlyContinue
  if ($conns) { $daemonPids = $conns | Select-Object -ExpandProperty OwningProcess -Unique }
} catch { }

if ($daemonPids.Count -gt 0) {
  Write-Say "engram serve daemon listening on :7437 (PIDs: $($daemonPids -join ', '))"
} else {
  Write-Say "No engram serve daemon detected on :7437."
}

$otherPids = @()
try {
  $procs = Get-Process -Name engram -ErrorAction SilentlyContinue
  if ($procs) { $otherPids = $procs | Select-Object -ExpandProperty Id }
} catch { }

if ($otherPids.Count -gt 0) {
  Write-Say "All engram processes alive (PIDs: $($otherPids -join ', '))"
  Write-Say "  (MCP servers spawned by AI agents will reappear when those agents reconnect.)"
}

# ───────────────────────── 3. plan ──────────────────────────
Write-Hdr "3. Plan"
$backupNote = if ($NoBackup) { ' (SKIPPED via -NoBackup)' } else { '' }
$stopNote   = if ($NoStop)   { ' (SKIPPED via -NoStop)'   } else { '' }
Write-Say "  - Backup the database$backupNote"
Write-Say "  - Stop the engram serve daemon$stopNote"
Write-Say "  - go install $Target"
Write-Say "  - Copy fresh binary to $activeBin"
Write-Say "  - Verify new version"
if ($Restart) { Write-Say "  - Restart engram serve in background" }
else { Write-Say "  - (-Restart not set, daemon will be left stopped)" }

if (-not $Yes -and -not $DryRun) {
  $reply = Read-Host "`nProceed? [y/N]"
  if ($reply -notmatch '^(y|yes)$') { Write-Say "Aborted."; exit 0 }
}

# ───────────────────────── 4. backup ────────────────────────
Write-Hdr "4. Backup"
$dbDir = if ($env:ENGRAM_DATA_DIR) { $env:ENGRAM_DATA_DIR } else { Join-Path $HOME '.engram' }
$dbFile = Join-Path $dbDir 'engram.db'
$ts = Get-Date -Format 'yyyyMMdd-HHmmss'
$backupFile = $null

if (-not $NoBackup) {
  if (Test-Path $dbFile) {
    $backupFile = "$dbFile.bak-$currentVersion-$ts"
    Invoke-Step "Copy $dbFile -> $backupFile" {
      Copy-Item $dbFile $backupFile
      foreach ($suffix in '-wal','-shm') {
        $extra = "$dbFile$suffix"
        if (Test-Path $extra) { Copy-Item $extra "$backupFile$suffix" }
      }
    }
    Write-Say "Backup written: $backupFile"
  } else {
    Write-Warn "DB not found at $dbFile - skipping backup."
  }
} else {
  Write-Say "Backup skipped."
}

# ───────────────────────── 5. stop daemon ───────────────────
Write-Hdr "5. Stop daemon"
if (-not $NoStop -and $daemonPids.Count -gt 0) {
  foreach ($daemonPid in $daemonPids) {
    Invoke-Step "Stop-Process -Id $daemonPid -Force" {
      Stop-Process -Id $daemonPid -Force -ErrorAction SilentlyContinue
    }
  }
  Write-Say "Daemon stopped."
} else {
  Write-Say "Nothing to stop (or skipped)."
}

# ───────────────────────── 6. go install ────────────────────
Write-Hdr "6. go install"
Invoke-Step "go install $Target" {
  & go install $Target
  if ($LASTEXITCODE -ne 0) { Fail "go install failed with exit code $LASTEXITCODE" }
}

$newBin = Join-Path $gobin 'engram.exe'
if (-not (Test-Path $newBin) -and -not $DryRun) {
  $newBin = Join-Path $gobin 'engram'
}
if (-not $DryRun -and -not (Test-Path $newBin)) {
  Fail "Expected freshly built binary at $newBin, but it's missing."
}

# ───────────────────────── 7. promote ───────────────────────
Write-Hdr "7. Promote to active PATH location"

$shouldPromote = $true
if (-not $Force -and -not $DryRun) {
  $newVersionPeek = Get-EngramVersion $newBin
  if ($newVersionPeek -and $newVersionPeek -eq $currentVersion) {
    Write-Say "Already on $currentVersion - nothing to promote. Use -Force to copy anyway."
    $shouldPromote = $false
  }
}

if ($shouldPromote -and ($newBin -ne $activeBin)) {
  Invoke-Step "Copy-Item $newBin $activeBin -Force" {
    Copy-Item $newBin $activeBin -Force
  }
} elseif ($newBin -eq $activeBin) {
  Write-Say "Active path already points at the go install location."
}

# ───────────────────────── 8. verify ────────────────────────
Write-Hdr "8. Verify"
if (-not $DryRun) {
  $postVersion = Get-EngramVersion $activeBin
  Write-Say "Active engram is now: $postVersion (was $currentVersion)"
} else {
  Write-Say "(dry-run - skipping version check)"
}

# ───────────────────────── 9. restart ───────────────────────
Write-Hdr "9. Daemon"
if ($Restart) {
  $logPath = Join-Path $dbDir 'serve.log'
  Invoke-Step "Start-Process engram serve (log: $logPath)" {
    $proc = Start-Process -FilePath engram -ArgumentList 'serve' `
      -RedirectStandardOutput $logPath -RedirectStandardError "$logPath.err" `
      -WindowStyle Hidden -PassThru
    Start-Sleep -Seconds 1
    if ($proc.HasExited) {
      Write-Warn "Daemon exited immediately - check $logPath / $logPath.err"
    } else {
      Write-Say "Daemon up (pid $($proc.Id), log: $logPath)"
    }
  }
} else {
  Write-Say "Daemon left stopped. Start it manually when ready: engram serve"
}

# ───────────────────────── 10. summary ──────────────────────
Write-Hdr "Done"
Write-Say "Old: $currentVersion"
if (-not $DryRun) {
  $finalVersion = Get-EngramVersion $activeBin
  Write-Say "New: $finalVersion"
}
if ($backupFile) { Write-Say "Backup: $backupFile" } else { Write-Say "Backup: (none)" }
Write-Host ""
Write-Say "Next steps you may want to run from the dashboard repo:"
Write-Say "  pnpm test              # 72 tests; should still pass"
Write-Say "  pnpm dev               # restart backend so it re-opens the (possibly migrated) DB"
Write-Say "  curl http://127.0.0.1:8787/api/health"
