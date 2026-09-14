# Run directly in the current PowerShell; does not launch another shell.
[CmdletBinding()]
param(
    [string] $Root = (Get-Location).Path,
    [string[]] $TestFiles = @('test/*.test.mjs'),
    [ValidateRange(1, 60)][int] $TimeoutSeconds = 60
)

function Invoke-ClauductNodeTests {
    param([string] $Root, [string[]] $TestFiles, [ValidateRange(1, 60)][int] $TimeoutSeconds = 60)
    $ErrorActionPreference = 'Stop'
    if ($PSVersionTable.PSVersion.Major -lt 7) { throw 'POWERSHELL_7_REQUIRED' }
    $taskRoot = (Resolve-Path -LiteralPath $Root).ProviderPath.TrimEnd('\', '/')
    if ((Get-Item -Force -LiteralPath $taskRoot).Attributes -band [IO.FileAttributes]::ReparsePoint) {
        throw 'TEST_ROOT_REPARSE_POINT'
    }
    $files = @()
    foreach ($pattern in $TestFiles) {
        if ([IO.Path]::IsPathRooted($pattern) -or $pattern -match '(^|[\\/])\.\.([\\/]|$)|[:\r\n"]' -or $pattern.StartsWith('-')) {
            throw 'INVALID_TEST_PATH'
        }
        $matches = @(Resolve-Path -Path (Join-Path $taskRoot $pattern) -ErrorAction SilentlyContinue)
        if ($matches.Count -eq 0) { throw 'TEST_FILE_NOT_FOUND' }
        foreach ($match in $matches) {
            $path = $match.ProviderPath
            if (-not $path.StartsWith($taskRoot + [IO.Path]::DirectorySeparatorChar, [StringComparison]::OrdinalIgnoreCase) -or
                [IO.Path]::GetExtension($path) -ne '.mjs') { throw 'INVALID_TEST_PATH' }
            $current = $path
            while ($current -ne $taskRoot) {
                if ((Get-Item -Force -LiteralPath $current).Attributes -band [IO.FileAttributes]::ReparsePoint) {
                    throw 'TEST_PATH_REPARSE_POINT'
                }
                $current = Split-Path -Parent $current
            }
            $files += $path
        }
    }
    if ($files.Count -eq 0) { throw 'NO_TEST_FILES' }
    $temporaryRoot = Join-Path $taskRoot '.tmp'
    if (Test-Path -LiteralPath $temporaryRoot) {
        if ((Get-Item -Force -LiteralPath $temporaryRoot).Attributes -band [IO.FileAttributes]::ReparsePoint) {
            throw 'TEST_TEMP_REPARSE_POINT'
        }
    } else { [void](New-Item -ItemType Directory -Path $temporaryRoot) }
    $nodeExecutable = (Get-Command node.exe -CommandType Application | Select-Object -First 1).Source
    $windowsRoot = [Environment]::GetFolderPath([Environment+SpecialFolder]::Windows)
    if (-not $windowsRoot -or -not (Test-Path -LiteralPath $windowsRoot -PathType Container)) {
        throw 'WINDOWS_ROOT_UNAVAILABLE'
    }
    $info = [Diagnostics.ProcessStartInfo]::new()
    $info.FileName = $nodeExecutable
    $info.WorkingDirectory = $taskRoot
    $info.UseShellExecute = $false
    $info.CreateNoWindow = $true
    $info.RedirectStandardOutput = $true
    $info.RedirectStandardError = $true
    $info.Environment.Clear()
    $info.Environment['SystemRoot'] = $windowsRoot
    $info.Environment['WINDIR'] = $windowsRoot
    $info.Environment['TEMP'] = $temporaryRoot
    $info.Environment['TMP'] = $temporaryRoot
    foreach ($argument in @('--test', '--test-concurrency=1') + @($files | Select-Object -Unique)) {
        $info.ArgumentList.Add($argument)
    }
    $process = [Diagnostics.Process]::Start($info)
    try {
        $stdout = $process.StandardOutput.ReadToEndAsync()
        $stderr = $process.StandardError.ReadToEndAsync()
        $timedOut = -not $process.WaitForExit($TimeoutSeconds * 1000)
        if ($timedOut) { $process.Kill($true); $process.WaitForExit() }
        $out = $stdout.GetAwaiter().GetResult()
        $err = $stderr.GetAwaiter().GetResult()
        # Reporting cap, not a memory or OS sandbox. Run reviewed test code only.
        if ($out.Length + $err.Length -gt 2MB) { throw 'TEST_OUTPUT_LIMIT_EXCEEDED' }
        [pscustomobject]@{ ExitCode = $(if ($timedOut) { 124 } else { $process.ExitCode });
            Stdout = $out; Stderr = $err; TimedOut = $timedOut }
    } finally { $process.Dispose() }
}

if ($MyInvocation.InvocationName -ne '.') {
    try {
        $result = Invoke-ClauductNodeTests -Root $Root -TestFiles $TestFiles -TimeoutSeconds $TimeoutSeconds
        Write-Output $result.Stdout
        if ($result.Stderr) { [Console]::Error.WriteLine($result.Stderr) }
        if ($result.TimedOut) { [Console]::Error.WriteLine('TEST_TIMEOUT') }
        exit $result.ExitCode
    } catch {
        $known = @('POWERSHELL_7_REQUIRED', 'TEST_ROOT_REPARSE_POINT', 'INVALID_TEST_PATH',
            'TEST_FILE_NOT_FOUND', 'TEST_PATH_REPARSE_POINT', 'NO_TEST_FILES', 'TEST_TEMP_REPARSE_POINT',
            'WINDOWS_ROOT_UNAVAILABLE', 'TEST_OUTPUT_LIMIT_EXCEEDED')
        $code = $_.Exception.Message
        if ($code -notin $known) { $code = 'TEST_RUNNER_FAILED' }
        [Console]::Error.WriteLine($code)
        exit 1
    }
}
