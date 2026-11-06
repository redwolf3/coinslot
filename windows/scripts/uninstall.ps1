#Requires -RunAsAdministrator
<#
Removes the scheduled task and installed files. Per-user logs under
%LOCALAPPDATA%\Coinslot\logs are left in place.
#>
param([string]$Dest = (Join-Path $env:ProgramFiles 'Coinslot'))
$ErrorActionPreference = 'Stop'
Unregister-ScheduledTask -TaskName 'Collector' -TaskPath '\Coinslot\' -Confirm:$false -ErrorAction SilentlyContinue
Get-Process -Name coinslotw -ErrorAction SilentlyContinue | Stop-Process -Force
if (Test-Path $Dest) { Remove-Item -Recurse -Force $Dest }
Write-Host "Uninstalled. Logs remain in each user's %LOCALAPPDATA%\Coinslot\logs."
