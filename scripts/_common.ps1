<#
.SYNOPSIS
    Shared helpers for clim development scripts.
.DESCRIPTION
    Dot-source this from other scripts: . "$PSScriptRoot/_common.ps1"
#>

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
