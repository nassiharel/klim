. "$PSScriptRoot/_common.ps1"
Assert-Command go
Assert-Command git
Assert-RepoRoot

Write-Step "Checking go mod tidy"
& go mod tidy
if ($LASTEXITCODE -ne 0) { Write-Err "go mod tidy failed"; exit 1 }
if (git diff --name-only go.mod go.sum 2>&1) {
    Write-Err "go.mod/go.sum not tidy — commit after 'go mod tidy'"; exit 1
}
Write-OK "Modules tidy"
