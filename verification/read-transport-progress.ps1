function ConvertFrom-ClauductFixtureAccounting {
    param([string] $UsageText, [string] $JournalText, [int] $RequestLimit)
    try {
        if ($UsageText.Length -gt 256KB -or $JournalText.Length -gt 2048 -or $RequestLimit -lt 1 -or $RequestLimit -gt 256) { throw 'INVALID' }
        $parse = {
            param([string] $Text, [string[]] $Keys)
            $document = [System.Text.Json.JsonDocument]::Parse($Text)
            try {
                if ($document.RootElement.ValueKind -ne [System.Text.Json.JsonValueKind]::Object) { throw 'INVALID' }
                $names = [Collections.Generic.HashSet[string]]::new([StringComparer]::Ordinal)
                foreach ($property in $document.RootElement.EnumerateObject()) {
                    if ($property.Name -cnotin $Keys -or -not $names.Add($property.Name)) { throw 'INVALID' }
                }
                if ($names.Count -ne $Keys.Count) { throw 'INVALID' }
            } finally { $document.Dispose() }
            return $Text | ConvertFrom-Json -AsHashtable -Depth 4 -ErrorAction Stop
        }
        $counters = @('inputTokens','outputTokens','completions','requestAttempts','imageFormatMask','unobservedCompletions')
        $journal = & $parse $JournalText ($counters + @('version','recordCount','finalRecorded','truncatedTail'))
        foreach ($key in $counters + @('version','recordCount')) {
            if (($journal[$key] -isnot [int] -and $journal[$key] -isnot [long]) -or $journal[$key] -lt 0 -or $journal[$key] -gt 9007199254740991) { throw 'INVALID' }
        }
        if ($journal.version -notin @(1,2) -or $journal.finalRecorded -isnot [bool] -or $journal.truncatedTail -isnot [bool] -or
            ($journal.finalRecorded -and $journal.truncatedTail) -or $journal.recordCount -lt 1 -or $journal.recordCount -gt 4096 -or
            $journal.requestAttempts -gt $RequestLimit -or $journal.imageFormatMask -gt 15 -or
            $journal.completions + $journal.unobservedCompletions -gt $journal.requestAttempts -or
            ($journal.version -eq 1 -and $journal.unobservedCompletions -ne 0) -or
            $journal.recordCount -lt 1 + $journal.requestAttempts + $journal.completions + $journal.unobservedCompletions + [int]$journal.finalRecorded) { throw 'INVALID' }
        $prefix = 'CLAUDUCT_FIXTURE_USAGE '
        $lines = @($UsageText -split '\r?\n' | Where-Object { $_.StartsWith($prefix) })
        if ($lines.Count -gt 1 -or ($lines.Count -eq 1 -and $lines[0].Length -gt 1024)) { throw 'INVALID' }
        $usage = @{ maxInputTokens = 131072; maxOutputTokens = 32768 }
        foreach ($key in $counters) { $usage[$key] = $journal[$key] }
        $matched = $null; $source = 'journal'
        if ($lines.Count -eq 1) {
            $footerKeys = $counters + @('maxInputTokens','maxOutputTokens')
            if ($journal.version -eq 1) { $footerKeys = @($footerKeys | Where-Object { $_ -cne 'unobservedCompletions' }) }
            $footer = & $parse $lines[0].Substring($prefix.Length) $footerKeys
            foreach ($key in $footerKeys) {
                if (($footer[$key] -isnot [int] -and $footer[$key] -isnot [long]) -or $footer[$key] -ne $usage[$key]) { throw 'INVALID' }
            }
            $matched = $true; $source = 'footer'
        }
        return @{ usage = $usage; source = $source; journal = @{ version = $journal.version; records = $journal.recordCount;
            finalRecorded = $journal.finalRecorded; truncatedTail = $journal.truncatedTail; matchedFooter = $matched } }
    } catch { throw 'VERIFICATION_USAGE_INVALID' }
}

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

function ConvertFrom-ClauductConnectionFault {
    param([string] $Text, [int] $RequestLimit, [object[]] $Attempts)
    try {
        if ($Text.Length -gt 262144 -or $RequestLimit -lt 2 -or $RequestLimit -gt 256 -or
            $Text.Contains('GUARDED_ENTRY VERIFICATION_CONNECTION_FAULT_REJECTED')) { throw 'INVALID' }
        $prefix = 'CLAUDUCT_CONNECTION_FAULT '
        $records = @($Text -split "`n" | Where-Object { $_.StartsWith($prefix) })
        if ($records.Count -ne 1 -or $records[0].Length -gt 1024) { throw 'INVALID' }
        $value = $records[0].Substring($prefix.Length) | ConvertFrom-Json -AsHashtable
        if ($value -isnot [Collections.IDictionary] -or $value.Count -ne 3 -or
            @($value.Keys | Where-Object { $_ -notin @('connectionCalls','injected','restored') }).Count -or
            ($value.connectionCalls -isnot [int] -and $value.connectionCalls -isnot [long]) -or
            $value.connectionCalls -lt 2 -or $value.connectionCalls -gt $RequestLimit -or
            ($value.injected -isnot [int] -and $value.injected -isnot [long]) -or $value.injected -ne 1 -or
            $value.restored -isnot [bool] -or $value.restored -ne $true -or $Attempts.Count -gt 256) { throw 'INVALID' }
        $dns = @($Attempts | Where-Object { $_.failureCategory -eq 'UPSTREAM_DNS_ERROR' })
        $success = @($Attempts | Where-Object { $_.completed -is [bool] -and $_.completed -eq $true -and
            ($_.status -is [int] -or $_.status -is [long]) -and $_.status -eq 200 })
        if ($dns.Count -ne 1 -or $dns[0] -isnot [Collections.IDictionary] -or -not $dns[0].Contains('status') -or
            ($dns[0].attempt -isnot [int] -and $dns[0].attempt -isnot [long]) -or $dns[0].attempt -ne 1 -or
            $null -ne $dns[0].status -or $dns[0].completed -isnot [bool] -or $dns[0].completed -ne $false -or $success.Count -lt 1) { throw 'INVALID' }
        return @{ connectionCalls = $value.connectionCalls; injected = 1; restored = $true }
    } catch { throw 'VERIFICATION_CONNECTION_FAULT_INVALID' }
}
