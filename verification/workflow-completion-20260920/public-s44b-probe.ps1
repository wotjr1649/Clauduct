param(
    [Parameter(Mandatory)][ValidatePattern('^s44b-[a-f0-9]{12}$')][string]$ProbeId,
    [switch]$Leaf
)
$ErrorActionPreference = 'Stop'
# Public bounded workload only. No cleanup: native TaskStop must reap both processes.
$dir = Join-Path $PSScriptRoot $ProbeId
[IO.Directory]::CreateDirectory($dir) | Out-Null
$role = if ($Leaf) { 'leaf' } else { 'root' }
$identity = [Diagnostics.Process]::GetCurrentProcess()
$marker = @{ probe = $ProbeId; role = $role; pid = $PID;
    startedTicks = $identity.StartTime.ToUniversalTime().Ticks.ToString();
    observedUtc = [DateTime]::UtcNow.ToString('o'); durationSeconds = 180;
    engine = $identity.MainModule.FileName; version = $PSVersionTable.PSVersion.ToString();
    policy = (Get-ExecutionPolicy).ToString(); processPolicy = (Get-ExecutionPolicy -Scope Process).ToString() }
$bytes = [Text.Encoding]::UTF8.GetBytes(($marker | ConvertTo-Json -Compress))
$stream = [IO.File]::Open((Join-Path $dir ($role + '-started.json')), [IO.FileMode]::CreateNew)
try { $stream.Write($bytes, 0, $bytes.Length); $stream.Flush() } finally { $stream.Dispose() }
Write-Output "S44B_STARTED role=$role pid=$PID"
if (-not $Leaf) {
    $script = $PSCommandPath
    $child = Start-Process -FilePath $identity.MainModule.FileName -ArgumentList @(
        '-NoProfile', '-File', ('"' + $script + '"'), '-ProbeId', $ProbeId, '-Leaf'
    ) -WindowStyle Hidden -PassThru
    Write-Output "S44B_CHILD pid=$($child.Id)"
}
Start-Sleep -Seconds 180
[IO.File]::WriteAllText((Join-Path $dir ($role + '-finished.txt')), 'S44B_FINISHED')
Write-Output "S44B_FINISHED role=$role"
