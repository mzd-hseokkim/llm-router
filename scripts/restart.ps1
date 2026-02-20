# restart.ps1 - Stop the running gateway and start it again

param(
    [int]$Port = 8080
)

Write-Host "Restarting gateway on port $Port..." -ForegroundColor Cyan

& "$PSScriptRoot\stop.ps1" -Port $Port

Start-Sleep -Milliseconds 500

& "$PSScriptRoot\start.ps1"
