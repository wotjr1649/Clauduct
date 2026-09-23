param([string] $New, [string] $Old, [string] $Root)
$ErrorActionPreference = 'Stop'
function Digest([string] $p) { (Get-FileHash -Algorithm SHA256 -LiteralPath $p).Hash.ToLowerInvariant() }
function Sums([string] $dir) {
    $m = @{}
    foreach ($l in Get-Content -LiteralPath (Join-Path $dir 'SHA256SUMS')) { $f = -split $l.Trim(); if ($f.Count -eq 2) { $m[$f[1].TrimStart('*')] = $f[0].ToLowerInvariant() } }
    $m
}
function UserPath { [string] [Microsoft.Win32.Registry]::CurrentUser.OpenSubKey('Environment').GetValue('Path', '', 'DoNotExpandEnvironmentNames') }
function Check([string] $label, [string] $dir, [string] $assets) {
    $sums = Sums $assets
    $bad = @(foreach ($n in 'clauduct.exe', 'clauduct-hook.exe', 'clauduct-dev.exe') { if ((Digest (Join-Path $dir $n)) -cne $sums[$n]) { $n } })
    $version = (& (Join-Path $dir 'clauduct-dev.exe') version) -join ' '
    $old = @(Get-ChildItem -LiteralPath $dir -Filter '*.old' -File).Count
    $ok = $bad.Count -eq 0 -and $old -eq 0
    Write-Output ("{0}: digests={1} old_files={2} version=[{3}] -> {4}" -f $label, $(if ($bad.Count) { 'MISMATCH ' + ($bad -join ',') } else { 'match' }), $old, ($version -replace '\s+', ' '), $(if ($ok) { 'PASS' } else { 'FAIL' }))
    if (-not $ok) { $script:failed = $true }
}
$failed = $false
$pathBefore = UserPath
$quiet = { param($script, $from, $into) & pwsh -NoProfile -File $script -FromPath $from -InstallRoot $into -NoPathUpdate *> $null; if ($LASTEXITCODE) { throw "install failed: $from -> $into" } }

& $quiet (Join-Path $New 'install.ps1') $New (Join-Path $Root 'fresh')
Check 'new install 0.3.2' (Join-Path $Root 'fresh') $New

$upgrade = Join-Path $Root 'upgrade'
& $quiet (Join-Path $Old 'install.ps1') $Old $upgrade
Check 'install 0.3.1 from its published assets' $upgrade $Old
& $quiet (Join-Path $New 'install.ps1') $New $upgrade
Check 'update 0.3.1 -> 0.3.2' $upgrade $New
& $quiet (Join-Path $Old 'install.ps1') $Old $upgrade
Check 'rollback 0.3.2 -> 0.3.1' $upgrade $Old

$pathSame = (UserPath) -ceq $pathBefore
Write-Output ("user PATH unchanged: {0}" -f $pathSame)
if (-not $pathSame) { $failed = $true }
Write-Output $(if ($failed) { 'SHIP_INSTALL FAIL' } else { 'SHIP_INSTALL PASS' })
exit [int] $failed
