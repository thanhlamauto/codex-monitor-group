#Requires -Version 5.1
[CmdletBinding()]
param()

$ErrorActionPreference = "Stop"
$identity = [Security.Principal.WindowsIdentity]::GetCurrent()
$principal = [Security.Principal.WindowsPrincipal]::new($identity)
if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    throw "Open PowerShell with 'Run as Administrator' and run the command again."
}

$InstallDir = Join-Path $env:ProgramFiles "CodexGuard"
$DataDir = Join-Path $env:ProgramData "CodexGuard"
$ConfigPath = Join-Path $DataDir "config.json"
$AgentPath = Join-Path $InstallDir "codex-guard.exe"

& schtasks.exe /End /TN CodexGuardWatchdog 2>$null | Out-Null
& schtasks.exe /Delete /TN CodexGuardWatchdog /F 2>$null | Out-Null
$service = Get-Service -Name CodexGuard -ErrorAction SilentlyContinue
if ($service) {
    Stop-Service -Name CodexGuard -Force -ErrorAction SilentlyContinue
    $service.WaitForStatus("Stopped", [TimeSpan]::FromSeconds(30))
}
if ((Test-Path -LiteralPath $AgentPath) -and (Test-Path -LiteralPath $ConfigPath)) {
    & $AgentPath uninstall-notice --config $ConfigPath 2>$null
}
if ($service) {
    & sc.exe delete CodexGuard | Out-Null
}
if (Test-Path -LiteralPath $InstallDir) { Remove-Item -LiteralPath $InstallDir -Recurse -Force }
Write-Host "Codex Classroom Monitor removed. DPAPI credentials and integrity state remain in $DataDir for recovery. The dashboard will mark this device UNREACHABLE after its timeout."
