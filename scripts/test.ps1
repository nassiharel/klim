

[CmdletBinding()]
param()

. "$PSScriptRoot/_common.ps1"

Assert-Command go
Assert-RepoRoot

Write-Step "Running tests"

$testArgs = @("test", "-count=1", "./...")
$cgoEnabled = $env:CGO_ENABLED
if ($cgoEnabled -eq "0") {
    # Explicitly disabled
} elseif ((Get-Command gcc -ErrorAction SilentlyContinue) -or ($cgoEnabled -eq "1")) {
    $testArgs = @("test", "-race", "-count=1", "./...")
}

& go @testArgs
if ($LASTEXITCODE -ne 0) {
    Write-Err "Tests failed"; exit 1
}
Write-OK "All tests passed"
