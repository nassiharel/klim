<#
.SYNOPSIS
    Assemble marketplace.yaml from individual tool/pack YAML files.
.PARAMETER FetchGitHub
    Fetch GitHub repository metadata (stars, description, license, etc.).
.PARAMETER OutputFile
    Output path. Default: marketplace.yaml
.PARAMETER MarketplaceDir
    Source directory. Default: marketplace
#>

[CmdletBinding()]
param(
    [switch]$FetchGitHub,
    [string]$OutputFile = "marketplace.yaml",
    [string]$MarketplaceDir = "marketplace"
)

. "$PSScriptRoot/_common.ps1"

Assert-Command go
Assert-RepoRoot

Write-Step "Assembling marketplace.yaml"

$assembleArgs = @("run", "./internal/marketplace/assemble", "-o", $OutputFile)
if ($MarketplaceDir -ne "marketplace") {
    $assembleArgs += @("-dir", $MarketplaceDir)
}
if ($FetchGitHub) {
    $assembleArgs += "-fetch-github"
}

& go @assembleArgs
if ($LASTEXITCODE -ne 0) {
    Write-Err "Assembly failed"; exit 1
}

$toolCount = (Get-ChildItem "$MarketplaceDir/tools/*.yaml").Count
$packCount = (Get-ChildItem "$MarketplaceDir/packs/*.yaml").Count
$fileSize  = "{0:N0} KB" -f ((Get-Item $OutputFile).Length / 1KB)

Write-OK "Assembled $toolCount tools + $packCount packs ($fileSize) -> $OutputFile"
