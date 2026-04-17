<#
.SYNOPSIS
    Publish assembled marketplace.yaml to the 'marketplace' branch on origin.
.DESCRIPTION
    Runs full verification (validate + lint + tidy + test + assemble), then
    pushes the assembled catalog to the orphan 'marketplace' branch.
    Assembles to a temp file outside the repo so git clean can't nuke it.
.PARAMETER FetchGitHub
    Fetch GitHub repository metadata during assembly.
.PARAMETER MarketplaceDir
    Source directory. Default: marketplace
.PARAMETER SkipVerify
    Skip lint/tidy/test — only validate and assemble before publishing.
#>

[CmdletBinding()]
param(
    [switch]$FetchGitHub,
    [switch]$SkipVerify,
    [string]$MarketplaceDir = "marketplace"
)

. "$PSScriptRoot/_common.ps1"

Assert-Command go
Assert-Command git
Assert-RepoRoot

# --- Verify ---
if ($SkipVerify) {
    & "$PSScriptRoot/validate.ps1" -MarketplaceDir $MarketplaceDir
    if ($LASTEXITCODE -ne 0) { exit 1 }
} else {
    $verifyArgs = @{ MarketplaceDir = $MarketplaceDir }
    # Don't pass FetchGitHub to verify — we assemble separately to temp below
    & "$PSScriptRoot/verify.ps1" @verifyArgs
    if ($LASTEXITCODE -ne 0) { exit 1 }
}

# --- Assemble to temp (outside repo, survives git clean) ---
Write-Step "Assembling to temp file"

$tempOutput = [System.IO.Path]::Combine(
    [System.IO.Path]::GetTempPath(),
    "clim-marketplace-$([System.Guid]::NewGuid().ToString('N').Substring(0,8)).yaml"
)

$assembleArgs = @("run", "./internal/marketplace/assemble", "-o", $tempOutput)
if ($MarketplaceDir -ne "marketplace") {
    $assembleArgs += @("-dir", $MarketplaceDir)
}
if ($FetchGitHub) {
    $assembleArgs += "-fetch-github"
}

& go @assembleArgs
if ($LASTEXITCODE -ne 0) {
    Remove-Item $tempOutput -ErrorAction SilentlyContinue
    Write-Err "Assembly failed"; exit 1
}

$toolCount = (Get-ChildItem "$MarketplaceDir/tools/*.yaml").Count
$packCount = (Get-ChildItem "$MarketplaceDir/packs/*.yaml").Count
Write-OK "Assembled $toolCount tools + $packCount packs"

# --- Publish ---
Write-Step "Publishing to marketplace branch"

$currentBranch = git rev-parse --abbrev-ref HEAD 2>&1
$currentCommit = git rev-parse --short HEAD 2>&1

$stashed = $false
$status = git status --porcelain 2>&1
if ($status) {
    Write-Warn "Stashing uncommitted changes..."
    git stash push -m "publish-marketplace auto-stash" --quiet
    $stashed = $true
}

try {
    git config user.name  "publish-marketplace"
    git config user.email "noreply@local"

    git checkout --orphan marketplace-staging 2>&1 | Out-Null
    git rm -rf --ignore-unmatch . 2>&1 | Out-Null
    git clean -fdx 2>&1 | Out-Null

    Copy-Item $tempOutput -Destination "marketplace.yaml" -Force
    git add marketplace.yaml

    $commitMsg = "Update marketplace catalog`n`nSource: $currentCommit on $currentBranch`nTools: $toolCount | Packs: $packCount"
    git commit -m $commitMsg --quiet

    git push -f origin marketplace-staging:marketplace
    if ($LASTEXITCODE -ne 0) {
        Write-Err "Push failed"; throw "git push failed"
    }

    Write-OK "Published to origin/marketplace"
}
finally {
    git checkout $currentBranch --force --quiet 2>&1 | Out-Null
    git branch -D marketplace-staging 2>&1 | Out-Null

    if ($stashed) {
        git stash pop --quiet 2>&1 | Out-Null
        Write-Warn "Restored stashed changes"
    }

    Remove-Item $tempOutput -ErrorAction SilentlyContinue
}

Write-Host "`nDone. Marketplace published to origin/marketplace." -ForegroundColor Green
