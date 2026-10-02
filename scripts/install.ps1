#requires -Version 7
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

# One file since v0.4.0: clauduct.exe is also the hook, the PDF renderer and `clauduct --dev`
# (#112). The names a 0.3.x installation also had are removed once it is in place; releases
# through v0.4.x still publish them as copies, for the 0.3.x updater that requires all three.
$Names    = @('clauduct.exe')
$Retired  = @('clauduct-hook.exe', 'clauduct-dev.exe')
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

# Nothing is copied until every file matches. A half-installed set is worse than a refused
# install: a new binary beside an old one is a combination no release was tested as.
function Assert-Digests([string] $Dir) {
    $sumsPath = Join-Path $Dir $SumsName
    if (-not (Test-Path -LiteralPath $sumsPath -PathType Leaf)) { throw "INSTALL_SUMS_MISSING $sumsPath" }
    $sums = Read-Sums $sumsPath
    foreach ($name in $Names) {
        $file = Join-Path $Dir $name
        if (-not (Test-Path -LiteralPath $file -PathType Leaf)) { throw "INSTALL_SOURCE_MISSING $name" }
        if (-not $sums.ContainsKey($name)) { throw "INSTALL_SUMS_INCOMPLETE $name" }
        $have = (Get-FileHash -Algorithm SHA256 -LiteralPath $file).Hash.ToLowerInvariant()
        # -cne, not -ne: PowerShell compares strings case-insensitively by default, which
        # would quietly accept a digest this script failed to normalise. Both sides are
        # lowercased above, so the exact comparison is the one that means something.
        if ($have -cne $sums[$name]) { throw "INSTALL_DIGEST_MISMATCH $name" }
    }
}

# All or none once copying starts too, the way `clauduct --update` does it
# (update.Apply). Each is staged beside its target first, so a failed copy replaces nothing.
# Then each current binary is renamed to .old and the staged one takes its name: a running
# executable cannot be overwritten but can be renamed, and a running one is the lock a plain
# copy loop most plausibly hit. Any failure puts every original back.
function Install-Set([string] $Source, [string] $Root) {
    $targets = @($Names | ForEach-Object { Join-Path $Root $_ })
    $swapped = @()
    try {
        foreach ($target in $targets) {
            Copy-Item -LiteralPath (Join-Path $Source (Split-Path $target -Leaf)) -Destination "$target.new" -Force
            # These bytes were just checked against the release's own digest, which is the
            # question SmartScreen's dialog asks on every double-click. Answer it once, here.
            Unblock-File -LiteralPath "$target.new" -ErrorAction SilentlyContinue
        }
        foreach ($target in $targets) {
            Remove-Item -LiteralPath "$target.old" -Force -ErrorAction SilentlyContinue
            $had = Test-Path -LiteralPath $target -PathType Leaf
            if ($had) { Move-Item -LiteralPath $target -Destination "$target.old" }
            $swapped += @{ Path = $target; Had = $had }
            Move-Item -LiteralPath "$target.new" -Destination $target
        }
    } catch {
        $why = $_.Exception.Message
        $lost = @()
        for ($i = $swapped.Count - 1; $i -ge 0; $i--) {
            $s = $swapped[$i]
            try {
                if (Test-Path -LiteralPath $s.Path) { Remove-Item -LiteralPath $s.Path -Force }
                if ($s.Had) { Move-Item -LiteralPath "$($s.Path).old" -Destination $s.Path }
            } catch { $lost += $s.Path }
        }
        foreach ($target in $targets) { Remove-Item -LiteralPath "$target.new" -Force -ErrorAction SilentlyContinue }
        if ($lost.Count -gt 0) {
            throw "INSTALL_SWAP_FAILED $why -- could not put back $($lost -join ', '); each original is beside it as .old"
        }
        throw "INSTALL_SWAP_FAILED $why -- the previous set is back in place"
    }
    foreach ($target in $targets) {
        Write-Host "installed $target"
        Remove-Item -LiteralPath "$target.old" -Force -ErrorAction SilentlyContinue
        if (Test-Path -LiteralPath "$target.old") { Write-Host "note: $target.old is still running; delete it once it exits" }
    }
    # Only after the new set is in place, so a failure above leaves a 0.3.x installation whole.
    foreach ($name in $Retired) {
        $path = Join-Path $Root $name
        Remove-Item -LiteralPath $path -Force -ErrorAction SilentlyContinue
        if (Test-Path -LiteralPath $path) { Write-Host "note: $path is still running; delete it once it exits" }
    }
}

function Get-Release([string] $Dir) {
    # Keep scripted downloads quiet; certificate checks and TLS 1.2 stay enabled.
    $ProgressPreference = 'SilentlyContinue'
    if ($Tag -eq 'latest') { $base = "https://github.com/$Repo/releases/latest/download" }
    else                   { $base = "https://github.com/$Repo/releases/download/$Tag" }
    # The digests first: they decide which files this release is (Select-Names).
    try { Invoke-WebRequest -Uri "$base/$SumsName" -OutFile (Join-Path $Dir $SumsName) -SslProtocol Tls12 }
    catch { throw "INSTALL_DOWNLOAD_FAILED $SumsName $base/$SumsName" }
    Select-Names $Dir
    foreach ($name in $Names) {
        try { Invoke-WebRequest -Uri "$base/$name" -OutFile (Join-Path $Dir $name) -SslProtocol Tls12 }
        catch { throw "INSTALL_DOWNLOAD_FAILED $name $base/$name" }
    }
}

# Which files a release is. From v0.4.0 clauduct-hook.exe and clauduct-dev.exe are copies of
# clauduct.exe, published only for 0.3.x updaters, and one file is the installation. A release
# whose clauduct-hook.exe differs from its clauduct.exe is a 0.3.x one, where they are separate
# programs that must sit beside it -- installing its clauduct.exe alone (a rollback with -Tag)
# would leave a session with no hook. Decided by the release's own digests, not its tag.
function Select-Names([string] $Dir) {
    # Missing digests are Assert-Digests' refusal to make, by its own name.
    if (-not (Test-Path -LiteralPath (Join-Path $Dir $SumsName) -PathType Leaf)) { return }
    $sums = Read-Sums (Join-Path $Dir $SumsName)
    if ($sums.ContainsKey('clauduct-hook.exe') -and $sums['clauduct-hook.exe'] -cne $sums['clauduct.exe']) {
        $script:Names = @('clauduct.exe') + $Retired
        $script:Retired = @()
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

# The broadcast is a convenience, and Add-Type is one of the cmdlets that can be missing.
# Failing the install over it would trade a working PATH entry for a cosmetic refresh.
function Try-PublishEnvironmentChange {
    try { Publish-EnvironmentChange; return $true } catch { return $false }
}

# Mirrors platform.Resolver, including the parts that look like overkill in an
# installer: the standalone location first, then PATH capped at 64 raw entries, unquoted,
# absolute only, with the working directory dropped and duplicates removed. Searching wider
# than the launcher does would pass on a machine where the launcher then reports
# CLAUDE_NOT_FOUND -- a preflight that disagrees with runtime is worse than none.
#
# One difference, stated rather than hidden: the launcher reads the OS user record for the
# home directory because %USERPROFILE% can be handed to it poisoned. Here the user is running
# their own shell, so $env:USERPROFILE is what they mean.
#
# Codex has two places more, as platform.Resolver.Codex does: the Codex app's standalone
# install before PATH, and last the native codex.exe an npm install carries.
function Find-NativeTool([string] $Name) {
    try { $cwd = [IO.Path]::GetFullPath((Get-Location).Path).TrimEnd('\').ToLowerInvariant() }
    catch { $cwd = '' }

    $raw = @($env:PATH -split ';')
    if ($raw.Count -gt 64) { $raw = $raw[0..63] }
    $seen = @{}
    $dirs = @()
    foreach ($entry in $raw) {
        $dir = $entry.Trim()
        if ($dir.Length -ge 2 -and $dir.StartsWith('"') -and $dir.EndsWith('"')) {
            $dir = $dir.Substring(1, $dir.Length - 2)
        }
        if (-not $dir -or -not [IO.Path]::IsPathRooted($dir)) { continue }
        try { $key = [IO.Path]::GetFullPath($dir).TrimEnd('\').ToLowerInvariant() } catch { continue }
        if ($key -eq $cwd -or $seen.ContainsKey($key)) { continue }
        $seen[$key] = $true
        $dirs += $dir
    }

    $candidates = @(Join-Path $env:USERPROFILE ".local\bin\$Name")
    if ($Name -eq 'codex.exe') { $candidates += Join-Path $env:USERPROFILE 'AppData\Local\Programs\OpenAI\Codex\bin\codex.exe' }
    $candidates += @($dirs | ForEach-Object { Join-Path $_ $Name })
    if ($Name -eq 'codex.exe') {
        $npm = 'node_modules\@openai\codex\node_modules\@openai\codex-win32-x64\vendor\x86_64-pc-windows-msvc\bin\codex.exe'
        $candidates += @((@(Join-Path $env:USERPROFILE 'AppData\Roaming\npm') + $dirs) | ForEach-Object { Join-Path $_ $npm })
    }
    foreach ($candidate in $candidates) {
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

# Runs the installed clauduct.exe with fixed arguments, no shell, and a 10 s bound, and
# returns its exit code and what it printed. $Code and $Role name a failure to start or finish.
function Invoke-Installed([string] $Arguments, [string] $Code, [string] $Role) {
    $process = [Diagnostics.Process]::new()
    $process.StartInfo.FileName = Join-Path $InstallRoot 'clauduct.exe'
    $process.StartInfo.Arguments = $Arguments
    $process.StartInfo.UseShellExecute = $false
    $process.StartInfo.CreateNoWindow = $true
    $process.StartInfo.RedirectStandardOutput = $true
    $process.StartInfo.RedirectStandardError = $true
    try {
        if (-not $process.Start()) { throw "${Code}: $Role did not start" }
        # Read while it runs, so a full pipe can never hold the process.
        $stdout = $process.StandardOutput.ReadToEndAsync()
        $stderr = $process.StandardError.ReadToEndAsync()
        if (-not $process.WaitForExit(10000)) {
            $process.Kill()
            if (-not $process.WaitForExit(5000)) { throw "${Code}: $Role cleanup unconfirmed" }
            throw "${Code}: $Role timed out"
        }
        return [pscustomobject]@{ ExitCode = $process.ExitCode; Output = ($stdout.Result + $stderr.Result).Trim() }
    } finally {
        $process.Dispose()
    }
}

# The verified installed program owns the settings document. An absent settings.json is
# created through its no-replace publication path. An existing one gets only the top-level
# keys this release added, after the program has published a backup of the original bytes;
# no value already in the file changes. The old three-program layout has no --dev command.
function Ensure-Settings {
    $dir = Join-Path $env:USERPROFILE '.clauduct'
    $target = Join-Path $dir 'settings.json'
    if (Test-Path -LiteralPath $target -PathType Leaf) {
        if ($Names.Count -ne 1) { return }
        $sync = Invoke-Installed '--dev --sync-settings' 'CLAUDUCT_SETTINGS_SYNC_FAILED' 'settings sync'
        # Every release since v0.4.0 answers an unknown --dev word with its usage line and
        # exit 2. A crash can also exit 2, so the usage line is what identifies an old release.
        if ($sync.ExitCode -eq 2 -and $sync.Output.Contains('usage: clauduct --dev [')) {
            Write-Host 'note: this release predates settings sync; existing preferences were preserved'
        } elseif ($sync.ExitCode -ne 0) {
            # The program's own refusal is one line: a code, and the backup path when it made one.
            $reason = ($sync.Output -split "`r?`n")[0]
            throw "CLAUDUCT_SETTINGS_SYNC_FAILED: binary installed; settings sync failed; original preserved (backup if any): $reason"
        } elseif ($sync.Output) {
            Write-Host $sync.Output
        }
        return
    }
    if (Test-Path -LiteralPath $target) { throw 'CLAUDUCT_SETTINGS_CREATE_FAILED: settings.json is not a file' }
    if ($Names.Count -ne 1) {
        Write-Host 'note: this historical release has no settings initializer; existing preferences were preserved'
        return
    }
    $init = Invoke-Installed '--dev --init-settings' 'CLAUDUCT_SETTINGS_CREATE_FAILED' 'initializer'
    if ($init.ExitCode -eq 2) {
        Write-Host 'note: this historical release has no settings initializer; existing preferences were preserved'
    } elseif ($init.ExitCode -ne 0) {
        throw 'CLAUDUCT_SETTINGS_CREATE_FAILED: initializer failed; the installed binary is available'
    } elseif (-not (Test-Path -LiteralPath $target -PathType Leaf)) {
        throw 'CLAUDUCT_SETTINGS_CREATE_FAILED: initializer did not publish settings.json'
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
        Select-Names $source
    } else {
        $staging = Join-Path ([IO.Path]::GetTempPath()) ('clauduct-install-' + [guid]::NewGuid().ToString('N'))
        New-Item -ItemType Directory -Path $staging -Force | Out-Null
        Get-Release $staging
        $source = $staging
    }

    Assert-Digests $source

    New-Item -ItemType Directory -Path $InstallRoot -Force | Out-Null
    Install-Set $source $InstallRoot
    Ensure-Settings
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
    if (Try-PublishEnvironmentChange) {
        Write-Host "PATH: added $InstallRoot to the user PATH. Open a new terminal."
    } else {
        Write-Host "PATH: added $InstallRoot to the user PATH, but could not broadcast the change. Sign out and back in."
    }
} else {
    Write-Host "PATH: $InstallRoot was already on it."
}

# Two commands because they prove different things, and the difference has confused a
# reader already: this launcher passes everything it does not own to the client, so
# `clauduct --version` is answered by Claude Code. It proves the launch path works.
# What this build calls itself is a question for clauduct --dev.
Write-Host "verify: clauduct --dev --version   (this build)"
Write-Host "        clauduct --version         (the client, through it)"
