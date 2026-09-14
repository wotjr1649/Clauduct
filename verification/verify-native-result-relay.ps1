param([ValidateSet('sol','luna')][string]$Model = 'sol', [ValidateSet('success','failure','cancel')][string]$Mode = 'failure',
    [switch]$UseCurrentGlobalBypass)
$ErrorActionPreference = 'Stop'
$project = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot '..')).ProviderPath
$globalBypassModeVerified = $false
if ($UseCurrentGlobalBypass) {
    # Inspect only the effective global mode. Never copy personal hooks, plugin
    # settings, environment values or the original profile into this fixture.
    $configuredRoot = [Environment]::GetEnvironmentVariable('CLAUDE_CONFIG_DIR')
    $globalRoot = if ($configuredRoot) { [IO.Path]::GetFullPath($configuredRoot) } else { Join-Path ([Environment]::GetFolderPath('UserProfile')) '.claude' }
    $globalPath = Join-Path $globalRoot 'settings.json'
    try {
        $globalInfo = Get-Item -LiteralPath $globalPath -Force
        if ($globalInfo.PSIsContainer -or $globalInfo.Length -gt 256KB -or ($globalInfo.Attributes -band [IO.FileAttributes]::ReparsePoint)) { throw 'BOUNDARY' }
        $globalSettings = [IO.File]::ReadAllText($globalPath) | ConvertFrom-Json -AsHashtable
        $globalBypassModeVerified = $globalSettings.permissions.defaultMode -is [string] -and $globalSettings.permissions.defaultMode -ceq 'bypassPermissions'
        $globalSettings = $null
    } catch { throw 'GLOBAL_BYPASS_SETTINGS_UNVERIFIED' }
    if (-not $globalBypassModeVerified) { throw 'GLOBAL_BYPASS_SETTINGS_UNVERIFIED' }
}
$run = Join-Path $project ('.tmp\native-result-relay-' + [Guid]::NewGuid().ToString('N'))
foreach ($path in @($project, (Join-Path $project '.tmp'))) {
    if ((Get-Item -LiteralPath $path -Force).Attributes -band [IO.FileAttributes]::ReparsePoint) { throw 'FIXTURE_ROOT_BOUNDARY' }
}
[void][IO.Directory]::CreateDirectory($run)
foreach ($name in @('config','temp','work')) { [void][IO.Directory]::CreateDirectory((Join-Path $run $name)) }
if ($globalBypassModeVerified) {
    [IO.File]::WriteAllText((Join-Path $run 'config/settings.json'), '{"permissions":{"defaultMode":"bypassPermissions"}}' + "`n")
}
$info = [Diagnostics.ProcessStartInfo]::new()
$info.FileName = (Get-Command node.exe -CommandType Application | Select-Object -First 1).Source
$info.WorkingDirectory = Join-Path $run 'work'
$info.UseShellExecute = $false; $info.CreateNoWindow = $true
$info.RedirectStandardInput = $true; $info.RedirectStandardOutput = $true; $info.RedirectStandardError = $true
$info.Environment.Clear()
foreach ($name in @('SystemRoot','WINDIR','SystemDrive','ComSpec','PATH','PATHEXT','USERPROFILE','HOMEDRIVE','HOMEPATH','APPDATA','LOCALAPPDATA','ProgramData','ProgramFiles','ProgramFiles(x86)','OS','PROCESSOR_ARCHITECTURE')) {
    $value = [Environment]::GetEnvironmentVariable($name)
    if ($value) { $info.Environment[$name] = $value }
}
$info.Environment['CLAUDE_CONFIG_DIR'] = Join-Path $run 'config'
foreach ($name in @('CLAUDE_CODE_TMPDIR','TEMP','TMP')) { $info.Environment[$name] = Join-Path $run 'temp' }
$info.Environment['CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC'] = '1'
$info.Environment['CLAUDE_CODE_POWERSHELL_RESPECT_EXECUTION_POLICY'] = '1'
$info.ArgumentList.Add((Join-Path $PSScriptRoot 'native-result-relay-entry.mjs'))
$info.ArgumentList.Add($run); $info.ArgumentList.Add($Model); $info.ArgumentList.Add($Mode)
$startedMs = [DateTimeOffset]::UtcNow.ToUnixTimeMilliseconds()
@{model=$Model;mode=$Mode;phaseMs=30000;wrapperMs=50000;requests=24;actualModelRequests=0;actualCredentialReads=0;
  globalBypassModeVerified=$globalBypassModeVerified;personalProfileCopied=$false;fullPersonalProfileLoaded=$false;
  basis='Actual native nested Agent/TaskOutput/SendMessage with fixed public responses, bounded local report and no authenticated transport.'} |
    ConvertTo-Json | Set-Content -LiteralPath (Join-Path $run 'budget.json') -Encoding utf8
$process = [Diagnostics.Process]::Start($info)
try {
    $process.StandardInput.Close()
    $stdout = $process.StandardOutput.ReadToEndAsync(); $stderr = $process.StandardError.ReadToEndAsync()
    $timedOut = -not $process.WaitForExit(30000)
    $helperArgs = @{RootPid=$process.Id;StartedAfterMs=$startedMs;RunRoot=$run}
    if ($timedOut) { $helperArgs.Stop = $true }
    $treeOutput = & (Join-Path $project 'verification\stop-owned-native-tree.ps1') @helperArgs
    if (-not $?) { throw 'FIXTURE_TREE_UNVERIFIED' }
    $tree = $treeOutput | ConvertFrom-Json
    if (-not $tree.stopped -or -not $process.WaitForExit(5000)) { throw 'FIXTURE_TREE_UNVERIFIED' }
    $out = $stdout.GetAwaiter().GetResult(); $err = $stderr.GetAwaiter().GetResult()
    if ($out.Length + $err.Length -gt 1MB) { throw 'FIXTURE_OUTPUT_LIMIT' }
    [IO.File]::WriteAllText((Join-Path $run 'stdout.txt'), $out)
    [IO.File]::WriteAllText((Join-Path $run 'stderr.txt'), $err)
    $entry = Get-Content -LiteralPath (Join-Path $run 'entry-result.json') -Raw | ConvertFrom-Json
    $lines = @($err -split "`n" | Where-Object { $_.StartsWith('CLAUDUCT_REQUEST_STATUS ') })
    if ($lines.Count -ne 1) { throw 'FIXTURE_STATUS_MISSING' }
    $status = $lines[0].Substring(24) | ConvertFrom-Json
    $init = @($out -split "`n" | Where-Object { $_ } | ForEach-Object { $_ | ConvertFrom-Json } | Where-Object { $_.type -eq 'system' -and $_.subtype -eq 'init' })
    $bypassObserved = $init.Count -eq 1 -and $init[0].permissionMode -ceq 'bypassPermissions'
    $routes = @($status.recentRequests | Where-Object { $_.selectionSource -eq 'verified-result-relay' })
    $cleanup = @($status.cleanup.PSObject.Properties).Count -eq 9 -and @($status.cleanup.PSObject.Properties | Where-Object { $_.Value -ne $true }).Count -eq 0
    $result = @{root=$run;model=$Model;mode=$Mode;timedOut=$timedOut;exitCode=$process.ExitCode;remaining=$tree.remaining;
      elapsedMs=([DateTimeOffset]::UtcNow.ToUnixTimeMilliseconds()-$startedMs);actualBackendRequests=0;credentialReads=0;
      entry=$entry;relayedRequests=$routes.Count;cleanupComplete=$cleanup;status=$status;
      globalBypassModeVerified=$globalBypassModeVerified;bypassObserved=$bypassObserved;personalProfileCopied=$false;fullPersonalProfileLoaded=$false;
      passed=($process.ExitCode -eq 0 -and -not $timedOut -and $entry.completed -and $entry.reportMatched -and $cleanup -and
        (-not $globalBypassModeVerified -or $bypassObserved) -and
        $(if ($Mode -eq 'cancel') { -not $entry.relaySent -and $entry.cancelObserved -and $routes.Count -eq 0 } else { $routes.Count -ge 1 }))}
    $result | ConvertTo-Json -Depth 15 | Set-Content -LiteralPath (Join-Path $run 'result.json') -Encoding utf8
    $result | Select-Object root,model,mode,passed,exitCode,timedOut,remaining,elapsedMs,relayedRequests,cleanupComplete,globalBypassModeVerified,bypassObserved,entry | ConvertTo-Json -Depth 5
} finally { $process.Dispose() }
