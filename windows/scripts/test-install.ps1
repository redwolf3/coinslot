#Requires -RunAsAdministrator
<#
CI test for install.ps1 / uninstall.ps1. Runs on a throwaway Windows machine (GitHub
windows-latest runs as admin). Do NOT run on the family PC: it installs, starts,
and uninstalls Coinslot.

  pwsh -File windows/scripts/test-install.ps1 -Package dist/coinslot

Hard failures: install/uninstall errors, wrong task registration, config handling.
Soft (warning only): the task actually launching the collector, since a CI runner's
logon session differs from a real one.
#>
param([Parameter(Mandatory)][string]$Package)
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

$Package = (Resolve-Path $Package).Path
$dest = Join-Path $env:ProgramFiles 'Coinslot'
$failures = 0
function Check([bool]$ok, [string]$what) {
    if ($ok) { Write-Host "PASS  $what" }
    else { Write-Host "FAIL  $what"; $script:failures++ }
}
function Hash([string]$p) { (Get-FileHash $p -Algorithm SHA256).Hash }

# ---- install ----
& (Join-Path $Package 'install.ps1')
Check (Test-Path (Join-Path $dest 'coinslot.exe')) 'coinslot.exe installed'
Check (Test-Path (Join-Path $dest 'coinslotw.exe')) 'coinslotw.exe installed'
Check (Test-Path (Join-Path $dest 'config.json')) 'config.json installed'
& (Join-Path $dest 'coinslot.exe') version
Check ($LASTEXITCODE -eq 0) 'installed coinslot.exe runs'

# ---- task registration ----
$task = Get-ScheduledTask -TaskPath '\Coinslot\' -TaskName 'Collector' -ErrorAction SilentlyContinue
Check ($null -ne $task) 'task \Coinslot\Collector registered'
if ($task) {
    $task | Format-List TaskPath, TaskName, State | Out-String | Write-Host
    $a = @($task.Actions)
    Check ($a.Count -eq 1 -and $a[0].Execute -eq (Join-Path $dest 'coinslotw.exe')) "action runs coinslotw.exe (got: $($a[0].Execute))"

    $t = @($task.Triggers)
    Check ($t.Count -eq 1) 'exactly one trigger'
    Check ($t[0].CimClass.CimClassName -eq 'MSFT_TaskLogonTrigger') "trigger is at-logon (got: $($t[0].CimClass.CimClassName))"
    Check ([string]::IsNullOrEmpty($t[0].UserId)) "logon trigger applies to any user (UserId: '$($t[0].UserId)')"

    $p = $task.Principal
    $sid = $null
    try { $sid = (New-Object System.Security.Principal.NTAccount $p.GroupId).Translate([System.Security.Principal.SecurityIdentifier]).Value } catch {}
    Check ($sid -eq 'S-1-5-32-545') "principal is BUILTIN\Users (GroupId: '$($p.GroupId)', SID: '$sid')"
    Check ($p.RunLevel -eq 'Limited') "run level Limited (got: $($p.RunLevel))"

    $s = $task.Settings
    Check ($s.MultipleInstances -eq 'Parallel') "multiple instances Parallel (got: $($s.MultipleInstances))"
    Check ($s.ExecutionTimeLimit -eq 'PT0S') "no execution time limit (got: '$($s.ExecutionTimeLimit)')"
    Check (-not $s.DisallowStartIfOnBatteries) 'starts on battery'
    Check (-not $s.StopIfGoingOnBatteries) 'keeps running on battery'
    Check ($s.RestartCount -eq 3) "restart count 3 (got: $($s.RestartCount))"
}

# ---- task launches the collector (soft) ----
$logDir = Join-Path $env:LOCALAPPDATA 'Coinslot\logs'
if (Test-Path $logDir) { Remove-Item -Recurse -Force $logDir }
Start-ScheduledTask -TaskPath '\Coinslot\' -TaskName 'Collector'
$proc = $null
for ($i = 0; $i -lt 20 -and -not $proc; $i++) {
    Start-Sleep -Seconds 1
    $proc = Get-Process -Name coinslotw -ErrorAction SilentlyContinue
}
if ($proc) {
    Write-Host "PASS  task started coinslotw.exe (pid $($proc.Id -join ','))"
    Start-Sleep -Seconds 7
    $samples = @(Get-ChildItem $logDir -Filter *.jsonl -ErrorAction SilentlyContinue |
        Select-String '"type":"sample"')
    if ($samples.Count -gt 0) { Write-Host "PASS  collector started by the task writes samples ($($samples.Count))" }
    else { Write-Host "::warning::coinslotw.exe started by the task but wrote no samples to $logDir" }
    # Second start in the same session must not create a second collector (per-session mutex).
    & (Join-Path $dest 'coinslotw.exe')
    Start-Sleep -Seconds 3
    $n = @(Get-Process -Name coinslotw -ErrorAction SilentlyContinue).Count
    Check ($n -eq 1) "single instance per session (running: $n)"
} else {
    Write-Host '::warning::Start-ScheduledTask did not launch coinslotw.exe on this runner (may be a CI session limitation; check on the real PC)'
    Get-ScheduledTaskInfo -TaskPath '\Coinslot\' -TaskName 'Collector' | Format-List | Out-String | Write-Host
}

# ---- reinstall keeps an edited config; -OverwriteConfig replaces it ----
# Also run the console build from the install dir, so reinstall must stop it too.
# Stop the task's collector first: its per-session mutex would make a second one exit.
Get-Process -Name coinslotw -ErrorAction SilentlyContinue | Stop-Process -Force -PassThru |
    Wait-Process -Timeout 10 -ErrorAction SilentlyContinue
$console = Start-Process -FilePath (Join-Path $dest 'coinslot.exe') -ArgumentList 'run' -WindowStyle Hidden -PassThru
Start-Sleep -Seconds 2
Check (-not $console.HasExited) 'console collector (coinslot.exe run) running before reinstall'
$cfg = Join-Path $dest 'config.json'
Add-Content -Path $cfg -Value ' '   # still valid JSON, different hash
$edited = Hash $cfg
& (Join-Path $Package 'install.ps1')
Check ((Hash $cfg) -eq $edited) 'reinstall keeps the existing config.json'
Check (@(Get-Process -Name coinslot, coinslotw -ErrorAction SilentlyContinue).Count -eq 0) 'reinstall stops running collectors (both builds)'
& (Join-Path $Package 'install.ps1') -OverwriteConfig
Check ((Hash $cfg) -eq (Hash (Join-Path $Package 'config.json'))) '-OverwriteConfig replaces config.json'

# ---- uninstall (with a console collector running from the install dir) ----
$console = Start-Process -FilePath (Join-Path $dest 'coinslot.exe') -ArgumentList 'run' -WindowStyle Hidden -PassThru
Start-Sleep -Seconds 2
& (Join-Path $Package 'uninstall.ps1')
Check ($null -eq (Get-ScheduledTask -TaskPath '\Coinslot\' -TaskName 'Collector' -ErrorAction SilentlyContinue)) 'uninstall removes the task'
Check (-not (Test-Path $dest)) 'uninstall removes the install dir'
Check (@(Get-Process -Name coinslot, coinslotw -ErrorAction SilentlyContinue).Count -eq 0) 'uninstall stops collectors (both builds)'

if ($failures -gt 0) { throw "$failures check(s) failed" }
Write-Host 'All install checks passed.'
