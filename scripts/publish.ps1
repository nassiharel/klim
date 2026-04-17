# Verify, assemble with GitHub metadata, and push to origin/marketplace.
[CmdletBinding()]
param(
    [switch]$FetchGitHub,
    [switch]$SkipVerify
)

. "$PSScriptRoot/_common.ps1"
Assert-Command go
Assert-Command git
Assert-RepoRoot

$fallback = Get-MarketplaceFallback

# --- Verify (or just validate) ---
if ($SkipVerify) {
    & "$PSScriptRoot/validate.ps1";  if ($LASTEXITCODE -ne 0) { exit 1 }
} else {
    & "$PSScriptRoot/verify.ps1" -FallbackFile $fallback
    if ($LASTEXITCODE -ne 0) { exit 1 }
}

# --- Assemble to temp (outside repo — survives git clean) ---
Write-Step "Assembling to temp"
$tmp = Join-Path ([System.IO.Path]::GetTempPath()) "clim-mp-$(Get-Random).yaml"
$goArgs = @("run", "./internal/marketplace/assemble", "-fallback", $fallback, "-o", $tmp)
if ($FetchGitHub) { $goArgs += "-fetch-github" }
& go @goArgs
if ($LASTEXITCODE -ne 0) { Remove-Item $tmp -EA 0; Write-Err "Assembly failed"; exit 1 }

$tools = (Get-ChildItem "marketplace/tools/*.yaml").Count
$packs = (Get-ChildItem "marketplace/packs/*.yaml").Count
Write-OK "Assembled $tools tools + $packs packs"

# --- Publish ---
Write-Step "Publishing to marketplace branch"
$branch = git rev-parse --abbrev-ref HEAD 2>&1
$commit = git rev-parse --short HEAD 2>&1

$stashed = $false
if (git status --porcelain 2>&1) {
    Write-Warn "Stashing uncommitted changes..."
    git stash push -m "publish-marketplace" --quiet
    $stashed = $true
}

try {
    git config user.name "publish-marketplace"
    git config user.email "noreply@local"

    git checkout --orphan marketplace-staging 2>&1 | Out-Null
    git rm -rf --ignore-unmatch .              2>&1 | Out-Null
    git clean -fdx                             2>&1 | Out-Null

    Copy-Item $tmp "marketplace.yaml" -Force
    git add marketplace.yaml
    git commit -m "Update marketplace catalog`n`nSource: $commit on $branch`nTools: $tools | Packs: $packs" --quiet

    git push -f origin marketplace-staging:marketplace
    if ($LASTEXITCODE -ne 0) { throw "git push failed" }
    Write-OK "Published to origin/marketplace"
}
finally {
    git checkout $branch --force --quiet 2>&1 | Out-Null
    git branch -D marketplace-staging    2>&1 | Out-Null
    if ($stashed) { git stash pop --quiet 2>&1 | Out-Null; Write-Warn "Restored stash" }
    Remove-Item $tmp, $fallback -EA 0
}

Write-Host "`nDone." -ForegroundColor Green
