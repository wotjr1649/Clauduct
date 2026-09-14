[CmdletBinding()]
param([string] $OutputDirectory)

$ErrorActionPreference = 'Stop'
if ($PSVersionTable.PSVersion.Major -lt 7) { throw 'POWERSHELL_7_REQUIRED' }
$taskRoot = (Resolve-Path -LiteralPath (Split-Path -Parent $PSScriptRoot)).ProviderPath
$git = (Get-Command git.exe -CommandType Application | Select-Object -First 1).Source
function Read-Git([string[]] $Arguments) {
    $result = @(& $git -C $taskRoot -c core.fsmonitor=false @Arguments)
    if ($LASTEXITCODE -ne 0) { throw 'RELEASE_GIT_FAILED' }
    return $result
}

$gitRoot = @(Read-Git -Arguments @('rev-parse', '--show-toplevel'))[0]
if ([IO.Path]::GetFullPath($gitRoot) -ne [IO.Path]::GetFullPath($taskRoot)) { throw 'RELEASE_NOT_CHECKOUT' }
$status = @(Read-Git -Arguments @('status', '--porcelain=v1', '--untracked-files=no'))
if ($status.Count -ne 0) { throw 'RELEASE_TRACKED_CHANGES' }
$commit = @(Read-Git -Arguments @('rev-parse', '--verify', 'HEAD'))[0]
if ($commit -notmatch '^[a-f0-9]{40}$') { throw 'RELEASE_COMMIT_INVALID' }
# Only this reviewed distribution surface. No profiles, status, prompts, audit
# transcripts, generated app-server schemas, VCS history, or untracked files.
$paths = @('clauduct.cmd', 'install.ps1', 'RELEASE.md', 'docs/installation.md', 'docs/claude-option-classification.md', 'docs/local-use-release-decision.md',
    'docs/release-completion-2026-09-14.md', 'docs/session-29-release-verdict.md', 'docs/tcp-shell-assessment-2026-09-14.md', 'src',
    'verification/manual-http-probe.mjs', 'verification/auth-store-selection.mjs', 'verification/test-manual-http-probe.mjs',
    'verification/verify-live.mjs', 'verification/verify-native-headless.ps1',
    'verification/fixtures/native-mcp.mjs', 'verification/build-release.ps1', 'verification/test-installer.ps1',
    'verification/development-fixture.mjs', 'verification/development-tasks.mjs', 'verification/development-source-policy.mjs', 'verification/development-source-files.mjs', 'verification/fixture-tool-policy.mjs',
    'verification/development-source-grammar.mjs', 'verification/registered-development-tasks.mjs', 'verification/development-artifacts.mjs', 'verification/development-change.mjs',
    'verification/fixtures/registered-budget-task.mjs',
    'verification/verification-ledger.mjs', 'verification/native-output.mjs',
    'verification/execution-reservation.mjs', 'verification/fixtures/reservation-worker.mjs', 'verification/fixtures/reservation-entry-probe.mjs',
    'verification/execution-account.mjs', 'verification/managed-development.mjs', 'verification/fixtures/account-worker.mjs',
    'verification/managed-ledger-account.mjs', 'verification/fixtures/legacy-ledger.mjs', 'verification/fixtures/legacy-ledger-worker.mjs',
    'verification/long-stage-policy.mjs', 'verification/managed-plan-entry.mjs', 'verification/fixtures/plan-worker.mjs',
    'verification/auth-owner-protocol.mjs', 'verification/verify-auth-owner-read.mjs',
    'verification/fixtures/auth-owner-public-worker.mjs',
    'verification/completion-relay-target.mjs',
    'verification/native-result-relay-entry.mjs', 'verification/verify-native-result-relay.ps1', 'verification/verify-result-relay-evidence.mjs',
    'verification/native-workflow-resume-entry.mjs', 'verification/verify-native-workflow-resume.ps1', 'verification/verify-workflow-resume-evidence.mjs',
    'verification/read-transport-progress.ps1',
    'verification/fixture-token-budget.mjs',
    'verification/guarded-headless-entry.mjs', 'verification/native-recovery-entry.mjs',
    'verification/native-development-entry.mjs',
    'verification/native-development-local-entry.mjs', 'verification/fixtures/development-responses.mjs',
    'verification/connection-fault.mjs', 'verification/dns-recovery-entry.mjs',
    'verification/stop-owned-native-tree.ps1', 'verification/unattended-recovery.mjs',
    'verification/verify-native-development.mjs', 'verification/verify-native-recovery.mjs',
    'verification/verify-retry-after-recovery.mjs',
    'verification/verify-native-service-recovery.mjs', 'verification/native-service-entry.mjs',
    'verification/native-service-live-entry.mjs',
    'verification/background-task-id.mjs', 'verification/native-task-entry.mjs',
    'verification/verify-native-task-isolation.mjs', 'verification/fixtures/native-task-worker.mjs',
    'verification/verify-native-permissions.mjs', 'verification/native-permission-entry.mjs', 'verification/fixtures/permission-worker.mjs',
    'verification/fixtures/service-faults.mjs',
    'verification/fixtures/development-mcp.mjs', 'verification/fixtures/development-oracle.mjs', 'verification/fixtures/development-window-oracle.mjs', 'verification/fixtures/development-integration-oracle.mjs', 'verification/fixtures/development-project-oracle.mjs', 'verification/fixtures/development-write-worker.mjs',
    'verification/fixtures/public-images.json',
    'verification/fixtures/ledger-worker.mjs',
    'verification/fixtures/native-recovery-mcp.mjs', 'verification/fixtures/process-tree.mjs',
    'verification/fixtures/recovery-worker.mjs', 'verification/fixtures/retry-recovery-worker.mjs')
$paths += @(Read-Git -Arguments @('ls-files', '--', 'poc/*.mjs'))
$tree = @(Read-Git -Arguments (@('ls-tree', '-r', $commit, '--') + $paths))
$files = @($tree | ForEach-Object {
    if ($_ -notmatch '^100644 blob [a-f0-9]{40}\t([a-zA-Z0-9_./-]+)$') { throw 'RELEASE_FILE_TYPE' }
    $path = $Matches[1]
    if ($path -match '(^|/)\.\.?(/|$)' -or $path.StartsWith('/')) { throw 'RELEASE_FILE_PATH' }
    $path
} | Sort-Object -Unique)
if ($files.Count -lt 20 -or $files.Count -gt 256 -or 'RELEASE.md' -notin $files) { throw 'RELEASE_FILES_INCOMPLETE' }

$temporaryRoot = Join-Path $taskRoot 'output'
foreach ($boundary in @($taskRoot, $temporaryRoot)) {
    if (Test-Path -LiteralPath $boundary) {
        $item = Get-Item -Force -LiteralPath $boundary
        if (-not $item.PSIsContainer -or ($item.Attributes -band [IO.FileAttributes]::ReparsePoint)) { throw 'RELEASE_PATH_BOUNDARY' }
    }
}
$releases = Join-Path $temporaryRoot 'releases'
if (-not (Test-Path -LiteralPath $releases)) { [void][IO.Directory]::CreateDirectory($releases) }
if ((Get-Item -LiteralPath $releases -Force).Attributes -band [IO.FileAttributes]::ReparsePoint) { throw 'RELEASE_PATH_BOUNDARY' }
$outputRoot = if ($OutputDirectory) { [IO.Path]::GetFullPath($OutputDirectory) } else { Join-Path $releases ($commit.Substring(0,12) + '-' + [Guid]::NewGuid().ToString('N')) }
if ([IO.Path]::GetDirectoryName($outputRoot) -ine $releases -or (Test-Path -LiteralPath $outputRoot)) { throw 'RELEASE_OUTPUT_BOUNDARY' }
[void](New-Item -ItemType Directory -Path $outputRoot)
$archivePath = Join-Path $outputRoot 'Clauduct-windows-x64.zip'
$null = Read-Git -Arguments (@('archive', '--format=zip', '--prefix=Clauduct/', ('--output=' + $archivePath), $commit, '--') + $files)

$archive = [IO.Compression.ZipFile]::OpenRead($archivePath)
try {
    $records = @()
    $seen = [Collections.Generic.HashSet[string]]::new([StringComparer]::Ordinal)
    $total = 0L
    foreach ($entry in $archive.Entries) {
        if ($entry.FullName.EndsWith('/')) { continue }
        if (-not $entry.FullName.StartsWith('Clauduct/', [StringComparison]::Ordinal)) { throw 'RELEASE_ARCHIVE_PATH' }
        $path = $entry.FullName.Substring(9)
        $total += $entry.Length
        if ($path -notin $files -or -not $seen.Add($path) -or $entry.Length -gt 4MB -or $total -gt 16MB) { throw 'RELEASE_ARCHIVE_CONTENT' }
        $stream = $entry.Open()
        try { $sha256 = [Convert]::ToHexString([Security.Cryptography.SHA256]::HashData($stream)).ToLowerInvariant() }
        finally { $stream.Dispose() }
        $records += [ordered]@{ path = $path; bytes = $entry.Length; sha256 = $sha256 }
    }
    if ($seen.Count -ne $files.Count) { throw 'RELEASE_ARCHIVE_MISSING_FILE' }
} finally { $archive.Dispose() }

$archiveHash = (Get-FileHash -LiteralPath $archivePath -Algorithm SHA256).Hash.ToLowerInvariant()
$manifest = [ordered]@{ format = 1; sourceCommit = $commit; archive = [IO.Path]::GetFileName($archivePath);
    sha256 = $archiveHash; fileCount = $records.Count; uncompressedBytes = $total; files = $records }
[IO.File]::WriteAllText((Join-Path $outputRoot 'manifest.json'), ($manifest | ConvertTo-Json -Depth 5) + "`n")
# Copy the installer from the committed archive, never an untracked workspace file.
$zip = [IO.Compression.ZipFile]::OpenRead($archivePath)
try {
    $entry = $zip.GetEntry('Clauduct/install.ps1')
    if (-not $entry) { throw 'RELEASE_INSTALLER_MISSING' }
    $installerStream = $entry.Open(); $file = [IO.File]::Open((Join-Path $outputRoot 'install.ps1'), [IO.FileMode]::CreateNew)
    try { $installerStream.CopyTo($file) } finally { $file.Dispose(); $installerStream.Dispose() }
} finally { $zip.Dispose() }
$sums = @('Clauduct-windows-x64.zip','manifest.json','install.ps1') | ForEach-Object {
    ((Get-FileHash -LiteralPath (Join-Path $outputRoot $_)).Hash.ToLowerInvariant() + '  ' + $_)
}
[IO.File]::WriteAllText((Join-Path $outputRoot 'SHA256SUMS'), ($sums -join "`n") + "`n")
[ordered]@{ passed = $true; sourceCommit = $commit; fileCount = $records.Count; uncompressedBytes = $total;
    sha256 = $archiveHash; archive = $archivePath; manifest = (Join-Path $outputRoot 'manifest.json');
    installer = (Join-Path $outputRoot 'install.ps1'); checksums = (Join-Path $outputRoot 'SHA256SUMS') } | ConvertTo-Json -Compress
