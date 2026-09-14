[CmdletBinding()]
param(
    [string] $Version = 'latest',
    [string] $PackagePath,
    [string] $ManifestPath,
    [string] $InstallRoot,
    [switch] $NoPathUpdate
)

$ErrorActionPreference = 'Stop'

function Assert-ClauductPath([string] $Path) {
    $full = [IO.Path]::GetFullPath($Path)
    if ($full -match '[%!"\r\n]') { throw 'INSTALL_PATH_UNSUPPORTED' }
    $cursor = $full
    while ($cursor) {
        if (Test-Path -LiteralPath $cursor) {
            if ((Get-Item -LiteralPath $cursor -Force).Attributes -band [IO.FileAttributes]::ReparsePoint) { throw 'INSTALL_REPARSE_PATH' }
        }
        $parent = [IO.Path]::GetDirectoryName($cursor)
        if ($parent -eq $cursor) { break }; $cursor = $parent
    }
    return $full
}

function Read-ClauductJson([string] $Path, [long] $Limit = 262144) {
    $null = Assert-ClauductPath $Path
    $item = Get-Item -LiteralPath $Path -Force
    if ($item.PSIsContainer -or $item.Length -gt $Limit) { throw 'INSTALL_JSON_BOUND' }
    try { return [IO.File]::ReadAllText($Path) | ConvertFrom-Json } catch { throw 'INSTALL_JSON_INVALID' }
}

function Get-ClauductHash([string] $Path) { return (Get-FileHash -LiteralPath $Path -Algorithm SHA256).Hash.ToLowerInvariant() }

function Test-ClauductMember([string] $Path) {
    if ($Path -cnotmatch '^[A-Za-z0-9_./-]+$' -or $Path.StartsWith('/') -or $Path.EndsWith('/')) { return $false }
    foreach ($part in $Path.Split('/')) {
        if (-not $part -or $part -in @('.','..') -or $part.EndsWith('.') -or $part -imatch '^(CON|PRN|AUX|NUL|COM[1-9]|LPT[1-9])(?:\.|$)') { return $false }
    }
    return $true
}

function Write-ClauductText([string] $Path, [string] $Text) {
    [IO.File]::WriteAllText($Path, $Text, [Text.UTF8Encoding]::new($false))
}

function Receive-ClauductFile([string] $Url, [string] $Destination, [long] $Limit) {
    $uri = [Uri]$Url
    $allowed = @('github.com','api.github.com','objects.githubusercontent.com','release-assets.githubusercontent.com')
    for ($redirect = 0; $redirect -le 5; $redirect++) {
        if ($uri.Scheme -cne 'https' -or $uri.Host -cnotin $allowed -or $uri.UserInfo) { throw 'INSTALL_DOWNLOAD_DESTINATION' }
        $response = $null; $stream = $null; $file = $null
        try {
            $request = [Net.HttpWebRequest]::Create($uri)
            $request.AllowAutoRedirect = $false; $request.Timeout = 30000; $request.ReadWriteTimeout = 30000
            $request.UserAgent = 'Clauduct-Installer'; $request.UseDefaultCredentials = $false
            $response = $request.GetResponse()
            $status = [int]$response.StatusCode
            if ($status -in @(301,302,303,307,308)) {
                $uri = [Uri]::new($uri, $response.Headers['Location']); continue
            }
            if ($status -ne 200 -or $response.ContentLength -gt $Limit) { throw 'INSTALL_DOWNLOAD_BOUND' }
            $stream = $response.GetResponseStream(); $file = [IO.File]::Open($Destination, [IO.FileMode]::CreateNew)
            $buffer = New-Object byte[] 65536; $total = 0L; $clock = [Diagnostics.Stopwatch]::StartNew()
            while (($count = $stream.Read($buffer, 0, $buffer.Length)) -gt 0) {
                $total += $count
                if ($total -gt $Limit -or $clock.Elapsed.TotalSeconds -gt 60) { throw 'INSTALL_DOWNLOAD_BOUND' }
                $file.Write($buffer, 0, $count)
            }
            return
        } catch { throw 'INSTALL_DOWNLOAD_FAILED' }
        finally { if ($file) { $file.Dispose() }; if ($stream) { $stream.Dispose() }; if ($response) { $response.Dispose() } }
    }
    throw 'INSTALL_DOWNLOAD_REDIRECT_LIMIT'
}

function Expand-ClauductPackage([string] $Archive, [string] $Manifest, [string] $Destination) {
    $null = Assert-ClauductPath $Archive; $null = Assert-ClauductPath $Destination
    if (Test-Path -LiteralPath $Destination) { throw 'INSTALL_STAGE_EXISTS' }
    $record = Read-ClauductJson $Manifest
    if ($record.format -ne 1 -or $record.sourceCommit -cnotmatch '^[a-f0-9]{40}$' -or $record.sha256 -cnotmatch '^[a-f0-9]{64}$' -or
        $record.fileCount -lt 3 -or $record.fileCount -gt 512 -or @($record.files).Count -ne $record.fileCount -or
        $record.uncompressedBytes -gt 16MB -or (Get-Item -LiteralPath $Archive).Length -gt 16MB -or
        (Get-ClauductHash $Archive) -cne $record.sha256) { throw 'INSTALL_MANIFEST_MISMATCH' }
    $expected = @{}; $bytes = 0L
    foreach ($item in $record.files) {
        if (-not (Test-ClauductMember $item.path) -or
            $expected.ContainsKey($item.path) -or $item.sha256 -cnotmatch '^[a-f0-9]{64}$' -or $item.bytes -lt 0 -or $item.bytes -gt 4MB) { throw 'INSTALL_MANIFEST_ENTRY' }
        $expected[$item.path] = $item; $bytes += $item.bytes
    }
    if ($bytes -ne $record.uncompressedBytes -or $bytes -gt 16MB -or -not $expected.ContainsKey('src/clauduct.mjs') -or
        -not $expected.ContainsKey('src/install-check.mjs')) { throw 'INSTALL_MANIFEST_CONTENT' }
    Add-Type -AssemblyName System.IO.Compression.FileSystem
    $zip = [IO.Compression.ZipFile]::OpenRead($Archive)
    try {
        $seen = @{}; $total = 0L
        foreach ($entry in $zip.Entries) {
            if ($entry.FullName -cnotmatch '^Clauduct/[A-Za-z0-9_./-]*$' -or $entry.FullName -match '(^|/)\.\.?(/|$)') { throw 'INSTALL_ARCHIVE_PATH' }
            $kind = ($entry.ExternalAttributes -shr 16) -band 61440
            if ($entry.FullName.EndsWith('/')) { if ($kind -notin @(0,16384)) { throw 'INSTALL_ARCHIVE_TYPE' }; continue }
            if ($kind -notin @(0,32768)) { throw 'INSTALL_ARCHIVE_TYPE' }
            $path = $entry.FullName.Substring(9)
            if (-not $expected.ContainsKey($path) -or $seen.ContainsKey($path) -or $entry.Length -ne $expected[$path].bytes) { throw 'INSTALL_ARCHIVE_ENTRY' }
            $seen[$path] = $true; $total += $entry.Length
        }
        if ($seen.Count -ne $record.fileCount -or $total -ne $record.uncompressedBytes) { throw 'INSTALL_ARCHIVE_CONTENT' }
        [void][IO.Directory]::CreateDirectory($Destination)
        foreach ($entry in $zip.Entries) {
            if ($entry.FullName.EndsWith('/')) { continue }
            $target = Join-Path $Destination $entry.FullName.Substring(9)
            [void][IO.Directory]::CreateDirectory([IO.Path]::GetDirectoryName($target))
            $inputStream = $entry.Open(); $outputStream = [IO.File]::Open($target, [IO.FileMode]::CreateNew)
            try {
                $buffer = New-Object byte[] 65536; $written = 0L
                while (($count = $inputStream.Read($buffer,0,$buffer.Length)) -gt 0) {
                    $written += $count
                    if ($written -gt $entry.Length -or $written -gt 4MB) { throw 'INSTALL_ARCHIVE_EXPANSION_BOUND' }
                    $outputStream.Write($buffer,0,$count)
                }
                if ($written -ne $entry.Length) { throw 'INSTALL_ARCHIVE_EXPANSION_BOUND' }
            } finally { $outputStream.Dispose(); $inputStream.Dispose() }
            if ((Get-ClauductHash $target) -cne $expected[$entry.FullName.Substring(9)].sha256) { throw 'INSTALL_FILE_HASH' }
        }
    } finally { $zip.Dispose() }
    return $record
}

function Invoke-ClauductNode([string] $Node, [string] $Script, [string[]] $Arguments, [string] $WorkingDirectory) {
    if ($env:NODE_OPTIONS -or $env:NODE_DEBUG -or $env:NODE_USE_ENV_PROXY -or $null -ne $env:NODE_TLS_REJECT_UNAUTHORIZED) { throw 'INSTALL_NODE_RUNTIME_UNSUPPORTED' }
    $info = New-Object Diagnostics.ProcessStartInfo
    $info.FileName = $Node; $info.WorkingDirectory = $WorkingDirectory
    $info.UseShellExecute = $false; $info.CreateNoWindow = $true
    $info.RedirectStandardOutput = $true; $info.RedirectStandardError = $true; $info.RedirectStandardInput = $true
    # Only fixed flags and reviewed absolute paths are accepted here. PS 5.1 has no ArgumentList.
    $items = @($Script) + @($Arguments)
    if (@($items | Where-Object { $_ -match '["\r\n]' -or $_.EndsWith('\') }).Count) { throw 'INSTALL_ARGUMENT' }
    $info.Arguments = ($items | ForEach-Object { '"' + $_ + '"' }) -join ' '
    $process = [Diagnostics.Process]::Start($info)
    try {
        $process.StandardInput.Close(); $stdout = $process.StandardOutput.ReadToEndAsync(); $stderr = $process.StandardError.ReadToEndAsync()
        if (-not $process.WaitForExit(15000)) {
            # PS 5.1 lacks Process.Kill(true). Kill only this still-owned process tree.
            if (-not $process.HasExited) { & (Join-Path $env:SystemRoot 'System32/taskkill.exe') /PID $process.Id /T /F 2>$null | Out-Null }
            [void]$process.WaitForExit(5000); throw 'INSTALL_CHECK_TIMEOUT'
        }
        $text = $stdout.GetAwaiter().GetResult(); $errorText = $stderr.GetAwaiter().GetResult()
        if ($process.ExitCode -ne 0 -or $text.Length + $errorText.Length -gt 65536) { throw 'INSTALL_CHECK_FAILED' }
        try { return $text | ConvertFrom-Json } catch { throw 'INSTALL_CHECK_INVALID' }
    } finally { $process.Dispose() }
}

function Install-ClauductPackage {
    param([string] $Archive, [string] $Manifest, [string] $BinRoot, [string] $Node,
        [scriptblock] $Check = { param($nodePath,$stage) Invoke-ClauductNode $nodePath (Join-Path $stage 'src/install-check.mjs') @() $stage })
    $bin = Assert-ClauductPath $BinRoot; $nodePath = Assert-ClauductPath $Node
    if (-not (Test-Path -LiteralPath $nodePath -PathType Leaf)) { throw 'NODE_NOT_INSTALLED' }
    [void][IO.Directory]::CreateDirectory($bin)
    $owner = Join-Path $bin 'clauduct'; $shim = Join-Path $bin 'clauduct.cmd'; $receiptPath = Join-Path $owner 'install.json'
    $null = Assert-ClauductPath $owner; $null = Assert-ClauductPath $shim
    if ((Test-Path -LiteralPath $owner) -and -not (Test-Path -LiteralPath $receiptPath)) { throw 'INSTALL_OWNER_UNKNOWN' }
    if ((Test-Path -LiteralPath $shim) -and -not (Test-Path -LiteralPath $receiptPath)) { throw 'INSTALL_COMMAND_COLLISION' }
    foreach ($name in @('clauduct.exe','clauduct.ps1','clauduct.bat','clauduct.com')) {
        if (Test-Path -LiteralPath (Join-Path $bin $name)) { throw 'INSTALL_COMMAND_COLLISION' }
    }
    [void][IO.Directory]::CreateDirectory($owner)
    $lockPath = Join-Path $owner 'install.lock'; $lock = $null; $stage = Join-Path $owner ('.stage-' + [Guid]::NewGuid().ToString('N'))
    $temporaryShim = Join-Path $bin ('.clauduct-' + [Guid]::NewGuid().ToString('N') + '.cmd')
    $temporaryReceipt = Join-Path $owner ('.receipt-' + [Guid]::NewGuid().ToString('N') + '.json')
    $priorShim = $null; $priorReceipt = $null; $switched = $false; $versionMoved = $false; $destination = $null; $versionsCreated = $false
    try {
        try { $lock = [IO.File]::Open($lockPath, [IO.FileMode]::CreateNew, [IO.FileAccess]::Write, [IO.FileShare]::None) }
        catch { throw 'INSTALL_ALREADY_RUNNING_OR_INTERRUPTED' }
        if (Test-Path -LiteralPath $receiptPath) {
            $previous = Read-ClauductJson $receiptPath
            if ($previous.product -cne 'Clauduct' -or $previous.format -ne 1 -or $previous.binRoot -ine $bin -or
                $previous.sourceCommit -cnotmatch '^[a-f0-9]{40}$' -or $previous.shimSha256 -cne (Get-ClauductHash $shim)) { throw 'INSTALL_OWNER_MISMATCH' }
            $priorShim = [IO.File]::ReadAllBytes($shim); $priorReceipt = [IO.File]::ReadAllBytes($receiptPath)
        }
        $manifestRecord = Expand-ClauductPackage $Archive $Manifest $stage
        $dependencies = & $Check $nodePath $stage
        if (-not $dependencies.passed) { throw 'INSTALL_DEPENDENCIES_FAILED' }
        $dry = Invoke-ClauductNode $nodePath (Join-Path $stage 'src/clauduct.mjs') @('--dry-run') $stage
        if ($dry.credentialReads -ne 0 -or $dry.childStarted -ne $false -or $dry.globalWrites -ne 0) { throw 'INSTALL_DRY_RUN_FAILED' }
        $versions = Join-Path $owner 'versions'; $null = Assert-ClauductPath $versions
        $versionsCreated = -not (Test-Path -LiteralPath $versions)
        [void][IO.Directory]::CreateDirectory($versions)
        $destination = Join-Path $versions $manifestRecord.sourceCommit
        $null = Assert-ClauductPath $destination
        if (Test-Path -LiteralPath $destination) {
            foreach ($item in $manifestRecord.files) {
                $existing = Join-Path $destination $item.path; $null = Assert-ClauductPath $existing
                if ((Get-ClauductHash $existing) -cne $item.sha256) { throw 'INSTALL_EXISTING_VERSION_CHANGED' }
            }
        } else { [IO.Directory]::Move($stage, $destination); $versionMoved = $true }
        $text = '@echo off' + "`r`n" + '"' + $nodePath + '" "%~dp0clauduct\versions\' + $manifestRecord.sourceCommit + '\src\clauduct.mjs" %*' + "`r`nexit /b %errorlevel%`r`n"
        Write-ClauductText $temporaryShim $text
        $receipt = [ordered]@{product='Clauduct';format=1;binRoot=$bin;sourceCommit=$manifestRecord.sourceCommit;
            archiveSha256=$manifestRecord.sha256;shimSha256=(Get-ClauductHash $temporaryShim);node=$nodePath;dependencies=$dependencies;files=$manifestRecord.files}
        Write-ClauductText $temporaryReceipt (($receipt | ConvertTo-Json -Depth 12) + "`n")
        if ($priorShim) {
            if ((Get-ClauductHash $shim) -cne $previous.shimSha256) { throw 'INSTALL_COMMAND_CHANGED' }
            [IO.File]::Replace($temporaryShim, $shim, [NullString]::Value)
        } else { [IO.File]::Move($temporaryShim, $shim) }
        $switched = $true
        if ($priorReceipt) { [IO.File]::Replace($temporaryReceipt, $receiptPath, [NullString]::Value) } else { [IO.File]::Move($temporaryReceipt, $receiptPath) }
        return [ordered]@{installed=$true;sourceCommit=$manifestRecord.sourceCommit;binRoot=$bin;command=$shim;
            versionPath=$destination;loginChecked=$false;modelRequests=0;dependencies=$dependencies}
    } catch {
        if ($switched) {
            if ($priorShim) { [IO.File]::WriteAllBytes($shim, $priorShim) } else { Remove-Item -LiteralPath $shim }
        }
        if ($versionMoved) {
            $checked = Assert-ClauductPath $destination
            if ([IO.Path]::GetDirectoryName($checked) -ine $versions -or [IO.Path]::GetFileName($checked) -cnotmatch '^[a-f0-9]{40}$') { throw 'INSTALL_ROLLBACK_BOUNDARY' }
            Remove-Item -LiteralPath $checked -Recurse
        }
        if ($versionsCreated -and (Test-Path -LiteralPath $versions) -and @(Get-ChildItem -LiteralPath $versions -Force).Count -eq 0) { Remove-Item -LiteralPath $versions }
        throw
    } finally {
        foreach ($path in @($temporaryShim,$temporaryReceipt)) { if (Test-Path -LiteralPath $path) { Remove-Item -LiteralPath $path } }
        if (Test-Path -LiteralPath $stage) {
            $checked = Assert-ClauductPath $stage
            if ([IO.Path]::GetDirectoryName($checked) -ine $owner -or [IO.Path]::GetFileName($checked) -notmatch '^\.stage-[a-f0-9]{32}$') { throw 'INSTALL_CLEANUP_BOUNDARY' }
            Remove-Item -LiteralPath $checked -Recurse
        }
        if ($lock) { $lock.Dispose(); Remove-Item -LiteralPath $lockPath }
        if (-not (Test-Path -LiteralPath $receiptPath) -and @(Get-ChildItem -LiteralPath $owner -Force).Count -eq 0) { Remove-Item -LiteralPath $owner }
    }
}

function Get-ClauductUserPath([string] $Current, [string] $BinRoot) {
    $current = $Current
    if (-not $current) { $current = '' }
    $present = @($current -split ';' | ForEach-Object { [Environment]::ExpandEnvironmentVariables($_.Trim().Trim('"')).TrimEnd('\') }) -icontains $BinRoot.TrimEnd('\')
    return @{ changed = (-not $present); value = $(if ($present) { $current } else { ($current.TrimEnd(';') + ';' + $BinRoot).TrimStart(';') }) }
}

function Add-ClauductUserPath([string] $BinRoot) {
    $update = Get-ClauductUserPath ([Environment]::GetEnvironmentVariable('Path', 'User')) $BinRoot
    if ($update.changed) { [Environment]::SetEnvironmentVariable('Path', $update.value, 'User') }
    if (@($env:Path -split ';' | ForEach-Object { $_.TrimEnd('\') }) -inotcontains $BinRoot.TrimEnd('\')) { $env:Path = $BinRoot + ';' + $env:Path }
    return $update.changed
}

if ($MyInvocation.InvocationName -ne '.') {
    $download = $null
    try {
        if ($PSVersionTable.PSVersion -lt [Version]'5.1') { throw 'POWERSHELL_51_REQUIRED' }
        if ($env:OS -ne 'Windows_NT') { throw 'WINDOWS_REQUIRED' }
        $nodeCommand = Get-Command node.exe -CommandType Application -ErrorAction SilentlyContinue | Select-Object -First 1
        if (-not $nodeCommand) { throw 'NODE_NOT_INSTALLED_INSTALL_NODE_24' }
        $node = Assert-ClauductPath $nodeCommand.Source
        if (-not $InstallRoot) { $InstallRoot = Join-Path ([Environment]::GetFolderPath('UserProfile')) '.local\bin' }
        if ([bool]$PackagePath -ne [bool]$ManifestPath) { throw 'PACKAGE_AND_MANIFEST_REQUIRED_TOGETHER' }
        if (-not $PackagePath) {
            $base = Assert-ClauductPath ([IO.Path]::GetTempPath())
            $download = Join-Path $base ('clauduct-download-' + [Guid]::NewGuid().ToString('N'))
            [void][IO.Directory]::CreateDirectory($download)
            if ($Version -ceq 'latest') {
                $releaseFile = Join-Path $download 'release.json'
                Receive-ClauductFile 'https://api.github.com/repos/wotjr1649/Clauduct/releases/latest' $releaseFile 262144
                $Version = (Read-ClauductJson $releaseFile).tag_name
            }
            if ($Version -cnotmatch '^[A-Za-z0-9][A-Za-z0-9._-]{0,99}$') { throw 'INSTALL_RELEASE_TAG' }
            $prefix = 'https://github.com/wotjr1649/Clauduct/releases/download/' + $Version + '/'
            $PackagePath = Join-Path $download 'Clauduct-windows-x64.zip'; $ManifestPath = Join-Path $download 'manifest.json'
            Receive-ClauductFile ($prefix + 'manifest.json') $ManifestPath 262144
            Receive-ClauductFile ($prefix + 'Clauduct-windows-x64.zip') $PackagePath 16777216
        }
        $result = Install-ClauductPackage -Archive $PackagePath -Manifest $ManifestPath -BinRoot $InstallRoot -Node $node
        $pathChanged = $false
        if (-not $NoPathUpdate) { $pathChanged = Add-ClauductUserPath $result.binRoot }
        $result.pathUpdated = $pathChanged; $result.restartParentTerminal = $pathChanged
        $result | ConvertTo-Json -Depth 6
        Write-Host 'Clauduct installed. Open a new terminal if PATH changed. Existing Codex login/file credential store is required; authentication files were not changed.'
    } catch { throw ('Clauduct installation failed: ' + $(if ($_.Exception.Message -cmatch '^[A-Z_0-9]+$') { $_.Exception.Message } else { 'INSTALL_FAILED' })) }
    finally {
        if ($download -and (Test-Path -LiteralPath $download)) {
            $checked = Assert-ClauductPath $download
            if ([IO.Path]::GetDirectoryName($checked).TrimEnd('\') -ine $base.TrimEnd('\') -or [IO.Path]::GetFileName($checked) -notmatch '^clauduct-download-[a-f0-9]{32}$') { throw 'DOWNLOAD_CLEANUP_BOUNDARY' }
            Remove-Item -LiteralPath $checked -Recurse
        }
    }
}
