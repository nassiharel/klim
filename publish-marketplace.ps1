<#
.SYNOPSIS
    Validate marketplace source files and publish an assembled marketplace.yaml
    to the 'marketplace' branch.

.DESCRIPTION
    1. Validates individual YAML files in marketplace/tools/ and marketplace/packs/
    2. Runs lint, tidy, and tests
    3. Assembles them into a single marketplace.yaml (with optional GitHub metadata enrichment)
    4. Pushes the result to the orphan 'marketplace' branch on origin

    Equivalent to what the GitHub Actions workflow (.github/workflows/marketplace.yml) does,
    but runnable locally.

.PARAMETER ValidateOnly
    Run validation only — skip assembly and publish.

.PARAMETER SkipPublish
    Validate and assemble but don't push to the marketplace branch.

.PARAMETER FetchGitHub
    Fetch GitHub repository metadata (stars, description, license, etc.).
    Requires GITHUB_TOKEN env var or gh CLI auth.

.PARAMETER OutputFile
    Path for the assembled marketplace.yaml. Defaults to a temp file when publishing,
    or .\marketplace.yaml when using -SkipPublish.

.PARAMETER MarketplaceDir
    Path to the marketplace source directory. Default: marketplace

.EXAMPLE
    .\publish-marketplace.ps1                          # validate + assemble + publish
    .\publish-marketplace.ps1 -ValidateOnly            # validate only
    .\publish-marketplace.ps1 -SkipPublish              # validate + assemble to .\marketplace.yaml
    .\publish-marketplace.ps1 -FetchGitHub -SkipPublish # assemble with GitHub metadata
#>

[CmdletBinding()]
param(
    [switch]$ValidateOnly,
    [switch]$SkipPublish,
    [switch]$FetchGitHub,
    [string]$OutputFile,
    [string]$MarketplaceDir = "marketplace"
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

function Write-Step($msg) { Write-Host "`n:: $msg" -ForegroundColor Cyan }
function Write-OK($msg)   { Write-Host "   $msg" -ForegroundColor Green }
function Write-Err($msg)  { Write-Host "   $msg" -ForegroundColor Red }

# --- Preflight ---
Write-Step "Preflight checks"

if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
    Write-Err "go not found on PATH"; exit 1
}

if (-not (Test-Path "$MarketplaceDir/categories.yaml")) {
    Write-Err "Cannot find $MarketplaceDir/categories.yaml — run from the repo root"; exit 1
}

if (-not $SkipPublish -and -not $ValidateOnly) {
    if (-not (Get-Command git -ErrorAction SilentlyContinue)) {
        Write-Err "git not found on PATH"; exit 1
    }
    $remotes = git remote -v 2>&1
    if ($LASTEXITCODE -ne 0) {
        Write-Err "Not a git repository"; exit 1
    }
}

Write-OK "All preflight checks passed"

# --- Validate ---
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

if ($ValidateOnly) {
    Write-Host "`nDone (validate only)." -ForegroundColor Green
    exit 0
}

# --- Lint ---
Write-Step "Linting (golangci-lint)"

if (Get-Command golangci-lint -ErrorAction SilentlyContinue) {
    & golangci-lint run --new-from-rev=HEAD~1
    if ($LASTEXITCODE -ne 0) {
        Write-Err "Lint failed"; exit 1
    }
    Write-OK "Lint passed"
} else {
    Write-Host "   golangci-lint not found — skipping" -ForegroundColor Yellow
}

# --- Tidy ---
Write-Step "Checking go mod tidy"

& go mod tidy
if ($LASTEXITCODE -ne 0) {
    Write-Err "go mod tidy failed"; exit 1
}

$tidyDiff = git diff --name-only go.mod go.sum 2>&1
if ($tidyDiff) {
    Write-Err "go.mod/go.sum not tidy — run 'go mod tidy' and commit"; exit 1
}
Write-OK "Modules tidy"

# --- Test ---
Write-Step "Running tests"

# Use -race when CGO is available, skip it otherwise (e.g. Windows without gcc)
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

# --- Assemble ---
Write-Step "Assembling marketplace.yaml"

# Always assemble to a temp file to avoid git clean nuking the output
$tempOutput = [System.IO.Path]::Combine([System.IO.Path]::GetTempPath(), "clim-marketplace-$([System.Guid]::NewGuid().ToString('N').Substring(0,8)).yaml")

if (-not $OutputFile) {
    if ($SkipPublish) {
        $OutputFile = "marketplace.yaml"
    } else {
        $OutputFile = $tempOutput
    }
}

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
$fileSize  = "{0:N0} KB" -f ((Get-Item $tempOutput).Length / 1KB)

if ($SkipPublish) {
    # Copy from temp to the requested output location
    if ($OutputFile -ne $tempOutput) {
        Copy-Item $tempOutput -Destination $OutputFile -Force
        Remove-Item $tempOutput -ErrorAction SilentlyContinue
    }
    Write-OK "Assembled $toolCount tools + $packCount packs ($fileSize) -> $OutputFile"
    Write-Host "`nDone (skip publish). Output: $OutputFile" -ForegroundColor Green
    exit 0
}

Write-OK "Assembled $toolCount tools + $packCount packs ($fileSize)"

# --- Publish ---
Write-Step "Publishing to marketplace branch"

$currentBranch = git rev-parse --abbrev-ref HEAD 2>&1
$currentCommit = git rev-parse --short HEAD 2>&1

# Stash any uncommitted changes
$stashed = $false
$status = git status --porcelain 2>&1
if ($status) {
    Write-Host "   Stashing uncommitted changes..." -ForegroundColor Yellow
    git stash push -m "publish-marketplace auto-stash" --quiet
    $stashed = $true
}

try {
    git config user.name  "publish-marketplace.ps1"
    git config user.email "noreply@local"

    # Create orphan branch with only marketplace.yaml
    git checkout --orphan marketplace-staging 2>&1 | Out-Null
    git rm -rf --ignore-unmatch . 2>&1 | Out-Null
    git clean -fdx 2>&1 | Out-Null

    # Copy from temp (survives git clean because it's outside the repo)
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
    # Always restore original branch
    git checkout $currentBranch --force --quiet 2>&1 | Out-Null
    git branch -D marketplace-staging 2>&1 | Out-Null

    if ($stashed) {
        git stash pop --quiet 2>&1 | Out-Null
        Write-Host "   Restored stashed changes" -ForegroundColor Yellow
    }

    # Clean up temp file
    Remove-Item $tempOutput -ErrorAction SilentlyContinue
}

Write-Host "`nDone. Marketplace published to origin/marketplace." -ForegroundColor Green
