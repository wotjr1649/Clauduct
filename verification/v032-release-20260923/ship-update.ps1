param([string] $Published, [string] $Root)
$ErrorActionPreference = 'Stop'
function Digest([string] $p) { (Get-FileHash -Algorithm SHA256 -LiteralPath $p).Hash.ToLowerInvariant() }
$sums = @{}
foreach ($l in Get-Content -LiteralPath (Join-Path $Published 'SHA256SUMS')) { $f = -split $l.Trim(); if ($f.Count -eq 2) { $sums[$f[1].TrimStart('*')] = $f[0].ToLowerInvariant() } }
function UserPath { [string] [Microsoft.Win32.Registry]::CurrentUser.OpenSubKey('Environment').GetValue('Path', '', 'DoNotExpandEnvironmentNames') }
$failed = $false
function Check([string] $label, [string] $dir, [int] $wantOld) {
    $bad = @(foreach ($n in 'clauduct.exe', 'clauduct-hook.exe', 'clauduct-dev.exe') { if ((Digest (Join-Path $dir $n)) -cne $sums[$n]) { $n } })
    $version = ((& (Join-Path $dir 'clauduct-dev.exe') version) -join ' ') -replace '\s+', ' '
    $old = @(Get-ChildItem -LiteralPath $dir -Filter '*.old' -File).Count
    $ok = $bad.Count -eq 0 -and $old -eq $wantOld -and $version -match '^clauduct 0\.3\.2 commit 5c2e719b8af0d1294ac6e19a4d2909a7c5e7b7a8 '
    Write-Output ("{0}: digests={1} old_files={2} version=[{3}] -> {4}" -f $label, $(if ($bad.Count) { 'MISMATCH' } else { 'match' }), $old, $version, $(if ($ok) { 'PASS' } else { 'FAIL' }))
    if (-not $ok) { $script:failed = $true }
}
$pathBefore = UserPath

# A pinned install downloads from releases/download/v0.3.2.
$pinned = Join-Path $Root 'pinned'
& pwsh -NoProfile -File (Join-Path $Published 'install.ps1') -Tag v0.3.2 -InstallRoot $pinned -NoPathUpdate *> $null
if ($LASTEXITCODE) { throw 'pinned install failed' }
Check 'install -Tag v0.3.2 from GitHub' $pinned 0

# The isolated 0.3.1 left by the rollback check updates itself from releases/latest.
$upgrade = Join-Path $Root 'upgrade'
$before = ((& (Join-Path $upgrade 'clauduct-dev.exe') version) -join ' ') -replace '\s+', ' '
Write-Output "before update: $before"
$out = & (Join-Path $upgrade 'clauduct.exe') --update --yes 2>&1
Write-Output ($out | Where-Object { $_ -match '^(installed|running|release|updated|already|clauduct:)' })
# The 0.3.1 process cannot delete its own image, so one .old is expected until the next launch.
Check 'clauduct --update --yes (0.3.1 -> 0.3.2)' $upgrade 1
& (Join-Path $upgrade 'clauduct.exe') --version *> $null
Check 'next clauduct launch sweeps the .old' $upgrade 0
$again = & (Join-Path $upgrade 'clauduct.exe') --update --yes 2>&1
Write-Output ($again | Where-Object { $_ -match '^(already|updated|clauduct:)' })
if (-not ($again -match '^already current')) { $failed = $true }
Check 'second --update is a no-op' $upgrade 0

$pathSame = (UserPath) -ceq $pathBefore
Write-Output ("user PATH unchanged: {0}" -f $pathSame)
if (-not $pathSame) { $failed = $true }
Write-Output $(if ($failed) { 'SHIP_UPDATE FAIL' } else { 'SHIP_UPDATE PASS' })
exit [int] $failed
