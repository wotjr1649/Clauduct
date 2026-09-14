$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot '../verification/read-transport-progress.ps1')
$journal = @{version=2;recordCount=4;finalRecorded=$true;truncatedTail=$false;inputTokens=25;outputTokens=12;completions=1;requestAttempts=1;imageFormatMask=0;unobservedCompletions=0}
$usage = @{inputTokens=25;outputTokens=12;completions=1;requestAttempts=1;imageFormatMask=0;unobservedCompletions=0;maxInputTokens=100;maxOutputTokens=30000}
$journalText = $journal | ConvertTo-Json -Compress
$footer = 'CLAUDUCT_FIXTURE_USAGE ' + ($usage | ConvertTo-Json -Compress)
$result = ConvertFrom-ClauductFixtureAccounting -UsageText $footer -JournalText $journalText -RequestLimit 1 -MaxInputTokens 100 -MaxOutputTokens 30000
if ($result.usage.maxOutputTokens -ne 30000 -or $result.usage.maxInputTokens -ne 100 -or -not $result.journal.matchedFooter) { throw 'LOWER_BUDGET_MISMATCH' }
$rejected = $false
try { [void](ConvertFrom-ClauductFixtureAccounting -UsageText $footer -JournalText $journalText -RequestLimit 1) }
catch { if ($_.Exception.Message -ne 'VERIFICATION_USAGE_INVALID') { throw }; $rejected = $true }
if (-not $rejected) { throw 'UNDECLARED_BUDGET_ACCEPTED' }
$fallback = ConvertFrom-ClauductFixtureAccounting -UsageText '' -JournalText $journalText -RequestLimit 1 -MaxInputTokens 100 -MaxOutputTokens 30000
if ($fallback.usage.maxOutputTokens -ne 30000 -or $fallback.source -ne 'journal') { throw 'JOURNAL_BUDGET_MISMATCH' }
@{checks=3;passed=$true;actualBackendRequests=0;credentialReads=0} | ConvertTo-Json -Compress
