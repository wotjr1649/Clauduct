$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot '../verification/read-transport-progress.ps1')
function New-Record {
    return @{ version=1; sequence=1; elapsedMs=1000; requestsStarted=1; activeCount=1; truncated=$false;
        active=@(@{request=1;elapsedMs=900;attempt=1;status=200;phase='stream';sawCompletion=$false}) }
}
function Encode-Record($Record) { return 'CLAUDUCT_FIXTURE_PROGRESS ' + ($Record | ConvertTo-Json -Depth 5 -Compress) }
function Assert-Rejected([string]$Text) {
    $rejected=$false
    try { [void](ConvertFrom-ClauductTransportProgress $Text) }
    catch { if ($_.Exception.Message -ne 'VERIFICATION_PROGRESS_INVALID') { throw 'WRONG_FAILURE' }; $rejected=$true }
    if (-not $rejected) { throw 'UNSAFE_PROGRESS_ACCEPTED' }
}
$checks=0
$valid=Encode-Record (New-Record)
$parsed=ConvertFrom-ClauductTransportProgress $valid
if($parsed.records.Count -ne 1 -or $parsed.summary.last.active[0].phase -ne 'stream'){throw 'VALID_PROGRESS_REJECTED'}
$checks++
foreach($fault in @('unknown-root','unknown-job','missing-status','wrong-phase','wrong-status','duplicate-request','truncated','wrong-count','bool-sequence','non-object','too-many-active')){
 $record=New-Record
 switch($fault){
  'unknown-root' {$record.extra='SYNTHETIC_PRIVATE'}
  'unknown-job' {$record.active[0].extra='SYNTHETIC_PRIVATE'}
  'missing-status' {$record.active[0].Remove('status');$record.active[0].extra='SYNTHETIC_PRIVATE'}
  'wrong-phase' {$record.active[0].phase='SYNTHETIC_PRIVATE'}
  'wrong-status' {$record.active[0].status='SYNTHETIC_PRIVATE'}
  'duplicate-request' {$record.active+=@{request=1;elapsedMs=900;attempt=1;status=200;phase='stream';sawCompletion=$false};$record.activeCount=2}
  'truncated' {$record.truncated=$true}
  'wrong-count' {$record.activeCount=0}
  'bool-sequence' {$record.sequence=$true}
  'non-object' {$record='SYNTHETIC_PRIVATE'}
  'too-many-active' {$record.active=@(1..17 | ForEach-Object {@{request=$_;elapsedMs=900;attempt=1;status=200;phase='stream';sawCompletion=$false}});$record.requestsStarted=17;$record.activeCount=17}
 }
 Assert-Rejected (Encode-Record $record);$checks++
}
$second=New-Record;$second.sequence=2;$second.elapsedMs=900
Assert-Rejected ($valid+"`n"+(Encode-Record $second));$checks++
$second.elapsedMs=2000;$second.requestsStarted=0
Assert-Rejected ($valid+"`n"+(Encode-Record $second));$checks++
Assert-Rejected (($valid+"`n")*129);$checks++
Assert-Rejected ('x' * 262145);$checks++
Assert-Rejected ($valid+"`nGUARDED_ENTRY VERIFICATION_PROGRESS_FAILED");$checks++
$second=New-Record;$second.sequence=2;$second.elapsedMs=2000;$second.active=@();$second.activeCount=0
$parsed=ConvertFrom-ClauductTransportProgress ($valid+"`n"+(Encode-Record $second))
if($parsed.records.Count -ne 2 -or $parsed.summary.last.activeCount -ne 0){throw 'FINAL_PROGRESS_REJECTED'}
$checks++
@{suite='transport-progress-parser';checks=$checks;externalRequests=0;actualCredentialReads=0;rawPayloadFields=0}|ConvertTo-Json -Compress
