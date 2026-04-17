<#
.SYNOPSIS
    Full verification pipeline: validate + lint + tidy + test + assemble.
.DESCRIPTION
    Runs all checks in sequence. Exits on first failure.
    Does not publish — use scripts/publish.ps1 for that.
.PARAMETER FetchGitHub
    Pass through to assemble step.
.PARAMETER MarketplaceDir
    Pass through to validate/assemble steps.
#>

[CmdletBinding()]
param(
    [switch]$FetchGitHub,
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

$assembleArgs = @{ MarketplaceDir = $MarketplaceDir }
if ($FetchGitHub) { $assembleArgs.FetchGitHub = $true }
& "$PSScriptRoot/assemble.ps1" @assembleArgs
if ($LASTEXITCODE -ne 0) { exit 1 }

Write-Host "`nAll checks passed." -ForegroundColor Green
