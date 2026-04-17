# Run all checks: validate → lint → tidy → test → assemble.
[CmdletBinding()]
param(
    [switch]$FetchGitHub,
    [string]$FallbackFile
)

$scripts = "$PSScriptRoot"

& "$scripts/validate.ps1";                        if ($LASTEXITCODE -ne 0) { exit 1 }
& "$scripts/lint.ps1" -NewOnly;                   if ($LASTEXITCODE -ne 0) { exit 1 }
& "$scripts/tidy.ps1";                            if ($LASTEXITCODE -ne 0) { exit 1 }
& "$scripts/test.ps1";                            if ($LASTEXITCODE -ne 0) { exit 1 }

$a = @{}
if ($FallbackFile) { $a.FallbackFile = $FallbackFile }
if ($FetchGitHub)  { $a.FetchGitHub = $true }
& "$scripts/assemble.ps1" @a;                     if ($LASTEXITCODE -ne 0) { exit 1 }

Write-Host "`nAll checks passed." -ForegroundColor Green
