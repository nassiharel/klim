. "$PSScriptRoot/_common.ps1"
Assert-Command go
Assert-RepoRoot
Invoke-Step "Validating marketplace" { go run ./internal/marketplace/validate }
