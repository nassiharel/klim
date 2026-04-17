<#
.SYNOPSIS
    Run golangci-lint on the codebase.
.PARAMETER NewOnly
    Only report issues introduced since HEAD~1.
#>

[CmdletBinding()]
param(
    [switch]$NewOnly
)

. "$PSScriptRoot/_common.ps1"

Assert-RepoRoot

Write-Step "Linting (golangci-lint)"

if (-not (Get-Command golangci-lint -ErrorAction SilentlyContinue)) {
    Write-Warn "golangci-lint not found — skipping"
    exit 0
}

$lintArgs = @("run")
if ($NewOnly) {
    $lintArgs += "--new-from-rev=HEAD~1"
}

& golangci-lint @lintArgs
if ($LASTEXITCODE -ne 0) {
    Write-Err "Lint failed"; exit 1
}
Write-OK "Lint passed"
