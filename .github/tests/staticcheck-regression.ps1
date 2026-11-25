#Requires -Version 7
param([Parameter(Mandatory)][string]$Staticcheck)

$ErrorActionPreference = 'Stop'
$Staticcheck = (Resolve-Path -LiteralPath $Staticcheck).Path
$check = Join-Path $PSScriptRoot 'staticcheck-check.ps1'
$temp = (Resolve-Path -LiteralPath ([IO.Path]::GetTempPath())).Path
$fixture = Join-Path $temp "clauduct-staticcheck-$([guid]::NewGuid().ToString('N'))"
$bad = @(
    'func First() int {'
    "`tx := value()"
    "`tx = 2"
    "`treturn x"
    '}'
) -join "`n"
$good = $bad.Replace("`tx := value()`n`tx = 2", "`tx := 2")
$branches = @'
package sample

func Branches(a, b bool) int {
	if a {
		return 2
	}
	if b {
		return 2
	}
	return 0
}
'@
$unusedValue = "`t`tx := value()`n`t`tx = 2`n`t`treturn x"
$firstBranch = $branches.Replace("`tif a {`n`t`treturn 2", "`tif a {`n$unusedValue")
$secondBranch = $branches.Replace("`tif b {`n`t`treturn 2", "`tif b {`n$unusedValue")

function Write-Fixture([string]$First, [string]$Second) {
    Set-Content -LiteralPath 'sample.go' -NoNewline -Value "package sample`n`n$First`n`n$Second`n`nfunc value() int { return 1 }`n"
}

function Assert-Check([string]$Name, [string]$First, [string]$Second, [string]$Expected = '') {
    Write-Fixture $First $Second
    $output = @(& $check -Staticcheck $Staticcheck -SourceBase $sourceBase -Baseline 'baseline.csv')
    $expectedExit = if ($Expected) { 1 } else { 0 }
    if ($LASTEXITCODE -ne $expectedExit -or ($output -join "`n") -cne $Expected) {
        throw "${Name}: exit $LASTEXITCODE, output: $($output -join "`n")"
    }
    Write-Output "PASS $Name"
}

$null = New-Item -ItemType Directory -Path $fixture
Push-Location -LiteralPath $fixture
try {
    Set-Content -LiteralPath '.gitattributes' -NoNewline -Value "* text eol=lf`n"
    Set-Content -LiteralPath 'go.mod' -NoNewline -Value "module example.com/staticcheck-baseline`n"
    Set-Content -LiteralPath 'baseline.csv' -Value @(
        'Path,Code,Message,Line,Column,Declaration,Scope'
        'sample.go,SA4006,this value of x is never used,4,2,3,3'
        'branches.go,SA4006,this value of x is never used,5,3,3,4'
    )
    $goodSecond = $good.Replace('First', 'Second')
    $badSecond = $bad.Replace('First', 'Second')
    Write-Fixture $bad $goodSecond
    Set-Content -LiteralPath 'branches.go' -NoNewline -Value "$firstBranch`n"
    git init --quiet
    if ($LASTEXITCODE -ne 0) { throw 'git init failed' }
    git add -- .gitattributes go.mod sample.go branches.go
    if ($LASTEXITCODE -ne 0) { throw 'git add failed' }
    git -c user.name=fixture -c user.email=fixture@example.com -c commit.gpgSign=false commit --quiet -m 'Baseline source'
    if ($LASTEXITCODE -ne 0) { throw 'git commit failed' }
    $sourceBase = git rev-parse HEAD
    if ($LASTEXITCODE -ne 0) { throw 'git rev-parse failed' }

    $shifted = $bad.Replace(" {`n", " {`n`t_ = 0`n")
    Assert-Check 'line shift' $shifted $goodSecond
    Assert-Check 'fixed SA4006 and identical new SA4006' $good $badSecond 'sample.go:9:2: this value of x is never used (SA4006)'
    Assert-Check 'new U1000' $bad 'func orphan() {}' 'sample.go:9:6: func orphan is unused (U1000)'
    Assert-Check 'original and new SA4006' $bad $badSecond 'sample.go:10:2: this value of x is never used (SA4006)'
    Assert-Check 'changed assignment in the same function' ($bad.Replace('x := value()', 'x := value() + 1')) $goodSecond 'sample.go:4:2: this value of x is never used (SA4006)'
    Set-Content -LiteralPath 'branches.go' -NoNewline -Value "$secondBranch`n"
    Assert-Check 'identical SA4006 moved to a different branch' $bad $goodSecond 'branches.go:8:3: this value of x is never used (SA4006)'
} finally {
    Pop-Location
    $resolved = (Resolve-Path -LiteralPath $fixture).Path
    if ((Split-Path -Parent $resolved) -ne $temp.TrimEnd([IO.Path]::DirectorySeparatorChar)) {
        throw "Unexpected fixture path: $resolved"
    }
    Remove-Item -LiteralPath $resolved -Recurse -Force
}
exit 0
