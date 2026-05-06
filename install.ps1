# klim installer — downloads the latest release binary for Windows.
#
# Usage:
#   irm https://raw.githubusercontent.com/nassiharel/klim/main/install.ps1 | iex
#
#   # Install a specific version:
#   $env:CLIM_VERSION = "v1.2.3"; irm https://raw.githubusercontent.com/nassiharel/klim/main/install.ps1 | iex
#
#   # Custom install directory:
#   $env:CLIM_INSTALL_DIR = "C:\tools\klim"; irm https://raw.githubusercontent.com/nassiharel/klim/main/install.ps1 | iex

$ErrorActionPreference = "Stop"

$BinaryName = "klim"
$GithubRepo = "nassiharel/klim"

# --- Configuration via env vars ---
$DesiredVersion = $env:CLIM_VERSION
$InstallDir = if ($env:CLIM_INSTALL_DIR) { $env:CLIM_INSTALL_DIR } else { "$env:LOCALAPPDATA\Programs\klim" }
$VerifyChecksum = if ($env:CLIM_VERIFY_CHECKSUM -eq "false") { $false } else { $true }

function Get-LatestVersion {
    if ($DesiredVersion) {
        $v = $DesiredVersion
        if (-not $v.StartsWith("v")) { $v = "v$v" }
        return $v
    }

    try {
        $release = Invoke-RestMethod -Uri "https://api.github.com/repos/$GithubRepo/releases/latest"
        return $release.tag_name
    }
    catch {
        # Fallback: follow redirect
        try {
            $response = Invoke-WebRequest -Uri "https://github.com/$GithubRepo/releases/latest" -MaximumRedirection 0 -ErrorAction SilentlyContinue
            $null = $response  # Suppress PSScriptAnalyzer warning; we only need the redirect exception
        }
        catch {
            if ($_.Exception.Response.Headers.Location) {
                $location = $_.Exception.Response.Headers.Location.ToString()
                if ($location -match "v[\d.]+") {
                    return $Matches[0]
                }
            }
        }
    }

    Write-Host "[error] Could not determine the latest version." -ForegroundColor Red
    Write-Host "[error] This may be caused by GitHub API rate limiting." -ForegroundColor Red
    Write-Host "[error] Set `$env:CLIM_VERSION = 'v1.0.0' and retry." -ForegroundColor Red
    Write-Host "[error] Check releases at: https://github.com/$GithubRepo/releases" -ForegroundColor Red
    exit 1
}

function Get-Arch {
    $arch = $env:PROCESSOR_ARCHITECTURE
    switch ($arch) {
        "AMD64" { return "amd64" }
        "x86"   { return "amd64" }  # 32-bit PS on 64-bit OS is common
        "ARM64" { return "arm64" }
        default {
            Write-Host "[warn]  Unknown architecture: $arch — will attempt go install fallback." -ForegroundColor Yellow
            return $null
        }
    }
}

function Test-InstalledVersion {
    param([string]$Tag)

    $exePath = Join-Path $InstallDir "$BinaryName.exe"
    if (Test-Path $exePath) {
        try {
            $output = & $exePath version 2>&1
            $installed = [regex]::Match($output, '\d+\.\d+\.\d+').Value
            $desired = $Tag.TrimStart("v")
            if ($installed -eq $desired) {
                Write-Host "[info]  $BinaryName $Tag is already installed." -ForegroundColor Green
                return $true
            }
        }
        catch {}
    }
    return $false
}

function Install-GoFallback {
    param([string]$Tag)

    $goCmd = Get-Command go -ErrorAction SilentlyContinue
    if (-not $goCmd) {
        Write-Host "[error] No prebuilt binary for this platform and Go is not installed." -ForegroundColor Red
        Write-Host "[error] Install Go (https://go.dev/dl/) or use a supported platform." -ForegroundColor Red
        exit 1
    }

    Write-Host "[info]  Building from source via go install..." -ForegroundColor Green
    & go install "github.com/nassiharel/klim/cmd/klim@$Tag"
    if ($LASTEXITCODE -ne 0) {
        Write-Host "[error] go install failed." -ForegroundColor Red
        exit 1
    }

    $gopath = & go env GOPATH
    $goBin = Join-Path $gopath "bin\$BinaryName.exe"
    if (-not (Test-Path $goBin)) {
        $goBin = & go env GOBIN
        $goBin = Join-Path $goBin "$BinaryName.exe"
    }

    if (-not (Test-Path $goBin)) {
        Write-Host "[error] go install succeeded but binary not found in GOPATH/bin or GOBIN." -ForegroundColor Red
        exit 1
    }

    if (-not (Test-Path $InstallDir)) {
        New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
    }

    Copy-Item -Path $goBin -Destination (Join-Path $InstallDir "$BinaryName.exe") -Force
    Write-Host "[info]  Installed $BinaryName.exe (built from source) to $InstallDir" -ForegroundColor Green
}

function Add-ToPathAndVerify {
    # Add to PATH if not already present
    $userPath = [Environment]::GetEnvironmentVariable("PATH", "User")
    if ($userPath -notlike "*$InstallDir*") {
        [Environment]::SetEnvironmentVariable("PATH", "$userPath;$InstallDir", "User")
        $env:PATH = "$env:PATH;$InstallDir"
        Write-Host "[info]  Added $InstallDir to user PATH." -ForegroundColor Green
        Write-Host "[info]  Restart your terminal for PATH changes to take effect." -ForegroundColor Yellow
    }

    # Verify installation
    try {
        $versionOutput = & (Join-Path $InstallDir "$BinaryName.exe") version 2>&1
        Write-Host "[info]  $versionOutput" -ForegroundColor Green
    }
    catch {
        Write-Host "[info]  $BinaryName installed successfully." -ForegroundColor Green
    }
}

function Install-Clim {
    $arch = Get-Arch
    $tag = Get-LatestVersion

    # Check if already installed
    if (Test-InstalledVersion -Tag $tag) {
        return
    }

    # Determine if prebuilt binary exists for this arch
    $hasPrebuilt = ($null -ne $arch) -and ($arch -eq "amd64")
    if (-not $hasPrebuilt) {
        Write-Host "[info]  No prebuilt binary for windows/$($arch ?? $env:PROCESSOR_ARCHITECTURE)." -ForegroundColor Yellow
        Install-GoFallback -Tag $tag
        Add-ToPathAndVerify
        return
    }

    Write-Host "[info]  Installing $BinaryName $tag for windows/$arch..." -ForegroundColor Green

    $version = $tag.TrimStart("v")
    $zipName = "${BinaryName}_${version}_windows_${arch}.zip"
    $downloadUrl = "https://github.com/$GithubRepo/releases/download/$tag/$zipName"
    $checksumUrl = "https://github.com/$GithubRepo/releases/download/$tag/checksums.txt"

    # Create temp directory
    $tempDir = Join-Path $env:TEMP "klim-install-$(Get-Random)"
    New-Item -ItemType Directory -Path $tempDir -Force | Out-Null

    $zipPath = Join-Path $tempDir $zipName
    $checksumPath = Join-Path $tempDir "checksums.txt"

    try {
        # Download archive
        Write-Host "[info]  Downloading $downloadUrl" -ForegroundColor Green
        [Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
        Invoke-WebRequest -Uri $downloadUrl -OutFile $zipPath

        # Download checksums
        Invoke-WebRequest -Uri $checksumUrl -OutFile $checksumPath

        # Verify checksum
        if ($VerifyChecksum) {
            Write-Host "[info]  Verifying checksum..." -ForegroundColor Green
            $actual = (Get-FileHash -Path $zipPath -Algorithm SHA256).Hash.ToLower()
            $checksumContent = Get-Content $checksumPath
            $expectedLine = $checksumContent | Where-Object { $_.Contains($zipName) }

            if ($expectedLine) {
                $expected = ($expectedLine -split '\s+')[0].ToLower()
                if ($actual -ne $expected) {
                    Write-Host "[error] Checksum verification failed!" -ForegroundColor Red
                    Write-Host "  Expected: $expected" -ForegroundColor Red
                    Write-Host "  Got:      $actual" -ForegroundColor Red
                    exit 1
                }
                Write-Host "[info]  Checksum verified." -ForegroundColor Green
            }
            else {
                Write-Host "[warn]  Checksum entry not found for $zipName. Skipping." -ForegroundColor Yellow
            }
        }

        # Extract
        $extractDir = Join-Path $tempDir "extract"
        Expand-Archive -Path $zipPath -DestinationPath $extractDir -Force

        # Create install directory
        if (-not (Test-Path $InstallDir)) {
            New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
        }

        # Copy binary
        $sourceBin = Join-Path $extractDir "$BinaryName.exe"
        if (-not (Test-Path $sourceBin)) {
            Write-Host "[error] Binary '$BinaryName.exe' not found in archive." -ForegroundColor Red
            exit 1
        }

        Copy-Item -Path $sourceBin -Destination (Join-Path $InstallDir "$BinaryName.exe") -Force
        Write-Host "[info]  Installed $BinaryName.exe to $InstallDir" -ForegroundColor Green

        Add-ToPathAndVerify
    }
    finally {
        # Cleanup
        if (Test-Path $tempDir) {
            Remove-Item -Path $tempDir -Recurse -Force -ErrorAction SilentlyContinue
        }
    }
}

# --- Main ---
Install-Clim
