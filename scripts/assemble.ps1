[CmdletBinding()]
param(
    [switch]$FetchGitHub,
    [string]$FallbackFile,
    [string]$OutputFile = "marketplace.yaml"
)

. "$PSScriptRoot/_common.ps1"
Assert-Command go
Assert-RepoRoot

Write-Step "Assembling marketplace.yaml"

if (-not $FallbackFile) { $FallbackFile = Get-MarketplaceFallback }

$goArgs = @("run", "./internal/marketplace/assemble", "-fallback", $FallbackFile, "-o", $OutputFile)
if ($FetchGitHub) { $goArgs += "-fetch-github" }

& go @goArgs
if ($LASTEXITCODE -ne 0) { Write-Err "Assembly failed"; exit 1 }

$tools = (Get-ChildItem "marketplace/tools/*.yaml").Count
$packs = (Get-ChildItem "marketplace/packs/*.yaml").Count
$size  = "{0:N0} KB" -f ((Get-Item $OutputFile).Length / 1KB)
Write-OK "Assembled $tools tools + $packs packs ($size) -> $OutputFile"
