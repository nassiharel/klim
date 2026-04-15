# clim installer — downloads the latest release binary for Windows.
#
# Usage:
#   irm https://raw.githubusercontent.com/nassiharel/clim/main/install.ps1 | iex
#
#   # Install a specific version:
#   $env:CLIM_VERSION = "v1.2.3"; irm https://raw.githubusercontent.com/nassiharel/clim/main/install.ps1 | iex
#
#   # Custom install directory:
#   $env:CLIM_INSTALL_DIR = "C:\tools\clim"; irm https://raw.githubusercontent.com/nassiharel/clim/main/install.ps1 | iex

$ErrorActionPreference = "Stop"

$BinaryName = "clim"
$GithubRepo = "nassiharel/clim"

# --- Configuration via env vars ---
$DesiredVersion = $env:CLIM_VERSION
$InstallDir = if ($env:CLIM_INSTALL_DIR) { $env:CLIM_INSTALL_DIR } else { "$env:LOCALAPPDATA\Programs\clim" }
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
    Write-Host "[error] Set `$env:CLIM_VERSION = 'v1.0.0' and retry." -ForegroundColor Red
    exit 1
}

function Get-Arch {
    $arch = $env:PROCESSOR_ARCHITECTURE
    switch ($arch) {
        "AMD64" { return "amd64" }
        "x86"   { return "amd64" }  # 32-bit PS on 64-bit OS is common
        default {
            Write-Host "[error] Unsupported architecture: $arch" -ForegroundColor Red
            exit 1
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

function Install-Clim {
    $arch = Get-Arch
    $tag = Get-LatestVersion

    Write-Host "[info]  Installing $BinaryName $tag for windows/$arch..." -ForegroundColor Green

    # Check if already installed
    if (Test-InstalledVersion -Tag $tag) {
        return
    }

    $version = $tag.TrimStart("v")
    $zipName = "${BinaryName}_${version}_windows_${arch}.zip"
    $downloadUrl = "https://github.com/$GithubRepo/releases/download/$tag/$zipName"
    $checksumUrl = "https://github.com/$GithubRepo/releases/download/$tag/checksums.txt"

    # Create temp directory
    $tempDir = Join-Path $env:TEMP "clim-install-$(Get-Random)"
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
    finally {
        # Cleanup
        if (Test-Path $tempDir) {
            Remove-Item -Path $tempDir -Recurse -Force -ErrorAction SilentlyContinue
        }
    }
}

# --- Main ---
Install-Clim
