function ConvertFrom-ClauductTransportProgress {
    param([string] $Text)
    try {
        if ($Text.Length -gt 256KB -or $Text.Contains('GUARDED_ENTRY VERIFICATION_PROGRESS_FAILED')) { throw 'INVALID' }
        $prefix = 'CLAUDUCT_FIXTURE_PROGRESS '
        $lines = @($Text -split '\r?\n' | Where-Object { $_.StartsWith($prefix) })
        if ($lines.Count -lt 1 -or $lines.Count -gt 128) { throw 'INVALID' }
        $records = @(); $previousElapsed = 0; $previousStarted = 0
        foreach ($line in $lines) {
            if ($line.Length -gt 8192) { throw 'INVALID' }
            $snapshot = $line.Substring($prefix.Length) | ConvertFrom-Json -AsHashtable -ErrorAction Stop
            if ($snapshot -isnot [Collections.IDictionary] -or $snapshot.Count -ne 7 -or
                @($snapshot.Keys | Where-Object { $_ -notin @('version','sequence','elapsedMs','requestsStarted','activeCount','truncated','active') }).Count -or
                @(@('version','sequence','elapsedMs','requestsStarted','activeCount') | Where-Object { ($snapshot[$_] -isnot [int] -and $snapshot[$_] -isnot [long]) -or $snapshot[$_] -lt 0 }).Count -or
                $snapshot.version -ne 1 -or $snapshot.sequence -ne $records.Count + 1 -or
                $snapshot.elapsedMs -lt $previousElapsed -or $snapshot.requestsStarted -lt $previousStarted -or
                $snapshot.truncated -isnot [bool] -or $snapshot.truncated -or $snapshot.active -isnot [array] -or
                $snapshot.active.Count -ne $snapshot.activeCount -or $snapshot.active.Count -gt 16) { throw 'INVALID' }
            $seen = @{}
            foreach ($job in $snapshot.active) {
                if ($job -isnot [Collections.IDictionary] -or $job.Count -ne 6 -or
                    @($job.Keys | Where-Object { $_ -notin @('request','elapsedMs','attempt','phase','status','sawCompletion') }).Count -or
                    @(@('request','elapsedMs','attempt') | Where-Object { ($job[$_] -isnot [int] -and $job[$_] -isnot [long]) -or $job[$_] -lt 0 }).Count -or
                    $job.request -lt 1 -or $job.request -gt $snapshot.requestsStarted -or $seen.ContainsKey([string]$job.request) -or
                    $job.sawCompletion -isnot [bool] -or $job.phase -notin @('credentials','request','headers','body','stream','cleanup') -or
                    ($null -ne $job.status -and (($job.status -isnot [int] -and $job.status -isnot [long]) -or $job.status -lt 100 -or $job.status -gt 599))) { throw 'INVALID' }
                $seen[[string]$job.request] = $true
            }
            $previousElapsed = $snapshot.elapsedMs; $previousStarted = $snapshot.requestsStarted; $records += $snapshot
        }
        return @{ records = $records; summary = @{ samples = $records.Count; last = $records[-1] } }
    } catch { throw 'VERIFICATION_PROGRESS_INVALID' }
}
