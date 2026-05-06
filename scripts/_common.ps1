Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

function Write-Step($msg) { Write-Host "`n:: $msg" -ForegroundColor Cyan }
function Write-OK($msg)   { Write-Host "   $msg" -ForegroundColor Green }
function Write-Err($msg)  { Write-Host "   $msg" -ForegroundColor Red }
function Write-Warn($msg) { Write-Host "   $msg" -ForegroundColor Yellow }

function Assert-Command($name) {
    if (-not (Get-Command $name -ErrorAction SilentlyContinue)) {
        Write-Err "$name not found on PATH"; exit 1
    }
}

function Assert-RepoRoot {
    if (-not (Test-Path "go.mod")) {
        Write-Err "Run from the repo root (go.mod not found)"; exit 1
    }
}

function Invoke-Step([string]$Label, [scriptblock]$Action) {
    Write-Step $Label
    & $Action
    if ($LASTEXITCODE -ne 0) {
        Write-Err "$Label failed"; exit 1
    }
    Write-OK "$Label passed"
}

# Fetch github_info fallback from origin/marketplace into a temp file.
# Returns the temp path. Returns an empty file if the branch doesn't exist.
function Get-MarketplaceFallback {
    $fbName = "klim-fallback-$PID-$([System.Guid]::NewGuid().ToString('N')).yaml"
    $fb = Join-Path ([System.IO.Path]::GetTempPath()) $fbName
    git fetch origin marketplace --depth=1 2>&1 | Out-Null
    $content = git show origin/marketplace:marketplace.yaml 2>&1
    if ($LASTEXITCODE -eq 0 -and $content) {
        $content | Out-File $fb -Encoding utf8
        Write-OK "Loaded fallback from origin/marketplace"
    } else {
        "" | Out-File $fb -Encoding utf8
        Write-Warn "No fallback available"
    }
    return $fb
}
