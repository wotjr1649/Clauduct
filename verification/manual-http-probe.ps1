# User-operated PowerShell 7 comparison. Never run --live or supply SEND from an agent.
# No profile, execution-policy, trust-store, authentication or global configuration changes.
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'
try {
    # One pin cannot hold on two machines: CI runs 7.6.5/10.0.11 and a developer box that has
    # taken a PowerShell update runs 7.6.6/10.0.12. The set is not a relaxation -- an
    # unlisted runtime is still refused. A version joins it only after this probe has been
    # run on it and its observations confirmed, which is what the pin was always for.
    if ($PSVersionTable.PSVersion.ToString() -cnotin @('7.6.5', '7.6.6') -or
        [System.Environment]::Version.ToString() -cnotin @('10.0.11', '10.0.12')) { throw 'TRANSPORT_RUNTIME_UNSUPPORTED' }
    # The probe reports what ran rather than a literal it cannot measure. The gate above is
    # what decides; this only carries the value into the result.
    $env:CLAUDUCT_PWSH_VERSION = $PSVersionTable.PSVersion.ToString()
    $taskMode = if ($args.Count -ge 1) { $args[0] } else { '' }
    if ($taskMode -ceq '--live') {
        if ($args.Count -ne 1 -or [Console]::IsInputRedirected -or [Console]::IsOutputRedirected -or
            $Host.Name -cne 'ConsoleHost') { throw 'USER_TERMINAL_REQUIRED' }
    } elseif ($taskMode -ceq '--loopback') {
        $taskPort = 0
        if ($args.Count -ne 2 -or ![int]::TryParse($args[1], [ref]$taskPort) -or
            $taskPort -lt 1 -or $taskPort -gt 65535) { throw 'LOCAL_CHECK_FAILED' }
    } elseif ($taskMode -cne '--self-test' -or $args.Count -ne 1) { throw 'USER_TERMINAL_REQUIRED' }

    Add-Type -Path (Join-Path $PSScriptRoot 'DotnetHttpProbe.cs') -ErrorAction Stop
    [ClauductVerification.DotnetHttpProbe]::CheckRuntime()
    $taskProbe = [ClauductVerification.DotnetHttpProbe]::new($PSScriptRoot)
    if ($taskMode -ceq '--self-test') {
        $taskProbe.SelfTest()
    } elseif ($taskMode -ceq '--loopback') {
        # Test-only route: fixed 127.0.0.1, no credential read and no authentication headers.
        for ($taskIndex = 0; $taskIndex -lt 64; $taskIndex++) {
            $taskLine = [Console]::In.ReadLineAsync().WaitAsync([TimeSpan]::FromSeconds(60)).GetAwaiter().GetResult()
            if ($null -eq $taskLine -or $taskLine -ceq 'STOP') { break }
            $taskProbe.Loopback($taskPort, $taskLine).GetAwaiter().GetResult()
        }
    } else {
        [Console]::WriteLine('USER-OPERATED TEST: gpt-6-astra/xhigh; dotnet-httpclient; one request to https://chatgpt.com/backend-api/codex/responses')
        [Console]::WriteLine('Uses account usage. Existing file cache in memory only; no refresh or writes. HTTP/1.1; no proxy, redirect or resend.')
        [Console]::Write('Type SEND to run once, or press Enter to cancel: ')
        if ([Console]::ReadLine() -cne 'SEND') { throw 'USER_CANCELLED' }
        $taskResult = $taskProbe.Live().GetAwaiter().GetResult()
        [Console]::WriteLine($taskResult)
        if (!(($taskResult | ConvertFrom-Json).passed)) { exit 1 }
    }
} catch {
    $taskCategory = 'LOCAL_CHECK_FAILED'
    if ($_.Exception.Message -cin @('TRANSPORT_RUNTIME_UNSUPPORTED', 'USER_TERMINAL_REQUIRED', 'USER_CANCELLED')) {
        $taskCategory = $_.Exception.Message
    }
    [Console]::WriteLine('{"passed":false,"category":"' + $taskCategory + '","transport":"dotnet-httpclient","requestAttempts":0,"credentialWrites":0,"retries":0}')
    exit 1
}
