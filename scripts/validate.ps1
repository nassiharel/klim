
[CmdletBinding()]
param(
    [string]$MarketplaceDir = "marketplace"
)

. "$PSScriptRoot/_common.ps1"

Assert-Command go
Assert-RepoRoot

Write-Step "Validating marketplace files"

$validateArgs = @("run", "./internal/marketplace/validate")
if ($MarketplaceDir -ne "marketplace") {
    $validateArgs += @("-dir", $MarketplaceDir)
}

& go @validateArgs
if ($LASTEXITCODE -ne 0) {
    Write-Err "Validation failed"; exit 1
}
Write-OK "Validation passed"
