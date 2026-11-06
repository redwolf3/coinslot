#Requires -RunAsAdministrator
<#
Removes the scheduled task and installed files. Per-user logs under
%LOCALAPPDATA%\Coinslot\logs are left in place.
#>
param([string]$Dest = (Join-Path $env:ProgramFiles 'Coinslot'))
$ErrorActionPreference = 'Stop'
Unregister-ScheduledTask -TaskName 'Collector' -TaskPath '\Coinslot\' -Confirm:$false -ErrorAction SilentlyContinue
# Stop running collectors (both builds: coinslotw.exe from the task, coinslot.exe run by
# hand) and wait for them to exit, so their files can be replaced or deleted.
$running = @(Get-Process -Name coinslot, coinslotw -ErrorAction SilentlyContinue)
$running | Stop-Process -Force
$running | Wait-Process -Timeout 10 -ErrorAction SilentlyContinue
if (Test-Path $Dest) { Remove-Item -Recurse -Force $Dest }
Write-Host "Uninstalled. Logs remain in each user's %LOCALAPPDATA%\Coinslot\logs."
