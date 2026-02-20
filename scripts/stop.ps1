# stop.ps1 - Stop the gateway server running on the configured port

param(
    [int]$Port = 8080
)

$conn = Get-NetTCPConnection -LocalPort $Port -State Listen -ErrorAction SilentlyContinue

if (-not $conn) {
    Write-Host "No process found on port $Port" -ForegroundColor Yellow
    exit 0
}

$pid = $conn.OwningProcess
$proc = Get-Process -Id $pid -ErrorAction SilentlyContinue

if ($proc) {
    Stop-Process -Id $pid -Force
    Write-Host "Stopped '$($proc.ProcessName)' (PID $pid) on port $Port" -ForegroundColor Green
} else {
    Write-Host "Could not find process with PID $pid" -ForegroundColor Red
    exit 1
}
