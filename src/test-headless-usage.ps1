$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot '../verification/read-transport-progress.ps1')
function New-Journal {
    return @{ version=2; recordCount=4; finalRecorded=$true; truncatedTail=$false; requestAttempts=1;
        inputTokens=10; outputTokens=2; completions=1; unobservedCompletions=0; imageFormatMask=0 }
}
function Encode($Value) { return $Value | ConvertTo-Json -Compress -Depth 4 }
function Footer($Journal) {
    $value=@{maxInputTokens=131072;maxOutputTokens=32768}
    foreach($key in @('requestAttempts','inputTokens','outputTokens','completions','imageFormatMask','unobservedCompletions')){
        if($key -ne 'unobservedCompletions' -or $Journal.version -eq 2){$value[$key]=$Journal[$key]}
    }
    return 'CLAUDUCT_FIXTURE_USAGE '+(Encode $value)
}
function Reject([string]$JournalText,[string]$UsageText='') {
    $denied=$false
    try{[void](ConvertFrom-ClauductFixtureAccounting -UsageText $UsageText -JournalText $JournalText -RequestLimit 1)}
    catch{if($_.Exception.Message -cne 'VERIFICATION_USAGE_INVALID'){throw 'WRONG_FAILURE'};$denied=$true}
    if(-not $denied){throw 'INVALID_ACCOUNTING_ACCEPTED'}
}
$checks=0
$journal=New-Journal
$value=ConvertFrom-ClauductFixtureAccounting -UsageText (Footer $journal) -JournalText (Encode $journal) -RequestLimit 1
if($value.journal.version -ne 2 -or $value.source -cne 'footer' -or -not $value.journal.matchedFooter -or $value.usage.inputTokens -ne 10){throw 'V2_REJECTED'};$checks++
$value=ConvertFrom-ClauductFixtureAccounting -JournalText (Encode $journal) -RequestLimit 1
if($value.source -cne 'journal' -or $null -ne $value.journal.matchedFooter){throw 'JOURNAL_FALLBACK_REJECTED'};$checks++
$journal.version=1
$value=ConvertFrom-ClauductFixtureAccounting -UsageText (Footer $journal) -JournalText (Encode $journal) -RequestLimit 1
if($value.journal.version -ne 1 -or $value.usage.unobservedCompletions -ne 0){throw 'LEGACY_VERSION_LOST'};$checks++
$journal=New-Journal;$journal.completions=0;$journal.unobservedCompletions=1;$journal.inputTokens=0;$journal.outputTokens=0
$value=ConvertFrom-ClauductFixtureAccounting -UsageText (Footer $journal) -JournalText (Encode $journal) -RequestLimit 1
if($value.usage.unobservedCompletions -ne 1 -or $value.usage.completions -ne 0){throw 'UNKNOWN_USAGE_LOST'};$checks++
$journal=New-Journal;$journal.finalRecorded=$false;$journal.truncatedTail=$true;$journal.recordCount=3
$value=ConvertFrom-ClauductFixtureAccounting -JournalText (Encode $journal) -RequestLimit 1
if($value.journal.finalRecorded -or -not $value.journal.truncatedTail){throw 'PARTIAL_STATE_LOST'};$checks++
foreach($fault in @('extra','missing','bool-counter','unsafe-counter','negative-counter','bad-version','old-unknown','too-many-attempts',
    'too-many-completions','invalid-mask','record-count','bool-final','closed-truncated','upper-key','duplicate-key','invalid-json','oversized')){
    $journal=New-Journal
    switch($fault){
        'extra' {$journal.extra='PUBLIC'}
        'missing' {$journal.Remove('version')}
        'bool-counter' {$journal.inputTokens=$true}
        'unsafe-counter' {$journal.inputTokens=9007199254740992L}
        'negative-counter' {$journal.outputTokens=-1}
        'bad-version' {$journal.version=3}
        'old-unknown' {$journal.version=1;$journal.completions=0;$journal.unobservedCompletions=1}
        'too-many-attempts' {$journal.requestAttempts=2}
        'too-many-completions' {$journal.completions=2}
        'invalid-mask' {$journal.imageFormatMask=16}
        'record-count' {$journal.recordCount=3}
        'bool-final' {$journal.finalRecorded=1}
        'closed-truncated' {$journal.truncatedTail=$true}
    }
    $text=Encode $journal
    switch($fault){
        'upper-key' {$text=$text.Replace('"version"','"Version"')}
        'duplicate-key' {$text=$text.Replace('"version":2','"version":2,"version":2')}
        'invalid-json' {$text='{'}
        'oversized' {$text=' '*2049}
    }
    Reject $text;$checks++
}
$journal=New-Journal;$text=Encode $journal;$footer=Footer $journal
$badFooters=@(($footer+"`n"+$footer), $footer.Replace('"inputTokens":10','"inputTokens":11'),
    $footer.Replace('"unobservedCompletions":0','"unobservedCompletions":true'),
    $footer.Replace('}',',"extra":"PUBLIC"}'), $footer.Replace('"completions":1','"completions":1,"completions":1'),
    'CLAUDUCT_FIXTURE_USAGE {', ('x'*262145))
if($badFooters.Count -ne 7){throw 'FOOTER_CASES_MISSING'}
foreach($bad in $badFooters){Reject $text $bad;$checks++}
$tokens=$null;$errors=$null
[void][Management.Automation.Language.Parser]::ParseFile((Join-Path $PSScriptRoot '../verification/verify-native-headless.ps1'),[ref]$tokens,[ref]$errors)
if($errors.Count){throw 'HEADLESS_PARSE_FAILED'};$checks++
@{suite='headless-usage-parser';checks=$checks;v1KeptDistinct=$true;v2Parsed=$true;unknownUsagePreserved=$true;
    actualModelRequests=0;actualCredentialReads=0}|ConvertTo-Json -Compress
