#Requires -Version 7
param(
    [Parameter(Mandatory)][string]$Staticcheck,
    [Parameter(Mandatory)][string]$SourceBase,
    [string]$Baseline = "$PSScriptRoot/staticcheck-baseline.csv"
)

$ErrorActionPreference = 'Stop'

function Get-OriginalLine([int]$Line, [string[]]$Diff) {
    $offset = 0
    foreach ($header in $Diff) {
        if ($header -cnotmatch '^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@') { continue }
        $oldCount = 1
        $newCount = 1
        if ($Matches[2]) { $oldCount = [int]$Matches[2] }
        if ($Matches[4]) { $newCount = [int]$Matches[4] }
        $newStart = [int]$Matches[3]
        # An empty new range names the line before the deletion.
        if ($newCount -eq 0) { $newStart++ }
        if ($Line -lt $newStart) { break }
        if ($Line -lt $newStart + $newCount) { return 0 }
        $offset += $oldCount - $newCount
    }
    return $Line + $offset
}

$diagnostics = & $Staticcheck -f json -fail= -checks 'SA*,U1000' ./...
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

$baselineEntries = [Collections.Generic.List[object]]::new()
$baselineEntries.AddRange([object[]]@(Import-Csv -LiteralPath $Baseline))
$failed = $false
foreach ($record in $diagnostics) {
    $finding = ConvertFrom-Json -InputObject $record
    $location = $finding.location
    $path = [IO.Path]::GetRelativePath($PWD.Path, $location.file).Replace('\', '/')
    $entries = $baselineEntries | Where-Object {
        $_.Path -ceq $path -and $_.Code -ceq $finding.code -and
        $_.Message -ceq $finding.message -and [int]$_.Column -eq $location.column
    }
    if ($entries) {
        $diff = @(git diff --no-color --no-ext-diff --no-textconv --no-renames --diff-algorithm=histogram --unified=0 --inter-hunk-context=0 $SourceBase -- $path)
        if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
        $originalLine = Get-OriginalLine $location.line $diff
        if ($originalLine) {
            # Go is formatted before this check. Also bind the line to its top-level
            # declaration so an identical line in another function cannot inherit it.
            $source = [IO.File]::ReadAllLines($location.file)
            $declaration = $location.line
            while ($declaration -gt 1 -and $source[$declaration - 1] -cnotmatch '^(func|var|const|type)\s') {
                $declaration--
            }
            $originalDeclaration = Get-OriginalLine $declaration $diff
            $match = $entries | Where-Object {
                [int]$_.Line -eq $originalLine -and [int]$_.Declaration -eq $originalDeclaration
            } | Select-Object -First 1
            if ($match -and $baselineEntries.Remove($match)) { continue }
        }
    }

    Write-Output "${path}:$($location.line):$($location.column): $($finding.message) ($($finding.code))"
    $failed = $true
}
if ($failed) { exit 1 }
exit 0
