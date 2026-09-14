[CmdletBinding()]
param()
$ErrorActionPreference = 'Stop'
$project = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot '..')).ProviderPath
. (Join-Path $project 'install.ps1')
$root = Join-Path $project ('.tmp/install-public-' + [Guid]::NewGuid().ToString('N'))
$null = Assert-ClauductPath $root; [void][IO.Directory]::CreateDirectory($root)
$node = (Get-Command node.exe -CommandType Application | Select-Object -First 1).Source
$checks = 0; $oldPath = $env:Path; $oldCwd = (Get-Location).Path
function Need($Condition, [string] $Name) { if (-not $Condition) { throw ('TEST_' + $Name) }; $script:checks++ }
function Rejected([scriptblock] $Action, [string] $Category) {
    $caught = $null
    try { & $Action | Out-Null } catch { $caught = $_.Exception.Message }
    Need ($caught -eq $Category) ('EXPECTED_' + $Category)
}
function Package([string] $Id, [string] $Fault = '') {
    $directory = Join-Path $root ('package-' + $Id + '-' + [Guid]::NewGuid().ToString('N'))
    [void][IO.Directory]::CreateDirectory($directory)
    $archive = Join-Path $directory 'Clauduct.zip'; $manifest = Join-Path $directory 'manifest.json'
    $sources = @{
        'src/clauduct.mjs' = "console.log(JSON.stringify({credentialReads:0,childStarted:false,globalWrites:0,cwd:process.cwd(),args:process.argv.slice(2),version:'$Id'}));"
        'src/install-check.mjs' = $(if ($Fault -eq 'dependency') { "console.log(JSON.stringify({passed:false}));" } else { "console.log(JSON.stringify({passed:true,fixture:true,modelRequests:0}));" })
        'RELEASE.md' = 'PUBLIC_INSTALLER_FIXTURE'
    }
    if ($Fault -eq 'traversal') { $sources['../outside.txt'] = 'PUBLIC_PATH_ATTACK' }
    if ($Fault -eq 'device') { $sources['src/NUL.txt'] = 'PUBLIC_DEVICE_ATTACK' }
    Add-Type -AssemblyName System.IO.Compression.FileSystem
    $zip = [IO.Compression.ZipFile]::Open($archive, [IO.Compression.ZipArchiveMode]::Create)
    $files = @(); $total = 0
    try {
        foreach ($name in $sources.Keys) {
            $bytes = [Text.Encoding]::UTF8.GetBytes($sources[$name]); $entry = $zip.CreateEntry('Clauduct/' + $name)
            $stream = $entry.Open(); try { $stream.Write($bytes,0,$bytes.Length) } finally { $stream.Dispose() }
            $sha = [Security.Cryptography.SHA256]::Create()
            try { $digest = ([BitConverter]::ToString($sha.ComputeHash($bytes))).Replace('-','').ToLowerInvariant() } finally { $sha.Dispose() }
            $files += @{path=$name;bytes=$bytes.Length;sha256=$digest}; $total += $bytes.Length
        }
    } finally { $zip.Dispose() }
    if ($Fault -eq 'filehash') { $files[0].sha256 = '0' * 64 }
    $record = @{format=1;sourceCommit=$Id;archive='Clauduct.zip';sha256=(Get-ClauductHash $archive);files=$files;fileCount=$files.Count;uncompressedBytes=$total}
    if ($Fault -eq 'archivehash') { $record.sha256 = '0' * 64 }
    Write-ClauductText $manifest ($record | ConvertTo-Json -Depth 6)
    return @{Archive=$archive;Manifest=$manifest}
}
try {
    $pathResult = Get-ClauductUserPath 'C:\Public\Tools;' 'C:\Public\Bin'
    Need ($pathResult.changed -and $pathResult.value -ceq 'C:\Public\Tools;C:\Public\Bin') 'PATH_PRESERVES_EXISTING'
    $pathAgain = Get-ClauductUserPath $pathResult.value 'c:\public\bin'
    Need (-not $pathAgain.changed -and $pathAgain.value -ceq $pathResult.value) 'PATH_IDEMPOTENT'
    Need ((Get-ClauductUserPath '' 'C:\Public\Bin').value -ceq 'C:\Public\Bin') 'PATH_EMPTY'
    $first = Package ('a' * 40); $second = Package ('b' * 40)
    $bin = Join-Path $root 'Public User/bin'
    $one = Install-ClauductPackage @first -BinRoot $bin -Node $node
    Need $one.installed 'FIRST_INSTALL'
    $receipt = Read-ClauductJson (Join-Path $bin 'clauduct/install.json')
    Need ($receipt.sourceCommit -eq ('a' * 40)) 'RECEIPT'
    $work = Join-Path $root 'Other Working Directory'; [void][IO.Directory]::CreateDirectory($work)
    Set-Location -LiteralPath $work; $env:Path = $bin + ';' + $oldPath
    foreach ($command in @('clauduct','Clauduct')) {
        $output = & $command --dry-run 'argument with spaces' | ConvertFrom-Json
        Need ($LASTEXITCODE -eq 0 -and $output.cwd -eq $work -and $output.args[1] -eq 'argument with spaces') 'CWD_CASE_ARGUMENTS'
    }
    $two = Install-ClauductPackage @second -BinRoot $bin -Node $node
    Need ($two.sourceCommit -eq ('b' * 40)) 'UPDATE'
    Need (Test-Path -LiteralPath $one.versionPath) 'OLD_VERSION_RETAINED'
    $again = Install-ClauductPackage @second -BinRoot $bin -Node $node
    Need ($again.versionPath -eq $two.versionPath) 'IDEMPOTENT'
    $stable = Get-ClauductHash (Join-Path $bin 'clauduct.cmd')
    foreach ($case in @(@('archivehash','INSTALL_MANIFEST_MISMATCH'),@('filehash','INSTALL_FILE_HASH'),@('traversal','INSTALL_MANIFEST_ENTRY'),@('device','INSTALL_MANIFEST_ENTRY'),@('dependency','INSTALL_DEPENDENCIES_FAILED'))) {
        $bad = Package ('c' * 40) $case[0]
        Rejected { Install-ClauductPackage @bad -BinRoot $bin -Node $node } $case[1]
        Need ((Get-ClauductHash (Join-Path $bin 'clauduct.cmd')) -ceq $stable) 'FAILED_UPDATE_PRESERVES_COMMAND'
    }
    $collision = Join-Path $root 'collision'; [void][IO.Directory]::CreateDirectory($collision)
    Write-ClauductText (Join-Path $collision 'clauduct.cmd') 'PUBLIC_UNRELATED_COMMAND'
    Rejected { Install-ClauductPackage @first -BinRoot $collision -Node $node } 'INSTALL_COMMAND_COLLISION'
    Need ([IO.File]::ReadAllText((Join-Path $collision 'clauduct.cmd')) -ceq 'PUBLIC_UNRELATED_COMMAND') 'COLLISION_PRESERVED'
    $lock = Join-Path $bin 'clauduct/install.lock'; Write-ClauductText $lock 'PUBLIC_INTERRUPTED_INSTALL'
    Rejected { Install-ClauductPackage @first -BinRoot $bin -Node $node } 'INSTALL_ALREADY_RUNNING_OR_INTERRUPTED'
    Need (Test-Path -LiteralPath $lock) 'INTERRUPTED_LOCK_PRESERVED'; Remove-Item -LiteralPath $lock
    $changed = Join-Path $two.versionPath 'src/clauduct.mjs'; [IO.File]::AppendAllText($changed, '// PUBLIC_MODIFICATION')
    Rejected { Install-ClauductPackage @second -BinRoot $bin -Node $node } 'INSTALL_EXISTING_VERSION_CHANGED'
    Need ((Get-ClauductHash (Join-Path $bin 'clauduct.cmd')) -ceq $stable) 'MODIFIED_VERSION_COMMAND_PRESERVED'
    # Restore cwd before emitting pipeline output so relative report paths resolve
    # in the caller's directory, not the temporary command-launch fixture.
    Set-Location -LiteralPath $oldCwd
    [ordered]@{passed=$true;checks=$checks;publicFixture=$true;actualModelRequests=0;credentialReads=0;userPathWrites=0;root=$root} | ConvertTo-Json -Compress
} finally {
    $env:Path = $oldPath; Set-Location -LiteralPath $oldCwd
    $checked = Assert-ClauductPath $root
    if ([IO.Path]::GetDirectoryName($checked) -ine (Join-Path $project '.tmp') -or [IO.Path]::GetFileName($checked) -notmatch '^install-public-[a-f0-9]{32}$') { throw 'TEST_CLEANUP_BOUNDARY' }
    Remove-Item -LiteralPath $checked -Recurse
}
