$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'run-node-tests.ps1')
$repository = Split-Path -Parent $PSScriptRoot
$fixture = 'src/fixtures/node-runner.test.mjs'
$originalSentinel = [Environment]::GetEnvironmentVariable('CLAUDUCT_ENV_SENTINEL')
$temporary = Join-Path $PSScriptRoot ('runner-fixture-' + [Guid]::NewGuid().ToString('N'))
try {
    [Environment]::SetEnvironmentVariable('CLAUDUCT_ENV_SENTINEL', 'synthetic-excluded')
    $result = Invoke-ClauductNodeTests -Root $repository -TestFiles @($fixture)
    if ($result.ExitCode -ne 0 -or $result.Stdout -notmatch 'pass 3') { throw 'ENVIRONMENT_FIXTURE_FAILED' }
    foreach ($invalid in @('../outside.mjs', '--eval', 'src/test-run-node-tests.ps1')) {
        $rejected = $false
        try { $null = Invoke-ClauductNodeTests -Root $repository -TestFiles @($invalid) }
        catch { $rejected = $_.Exception.Message -eq 'INVALID_TEST_PATH' }
        if (-not $rejected) { throw 'INVALID_PATH_ACCEPTED' }
    }
    # Task-created fixtures only; never execute an external or user-selected file.
    [void](New-Item -ItemType Directory -Path $temporary)
    $missing = $false
    try { $null = Invoke-ClauductNodeTests -Root $temporary -TestFiles @('test/*.test.mjs') }
    catch { $missing = $_.Exception.Message -eq 'TEST_FILE_NOT_FOUND' }
    if (-not $missing) { throw 'EMPTY_ROOT_MISCLASSIFIED' }
    [IO.File]::WriteAllText((Join-Path $temporary 'fail.mjs'), "process.exitCode = 7;")
    [IO.File]::WriteAllText((Join-Path $temporary 'wait.mjs'), "console.log('FIXTURE_PID=' + process.pid); setTimeout(() => {}, 10000);")
    $failed = Invoke-ClauductNodeTests -Root $temporary -TestFiles @('fail.mjs')
    if ($failed.ExitCode -ne 1 -or $failed.Stdout -notmatch 'fail 1') { throw 'FAILURE_HIDDEN' }
    $timeout = Invoke-ClauductNodeTests -Root $temporary -TestFiles @('wait.mjs') -TimeoutSeconds 1
    if ($timeout.ExitCode -ne 124 -or -not $timeout.TimedOut) { throw 'TIMEOUT_HIDDEN' }
    $pidMatch = [regex]::Match($timeout.Stdout, 'FIXTURE_PID=(\d+)')
    if (-not $pidMatch.Success) { throw 'TIMEOUT_CHILD_NOT_OBSERVED' }
    $fixtureProcess = $null
    try { $fixtureProcess = [Diagnostics.Process]::GetProcessById([int]$pidMatch.Groups[1].Value) }
    catch [ArgumentException] { }
    if ($null -ne $fixtureProcess) {
        try { if (-not $fixtureProcess.WaitForExit(1000)) { throw 'TIMEOUT_CHILD_STILL_RUNNING' } }
        finally { $fixtureProcess.Dispose() }
    }
    Write-Output 'Runner: environment 3/3; path rejection 3/3; empty root named; failure exit; timeout tree termination passed'
} finally {
    [Environment]::SetEnvironmentVariable('CLAUDUCT_ENV_SENTINEL', $originalSentinel)
    if (Test-Path -LiteralPath $temporary) {
        $resolved = (Resolve-Path -LiteralPath $temporary).ProviderPath
        if (-not $resolved.StartsWith($PSScriptRoot + [IO.Path]::DirectorySeparatorChar, [StringComparison]::OrdinalIgnoreCase)) {
            throw 'CLEANUP_BOUNDARY'
        }
        Remove-Item -LiteralPath $resolved -Recurse -Force
    }
}
