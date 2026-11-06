#Requires -RunAsAdministrator
<#
Installs the Coinslot collector machine-wide and registers a scheduled task that
starts it at logon for every user, as that user, in their own session.

Run from the packaged folder (contains coinslot.exe, coinslotw.exe, config.json):
  powershell -ExecutionPolicy Bypass -File .\install.ps1

An existing config.json in the install dir is kept unless -OverwriteConfig is given.
#>
param(
    [string]$Dest = (Join-Path $env:ProgramFiles 'Coinslot'),
    [switch]$OverwriteConfig
)
$ErrorActionPreference = 'Stop'
$src = $PSScriptRoot

# Stop running collectors so the exe can be replaced.
Get-Process -Name coinslotw -ErrorAction SilentlyContinue | Stop-Process -Force

New-Item -ItemType Directory -Force -Path $Dest | Out-Null
Copy-Item (Join-Path $src 'coinslot.exe'), (Join-Path $src 'coinslotw.exe') $Dest -Force
$cfg = Join-Path $Dest 'config.json'
if ($OverwriteConfig -or -not (Test-Path $cfg)) {
    Copy-Item (Join-Path $src 'config.json') $cfg -Force
}

# BUILTIN\Users by SID, so it works on localized Windows.
$users = (New-Object System.Security.Principal.SecurityIdentifier 'S-1-5-32-545').Translate(
    [System.Security.Principal.NTAccount]).Value

$action    = New-ScheduledTaskAction -Execute (Join-Path $Dest 'coinslotw.exe') -WorkingDirectory $Dest
$trigger   = New-ScheduledTaskTrigger -AtLogOn
$principal = New-ScheduledTaskPrincipal -GroupId $users -RunLevel Limited
# Parallel: one instance per logged-on user (fast user switching). The collector's
# per-session mutex prevents duplicates within a session.
# ExecutionTimeLimit 0: never stop it (default would kill it after 72h).
$settings  = New-ScheduledTaskSettingsSet -MultipleInstances Parallel `
    -ExecutionTimeLimit ([TimeSpan]::Zero) -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries `
    -RestartCount 3 -RestartInterval (New-TimeSpan -Minutes 1)

Register-ScheduledTask -TaskName 'Collector' -TaskPath '\Coinslot\' -Action $action -Trigger $trigger `
    -Principal $principal -Settings $settings -Force | Out-Null

Write-Host "Installed to $Dest. The collector starts at each user's next logon."
Write-Host "To start it now for the current user:  & '$Dest\coinslotw.exe'"
