param([Parameter(Mandatory)][ValidatePattern('^s44b-[a-f0-9]{12}$')][string]$ProbeId)
$ErrorActionPreference = 'Stop'
$project = 'D:/AIDEV/clauduct-s36-build/repair-20260919/verification/policy-repair-20260919/interactive-e268b95d-2048-47e3-b36b-5483000541a7/project'
$source = Join-Path $project $ProbeId
$output = Join-Path $PSScriptRoot $ProbeId
if (Test-Path -LiteralPath $output) { throw 'OBSERVATION_ALREADY_EXISTS' }
[IO.Directory]::CreateDirectory($output) | Out-Null
function Save-Evidence($name, $value) {
    $bytes = [Text.Encoding]::UTF8.GetBytes(($value | ConvertTo-Json -Depth 8))
    $file = [IO.File]::Open((Join-Path $output $name), [IO.FileMode]::CreateNew)
    try { $file.Write($bytes, 0, $bytes.Length) } finally { $file.Dispose() }
}
$until = [DateTime]::UtcNow.AddSeconds(90)
while (-not ((Test-Path -LiteralPath (Join-Path $source 'root-started.json')) -and (Test-Path -LiteralPath (Join-Path $source 'leaf-started.json')))) {
    if ([DateTime]::UtcNow -gt $until) { throw 'START_MARKERS_TIMEOUT' }
    Start-Sleep -Milliseconds 100
}
$held = @()
try {
$identities = @('root', 'leaf') | ForEach-Object {
    $file = Join-Path $source ($_ + '-started.json')
    if ((Get-Item -LiteralPath $file).Length -gt 4096) { throw 'MARKER_SIZE' }
    $m = Get-Content -LiteralPath $file -Raw | ConvertFrom-Json
    if ($m.probe -ne $ProbeId -or $m.role -ne $_ -or $m.durationSeconds -ne 180) { throw 'MARKER_IDENTITY' }
    $process = [Diagnostics.Process]::GetProcessById([int]$m.pid)
    # Force a stable OS handle before checking identity; keep it through cancellation.
    $null = $process.Handle
    $held += $process
    if ($process.StartTime.ToUniversalTime().Ticks.ToString() -ne $m.startedTicks -or $process.HasExited) { throw 'PROCESS_IDENTITY' }
    if ($m.policy -ne 'RemoteSigned' -or $m.processPolicy -ne 'RemoteSigned') { throw 'PROCESS_POLICY' }
    if ($process.MainModule.FileName -ne $m.engine) { throw 'ENGINE_IDENTITY' }
    $m
}
# Read process ancestry without exposing command lines or touching unrelated processes.
$cim = @(Get-CimInstance Win32_Process -OperationTimeoutSec 5 | Select-Object ProcessId,ParentProcessId,CreationDate,Name)
$leaf = $cim | Where-Object ProcessId -eq $identities[1].pid
if ($leaf.ParentProcessId -ne $identities[0].pid) { throw 'CHILD_ANCESTRY' }
$observed = @{ probe = $ProbeId; atUtc = [DateTime]::UtcNow.ToString('o'); identities = @($identities);
    childParentPid = [int]$leaf.ParentProcessId; method = 'held_OS_process_handles_and_start_time'; observerKillsProcesses = $false }
Save-Evidence 'before.json' $observed
$observed | ConvertTo-Json -Depth 8 -Compress
$remaining = @($identities)
$until = [DateTime]::UtcNow.AddSeconds(90)
$exits = @()
while ($remaining.Count -gt 0 -and [DateTime]::UtcNow -lt $until) {
    $alive = @()
    foreach ($m in $remaining) {
        $process = @($held | Where-Object Id -eq $m.pid)[0]
        if (-not $process.HasExited) { $alive += $m } else {
            $exits += @{ role = $m.role; pid = $m.pid; startedTicks = $m.startedTicks;
                exitedUtc = $process.ExitTime.ToUniversalTime().ToString('o') }
        }
    }
    $remaining = @($alive)
    if ($remaining.Count) { Start-Sleep -Milliseconds 100 }
}
$finished = @('root-finished.txt','leaf-finished.txt') | Where-Object { Test-Path -LiteralPath (Join-Path $source $_) }
$result = @{ probe = $ProbeId; atUtc = [DateTime]::UtcNow.ToString('o'); exits = @($exits); remaining = @($remaining | ForEach-Object { $_.pid });
    finishedMarkers = @($finished); observerKillsProcesses = $false; stoppedBeforeNaturalDeadline = $remaining.Count -eq 0 -and @($finished).Count -eq 0 }
Save-Evidence 'after.json' $result
$result | ConvertTo-Json -Depth 8 -Compress
if (-not $result.stoppedBeforeNaturalDeadline) { exit 1 }
} finally { foreach ($process in $held) { $process.Dispose() } }
