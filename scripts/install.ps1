[CmdletBinding()]
param(
    [string] $Tag = 'latest',
    [string] $Repo = 'wotjr1649/Clauduct',
    [string] $InstallRoot = (Join-Path $env:USERPROFILE '.local\bin'),
    [string] $FromPath,
    [switch] $NoPathUpdate,
    [switch] $SkipPreflight
)

$ErrorActionPreference = 'Stop'

# The three travel together. clauduct-hook has to sit beside clauduct, because findHook()
# looks only next to the executable; when it is missing the client still starts and exits 0,
# and role routing and the delegation menu's effort just stop working. Measured 2026-09-17:
# a copy run from a directory without it reports hookInstalled:false and says nothing else.
$Names    = @('clauduct.exe', 'clauduct-hook.exe', 'clauduct-dev.exe')
$SumsName = 'SHA256SUMS'

# Reads SHA256SUMS the way the built-in updater does (update.Sums): two whitespace-separated
# fields, a 64-character hex digest, and a name that may carry sha256sum's binary-mode star.
function Read-Sums([string] $Path) {
    $out = @{}
    foreach ($line in Get-Content -LiteralPath $Path) {
        $fields = -split $line.Trim()
        if ($fields.Count -ne 2) { continue }
        if ($fields[0] -notmatch '^[0-9a-fA-F]{64}$') { continue }
        $out[$fields[1].TrimStart('*')] = $fields[0].ToLowerInvariant()
    }
    return $out
}

# Nothing is copied until all three match. A half-installed set is worse than a refused
# install: two new binaries beside one old one is a combination no release was tested as.
function Assert-Digests([string] $Dir) {
    $sumsPath = Join-Path $Dir $SumsName
    if (-not (Test-Path -LiteralPath $sumsPath -PathType Leaf)) { throw "INSTALL_SUMS_MISSING $sumsPath" }
    $sums = Read-Sums $sumsPath
    foreach ($name in $Names) {
        $file = Join-Path $Dir $name
        if (-not (Test-Path -LiteralPath $file -PathType Leaf)) { throw "INSTALL_SOURCE_MISSING $name" }
        if (-not $sums.ContainsKey($name)) { throw "INSTALL_SUMS_INCOMPLETE $name" }
        $have = (Get-FileHash -LiteralPath $file -Algorithm SHA256).Hash.ToLowerInvariant()
        if ($have -ne $sums[$name]) { throw "INSTALL_DIGEST_MISMATCH $name" }
    }
}

function Get-Release([string] $Dir) {
    [Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
    $ProgressPreference = 'SilentlyContinue'
    if ($Tag -eq 'latest') { $base = "https://github.com/$Repo/releases/latest/download" }
    else                   { $base = "https://github.com/$Repo/releases/download/$Tag" }
    foreach ($name in ($Names + $SumsName)) {
        try { Invoke-WebRequest -Uri "$base/$name" -OutFile (Join-Path $Dir $name) -UseBasicParsing }
        catch { throw "INSTALL_DOWNLOAD_FAILED $name $base/$name" }
    }
}

# Read-modify-write on the user's Path has three ways to destroy it and all of them are quiet.
# .NET's GetEnvironmentVariable expands %USERPROFILE% before handing the string over, so
# writing the result back bakes the expansion in; SetEnvironmentVariable then stores it as
# REG_SZ, after which no remaining %VAR% entry ever expands again; and setx truncates at 1024
# characters. So: a raw read through the registry, and the same value kind on the way back.
function Add-UserPathEntry([string] $Dir) {
    $key = [Microsoft.Win32.Registry]::CurrentUser.OpenSubKey('Environment', $true)
    try {
        $kind = [Microsoft.Win32.RegistryValueKind]::ExpandString
        if ($null -ne $key.GetValue('Path')) { $kind = $key.GetValueKind('Path') }
        $raw  = [string] $key.GetValue('Path', '', [Microsoft.Win32.RegistryValueOptions]::DoNotExpandEnvironmentNames)
        $have = @($raw -split ';' | Where-Object { $_ })
        $want = $Dir.TrimEnd('\')
        foreach ($entry in $have) {
            if ($entry.TrimEnd('\') -eq $want) { return $false }
            if ([Environment]::ExpandEnvironmentVariables($entry).TrimEnd('\') -eq $want) { return $false }
        }
        $key.SetValue('Path', (($have + $Dir) -join ';'), $kind)
        return $true
    } finally { if ($key) { $key.Dispose() } }
}

# New processes read the registry, but Explorer hands its own copy of the environment to
# everything it launches. Without this broadcast a terminal opened from the taskbar keeps the
# old Path until the next sign-in, which reads to the user as "the installer did not work".
function Publish-EnvironmentChange {
    if (-not ('Clauduct.Native' -as [type])) {
        Add-Type -Namespace Clauduct -Name Native -MemberDefinition @"
[System.Runtime.InteropServices.DllImport("user32.dll", SetLastError = true, CharSet = System.Runtime.InteropServices.CharSet.Auto)]
public static extern System.IntPtr SendMessageTimeout(System.IntPtr hWnd, uint Msg, System.UIntPtr wParam,
    string lParam, uint fuFlags, uint uTimeout, out System.UIntPtr lpdwResult);
"@
    }
    $result = [UIntPtr]::Zero
    [void] [Clauduct.Native]::SendMessageTimeout([IntPtr] 0xffff, 0x1A, [UIntPtr]::Zero, 'Environment', 2, 5000, [ref] $result)
}

# Mirrors platform.Resolver.find, including the parts that look like overkill in an
# installer: the standalone location first, then PATH capped at 64 raw entries, unquoted,
# absolute only, with the working directory dropped and duplicates removed. Searching wider
# than the launcher does would pass on a machine where the launcher then reports
# CLAUDE_NOT_FOUND -- a preflight that disagrees with runtime is worse than none.
#
# One difference, stated rather than hidden: the launcher reads the OS user record for the
# home directory because %USERPROFILE% can be handed to it poisoned. Here the user is running
# their own shell, so $env:USERPROFILE is what they mean.
function Find-NativeTool([string] $Name) {
    $standalone = Join-Path $env:USERPROFILE ".local\bin\$Name"
    if (Test-Path -LiteralPath $standalone -PathType Leaf) { return $standalone }

    try { $cwd = [IO.Path]::GetFullPath((Get-Location).Path).TrimEnd('\').ToLowerInvariant() }
    catch { $cwd = '' }

    $raw = @($env:PATH -split ';')
    if ($raw.Count -gt 64) { $raw = $raw[0..63] }
    $seen = @{}
    foreach ($entry in $raw) {
        $dir = $entry.Trim()
        if ($dir.Length -ge 2 -and $dir.StartsWith('"') -and $dir.EndsWith('"')) {
            $dir = $dir.Substring(1, $dir.Length - 2)
        }
        if (-not $dir -or -not [IO.Path]::IsPathRooted($dir)) { continue }
        try { $key = [IO.Path]::GetFullPath($dir).TrimEnd('\').ToLowerInvariant() } catch { continue }
        if ($key -eq $cwd -or $seen.ContainsKey($key)) { continue }
        $seen[$key] = $true
        $candidate = Join-Path $dir $Name
        if (Test-Path -LiteralPath $candidate -PathType Leaf) { return $candidate }
    }
    return $null
}

# Both have to be there or this build has nothing to do: it launches claude.exe, and every
# request it forwards identifies itself with the installed Codex CLI's version. The names
# thrown here are the ones the launcher uses at runtime, so a search for either finds the
# same answer whichever surface reported it.
function Assert-Prerequisites {
    $missing = @()
    foreach ($tool in @(
        @{ Name = 'claude.exe'; Code = 'CLAUDE_NOT_FOUND'; What = 'Claude Code' },
        @{ Name = 'codex.exe';  Code = 'CODEX_NOT_FOUND';  What = 'Codex CLI' })) {
        $found = Find-NativeTool $tool.Name
        if ($found) { Write-Host "found $($tool.What): $found" }
        else { $missing += "$($tool.Code) ($($tool.What), $($tool.Name))" }
    }
    if ($missing.Count -gt 0) {
        throw ("INSTALL_PREREQUISITE_MISSING -- " + ($missing -join '; ') +
               ". Install them first, or pass -SkipPreflight to install anyway.")
    }
}

$InstallRoot = [IO.Path]::GetFullPath($InstallRoot)

# MSYS and Git Bash stop their PATH search at a directory carrying the command's name and
# never reach clauduct.exe, while cmd finds it anyway through PATHEXT -- so the same install
# works in one shell and not the other. v1 put its version store at exactly this path, so the
# guard is not hypothetical; it happened on this machine on 2026-09-17.
$shadow = Join-Path $InstallRoot 'clauduct'
if (Test-Path -LiteralPath $shadow -PathType Container) {
    throw "INSTALL_DIRECTORY_SHADOW $shadow -- rename it first: Rename-Item '$shadow' 'clauduct-node-store'"
}

# Before the download, not after: a machine that cannot run this build should not spend the
# bandwidth finding out.
if ($SkipPreflight) { Write-Host 'preflight: skipped (-SkipPreflight)' } else { Assert-Prerequisites }

$staging = $null
try {
    if ($FromPath) {
        $source = [IO.Path]::GetFullPath($FromPath)
    } else {
        $staging = Join-Path ([IO.Path]::GetTempPath()) ('clauduct-install-' + [guid]::NewGuid().ToString('N'))
        New-Item -ItemType Directory -Path $staging -Force | Out-Null
        Get-Release $staging
        $source = $staging
    }

    Assert-Digests $source

    New-Item -ItemType Directory -Path $InstallRoot -Force | Out-Null
    foreach ($name in $Names) {
        $target = Join-Path $InstallRoot $name
        Copy-Item -LiteralPath (Join-Path $source $name) -Destination $target -Force
        # These bytes were just checked against the release's own digest, which is the
        # question SmartScreen's dialog asks on every double-click. Answer it once, here.
        Unblock-File -LiteralPath $target -ErrorAction SilentlyContinue
        Write-Host "installed $target"
    }
} finally {
    if ($staging -and (Test-Path -LiteralPath $staging)) {
        Remove-Item -LiteralPath $staging -Recurse -Force -ErrorAction SilentlyContinue
    }
}

# v1's launcher answers to the same name. .EXE precedes .CMD in PATHEXT so this build wins in
# cmd and PowerShell, but a name that resolves two ways is a question someone gets to answer
# at a bad moment.
$legacy = Join-Path $InstallRoot 'clauduct.cmd'
if (Test-Path -LiteralPath $legacy -PathType Leaf) {
    Write-Host "note: $legacy is v1's launcher. Keep the way back under its own name: Rename-Item '$legacy' 'clauduct-node.cmd'"
}

if ($NoPathUpdate) {
    Write-Host "PATH: untouched (-NoPathUpdate). $InstallRoot has to be on PATH for 'clauduct' to work from any directory."
} elseif (Add-UserPathEntry $InstallRoot) {
    Publish-EnvironmentChange
    Write-Host "PATH: added $InstallRoot to the user PATH. Open a new terminal."
} else {
    Write-Host "PATH: $InstallRoot was already on it."
}

Write-Host "verify: clauduct --version"
