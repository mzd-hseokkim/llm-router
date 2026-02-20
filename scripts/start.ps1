# start.ps1 - Build and start the gateway server
# Loads .env.local from the project root if it exists.

$root = Resolve-Path "$PSScriptRoot\.."
$envFile = Join-Path $root ".env.local"

if (Test-Path $envFile) {
    Get-Content $envFile | ForEach-Object {
        if ($_ -match '^([A-Za-z_][A-Za-z0-9_]*)=(.+)$') {
            [System.Environment]::SetEnvironmentVariable($Matches[1], $Matches[2], 'Process')
            Write-Host "  loaded: $($Matches[1])"
        }
    }
    Write-Host ""
}

Set-Location $root
Write-Host "Starting gateway..." -ForegroundColor Cyan
go run ./cmd/gateway
