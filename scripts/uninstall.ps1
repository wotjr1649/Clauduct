[CmdletBinding()]
param(
    [string] $InstallRoot = (Join-Path $env:USERPROFILE '.local\bin'),
    [switch] $RemovePath,
    [switch] $Purge
)

$ErrorActionPreference = 'Stop'

# The same three the installer placed, plus what --update leaves behind: a running executable
# cannot be overwritten but can be renamed, so the updater moves the old one aside and tells
# the user to delete it once the process ends. This is that deletion.
$Names = @('clauduct.exe', 'clauduct-hook.exe', 'clauduct-dev.exe')

# Removing the entry because our files left would take everything else in the directory with
# it. claude.exe lives here on a default install.
function Remove-UserPathEntry([string] $Dir) {
    $key = [Microsoft.Win32.Registry]::CurrentUser.OpenSubKey('Environment', $true)
    try {
        if ($null -eq $key.GetValue('Path')) { return $false }
        $kind = $key.GetValueKind('Path')
        $raw  = [string] $key.GetValue('Path', '', [Microsoft.Win32.RegistryValueOptions]::DoNotExpandEnvironmentNames)
        $have = @($raw -split ';' | Where-Object { $_ })
        $want = $Dir.TrimEnd('\')
        $keep = @($have | Where-Object {
            $_.TrimEnd('\') -ne $want -and
            [Environment]::ExpandEnvironmentVariables($_).TrimEnd('\') -ne $want
        })
        if ($keep.Count -eq $have.Count) { return $false }
        $key.SetValue('Path', ($keep -join ';'), $kind)
        return $true
    } finally { if ($key) { $key.Dispose() } }
}

$InstallRoot = [IO.Path]::GetFullPath($InstallRoot)

$removed = 0
foreach ($name in ($Names + ($Names | ForEach-Object { "$_.old" }))) {
    $path = Join-Path $InstallRoot $name
    if (Test-Path -LiteralPath $path -PathType Leaf) {
        Remove-Item -LiteralPath $path -Force
        Write-Host "removed $path"
        $removed++
    }
}
if ($removed -eq 0) { Write-Host "nothing of this build was in $InstallRoot" }

# clauduct-node.cmd and clauduct-node-store are v1: a different product, and the way back from
# this one. Session state lives under CLAUDE_CONFIG_DIR and belongs to the client, which this
# build never writes to. Neither is ours to delete.
Write-Host "kept: clauduct-node.cmd, clauduct-node-store, and everything under CLAUDE_CONFIG_DIR"

$statusDir = Join-Path $env:TEMP 'clauduct'
$status = @()
if (Test-Path -LiteralPath $statusDir -PathType Container) {
    $status = @(Get-ChildItem -LiteralPath $statusDir -Filter 'status-*.json' -File -ErrorAction SilentlyContinue)
}
if ($Purge -and $status.Count -gt 0) {
    Remove-Item -LiteralPath $statusDir -Recurse -Force
    Write-Host "purged $($status.Count) session accounts from $statusDir"
} elseif ($status.Count -gt 0) {
    Write-Host "$($status.Count) session accounts left in $statusDir -- diagnostics, and the OS clears that directory. Pass -Purge to remove them now."
}

if (-not $RemovePath) {
    Write-Host "PATH: untouched. Pass -RemovePath to drop $InstallRoot when nothing else is using it."
} else {
    $others = @()
    if (Test-Path -LiteralPath $InstallRoot -PathType Container) {
        $others = @(Get-ChildItem -LiteralPath $InstallRoot -File -ErrorAction SilentlyContinue |
            Where-Object { $_.Extension -in '.exe', '.cmd', '.bat', '.ps1' })
    }
    if ($others.Count -gt 0) {
        Write-Host "PATH: kept -- UNINSTALL_PATH_SHARED. $InstallRoot still holds $($others.Count) executable(s), starting with $($others[0].Name)."
    } elseif (Remove-UserPathEntry $InstallRoot) {
        Write-Host "PATH: removed $InstallRoot from the user PATH. Open a new terminal."
    } else {
        Write-Host "PATH: $InstallRoot was not on it."
    }
}
