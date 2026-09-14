[CmdletBinding()]
param([Parameter(Mandatory)][int] $RootPid, [Parameter(Mandatory)][long] $StartedAfterMs,
    [Parameter(Mandatory)][string] $RunRoot, [switch] $Stop)
$ErrorActionPreference = 'Stop'
try {
    $project = (Resolve-Path -LiteralPath (Split-Path -Parent $PSScriptRoot)).ProviderPath
    $allowed = Join-Path $project '.tmp'
    $run = (Resolve-Path -LiteralPath $RunRoot).ProviderPath
    if (-not $run.StartsWith($allowed + [IO.Path]::DirectorySeparatorChar, [StringComparison]::OrdinalIgnoreCase)) { throw 'TREE_ROOT_REJECTED' }
    $cursor = $run
    while ($cursor -ne $project) {
        if ((Get-Item -Force -LiteralPath $cursor).Attributes -band [IO.FileAttributes]::ReparsePoint) { throw 'TREE_ROOT_REJECTED' }
        $cursor = Split-Path -Parent $cursor
    }
    $after = [DateTimeOffset]::FromUnixTimeMilliseconds($StartedAfterMs).UtcDateTime
    if ($StartedAfterMs -lt 1 -or $after -gt [DateTime]::UtcNow -or ([DateTime]::UtcNow - $after).TotalMinutes -gt 15) { throw 'TREE_TIME_REJECTED' }
    # Command lines are used only for exact local ownership classification. Never print them.
    $all = @(Get-CimInstance -ClassName Win32_Process -OperationTimeoutSec 5)
    $owner = @($all | Where-Object { $_.ProcessId -eq $RootPid })
    if ($owner.Count -and (-not $owner[0].CommandLine.Contains($run) -or $owner[0].CreationDate.ToUniversalTime() -lt $after)) { throw 'TREE_OWNER_REJECTED' }
    $owned = @{}
    foreach ($item in $all) {
        if ($item.CreationDate.ToUniversalTime() -ge $after -and $item.CommandLine -and $item.CommandLine.Contains($run) -and $item.ProcessId -ne $PID) {
            # The helper itself also has this path in its arguments; exclude all helper instances.
            if ($item.CommandLine.Contains('stop-owned-native-tree.ps1')) { continue }
            $owned[[int]$item.ProcessId] = $item
        }
    }
    do {
        $before = $owned.Count
        foreach ($item in $all) {
            if ($owned.ContainsKey([int]$item.ParentProcessId) -and $item.CreationDate.ToUniversalTime() -ge $after) {
                $owned[[int]$item.ProcessId] = $item
            }
        }
    } while ($owned.Count -gt $before)
    if ($owned.Count -gt 16) { throw 'TREE_PROCESS_LIMIT' }
    $identities = @($owned.Values | ForEach-Object { @{ pid = [int]$_.ProcessId; parentPid = [int]$_.ParentProcessId;
        createdMs = ([DateTimeOffset]$_.CreationDate.ToUniversalTime()).ToUnixTimeMilliseconds() } })
    if ($Stop) {
        foreach ($item in @($owned.Values | Sort-Object CreationDate)) {
            $target = $null
            try { $target = [Diagnostics.Process]::GetProcessById([int]$item.ProcessId) } catch [ArgumentException] { continue }
            try {
                if ([Math]::Abs(($target.StartTime.ToUniversalTime() - $item.CreationDate.ToUniversalTime()).TotalMilliseconds) -gt 2) { throw 'TREE_PID_REUSED' }
                if (-not $target.HasExited) { $target.Kill($true) }
                if (-not $target.WaitForExit(5000)) { throw 'TREE_STOP_TIMEOUT' }
            } finally { $target.Dispose() }
        }
    }
    $remaining = @()
    foreach ($item in $identities) {
        $target = $null
        try { $target = [Diagnostics.Process]::GetProcessById($item.pid) } catch [ArgumentException] { continue }
        try {
            $same = [Math]::Abs((([DateTimeOffset]$target.StartTime.ToUniversalTime()).ToUnixTimeMilliseconds()) - $item.createdMs) -le 2
            if ($same -and -not $target.HasExited) { $remaining += $item.pid }
        } finally { $target.Dispose() }
    }
    @{ stopped = $remaining.Count -eq 0; observed = $identities; remaining = $remaining } | ConvertTo-Json -Depth 4 -Compress
    if ($Stop -and $remaining.Count) { exit 1 }
} catch {
    [Console]::Error.WriteLine('OWNED_TREE_CHECK_FAILED')
    exit 1
}
