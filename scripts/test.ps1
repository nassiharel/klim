. "$PSScriptRoot/_common.ps1"
Assert-Command go
Assert-RepoRoot

Write-Step "Running tests"
$testArgs = @("test", "-count=1", "./...")
if ($env:CGO_ENABLED -ne "0" -and (Get-Command gcc -ErrorAction SilentlyContinue)) {
    $testArgs = @("test", "-race", "-count=1", "./...")
}
& go @testArgs
if ($LASTEXITCODE -ne 0) { Write-Err "Tests failed"; exit 1 }
Write-OK "All tests passed"
