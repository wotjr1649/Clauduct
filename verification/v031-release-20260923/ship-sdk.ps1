param([Parameter(Mandatory)][string]$PackageDir,[Parameter(Mandatory)][string]$RunDir,[switch]$InjectLoss)
$ErrorActionPreference='Stop'
if([Environment]::GetEnvironmentVariable('SSLKEYLOGFILE')){throw 'unsupported inherited TLS key logging request'}
$configuredCodex=[Environment]::GetEnvironmentVariable('CODEX_HOME')
$expectedCodex=Join-Path ([Environment]::GetFolderPath('UserProfile')) '.codex'
if($configuredCodex -and [IO.Path]::GetFullPath($configuredCodex) -ine [IO.Path]::GetFullPath($expectedCodex)){throw 'redirected Codex home requires review'}
$taskRoot=[IO.Path]::GetFullPath($PSScriptRoot)
$RunDir=[IO.Path]::GetFullPath($RunDir)
if(-not $RunDir.StartsWith($taskRoot+'\',[StringComparison]::OrdinalIgnoreCase) -or (Test-Path -LiteralPath $RunDir)){throw 'fresh task-owned run directory required'}
$project=Join-Path $RunDir 'public-project'
$profile=Join-Path $RunDir 'profile'
$temp=Join-Path $RunDir 'public-temp'
foreach($dir in @($project,$profile,$temp)){[IO.Directory]::CreateDirectory($dir)|Out-Null}
# An independent project boundary, containing only public synthetic inputs.
git -C $project init -q
if($LASTEXITCODE){throw 'public project init failed'}
foreach($dir in @('C:\Program Files\ClaudeCode','C:\ProgramData\ClaudeCode')){if(Test-Path -LiteralPath $dir){throw 'managed native settings require separate review'}}
$settings=@{permissions=@{blockReadsOutsideWorkingDirectories=$true;allow=@('Edit(/public-file.txt)','Edit(/public-once.txt)')}} | ConvertTo-Json -Compress -Depth 4
$roles='{"public-peer":{"description":"Public test worker","prompt":"Use no tools. Reply exactly with the public identifier requested.","model":"gpt-5.6-luna","effort":"low","tools":[]}}'
$psi=[Diagnostics.ProcessStartInfo]::new((Join-Path $PackageDir 'clauduct.exe'))
$psi.UseShellExecute=$false; $psi.CreateNoWindow=$true; $psi.WorkingDirectory=$project
$psi.RedirectStandardInput=$true; $psi.RedirectStandardOutput=$true; $psi.RedirectStandardError=$true
$psi.Environment.Clear()
foreach($name in @('SystemRoot','WINDIR','COMSPEC','PATHEXT','PATH','ProgramFiles','ProgramFiles(x86)','ProgramW6432','NUMBER_OF_PROCESSORS','PROCESSOR_ARCHITECTURE','OS')){
  $value=[Environment]::GetEnvironmentVariable($name);if($value){$psi.Environment[$name]=$value}
}
foreach($name in @('USERPROFILE','APPDATA','LOCALAPPDATA','CLAUDE_CONFIG_DIR')){$psi.Environment[$name]=$profile}
$psi.Environment['TEMP']=$temp; $psi.Environment['TMP']=$temp
$psi.Environment['CLAUDUCT_SESSION_TIMEOUT_MS']='240000'
$psi.Environment['CLAUDE_CODE_FORK_SUBAGENT']='1'
$arguments=@('-p','--input-format','stream-json','--output-format','stream-json','--verbose','--model','gpt-5.6-luna','--effort','low','--max-turns','16','--permission-mode','dontAsk','--tools','Read,Write,Agent,Workflow,ToolSearch,SendMessage','--allowedTools','Agent,Workflow,ToolSearch,SendMessage','--strict-mcp-config','--setting-sources','','--settings',$settings,'--agents',$roles,'--system-prompt','Public synthetic release verification. Follow only the supplied public task. Never inspect configuration, credentials, environment, or unrelated files. Use only the available tools and permitted project paths.')
foreach($arg in $arguments){$psi.ArgumentList.Add($arg)}
$events=[Collections.Generic.List[object]]::new()
$p=[Diagnostics.Process]::new();$p.StartInfo=$psi;$started=$false
$nativeId=0;$nativeStart=$null;$statusPath='';$phase=0;$notifications=0
$tools=@{};$results=@{};$completed=@{};$seenToolIds=@{}
$deadline=[DateTime]::UtcNow.AddSeconds(270)
$faultInjected=$false;$faultConnections=0;$phaseStartAttempts=0;$lastStatusAt=[DateTime]::MinValue
if($InjectLoss){
  # Close only established IPv4 loopback connections from this verified native
  # PID to a listener owned by this launcher. No filter, firewall or service changes.
  Add-Type -TypeDefinition @'
using System;
using System.Runtime.InteropServices;
public static class PublicOwnedConnectionFault {
  [StructLayout(LayoutKind.Sequential)] struct Row { public uint state, localAddr, localPort, remoteAddr, remotePort; }
  [DllImport("iphlpapi.dll")] static extern uint GetExtendedTcpTable(IntPtr p, ref int size, bool order, int family, int table, uint reserved);
  [DllImport("iphlpapi.dll")] static extern uint SetTcpEntry(ref Row row);
  public static int Close(int native, int launcher) {
    int size=0; GetExtendedTcpTable(IntPtr.Zero,ref size,false,2,5,0);
    if(size<4 || size>16777216) throw new Exception("TCP table outside bound");
    IntPtr p=Marshal.AllocHGlobal(size);
    try {
      if(GetExtendedTcpTable(p,ref size,false,2,5,0)!=0) throw new Exception("TCP table read failed");
      int count=Marshal.ReadInt32(p); if(count<0 || count>(size-4)/24) throw new Exception("TCP row count invalid");
      var listeners=new System.Collections.Generic.HashSet<uint>();
      for(int i=0;i<count;i++) { IntPtr q=IntPtr.Add(p,4+i*24); if(Marshal.ReadInt32(q,20)==launcher && Marshal.ReadInt32(q)==2 && (uint)Marshal.ReadInt32(q,4)==0x0100007f) listeners.Add((uint)Marshal.ReadInt32(q,8)); }
      int closed=0;
      for(int i=0;i<count;i++) {
        IntPtr q=IntPtr.Add(p,4+i*24);
        if(Marshal.ReadInt32(q,20)!=native || Marshal.ReadInt32(q)!=5 || (uint)Marshal.ReadInt32(q,4)!=0x0100007f || (uint)Marshal.ReadInt32(q,12)!=0x0100007f || !listeners.Contains((uint)Marshal.ReadInt32(q,16))) continue;
        Row row=Marshal.PtrToStructure<Row>(q); row.state=12;
        uint result=SetTcpEntry(ref row); if(result==0) closed++; else if(result!=1168) throw new Exception("Owned TCP close failed: "+result);
      }
      return closed;
    } finally { Marshal.FreeHGlobal(p); }
  }
}
'@
}
$workflow='{"script":"clauduct:plan-v1","args":{"steps":[{"id":"W","prompt":"Use no tools. Reply exactly PUBLIC_WORKFLOW_37.","model":"gpt-5.6-luna","effort":"low","tools":[]}]}}'
$agent='{"subagent_type":"public-peer","description":"public worker","prompt":"Use no tools. Reply exactly PUBLIC_AGENT_31.","model":"gpt-5.6-luna","effort":"low","run_in_background":true}'
$prompts=@(
  'Use Write exactly once to create public-file.txt in the current directory containing exactly PUBLIC_FILE_53 with no newline. Read it once. Reply exactly PUBLIC_FILE_READY.',
  ('Discover Workflow if needed. Start exactly one Agent with these arguments: '+$agent+'. Start exactly one Workflow with these arguments: '+$workflow+'. Start both before waiting. Do not repeat any worker. A launch is not completion. While either child is running, return no text or tools and wait for task-notification. Only after actual completion of both, reply exactly PUBLIC_AGENT_31 PUBLIC_WORKFLOW_37 PUBLIC_CHILDREN_DONE.'),
  'Use Write exactly once to create public-once.txt in the current directory containing exactly PUBLIC_ONCE_59 with no newline. Do not repeat the write. Reply exactly PUBLIC_ONCE_DONE.',
  'Use Read once to read public-once.txt. Do not repeat any earlier Write, Agent or Workflow. Reply with its content, the two earlier child identifiers, and PUBLIC_CYCLE_RECOVERED.'
)
if($InjectLoss){$prompts[2]='Use Write exactly once to create public-once.txt in the current directory containing exactly PUBLIC_ONCE_59 with no newline. Never repeat this write after any error. After its successful tool result, output the integers 1 through 3000 separated by spaces, without summarizing, then PUBLIC_ONCE_DONE.'}
function Send-PublicPrompt([int]$index){
  $line=@{type='user';message=@{role='user';content=$prompts[$index]}} | ConvertTo-Json -Compress -Depth 4
  $p.StandardInput.WriteLine($line);$p.StandardInput.Flush()
  Write-Output ('sent_phase='+($index+1))
}
function Read-Status {
  if($statusPath -and (Test-Path -LiteralPath $statusPath)){
    return [IO.File]::ReadAllText($statusPath) | ConvertFrom-Json -Depth 70
  }
  return $null
}
try {
  if(-not $p.Start()){throw 'launcher did not start'}
  $started=$true
  $statusPath=Join-Path $temp ('clauduct/status-'+$p.Id+'.json')
  $stderr=$p.StandardError.ReadToEndAsync()
  $lineTask=$p.StandardOutput.ReadLineAsync()
  Send-PublicPrompt 0
  while([DateTime]::UtcNow -lt $deadline){
    if(-not $nativeId){
      $child=@(Get-CimInstance Win32_Process -Filter ('ParentProcessId='+$p.Id) | Where-Object {$_.Name -eq 'claude.exe'})
      if($child.Count -eq 1){$nativeId=[int]$child[0].ProcessId;$nativeStart=(Get-Process -Id $nativeId).StartTime;Write-Output ('native_pid='+$nativeId)}
    }
    if($InjectLoss -and $phase -eq 2 -and -not $faultInjected -and $nativeId -and ([DateTime]::UtcNow-$lastStatusAt).TotalMilliseconds -ge 250){
      $lastStatusAt=[DateTime]::UtcNow;$observed=Read-Status
      if((Test-Path -LiteralPath (Join-Path $project 'public-once.txt')) -and $observed.attempts -ge ($phaseStartAttempts+2) -and $observed.gateway.requests.active -gt 0){
        if((Get-Process -Id $nativeId).StartTime -ne $nativeStart){throw 'native identity changed before injection'}
        $faultConnections=[PublicOwnedConnectionFault]::Close($nativeId,$p.Id)
        if($faultConnections -gt 0){$faultInjected=$true;Write-Output ('owned_connections_interrupted='+$faultConnections)}
      }
    }
    if(-not $lineTask.IsCompleted){if($p.HasExited){break};Start-Sleep -Milliseconds 40;continue}
    $line=$lineTask.GetAwaiter().GetResult()
    if($null -eq $line){break}
    if($line.Length -gt 4MB){throw 'native event exceeded bound'}
    $e=$line | ConvertFrom-Json -Depth 70
    $lineTask=$p.StandardOutput.ReadLineAsync()
    if($e.type -eq 'assistant'){
      foreach($part in $e.message.content){
        if($part.type -eq 'tool_use'){
          if($seenToolIds.ContainsKey($part.id)){throw 'duplicate tool use id'}
          $seenToolIds[$part.id]=$part.name
          $tools[$part.name]=1+[int]$tools[$part.name]
        }
      }
    }
    if($e.type -eq 'user'){
      foreach($part in $e.message.content){
        if($part.type -eq 'tool_result' -and -not $part.is_error -and $seenToolIds.ContainsKey($part.tool_use_id)){
          $name=$seenToolIds[$part.tool_use_id];$results[$name]=1+[int]$results[$name]
        }
      }
    }
    if($e.subtype -eq 'task_notification'){
      $notifications++;$events.Add(@{type='task_notification';status=$e.status;phase=$phase+1})
    }
    if($e.type -ne 'result'){continue}
    $status=Read-Status
    $marker=[string]$e.result
    $summary=@{type='result';phase=$phase+1;error=[bool]$e.is_error;subtype=$e.subtype;resultBytes=$marker.Length;attempts=$status.attempts;received=$status.gateway.agentResults.totals.parent_received}
    $events.Add($summary);$summary | ConvertTo-Json -Compress | Write-Output
    if($e.is_error -and -not ($InjectLoss -and $phase -eq 2 -and $faultInjected)){throw 'native reported a failed normal turn'}
    if(-not $marker -or $marker -in @('[Clauduct] Waiting for background task notification.','[Clauduct] Background task notification received; no additional response.') -or $completed.ContainsKey($marker)){continue}
    $required=switch($phase){0{@('PUBLIC_FILE_READY')}1{@('PUBLIC_AGENT_31','PUBLIC_WORKFLOW_37','PUBLIC_CHILDREN_DONE')}2{@('PUBLIC_ONCE_DONE')}3{@('PUBLIC_ONCE_59','PUBLIC_AGENT_31','PUBLIC_WORKFLOW_37','PUBLIC_CYCLE_RECOVERED')}}
    if($phase -eq 1){
      $checkpointDeadline=[DateTime]::UtcNow.AddSeconds(5)
      while($status.gateway.agentResults.totals.parent_received -lt 2 -and [DateTime]::UtcNow -lt $checkpointDeadline){Start-Sleep -Milliseconds 50;$status=Read-Status}
      if($status.gateway.agentResults.totals.parent_received -lt 2 -or $notifications -lt 2){continue}
    }
    if($InjectLoss -and $phase -eq 2){
      if(-not $faultInjected -or -not $e.is_error -or -not ($marker.Contains('NATIVE_REQUEST_REPLAY_BLOCKED') -or $marker.Contains('CANCELLED'))){throw 'injected loss did not end without replay'}
    }else{foreach($value in $required){if(-not $marker.Contains($value)){throw ('missing public marker at phase '+($phase+1))}}}
    if(-not $nativeId -or (Get-Process -Id $nativeId).StartTime -ne $nativeStart){throw 'native PID identity changed'}
    $completed[$marker]=$true;$phase++
    if($phase -eq $prompts.Count){$p.StandardInput.Close();break}
    $phaseStartAttempts=[int](Read-Status).attempts
    Send-PublicPrompt $phase
  }
  if($phase -ne $prompts.Count){throw 'scenario did not finish inside its bound'}
  if(-not $p.WaitForExit(10000)){throw 'launcher exit timeout'}
  if($p.ExitCode -ne 0){throw ('launcher exit='+$p.ExitCode)}
  $status=Read-Status
  [IO.File]::WriteAllText((Join-Path $RunDir 'status.json'),($status | ConvertTo-Json -Depth 70))
  if($status.gateway.requests.active -ne 0 -or -not $status.lifecycle.nativeReaped -or $status.gateway.agentResults.totals.parent_received -ne 2){throw 'final lifecycle or child receipt mismatch'}
  foreach($failure in $status.gateway.totals.failures.PSObject.Properties){
    if(-not $InjectLoss -or $failure.Name -notin @('CANCELLED','NATIVE_REQUEST_REPLAY_BLOCKED') -or $failure.Value -ne 1){throw 'unexpected gateway failure'}
  }
  if($tools['Write'] -ne 2 -or $tools['Agent'] -ne 1 -or $tools['Workflow'] -ne 1){throw 'duplicate or missing tool execution'}
  $workflowEvidence=@($status.gateway.features | Where-Object {$_.name -eq 'workflow_agent'})
  if($workflowEvidence.Count -ne 1 -or $workflowEvidence[0].completed -ne 1){throw 'missing actual Workflow completion'}
  if([IO.File]::ReadAllText((Join-Path $project 'public-file.txt')) -cne 'PUBLIC_FILE_53' -or [IO.File]::ReadAllText((Join-Path $project 'public-once.txt')) -cne 'PUBLIC_ONCE_59'){throw 'actual file contents mismatch'}
  if($InjectLoss -and -not $faultInjected){throw 'loss was not injected'}
  $verdict=@{verdict='PASS';nativePID=$nativeId;starts=1;phases=$phase;notifications=$notifications;tools=$tools;successfulTools=$results;attempts=$status.attempts;inferences=$status.inferences;exit=$p.ExitCode;lossInjected=$faultInjected;interruptedConnections=$faultConnections}
  [IO.File]::WriteAllText((Join-Path $RunDir 'verdict.json'),($verdict | ConvertTo-Json -Depth 5))
  $verdict | ConvertTo-Json -Compress -Depth 5 | Write-Output
} finally {
  if($started -and -not $p.HasExited){$p.StandardInput.Close();if(-not $p.WaitForExit(5000)){$p.Kill($true);$p.WaitForExit()}}
  [IO.File]::WriteAllText((Join-Path $RunDir 'events.json'),($events | ConvertTo-Json -Depth 5))
  $p.Dispose()
}
