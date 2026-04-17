
[CmdletBinding()]
param()

. "$PSScriptRoot/_common.ps1"

Assert-Command go
Assert-Command git
Assert-RepoRoot

Write-Step "Checking go mod tidy"

& go mod tidy
if ($LASTEXITCODE -ne 0) {
    Write-Err "go mod tidy failed"; exit 1
}

$tidyDiff = git diff --name-only go.mod go.sum 2>&1
if ($tidyDiff) {
    Write-Err "go.mod/go.sum not tidy — run 'go mod tidy' and commit"; exit 1
}
Write-OK "Modules tidy"
