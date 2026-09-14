$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot '../verification/read-transport-progress.ps1')
function New-Attempts {
    return @(@{attempt=1;status=$null;completed=$false;failureCategory='UPSTREAM_DNS_ERROR'},
        @{attempt=2;status=200;completed=$true;failureCategory=$null})
}
$valid = 'CLAUDUCT_CONNECTION_FAULT {"connectionCalls":2,"injected":1,"restored":true}'
$record = ConvertFrom-ClauductConnectionFault -Text $valid -RequestLimit 8 -Attempts (New-Attempts)
if ($record.connectionCalls -ne 2 -or -not $record.restored) { throw 'VALID_FAULT_REJECTED' }
$checks = 1
foreach ($fault in @('unknown-field','missing-field','bool-count','too-many-connections','not-restored','duplicate-record','oversized','rejection-marker','two-dns','missing-status','bool-attempt','not-first','completed-dns','no-success','wrong-success','string-success')) {
    $text = $valid; $attempts = New-Attempts
    switch ($fault) {
        'unknown-field' { $text = $valid.Replace('}',',"extra":"SYNTHETIC_PRIVATE"}') }
        'missing-field' { $text = $valid.Replace('"restored":true','"extra":"SYNTHETIC_PRIVATE"') }
        'bool-count' { $text = $valid.Replace('"injected":1','"injected":true') }
        'too-many-connections' { $text = $valid.Replace('"connectionCalls":2','"connectionCalls":9') }
        'not-restored' { $text = $valid.Replace('true','false') }
        'duplicate-record' { $text += "`n" + $valid }
        'oversized' { $text = 'x' * 262145 }
        'rejection-marker' { $text += "`nGUARDED_ENTRY VERIFICATION_CONNECTION_FAULT_REJECTED" }
        'two-dns' { $attempts += $attempts[0] }
        'missing-status' { $attempts[0].Remove('status') }
        'bool-attempt' { $attempts[0].attempt = $true }
        'not-first' { $attempts[0].attempt = 2 }
        'completed-dns' { $attempts[0].completed = $true }
        'no-success' { $attempts = @($attempts[0]) }
        'wrong-success' { $attempts[1].status = 503 }
        'string-success' { $attempts[1].status = '200' }
    }
    $rejected = $false
    try { [void](ConvertFrom-ClauductConnectionFault -Text $text -RequestLimit 8 -Attempts $attempts) }
    catch { if ($_.Exception.Message -ne 'VERIFICATION_CONNECTION_FAULT_INVALID') { throw 'WRONG_FAILURE' }; $rejected = $true }
    if (-not $rejected) { throw 'INVALID_FAULT_ACCEPTED' }
    $checks++
}
@{suite='connection-fault-parser';checks=$checks;externalRequests=0;actualCredentialReads=0}|ConvertTo-Json -Compress
