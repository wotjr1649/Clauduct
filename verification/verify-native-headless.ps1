[CmdletBinding()]
param([switch] $Live, [ValidateSet('text', 'stream-json', 'read-edit', 'agent', 'workflow', 'mcp', 'resume', 'failure-resume', 'image', 'webfetch', 'websearch', 'build', 'build-powershell', 'background', 'cancel-task')][string] $Case = 'text',
    [ValidateSet('astra', 'sol', 'terra', 'luna')][string] $Model = 'luna',
    [ValidateSet('low', 'medium', 'high', 'xhigh', 'max')][string] $Effort = 'low',
    [ValidateRange(1, 120)][int] $TimeoutSeconds = 90)

$ErrorActionPreference = 'Stop'
if (-not $Live) { throw 'LIVE_FLAG_REQUIRED' }
if ($PSVersionTable.PSVersion.Major -lt 7) { throw 'POWERSHELL_7_REQUIRED' }
$taskRoot = (Resolve-Path -LiteralPath (Split-Path -Parent $PSScriptRoot)).ProviderPath
$temporaryRoot = Join-Path $taskRoot '.tmp'
foreach ($boundary in @($taskRoot, $temporaryRoot)) {
    if ((Test-Path -LiteralPath $boundary) -and ((Get-Item -Force -LiteralPath $boundary).Attributes -band [IO.FileAttributes]::ReparsePoint)) {
        throw 'VERIFICATION_ROOT_REPARSE_POINT'
    }
}
$fixtureRoot = Join-Path $temporaryRoot ('native-headless-' + [Guid]::NewGuid().ToString('N'))
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
    # Public 32x32 solid-red RGB PNG, generated from constant pixels; no user image.
    [IO.File]::WriteAllBytes((Join-Path $workingRoot 'square.png'), [Convert]::FromBase64String('iVBORw0KGgoAAAANSUhEUgAAACAAAAAgCAIAAAD8GO2jAAAAKElEQVR4nO3NsQ0AAAzCMP5/un0CNkuZ41wybXsHAAAAAAAAAAAAxR4yw/wuPL6QkAAAAABJRU5ErkJggg=='))
    $marker = 'RED'
    $prompt = 'Read square.png exactly once with Read. Look at the image and reply only with its dominant color name in uppercase.'
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
    $prompt = 'Use Agent exactly once with subagent_type clauduct-probe-luna, without a model argument, and run_in_background false. Ask it to follow its read-only probe instructions. Wait for its MODEL-PROBE-COMPLETED result, then reply with exactly CLAUDUCT_NATIVE_AGENT_OK.'
    $nativeTools = 'Agent,TaskOutput,Read'
    $turnLimit = '6'
    $extraArgs = @('--verify-agent-models')
}
if ($Case -eq 'workflow') {
    $workflowScript = "export const meta = { name: 'clauduct-check', description: 'Public arithmetic check' }; return await agent('Compute 2 + 3. Return the sum using StructuredOutput. Do not use other tools.', { label: 'one', model: 'luna', schema: { type: 'object', properties: { sum: { type: 'number' } }, required: ['sum'], additionalProperties: false } });"
    $marker = 'CLAUDUCT_NATIVE_WORKFLOW_OK'
    $prompt = "Call Workflow exactly once with this exact inline script, without a scriptPath or a saved workflow name: $workflowScript Use TaskOutput exactly once to wait for the returned task ID. After its structured result has sum=5, reply with exactly CLAUDUCT_NATIVE_WORKFLOW_OK. Do not launch another workflow or agent."
    $nativeTools = 'Workflow,TaskOutput'
    $turnLimit = '6'
    $extraArgs = @()
}
if ($Case -in @('mcp', 'failure-resume')) {
    $marker = 'CLAUDUCT_NATIVE_MCP_OK'
    $prompt = 'Use the local MCP fixture add tool exactly once with a=2 and b=3. Discover it with ToolSearch if needed. After its result is 5, reply with exactly CLAUDUCT_NATIVE_MCP_OK.'
    $nativeTools = 'ToolSearch'
    $turnLimit = '5'
    $mcpRecord = Join-Path $workingRoot 'mcp-calls.jsonl'
    $mcpConfig = @{ mcpServers = @{ fixture = @{ type = 'stdio'; command = (Get-Command node.exe -CommandType Application).Source;
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
$info.FileName = (Get-Command node.exe -CommandType Application).Source
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
if ($Case -eq 'build-powershell') { $info.Environment['CLAUDE_CODE_USE_POWERSHELL_TOOL'] = '1' }
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
foreach ($argument in @((Join-Path $taskRoot 'src/clauduct.mjs'), '--model', $Model, '--effort', $Effort,
    '-p', '--output-format', $(if ($Case -eq 'stream-json') { 'stream-json' } else { 'json' }), '--tools', $nativeTools, '--allowedTools', $allowedTools,
    '--max-turns', $turnLimit) + $extraArgs + @('--', $prompt)) {
    $info.ArgumentList.Add($argument)
}
$process = [Diagnostics.Process]::Start($info)
try {
    $process.StandardInput.Close()
    $stdoutTask = $process.StandardOutput.ReadToEndAsync()
    $stderrTask = $process.StandardError.ReadToEndAsync()
    $timedOut = -not $process.WaitForExit($TimeoutSeconds * 1000)
    if ($timedOut) { $process.Kill($true); $process.WaitForExit() }
    $stdout = $stdoutTask.GetAwaiter().GetResult()
    $stderr = $stderrTask.GetAwaiter().GetResult()
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
    $workerStopped = $null
    $expectedModel = @{ astra = 'gpt-6-astra'; sol = 'gpt-5.6-sol'; terra = 'gpt-5.6-terra'; luna = 'gpt-5.6-luna' }[$Model]
    $modelMatched = @($status.recentRequests | Where-Object { $_.subagent -ne $true -and $_.model -eq $expectedModel -and $_.effort -eq $Effort }).Count -gt 0
    if ($Case -eq 'read-edit') { $fixtureMatched = [IO.File]::ReadAllText((Join-Path $workingRoot 'math.mjs')).Trim() -eq $expectedFile }
    if ($Case -eq 'agent') { $agentRouted = @($status.recentRequests | Where-Object { $_.subagent -eq $true -and $_.selectionSource -eq 'definition-model' -and $_.model -eq 'gpt-5.6-luna' -and $_.effort -eq 'max' }).Count -gt 0 }
    if ($Case -eq 'workflow') {
        $agentRouted = @($status.recentRequests | Where-Object { $_.subagent -eq $true -and $_.selectionSource -eq 'workflow-result' -and $_.role -eq 'workflow-subagent' -and $_.model -eq 'gpt-5.6-luna' -and $_.effort -eq $Effort -and $_.success -eq $true }).Count -gt 0
    }
    if ($Case -in @('mcp', 'failure-resume')) {
        $mcpVerified = $false
        if (Test-Path -LiteralPath $mcpRecord) {
            $mcpCalls = @(Get-Content -LiteralPath $mcpRecord | ForEach-Object { ConvertFrom-Json -InputObject $_ -AsHashtable })
            $mcpVerified = $mcpCalls.Count -eq 1 -and $mcpCalls[0].a -eq 2 -and $mcpCalls[0].b -eq 3
        }
    }
    if ($Case -in @('resume', 'failure-resume')) { $sameSession = $result.session_id -eq $resumeId }
    if ($Case -in @('image', 'webfetch', 'websearch', 'build', 'build-powershell', 'background', 'cancel-task', 'workflow')) {
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
                    $featureVerified = $toolObserved -and [IO.Path]::GetFullPath($calls[0].input.file_path, $workingRoot) -eq (Join-Path $workingRoot 'square.png') -and @($results[0].content | Where-Object { $_.type -eq 'image' }).Count -eq 1
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
    $outcomeMatched = $process.ExitCode -eq 0 -and $exactReply -and $result.is_error -eq $false -and $status.requestOutcome -eq 'all-succeeded'
    if ($phase -eq 'failure') {
        $failedRequests = @($status.failureHistory.records | Where-Object { $_.failureCategory -eq 'UPSTREAM_ERROR_EVENT' -and $_.firstDownstreamWriteMs -ne $null -and $_.attempts.Count -eq 1 })
        $failurePreserved = $process.ExitCode -ne 0 -and $result.is_error -eq $true -and $status.lifetime.injectedStreamErrors -eq 1 -and $status.lifetime.failed -eq 1 -and $status.lifetime.started -eq 1 -and $failedRequests.Count -eq 1
        $outcomeMatched = $failurePreserved
    }
    $passed = -not $timedOut -and $outcomeMatched -and $clean -and $modelMatched -and $fixtureMatched -ne $false -and $agentRouted -ne $false -and $mcpVerified -ne $false -and $sameSession -ne $false -and $featureVerified -ne $false -and $streamVerified -ne $false
    [ordered]@{ suite = 'native-headless-live'; case = $Case; phase = $phase; passed = $passed; exitCode = $process.ExitCode; timedOut = $timedOut;
        stdoutJsonValid = $result -is [Collections.IDictionary]; exactReply = $exactReply; cleanupComplete = $clean;
        resultChars = $(if ($result.result -is [string]) { $result.result.Length } else { $null });
        nativeError = $result.is_error -eq $true;
        nativeResultKind = $(if ($result.subtype -in @('success', 'error_max_turns', 'error_during_execution', 'error_max_budget_usd')) { $result.subtype } else { 'OTHER' });
        fixtureMatched = $fixtureMatched; agentRouted = $agentRouted; mcpVerified = $mcpVerified; sameSession = $sameSession;
        failurePreserved = $failurePreserved;
        featureVerified = $featureVerified;
        model = $Model; effort = $Effort; modelMatched = $modelMatched;
        workerStopped = $workerStopped;
        streamVerified = $streamVerified;
        statusPresent = $null -ne $status; stdoutChars = $stdout.Length; stderrChars = $stderr.Length;
        profile = 'new-task-local'; fixtureRoot = $fixtureRoot } | ConvertTo-Json -Compress
    if (-not $passed) { exit 1 }
} finally { $process.Dispose() }
}
