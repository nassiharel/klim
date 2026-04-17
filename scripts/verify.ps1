[CmdletBinding()]
param(
    [switch]$FetchGitHub,
    [Parameter(Mandatory)][string]$FallbackFile,
    [string]$MarketplaceDir = "marketplace"
)

. "$PSScriptRoot/_common.ps1"

& "$PSScriptRoot/validate.ps1" -MarketplaceDir $MarketplaceDir
if ($LASTEXITCODE -ne 0) { exit 1 }

& "$PSScriptRoot/lint.ps1" -NewOnly
if ($LASTEXITCODE -ne 0) { exit 1 }

& "$PSScriptRoot/tidy.ps1"
if ($LASTEXITCODE -ne 0) { exit 1 }

& "$PSScriptRoot/test.ps1"
if ($LASTEXITCODE -ne 0) { exit 1 }

$assembleArgs = @{ FallbackFile = $FallbackFile; MarketplaceDir = $MarketplaceDir }
if ($FetchGitHub) { $assembleArgs.FetchGitHub = $true }
& "$PSScriptRoot/assemble.ps1" @assembleArgs
if ($LASTEXITCODE -ne 0) { exit 1 }

Write-Host "`nAll checks passed." -ForegroundColor Green
