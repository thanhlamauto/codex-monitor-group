#Requires -Version 5.1
[CmdletBinding()]
param(
    [string]$Server = $(if ($env:CODEX_GUARD_SERVER) { $env:CODEX_GUARD_SERVER } else { "https://codex-classroom-monitor.vercel.app" }),
    [string]$Name = "",
    [string]$DeviceLabel = "",
    [string]$CodexHome = "",
    [switch]$Upgrade
)

$ErrorActionPreference = "Stop"
$ProgressPreference = "SilentlyContinue"
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12

$identity = [Security.Principal.WindowsIdentity]::GetCurrent()
$principal = [Security.Principal.WindowsPrincipal]::new($identity)
if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    throw "Open PowerShell with 'Run as Administrator' and run the command again."
}

$Server = $Server.TrimEnd("/")
if (-not $Server.StartsWith("https://")) { throw "Server must use HTTPS." }

$architecture = [Runtime.InteropServices.RuntimeInformation]::OSArchitecture.ToString().ToLowerInvariant()
switch ($architecture) {
    "x64" { $arch = "amd64" }
    "arm64" { $arch = "arm64" }
    default { throw "Unsupported Windows architecture: $architecture" }
}

$InstallDir = Join-Path $env:ProgramFiles "CodexGuard"
$DataDir = Join-Path $env:ProgramData "CodexGuard"
$StateDir = Join-Path $DataDir "state"
$ConfigPath = Join-Path $DataDir "config.json"
$AgentPath = Join-Path $InstallDir "codex-guard.exe"
$CCUsagePath = Join-Path $InstallDir "ccusage.exe"

if (-not $CodexHome) {
    if ($env:CODEX_HOME) { $CodexHome = $env:CODEX_HOME }
    else { $CodexHome = Join-Path $env:USERPROFILE ".codex" }
}
$CodexHome = [IO.Path]::GetFullPath($CodexHome)
if (-not $DeviceLabel) { $DeviceLabel = $env:COMPUTERNAME }
if (-not (Test-Path -LiteralPath $ConfigPath)) {
    if (-not $Name) { $Name = Read-Host "Tên hiển thị trên dashboard" }
    $Name = $Name.Trim()
    if (-not $Name) { throw "Name cannot be empty. Re-run with -Name 'Your name'." }
    if ($Name.Length -gt 200) { throw "Name cannot be longer than 200 characters." }
}

$codexCommand = Get-Command codex.exe -ErrorAction SilentlyContinue
if (-not $codexCommand) { $codexCommand = Get-Command codex -ErrorAction SilentlyContinue }
$CodexPath = if ($codexCommand) { $codexCommand.Source } else { "codex.exe" }

$tempDir = Join-Path ([IO.Path]::GetTempPath()) ("codex-guard-" + [Guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Path $tempDir | Out-Null
try {
    Write-Host "Downloading verified Windows release for $arch..."
    $checksumsPath = Join-Path $tempDir "checksums.txt"
    Invoke-WebRequest -UseBasicParsing -Uri "$Server/downloads/checksums.txt" -OutFile $checksumsPath
    $expected = @{}
    foreach ($line in Get-Content -LiteralPath $checksumsPath) {
        if ($line -match '^([0-9a-fA-F]{64})\s+\*?(.+)$') { $expected[$Matches[2].Trim()] = $Matches[1].ToLowerInvariant() }
    }
    $artifacts = @("codex-guard-windows-$arch.exe", "ccusage-windows-$arch.exe")
    foreach ($artifact in $artifacts) {
        if (-not $expected.ContainsKey($artifact)) { throw "No checksum published for $artifact" }
        $destination = Join-Path $tempDir $artifact
        Invoke-WebRequest -UseBasicParsing -Uri "$Server/downloads/$artifact" -OutFile $destination
        $actual = (Get-FileHash -Algorithm SHA256 -LiteralPath $destination).Hash.ToLowerInvariant()
        if ($actual -ne $expected[$artifact]) { throw "Checksum verification failed for $artifact" }
    }

    # Stop both existing execution paths only after the replacement artifacts
    # have passed verification. This also releases the .exe for an in-place update.
    & schtasks.exe /End /TN CodexGuardWatchdog 2>$null | Out-Null
    & schtasks.exe /Delete /TN CodexGuardWatchdog /F 2>$null | Out-Null
    $existingService = Get-Service -Name CodexGuard -ErrorAction SilentlyContinue
    if ($existingService -and $existingService.Status -ne "Stopped") {
        Stop-Service -Name CodexGuard -Force
        $existingService.WaitForStatus("Stopped", [TimeSpan]::FromSeconds(30))
    }

    New-Item -ItemType Directory -Force -Path $InstallDir, $DataDir, $StateDir, $CodexHome | Out-Null
    Copy-Item -Force -LiteralPath (Join-Path $tempDir "codex-guard-windows-$arch.exe") -Destination $AgentPath
    Copy-Item -Force -LiteralPath (Join-Path $tempDir "ccusage-windows-$arch.exe") -Destination $CCUsagePath

    if (-not (Test-Path -LiteralPath $ConfigPath)) {
        Write-Host "Đang thêm $Name ($DeviceLabel) lên dashboard..."
        & $AgentPath enroll --server $Server --name $Name --device-label $DeviceLabel --codex-home $CodexHome --codex $CodexPath --ccusage $CCUsagePath --agent $AgentPath --state-dir $StateDir --config $ConfigPath
        if ($LASTEXITCODE -ne 0) { throw "Device enrollment failed." }
    }
    & $AgentPath configure-telemetry --config $ConfigPath
    if ($LASTEXITCODE -ne 0) { throw "Codex telemetry configuration failed." }

    foreach ($protectedDir in @($InstallDir, $DataDir)) {
        & icacls.exe $protectedDir /inheritance:r /grant:r '*S-1-5-18:(OI)(CI)F' '*S-1-5-32-544:(OI)(CI)F' /T /C | Out-Null
        if ($LASTEXITCODE -ne 0) { throw "Failed to secure ACL on $protectedDir" }
    }

    $serviceCommand = '"' + $AgentPath + '" run --config "' + $ConfigPath + '"'
    if (Get-Service -Name CodexGuard -ErrorAction SilentlyContinue) {
        & sc.exe config CodexGuard binPath= $serviceCommand start= delayed-auto obj= LocalSystem | Out-Null
    } else {
        & sc.exe create CodexGuard binPath= $serviceCommand start= delayed-auto obj= LocalSystem DisplayName= "Codex Classroom Monitor" | Out-Null
    }
    if ($LASTEXITCODE -ne 0) { throw "Failed to create the CodexGuard Windows service." }
    & sc.exe description CodexGuard "Signed token-usage and integrity monitor for Codex CLI" | Out-Null
    & sc.exe failure CodexGuard reset= 86400 actions= restart/5000/restart/15000/restart/60000 | Out-Null
    & sc.exe failureflag CodexGuard 1 | Out-Null

    $watchdogCommand = '"' + $AgentPath + '" watchdog --config "' + $ConfigPath + '"'
    & schtasks.exe /Create /TN CodexGuardWatchdog /SC MINUTE /MO 2 /RU SYSTEM /RL HIGHEST /TR $watchdogCommand /F | Out-Null
    if ($LASTEXITCODE -ne 0) { throw "Failed to create the CodexGuard watchdog task." }

    Start-Service -Name CodexGuard
    Start-Sleep -Seconds 2
    $service = Get-Service -Name CodexGuard
    if ($service.Status -ne "Running") { throw "CodexGuard service did not start." }

    Write-Host ""
    Write-Host "Codex Classroom Monitor installed."
    Write-Host ""
    & $AgentPath status --config $ConfigPath
    Write-Host "Protection   Windows Service + recovery + watchdog + DPAPI + restricted ACL"
}
finally {
    if (Test-Path -LiteralPath $tempDir) { Remove-Item -LiteralPath $tempDir -Recurse -Force }
}
