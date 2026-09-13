[CmdletBinding()]
param([switch] $Live, [ValidateSet('text', 'stream-json', 'read-edit', 'agent', 'completion', 'workflow', 'mcp', 'resume', 'failure-resume', 'image', 'webfetch', 'websearch', 'build', 'build-powershell', 'background', 'cancel-task')][string] $Case = 'text',
    [ValidateSet('astra', 'sol', 'terra', 'luna')][string] $Model = 'luna',
    [ValidateSet('low', 'medium', 'high', 'xhigh', 'max')][string] $Effort = 'low',
    [ValidateSet('png', 'jpeg', 'gif', 'webp')][string] $ImageFormat = 'png',
    [ValidateSet('foreground', 'fork', 'relay')][string] $CompletionMode = 'foreground',
    [switch] $StopAfterFirstCompletion,
    [switch] $FailFirstConnection,
    [ValidatePattern('(?-i)^(?:[a-f0-9]{32})?$')][string] $RunId = '',
    [ValidateRange(1, 120)][int] $TimeoutSeconds = 90,
    [ValidateRange(1, 256)][int] $RequestLimit = 16)

$ErrorActionPreference = 'Stop'
if (-not $Live) { throw 'LIVE_FLAG_REQUIRED' }
if ($StopAfterFirstCompletion -and $Case -ne 'image') { throw 'STOP_FIXTURE_REQUIRES_IMAGE_CASE' }
if ($FailFirstConnection -and $Case -ne 'text') { throw 'CONNECTION_FAULT_REQUIRES_TEXT_CASE' }
if ($FailFirstConnection -and $RequestLimit -lt 2) { throw 'CONNECTION_FAULT_REQUEST_LIMIT' }
if ($CompletionMode -ne 'foreground' -and $Case -ne 'completion') { throw 'COMPLETION_MODE_REQUIRES_COMPLETION_CASE' }
$completionFork = $Case -eq 'completion' -and $CompletionMode -eq 'fork'
$completionRelay = $Case -eq 'completion' -and $CompletionMode -eq 'relay'
$runClock = [Diagnostics.Stopwatch]::StartNew()
if ($PSVersionTable.PSVersion.Major -lt 7) { throw 'POWERSHELL_7_REQUIRED' }
$taskRoot = (Resolve-Path -LiteralPath (Split-Path -Parent $PSScriptRoot)).ProviderPath
. (Join-Path $taskRoot 'verification/read-transport-progress.ps1')
$temporaryRoot = Join-Path $taskRoot '.tmp'
foreach ($boundary in @($taskRoot, $temporaryRoot)) {
    if ((Test-Path -LiteralPath $boundary) -and ((Get-Item -Force -LiteralPath $boundary).Attributes -band [IO.FileAttributes]::ReparsePoint)) {
        throw 'VERIFICATION_ROOT_REPARSE_POINT'
    }
}
$fixtureRoot = Join-Path $temporaryRoot ('native-headless-' + $(if ($RunId) { $RunId } else { [Guid]::NewGuid().ToString('N') }))
if (Test-Path -LiteralPath $fixtureRoot) { throw 'VERIFICATION_RUN_EXISTS' }
$profileRoot = Join-Path $fixtureRoot 'config'
$workingRoot = Join-Path $fixtureRoot 'work'
$tempRoot = Join-Path $fixtureRoot 'temp'
foreach ($directory in @($profileRoot, $workingRoot, $tempRoot)) { [void](New-Item -ItemType Directory -Path $directory) }
$marker = 'CLAUDUCT_NATIVE_HEADLESS_OK'
$prompt = 'Reply with exactly CLAUDUCT_NATIVE_HEADLESS_OK. Do not use tools.'
$nativeTools = ''
$turnLimit = '1'
$extraArgs = @('--no-session-persistence')
if ($Case -eq 'stream-json') { $extraArgs += @('--verbose', '--include-partial-messages') }
$expectedFile = 'export function add(a, b) { return a + b; }'
if ($Case -eq 'image') {
    $imageDataPath = Join-Path $taskRoot 'verification/fixtures/public-images.json'
    $imageDataInfo = Get-Item -Force -LiteralPath $imageDataPath
    if ($imageDataInfo.Length -gt 16384 -or ($imageDataInfo.Attributes -band [IO.FileAttributes]::ReparsePoint)) { throw 'IMAGE_FIXTURE_BOUNDARY' }
    $imageData = ConvertFrom-Json -InputObject ([IO.File]::ReadAllText($imageDataPath)) -AsHashtable
    $imageBase64 = $imageData[$ImageFormat]
    if ($imageData.version -ne 1 -or $imageData.width -ne 32 -or $imageData.height -ne 32 -or $imageData.color -ne 'RED' -or
        $imageBase64 -isnot [string] -or $imageBase64.Length -gt 8192 -or $imageBase64 -notmatch '^[A-Za-z0-9+/]+={0,2}$') { throw 'IMAGE_FIXTURE_DATA' }
    $imageFileName = 'square.' + $(if ($ImageFormat -eq 'jpeg') { 'jpg' } else { $ImageFormat })
    $imagePath = Join-Path $workingRoot $imageFileName
    [IO.File]::WriteAllBytes($imagePath, [Convert]::FromBase64String($imageBase64))
    $marker = 'RED'
    $prompt = "Read $imageFileName exactly once with Read. Look at the image and reply only with its dominant color name in uppercase."
    $nativeTools = 'Read'
}
if ($Case -eq 'webfetch') {
    $marker = 'Example Domain'
    $prompt = 'Use WebFetch exactly once on https://example.com to extract its page title. Reply with only that title, no formatting. Do not access another URL.'
    $nativeTools = 'WebFetch'
}
if ($Case -eq 'websearch') {
    $marker = 'CLAUDUCT_NATIVE_SEARCH_OK'
    $prompt = 'Use WebSearch exactly once for the public query Node.js documentation, with allowed_domains containing only nodejs.org. After receiving at least one result from nodejs.org, reply with exactly CLAUDUCT_NATIVE_SEARCH_OK.'
    $nativeTools = 'WebSearch'
}
if ($Case -in @('build', 'build-powershell')) {
    [IO.File]::WriteAllText((Join-Path $workingRoot 'verify.mjs'), "import assert from 'node:assert/strict'; assert.equal(2 + 3, 5); process.stdout.write('CLAUDUCT_NATIVE_BUILD_OK');")
    $marker = 'CLAUDUCT_NATIVE_BUILD_OK'
    $nativeTools = if ($Case -eq 'build') { 'Bash' } else { 'PowerShell' }
    $prompt = "Run exactly node verify.mjs once with $nativeTools, in the foreground. Do not run any other command. After its assertion succeeds, reply with exactly CLAUDUCT_NATIVE_BUILD_OK."
}
if ($Case -in @('background', 'cancel-task')) {
    $workerDelay = if ($Case -eq 'background') { 1000 } else { 15000 }
    [IO.File]::WriteAllText((Join-Path $workingRoot 'worker.mjs'), "import {appendFileSync} from 'node:fs'; appendFileSync('worker-events.jsonl', JSON.stringify({event:'started',pid:process.pid})+'\n'); setTimeout(()=>{appendFileSync('worker-events.jsonl', JSON.stringify({event:'finished'})+'\n'); process.stdout.write('CLAUDUCT_BACKGROUND_DONE');}, $workerDelay);")
    if ($Case -eq 'background') {
        $marker = 'CLAUDUCT_BACKGROUND_DONE'
        $nativeTools = 'Bash,TaskOutput'
        $prompt = 'Run exactly node worker.mjs once using Bash with run_in_background true. Use TaskOutput exactly once to wait for that task ID and read its result. After it succeeds, reply with exactly CLAUDUCT_BACKGROUND_DONE. Do not run any other command.'
    } else {
        $marker = 'CLAUDUCT_CANCELLED_OK'
        $nativeTools = 'Bash,TaskStop'
        $prompt = 'Run exactly node worker.mjs once using Bash with run_in_background true. Immediately use TaskStop exactly once on the returned task ID. After cancellation succeeds, reply with exactly CLAUDUCT_CANCELLED_OK. Do not run any other command.'
    }
}
if ($Case -in @('image', 'webfetch', 'websearch', 'build', 'build-powershell', 'background', 'cancel-task')) {
    $turnLimit = '5'
    $extraArgs = @() # Retain only this public fixture's native tool-call evidence.
}
if ($Case -eq 'read-edit') {
    [IO.File]::WriteAllText((Join-Path $workingRoot 'math.mjs'), 'export function add(a, b) { return a - b; }' + "`n")
    $marker = 'CLAUDUCT_NATIVE_EDIT_OK'
    $prompt = 'Read math.mjs. Change only the minus in add to plus so it adds a and b. Use Read and Edit only on math.mjs. Reply with exactly CLAUDUCT_NATIVE_EDIT_OK after editing.'
    $nativeTools = 'Read,Edit'
    $turnLimit = '5'
}
if ($Case -eq 'agent') {
    $marker = 'CLAUDUCT_NATIVE_AGENT_OK'
    $prompt = 'Use Agent exactly once with subagent_type clauduct-probe-inherit, without a model argument, and run_in_background false. Ask it to follow its read-only probe instructions. Wait for its MODEL-PROBE-COMPLETED result, then reply with exactly CLAUDUCT_NATIVE_AGENT_OK.'
    $nativeTools = 'Agent,TaskOutput,Read'
    $turnLimit = '6'
    $extraArgs = @('--verify-agent-models')
}
if ($Case -eq 'workflow') {
    $workflowScript = "export const meta = { name: 'clauduct-check', description: 'Public arithmetic check' }; return await agent('Compute 2 + 3. Return the sum using StructuredOutput. Do not use other tools.', { label: 'one', model: '$Model', schema: { type: 'object', properties: { sum: { type: 'number' } }, required: ['sum'], additionalProperties: false } });"
    $marker = 'CLAUDUCT_NATIVE_WORKFLOW_OK'
    $prompt = "Call Workflow exactly once with this exact inline script, without a scriptPath or a saved workflow name: $workflowScript Use TaskOutput exactly once to wait for the returned task ID. After its structured result has sum=5, reply with exactly CLAUDUCT_NATIVE_WORKFLOW_OK. Do not launch another workflow or agent."
    $nativeTools = 'Workflow,TaskOutput'
    $turnLimit = '6'
    $extraArgs = @()
}
if ($Case -eq 'completion') {
    $childPrompts = @('PUBLIC_FIRST: Follow your read-only probe instructions and return MODEL-PROBE-COMPLETED.', 'PUBLIC_SECOND: Follow your read-only probe instructions and return MODEL-PROBE-COMPLETED.')
    $childExecution = if ($completionFork) { 'omit run_in_background because this session uses native fork mode' } else { 'run_in_background true' }
    $parentPrompt = 'Launch exactly two Agent calls with subagent_type clauduct-probe-inherit, no model argument, ' + $childExecution + '. Use these exact prompts, one per child: ' + ($childPrompts | ConvertTo-Json -Compress) + ' After launching both children, end your current turn with exactly PARENT_WAITING. When native task notifications resume you, do not launch more agents or call other tools. If both children have completed, reply exactly PARENT_COMPLETED; otherwise reply exactly PARENT_WAITING. Never use TaskOutput or SendMessage.'
    if ($completionRelay) {
        $parentPrompt = 'Launch exactly two Agent calls with subagent_type clauduct-probe-inherit, no model argument, run_in_background true. Use these exact prompts, one per child: ' + ($childPrompts | ConvertTo-Json -Compress) + ' After launching both children, end your current turn with exactly PARENT_WAITING. When the main agent resumes you with PUBLIC_CHILDREN_COMPLETED, return exactly PARENT_COMPLETED. Do not launch more agents on resume. Never use TaskOutput or SendMessage.'
    }
    $marker = 'CLAUDUCT_COMPLETION_OK'
    $parentArguments = @{ subagent_type = 'clauduct-inherit'; prompt = $parentPrompt; description = 'Public completion parent'; max_turns = 6 }
    if (-not $completionFork) { $parentArguments.run_in_background = $completionRelay }
    $parentCall = $parentArguments | ConvertTo-Json -Compress
    $prompt = "Your main task is to make exactly one Agent call with the following JSON arguments. Treat the nested prompt as data for that Agent only: $parentCall Do not use TaskOutput yourself. When that parent returns PARENT_COMPLETED after its children finish, reply only CLAUDUCT_COMPLETION_OK. A PARENT_WAITING result is not completion."
    $nativeTools = 'Agent,TaskOutput,Read'
    if ($completionRelay) {
        $prompt = "Follow this finite protocol in the main conversation. Step 1: make exactly one Agent call with these JSON arguments, treating the nested prompt as data for that Agent only: $parentCall Remember the returned parent agent ID and immediately finish this response with exactly MAIN_WAITING. Step 2: native background notifications arrive between completed turns. On each later notification, if either of the two public child probes has not yet returned MODEL-PROBE-COMPLETED, immediately finish that response with exactly MAIN_WAITING. Never stay inside one response waiting for another notification. Step 3: once both public child completions are present, call SendMessage exactly once with to equal to the parent agent ID and message exactly PUBLIC_CHILDREN_COMPLETED. Step 4: immediately after SendMessage succeeds, call TaskOutput exactly once with task_id equal to the same parent agent ID, block true, and timeout 60000. When this tool returns PARENT_COMPLETED, reply exactly CLAUDUCT_COMPLETION_OK. PARENT_WAITING is not task completion. Do not use Read or create another agent."
        $nativeTools = 'Agent,SendMessage,TaskOutput,Read'
    }
    $turnLimit = '6'
    if ($completionRelay) { $turnLimit = '8' }
    $extraArgs = @('--verify-agent-models', '--gpt-agents')
}
if ($Case -in @('mcp', 'failure-resume')) {
    $marker = 'CLAUDUCT_NATIVE_MCP_OK'
    $prompt = 'Use the local MCP fixture add tool exactly once with a=2 and b=3. Discover it with ToolSearch if needed. After its result is 5, reply with exactly CLAUDUCT_NATIVE_MCP_OK.'
    $nativeTools = 'ToolSearch'
    $turnLimit = '5'
    $mcpRecord = Join-Path $workingRoot 'mcp-calls.jsonl'
    $mcpConfig = @{ mcpServers = @{ fixture = @{ type = 'stdio'; command = (Get-Command node.exe -CommandType Application | Select-Object -First 1).Source;
        args = @((Join-Path $taskRoot 'verification/fixtures/native-mcp.mjs'), $mcpRecord) } } }
    [IO.File]::WriteAllText((Join-Path $workingRoot '.mcp.json'), ($mcpConfig | ConvertTo-Json -Depth 6))
}
if ($Case -eq 'resume') {
    $marker = 'CLAUDUCT_RESUME_SEEDED'
    $prompt = 'Remember this public verification code for my next message: PUBLIC-RESUME-4729. Reply with exactly CLAUDUCT_RESUME_SEEDED.'
}
if ($Case -in @('resume', 'failure-resume')) {
    $resumeId = [Guid]::NewGuid().ToString()
    $extraArgs = @('--session-id', $resumeId)
}
$info = [Diagnostics.ProcessStartInfo]::new()
$info.FileName = (Get-Command node.exe -CommandType Application | Select-Object -First 1).Source
$info.WorkingDirectory = $workingRoot
$info.UseShellExecute = $false
$info.CreateNoWindow = $true
$info.RedirectStandardInput = $true
$info.RedirectStandardOutput = $true
$info.RedirectStandardError = $true
$info.Environment.Clear()
foreach ($name in @('SystemRoot', 'WINDIR', 'SystemDrive', 'ComSpec', 'PATH', 'PATHEXT', 'USERPROFILE',
    'HOMEDRIVE', 'HOMEPATH', 'APPDATA', 'LOCALAPPDATA', 'ProgramData', 'ProgramFiles', 'ProgramFiles(x86)',
    'OS', 'PROCESSOR_ARCHITECTURE', 'CODEX_HOME', 'NODE_DEBUG', 'NODE_OPTIONS', 'NODE_USE_ENV_PROXY', 'NODE_TLS_REJECT_UNAUTHORIZED',
    'CLAUDE_CODE_DISABLE_WORKFLOWS')) {
    $value = [Environment]::GetEnvironmentVariable($name)
    if ($null -ne $value) { $info.Environment[$name] = $value }
}
$info.Environment['CLAUDE_CONFIG_DIR'] = $profileRoot
$info.Environment['CLAUDE_CODE_TMPDIR'] = $tempRoot
$info.Environment['TEMP'] = $tempRoot
$info.Environment['TMP'] = $tempRoot
$info.Environment['CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC'] = '1'
$info.Environment['CLAUDE_CODE_POWERSHELL_RESPECT_EXECUTION_POLICY'] = '1'
if ($completionFork) { $info.Environment['CLAUDE_CODE_FORK_SUBAGENT'] = '1' }
if ($Case -eq 'build-powershell') { $info.Environment['CLAUDE_CODE_USE_POWERSHELL_TOOL'] = '1' }
$entryArgs = @((Join-Path $taskRoot 'src/clauduct.mjs'))
$guardedFixture = $Case -in @('text', 'stream-json', 'agent', 'completion', 'workflow', 'image', 'webfetch', 'websearch') -or $FailFirstConnection
if ($guardedFixture) {
    $policyPath = Join-Path $fixtureRoot 'tool-policy.json'
    $policy = @{ version = 1; kind = $(if ($Case -in @('text', 'stream-json') -or $FailFirstConnection) { 'none' } else { $Case }); workingRoot = $workingRoot }
    if ($Case -in @('agent', 'completion')) { $policy.readPath = Join-Path $taskRoot 'src/models.mjs' }
    elseif ($Case -eq 'image') { $policy.readPath = $imagePath }
    elseif ($Case -eq 'workflow') { $policy.workflowScript = $workflowScript }
    if ($Case -eq 'completion') { $policy.parentPrompt = $parentPrompt; $policy.childPrompts = $childPrompts; $policy.completionMode = $CompletionMode }
    [IO.File]::WriteAllText($policyPath, ($policy | ConvertTo-Json -Depth 4))
    $entryArgs = @((Join-Path $taskRoot 'verification/guarded-headless-entry.mjs'), $policyPath)
    if ($FailFirstConnection) { $entryArgs[0] = Join-Path $taskRoot 'verification/dns-recovery-entry.mjs' }
}
$phases = if ($Case -eq 'failure-resume') { @('seed', 'failure', 'resume') } elseif ($Case -eq 'resume') { @('seed', 'resume') } else { @('single') }
foreach ($phase in $phases) {
if ($phase -eq 'failure') {
    $marker = 'CLAUDUCT_INJECTED_FAILURE'
    $prompt = 'Reply with exactly CLAUDUCT_INJECTED_FAILURE. Do not call any tools.'
    $nativeTools = ''
    $turnLimit = '1'
    $extraArgs = @('--resume', $resumeId, '--verify-fallback', 'blocked', '--disallowedTools', 'mcp__fixture__add')
}
if ($phase -eq 'resume') {
    $marker = 'PUBLIC-RESUME-4729'
    $prompt = 'What public verification code did I ask you to remember? Reply with exactly that code, and nothing else.'
    $extraArgs = @('--resume', $resumeId)
    if ($Case -eq 'failure-resume') {
        $marker = 'CLAUDUCT_NATIVE_RECOVERED'
        $prompt = 'Read mcp-calls.jsonl. If it contains exactly one entry with a=2 and b=3, reply with exactly CLAUDUCT_NATIVE_RECOVERED. Do not repeat the MCP call or modify any file.'
        $nativeTools = 'Read'
        $turnLimit = '4'
        $extraArgs += @('--disallowedTools', 'mcp__fixture__add')
    }
}
$info.ArgumentList.Clear()
$allowedTools = if ($nativeTools -eq 'ToolSearch') { 'ToolSearch,mcp__fixture__add' } elseif ($Case -eq 'webfetch') { 'WebFetch(domain:example.com)' } elseif ($Case -in @('build', 'build-powershell')) { "$nativeTools(node verify.mjs)" } elseif ($Case -eq 'background') { 'Bash(node worker.mjs),TaskOutput' } elseif ($Case -eq 'cancel-task') { 'Bash(node worker.mjs),TaskStop' } else { $nativeTools }
foreach ($argument in $entryArgs + @('--model', $Model, '--effort', $Effort, '--verify-model-route',
    '--verify-request-limit', [string]$RequestLimit,
    '-p', '--output-format', $(if ($Case -eq 'stream-json') { 'stream-json' } else { 'json' }), '--tools', $nativeTools, '--allowedTools', $allowedTools,
    '--max-turns', $turnLimit) + $extraArgs + @('--', $prompt)) {
    $info.ArgumentList.Add($argument)
}
$process = [Diagnostics.Process]::Start($info)
$executionClock = [Diagnostics.Stopwatch]::StartNew()
try {
    $process.StandardInput.Close()
    $stdoutTask = $process.StandardOutput.ReadToEndAsync()
    $stderrTask = $process.StandardError.ReadToEndAsync()
    $injectedStop = $false
    $injectedStopAfterCompletions = $null
    $stopControlFailure = $false
    $stopJournalBoundaryRejected = $false
    if ($StopAfterFirstCompletion) {
        $journalPath = Join-Path $fixtureRoot 'tool-usage.jsonl'
        while ($executionClock.ElapsedMilliseconds -lt $TimeoutSeconds * 1000 -and -not $process.WaitForExit(25)) {
            if (-not (Test-Path -LiteralPath $journalPath -PathType Leaf)) { continue }
            try {
                $journalInfo = Get-Item -Force -LiteralPath $journalPath
                if ($journalInfo.Length -gt 262144 -or ($journalInfo.Attributes -band [IO.FileAttributes]::ReparsePoint)) {
                    $stopJournalBoundaryRejected = $true; throw 'STOP_JOURNAL_BOUNDARY'
                }
                $stream = [IO.File]::Open($journalPath, [IO.FileMode]::Open, [IO.FileAccess]::Read, [IO.FileShare]::ReadWrite)
                try {
                    $textReader = [IO.StreamReader]::new($stream)
                    try { $journalText = $textReader.ReadToEnd() } finally { $textReader.Dispose() }
                } finally { $stream.Dispose() }
            } catch { $stopControlFailure = $true; break }
            $boundary = $journalText.LastIndexOf("`n")
            if ($boundary -lt 0) { continue }
            try { $last = ($journalText.Substring(0, $boundary) -split "`n")[-1] | ConvertFrom-Json -AsHashtable } catch { continue }
            if ($last.version -in @(1,2) -and $last.kind -in @('attempt','usage') -and $last.completions -ge 1 -and
                $last.requestAttempts -ge 1 -and $last.requestAttempts -le $RequestLimit) {
                $injectedStop = $true; $injectedStopAfterCompletions = $last.completions
                $process.Kill($true); [void]$process.WaitForExit(10000)
                break
            }
        }
        $timedOut = -not $process.HasExited -and -not $stopControlFailure
    } else { $timedOut = -not $process.WaitForExit($TimeoutSeconds * 1000) }
    if (($timedOut -or $stopControlFailure) -and -not $injectedStop) { $process.Kill($true); [void]$process.WaitForExit(10000) }
    $nativeStopConfirmed = $process.HasExited
    $nativeExitCode = if ($nativeStopConfirmed) { $process.ExitCode } else { $null }
    $outputSettled = $false; $outputCollectionFailed = $false
    try { $outputSettled = $stdoutTask.Wait(10000) -and $stderrTask.Wait(10000) }
    catch { $outputCollectionFailed = $true }
    $stdout = if ($stdoutTask.IsCompletedSuccessfully) { $stdoutTask.GetAwaiter().GetResult() } else { '' }
    $stderr = if ($stderrTask.IsCompletedSuccessfully) { $stderrTask.GetAwaiter().GetResult() } else { '' }
    $result = $null
    $status = $null
    $streamVerified = $null
    try {
        if ($stdout.Length -le 1MB) {
            if ($Case -eq 'stream-json') {
                $streamRecords = @($stdout -split '\r?\n' | Where-Object { $_.Length -gt 0 } | ForEach-Object { ConvertFrom-Json -InputObject $_ -AsHashtable })
                $result = $streamRecords[-1]
                $streamText = (@($streamRecords | Where-Object { $_.type -eq 'stream_event' -and $_.event.type -eq 'content_block_delta' -and $_.event.delta.type -eq 'text_delta' } | ForEach-Object { $_.event.delta.text }) -join '')
                $streamVerified = $result.type -eq 'result' -and @($streamRecords | Where-Object { $_.type -eq 'result' }).Count -eq 1 -and $streamText.Trim() -eq $marker
            } else { $result = ConvertFrom-Json -InputObject $stdout -AsHashtable }
        }
    } catch { if ($Case -eq 'stream-json') { $streamVerified = $false } }
    $statusLine = @($stderr -split '\r?\n' | Where-Object { $_.StartsWith('CLAUDUCT_REQUEST_STATUS ') } | Select-Object -Last 1)
    try { if ($statusLine.Count -eq 1 -and $statusLine[0].Length -le 256KB) { $status = ConvertFrom-Json -InputObject $statusLine[0].Substring(24) -AsHashtable } } catch { }
    $exactReply = $result -is [Collections.IDictionary] -and $result.result -is [string] -and $result.result.Trim() -eq $marker
    $clean = $status -is [Collections.IDictionary] -and $status.cleanup -is [Collections.IDictionary] -and $status.cleanup.Count -eq 9 -and @($status.cleanup.Values | Where-Object { $_ -ne $true }).Count -eq 0
    $fixtureMatched = $null
    $agentRouted = $null
    $mcpVerified = $null
    $sameSession = $null
    $featureVerified = $null
    $nativeImageMediaType = $null
    $upstreamImageMediaType = $null
    $completionEvidence = $null
    $workerStopped = $null
    $expectedModel = @{ astra = 'gpt-6-astra'; sol = 'gpt-5.6-sol'; terra = 'gpt-5.6-terra'; luna = 'gpt-5.6-luna' }[$Model]
    $modelMatched = @($status.recentRequests | Where-Object { $_.subagent -ne $true -and $_.model -eq $expectedModel -and $_.effort -eq $Effort }).Count -gt 0
    if ($Case -eq 'read-edit') { $fixtureMatched = [IO.File]::ReadAllText((Join-Path $workingRoot 'math.mjs')).Trim() -eq $expectedFile }
    if ($Case -eq 'agent') { $agentRouted = @($status.recentRequests | Where-Object { $_.subagent -eq $true -and $_.selectionSource -eq 'definition-inherit' -and $_.model -eq $expectedModel -and $_.effort -eq $Effort }).Count -gt 0 }
    if ($Case -eq 'workflow') {
        $agentRouted = @($status.recentRequests | Where-Object { $_.subagent -eq $true -and $_.selectionSource -eq 'workflow-result' -and $_.role -eq 'workflow-subagent' -and $_.model -eq $expectedModel -and $_.effort -eq $Effort -and $_.success -eq $true }).Count -gt 0
    }
    if ($Case -in @('mcp', 'failure-resume')) {
        $mcpVerified = $false
        if (Test-Path -LiteralPath $mcpRecord) {
            $mcpCalls = @(Get-Content -LiteralPath $mcpRecord | ForEach-Object { ConvertFrom-Json -InputObject $_ -AsHashtable })
            $mcpVerified = $mcpCalls.Count -eq 1 -and $mcpCalls[0].a -eq 2 -and $mcpCalls[0].b -eq 3
        }
    }
    if ($Case -in @('resume', 'failure-resume')) { $sameSession = $result.session_id -eq $resumeId }
    if ($Case -in @('image', 'webfetch', 'websearch', 'build', 'build-powershell', 'background', 'cancel-task', 'workflow', 'completion')) {
        $featureVerified = $false
        if ($result.session_id -match '^[a-f0-9-]{36}$') {
            $transcripts = @(Get-ChildItem -LiteralPath (Join-Path $profileRoot 'projects') -File -Recurse -Filter ($result.session_id + '.jsonl'))
            if ($transcripts.Count -eq 1 -and $transcripts[0].Length -le 16MB) {
                $calls = @()
                $results = @()
                foreach ($line in [IO.File]::ReadLines($transcripts[0].FullName)) {
                    if ($line.Length -gt 4MB) { throw 'VERIFICATION_TRANSCRIPT_TOO_LARGE' }
                    $record = ConvertFrom-Json -InputObject $line -AsHashtable
                    foreach ($block in $record.message.content) {
                        if ($record.type -eq 'assistant' -and $block.type -eq 'tool_use') { $calls += $block }
                        if ($record.type -eq 'user' -and $block.type -eq 'tool_result') { $results += $block }
                    }
                }
                $toolObserved = $calls.Count -eq 1 -and $calls[0].name -eq $nativeTools -and @($results | Where-Object { $_.tool_use_id -eq $calls[0].id -and $_.is_error -ne $true }).Count -eq 1
                if ($Case -eq 'image') {
                    $imageBlocks = @($results[0].content | Where-Object { $_.type -eq 'image' })
                    $nativeImageMediaType = if ($imageBlocks.Count -eq 1 -and $imageBlocks[0].source.media_type -in @('image/png','image/jpeg','image/gif','image/webp')) { $imageBlocks[0].source.media_type } else { $null }
                    $featureVerified = $toolObserved -and [IO.Path]::GetFullPath($calls[0].input.file_path, $workingRoot) -eq $imagePath -and $imageBlocks.Count -eq 1 -and $null -ne $nativeImageMediaType
                } elseif ($Case -eq 'webfetch') {
                    $featureVerified = $toolObserved -and $calls[0].input.url -in @('https://example.com', 'https://example.com/')
                } elseif ($Case -in @('build', 'build-powershell')) {
                    $featureVerified = $toolObserved -and $calls[0].input.command -eq 'node verify.mjs' -and $calls[0].input.run_in_background -ne $true -and ($results[0].content | ConvertTo-Json -Depth 10 -Compress).Contains('CLAUDUCT_NATIVE_BUILD_OK')
                } elseif ($Case -in @('background', 'cancel-task')) {
                    $workerEvents = @()
                    if (Test-Path -LiteralPath (Join-Path $workingRoot 'worker-events.jsonl')) {
                        $workerEvents = @(Get-Content -LiteralPath (Join-Path $workingRoot 'worker-events.jsonl') | ForEach-Object { ConvertFrom-Json -InputObject $_ -AsHashtable })
                    }
                    $taskTool = if ($Case -eq 'background') { 'TaskOutput' } else { 'TaskStop' }
                    $featureVerified = $calls.Count -eq 2 -and $calls[0].name -eq 'Bash' -and $calls[0].input.command -eq 'node worker.mjs' -and $calls[0].input.run_in_background -eq $true -and $calls[1].name -eq $taskTool -and $results.Count -eq 2 -and @($results | Where-Object { $_.is_error -eq $true }).Count -eq 0 -and $workerEvents[0].event -eq 'started'
                    if ($Case -eq 'background') {
                        $featureVerified = $featureVerified -and $workerEvents.Count -eq 2 -and $workerEvents[1].event -eq 'finished'
                    } else {
                        $workerStopped = $false
                        if ($workerEvents.Count -eq 1 -and $workerEvents[0].pid -is [long] -and $workerEvents[0].pid -gt 0) {
                            try {
                                $worker = [Diagnostics.Process]::GetProcessById([int]$workerEvents[0].pid)
                                try { $workerStopped = $worker.HasExited } finally { $worker.Dispose() }
                            } catch [ArgumentException] { $workerStopped = $true }
                        }
                        $featureVerified = $featureVerified -and $workerStopped
                    }
                } elseif ($Case -eq 'completion') {
                    $subagentRoot = Join-Path (Join-Path $transcripts[0].DirectoryName $result.session_id) 'subagents'
                    $metadata = @(if (Test-Path -LiteralPath $subagentRoot -PathType Container) { Get-ChildItem -LiteralPath $subagentRoot -File -Filter 'agent-*.meta.json' | Where-Object { $_.Length -le 16384 } | ForEach-Object {
                        $value = Get-Content -LiteralPath $_.FullName -Raw | ConvertFrom-Json -AsHashtable
                        if ($_.Name -match '^agent-([A-Za-z0-9_-]{1,200})\.meta\.json$') { $value.fixtureId = $Matches[1]; $value }
                    } })
                    $parents = @($metadata | Where-Object { $_.agentType -eq 'clauduct-inherit' })
                    $children = @($metadata | Where-Object { $_.agentType -eq 'clauduct-probe-inherit' })
                    $notifications = @(); $parentAgentCalls = @(); $parentCompleted = $false; $parentWaited = $false
                    if ($parents.Count -eq 1) {
                        $parentTranscript = Join-Path $subagentRoot ('agent-' + $parents[0].fixtureId + '.jsonl')
                        if ((Get-Item -LiteralPath $parentTranscript).Length -le 4MB) {
                            foreach ($line in [IO.File]::ReadLines($parentTranscript)) {
                                if ($line.Length -gt 1MB) { throw 'VERIFICATION_TRANSCRIPT_TOO_LARGE' }
                                $row = $line | ConvertFrom-Json -AsHashtable
                                if ($row.type -eq 'user' -and $row.isMeta -eq $true -and $row.origin.kind -eq 'task-notification' -and $row.message.content -is [string] -and
                                    $row.message.content -match '<task-id>([A-Za-z0-9_-]{1,200})</task-id>' -and $row.message.content.Contains('<status>completed</status>')) { $notifications += $Matches[1] }
                                foreach ($block in $row.message.content) {
                                    if ($row.type -eq 'assistant' -and $block.type -eq 'tool_use' -and $block.name -eq 'Agent') { $parentAgentCalls += $block }
                                    if ($row.type -eq 'assistant' -and $block.type -eq 'text' -and $block.text.Trim() -eq 'PARENT_COMPLETED') { $parentCompleted = $true }
                                    if ($row.type -eq 'assistant' -and $block.type -eq 'text' -and $block.text.Trim() -eq 'PARENT_WAITING' -and $notifications.Count -eq 0) { $parentWaited = $true }
                                }
                            }
                        }
                    }
                    $resumeSource = if ($completionRelay) { 'verified-resume' } else { 'verified-completion-resume' }
                    $resumeRoutes = @($status.recentRequests | Where-Object { $_.selectionSource -eq $resumeSource -and $_.success -eq $true -and $_.model -eq $expectedModel -and $_.effort -eq $Effort })
                    $agentRefs = @($status.recentRequests | Where-Object { $_.subagent -eq $true -and $_.success -eq $true } | ForEach-Object { $_.agentRef } | Sort-Object -Unique)
                    $relayCalls = @($calls | Where-Object { $_.name -eq 'SendMessage' })
                    $completionDelivery = if ($completionRelay) {
                        $parents.Count -eq 1 -and $calls.Count -eq 3 -and $relayCalls.Count -eq 1 -and
                        $relayCalls[0].input.to -eq $parents[0].fixtureId -and $relayCalls[0].input.message -ceq 'PUBLIC_CHILDREN_COMPLETED' -and
                        @($calls | Where-Object { $_.name -eq 'TaskOutput' -and $_.input.task_id -eq $parents[0].fixtureId -and $_.input.block -eq $true -and $_.input.timeout -eq 60000 }).Count -eq 1
                    } else { @($children | Where-Object { $_.fixtureId -notin $notifications }).Count -eq 0 }
                    $completionEvidence = @{ parentCount = $parents.Count; childCount = $children.Count; parentAgentCalls = $parentAgentCalls.Count;
                        notifications = @($notifications | Sort-Object -Unique).Count; resumeRoutes = $resumeRoutes.Count; routedAgents = $agentRefs.Count; parentCompleted = $parentCompleted; parentWaited = $parentWaited;
                        relayCalls = $relayCalls.Count; resumeSource = $resumeSource }
                    $agentRouted = $resumeRoutes.Count -gt 0 -and $agentRefs.Count -eq 3
                    $featureVerified = $parents.Count -eq 1 -and $children.Count -eq 2 -and $parentAgentCalls.Count -eq 2 -and $parentCompleted -and $parentWaited -and
                        @($children | Where-Object { $_.parentAgentId -ne $parents[0].fixtureId }).Count -eq 0 -and
                        $completionDelivery -and
                        @($parentAgentCalls | Where-Object { $_.input.subagent_type -ne 'clauduct-probe-inherit' -or $_.input.prompt -cnotin $childPrompts -or $(if ($completionFork) { $_.input.ContainsKey('run_in_background') } else { $_.input.run_in_background -ne $true }) }).Count -eq 0 -and
                        @($calls | Where-Object { $_.name -eq 'Agent' -and $_.input.subagent_type -eq 'clauduct-inherit' -and $_.input.prompt -ceq $parentPrompt -and $(if ($completionFork) { -not $_.input.ContainsKey('run_in_background') } else { $_.input.run_in_background -eq $completionRelay }) }).Count -eq 1 -and
                        @($results | Where-Object { $_.is_error -eq $true }).Count -eq 0
                } elseif ($Case -eq 'workflow') {
                    $journals = @(Get-ChildItem -LiteralPath (Join-Path $transcripts[0].DirectoryName $result.session_id) -File -Recurse -Filter 'journal.jsonl')
                    $journal = @()
                    if ($journals.Count -eq 1 -and $journals[0].Length -le 1MB) {
                        $journal = @(Get-Content -LiteralPath $journals[0].FullName | ForEach-Object { ConvertFrom-Json -InputObject $_ -AsHashtable })
                    }
                    $completed = @($journal | Where-Object { $_.type -eq 'result' })
                    $featureVerified = $calls.Count -eq 2 -and $calls[0].name -eq 'Workflow' -and $calls[0].input.script -ceq $workflowScript -and $calls[1].name -eq 'TaskOutput' -and $results.Count -eq 2 -and @($results | Where-Object { $_.is_error -eq $true }).Count -eq 0 -and @($journal | Where-Object { $_.type -eq 'started' }).Count -eq 1 -and $completed.Count -eq 1 -and $completed[0].result.sum -eq 5
                } else {
                    $featureVerified = $toolObserved -and $status.lifetime.webSearchRequests -eq 1 -and $status.lifetime.webSearchCalls -eq 1 -and $status.lifetime.webSearchLinks -gt 0
                }
            }
        }
    }
    $failurePreserved = $null
    $outcomeMatched = $nativeExitCode -eq 0 -and $exactReply -and $result.is_error -eq $false -and $status.requestOutcome -eq 'all-succeeded'
    if ($phase -eq 'failure') {
        $failedRequests = @($status.failureHistory.records | Where-Object { $_.failureCategory -eq 'UPSTREAM_ERROR_EVENT' -and $_.firstDownstreamWriteMs -ne $null -and $_.attempts.Count -eq 1 })
        $failurePreserved = $nativeExitCode -ne 0 -and $result.is_error -eq $true -and $status.lifetime.injectedStreamErrors -eq 1 -and $status.lifetime.failed -eq 1 -and $status.lifetime.started -eq 1 -and $failedRequests.Count -eq 1
        $outcomeMatched = $failurePreserved
    }
    $passed = $nativeStopConfirmed -and $outputSettled -and -not $StopAfterFirstCompletion -and -not $timedOut -and $outcomeMatched -and $clean -and $modelMatched -and $fixtureMatched -ne $false -and $agentRouted -ne $false -and $mcpVerified -ne $false -and $sameSession -ne $false -and $featureVerified -ne $false -and $streamVerified -ne $false
    $fixtureUsage = $null
    $fixtureUsageSource = $null
    $fixtureJournal = $null
    $ledgerReaderStopped = $null
    $fixtureJournalState = 'not-requested'
    $transportProgressState = 'not-requested'; $transportProgress = $null
    if ($guardedFixture) {
        $transportProgressState = 'invalid'
        try {
            $progress = ConvertFrom-ClauductTransportProgress $stderr
            [IO.File]::WriteAllText((Join-Path $fixtureRoot ('transport-progress-' + $phase + '.json')), (ConvertTo-Json -InputObject $progress.records -Depth 6) + "`n")
            $transportProgress = $progress.summary
            $transportProgressState = 'valid'
        } catch { $transportProgressState = 'invalid' }
        $passed = $passed -and $transportProgressState -eq 'valid'
        $readerInfo = [Diagnostics.ProcessStartInfo]::new($info.FileName)
        $readerInfo.UseShellExecute = $false; $readerInfo.CreateNoWindow = $true
        $readerInfo.WorkingDirectory = $fixtureRoot
        $readerInfo.RedirectStandardOutput = $true; $readerInfo.RedirectStandardError = $true
        $readerInfo.Environment.Clear()
        $readerInfo.Environment['SystemRoot'] = [Environment]::GetFolderPath([Environment+SpecialFolder]::Windows)
        $readerInfo.Environment['TEMP'] = $tempRoot; $readerInfo.Environment['TMP'] = $tempRoot
        $readerInfo.ArgumentList.Add((Join-Path $taskRoot 'verification/verification-ledger.mjs'))
        $readerInfo.ArgumentList.Add((Join-Path $fixtureRoot 'tool-usage.jsonl'))
        $reader = $null
        $ledgerReaderStopped = $false
        $fixtureJournalState = 'unavailable'
        try {
            if ($stopJournalBoundaryRejected) { throw 'STOP_JOURNAL_BOUNDARY' }
            $reader = [Diagnostics.Process]::Start($readerInfo)
            $readerOut = $reader.StandardOutput.ReadToEndAsync(); $readerErr = $reader.StandardError.ReadToEndAsync()
            if (-not $reader.WaitForExit(5000)) { $reader.Kill($true); [void]$reader.WaitForExit(5000) }
            $ledgerReaderStopped = $reader.HasExited
            if ($reader.HasExited -and $reader.ExitCode -eq 0 -and $readerOut.Wait(1000) -and $readerErr.Wait(1000)) {
                $readerText = $readerOut.GetAwaiter().GetResult()
                if ($readerText.Length -lt 2048) {
                    try {
                        if ($readerErr.GetAwaiter().GetResult().Length -ne 0) { throw 'VERIFICATION_LEDGER_READER_STDERR' }
                        $accounting = ConvertFrom-ClauductFixtureAccounting -UsageText $stderr -JournalText $readerText -RequestLimit $RequestLimit
                        $fixtureUsage = $accounting.usage; $fixtureUsageSource = $accounting.source
                        $fixtureJournal = $accounting.journal; $fixtureJournalState = 'valid'
                    } catch { }
                }
            }
        } catch {
            $fixtureJournalState = 'unavailable'
            $ledgerReaderStopped = $null -eq $reader -or $reader.HasExited
        } finally { if ($null -ne $reader) { $reader.Dispose() } }
        $passed = $passed -and $null -ne $fixtureUsage -and $fixtureUsage.completions -gt 0 -and
            $fixtureUsage.unobservedCompletions -eq 0 -and $fixtureJournal.version -eq 2 -and
            $fixtureUsage.inputTokens -le 131072 -and $fixtureUsage.outputTokens -le 32768 -and
            $fixtureUsage.requestAttempts -gt 0 -and $fixtureUsage.requestAttempts -le $RequestLimit -and
            $ledgerReaderStopped -eq $true -and $null -ne $fixtureJournal -and $fixtureJournal.finalRecorded -and -not $fixtureJournal.truncatedTail -and $fixtureJournal.matchedFooter -ne $false
        if ($Case -eq 'image') {
            $upstreamImageMediaType = @{ 1 = 'image/png'; 2 = 'image/jpeg'; 4 = 'image/gif'; 8 = 'image/webp' }[[int]$fixtureUsage.imageFormatMask]
            $featureVerified = $featureVerified -and $null -ne $upstreamImageMediaType -and $upstreamImageMediaType -eq $nativeImageMediaType
            $passed = $passed -and $featureVerified
        }
    }
    $connectionFault = $null
    $connectionFaultVerified = -not $FailFirstConnection
    if ($FailFirstConnection) {
        try {
            $connectionFault = ConvertFrom-ClauductConnectionFault -Text $stderr -RequestLimit $RequestLimit -Attempts @($status.recentRequests | ForEach-Object { $_.attempts })
            $connectionFaultVerified = $true
        } catch { $connectionFaultVerified = $false }
        $passed = $passed -and $connectionFaultVerified
    }
    $executionClock.Stop()
    $safeUsage = @{}
    foreach ($usageKey in @('input_tokens','output_tokens','cache_read_input_tokens','cache_creation_input_tokens')) {
        $usageValue = if ($result.usage -is [Collections.IDictionary]) { $result.usage[$usageKey] } else { $null }
        if ($usageValue -is [long] -or $usageValue -is [int]) { $safeUsage[$usageKey] = $usageValue }
    }
    $summary = [ordered]@{ suite = 'native-headless-live'; case = $Case; phase = $phase; passed = $passed; exitCode = $nativeExitCode; timedOut = $timedOut;
        injectedStop = $injectedStop; injectedStopAfterCompletions = $injectedStopAfterCompletions;
        stopControlFailure = $stopControlFailure; stopJournalBoundaryRejected = $stopJournalBoundaryRejected;
        connectionFaultRequested = [bool]$FailFirstConnection; connectionFaultVerified = $connectionFaultVerified; connectionFault = $connectionFault;
        nativeStopConfirmed = $nativeStopConfirmed; outputSettled = $outputSettled;
        outputCollectionFailed = $outputCollectionFailed;
        elapsedMs = $executionClock.ElapsedMilliseconds; runElapsedMs = $runClock.ElapsedMilliseconds; usage = $safeUsage;
        requests = $status.lifetime.started; succeeded = $status.lifetime.succeeded; failed = $status.lifetime.failed;
        upstreamAttempts = $(if ($null -ne $fixtureUsage) { $fixtureUsage.requestAttempts } elseif ($null -ne $status) { (@($status.recentRequests | ForEach-Object { $_.attempts.Count }) | Measure-Object -Sum).Sum } else { $null });
        failureCategories = @($status.failureHistory.records | ForEach-Object { $_.failureCategory });
        entryCategories = @([regex]::Matches($stderr, '(?m)^Clauduct: ([A-Z_]{1,64})(?: |\r?$)') | ForEach-Object { $_.Groups[1].Value });
        fixtureUsage = $fixtureUsage;
        usageUnobservedAttempts = $(if ($null -ne $fixtureUsage) { $fixtureUsage.requestAttempts - $fixtureUsage.completions } else { $null });
        fixtureUsageSource = $fixtureUsageSource; fixtureJournal = $fixtureJournal;
        ledgerReaderStopped = $ledgerReaderStopped;
        fixtureJournalState = $fixtureJournalState;
        transportProgressState = $transportProgressState; transportProgress = $transportProgress;
        stdoutJsonValid = $result -is [Collections.IDictionary]; exactReply = $exactReply; cleanupComplete = $clean;
        resultChars = $(if ($result.result -is [string]) { $result.result.Length } else { $null });
        nativeError = $result.is_error -eq $true;
        nativeMessageCategories = @(foreach ($label in @('ECONNRESET','ConnectionReset','ConnectionClosed','connection','socket','network','fetch','quota','rate','permission','API Error','timeout')) {
            if ($result.is_error -eq $true -and $result.result -is [string] -and $result.result -match [regex]::Escape($label)) { $label }
        });
        nativeResultKind = $(if ($result.subtype -in @('success', 'error_max_turns', 'error_during_execution', 'error_max_budget_usd')) { $result.subtype } else { 'OTHER' });
        fixtureMatched = $fixtureMatched; agentRouted = $agentRouted; mcpVerified = $mcpVerified; sameSession = $sameSession;
        failurePreserved = $failurePreserved;
        featureVerified = $featureVerified;
        imageFormat = $(if ($Case -eq 'image') { $ImageFormat } else { $null });
        nativeImageMediaType = $nativeImageMediaType;
        upstreamImageMediaType = $upstreamImageMediaType;
        completionEvidence = $completionEvidence;
        completionMode = $(if ($Case -eq 'completion') { $CompletionMode } else { $null });
        model = $Model; effort = $Effort; modelMatched = $modelMatched;
        workerStopped = $workerStopped;
        streamVerified = $streamVerified;
        statusPresent = $null -ne $status; stdoutChars = $stdout.Length; stderrChars = $stderr.Length;
        profile = 'new-task-local'; fixtureRoot = $fixtureRoot }
    $summaryJson = $summary | ConvertTo-Json -Depth 8 -Compress
    [IO.File]::WriteAllText((Join-Path $fixtureRoot ('result-' + $phase + '.json')), $summaryJson + "`n")
    Write-Output $summaryJson
    if (-not $passed) { exit 1 }
} finally { $process.Dispose() }
}
