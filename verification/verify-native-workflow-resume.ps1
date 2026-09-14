param([ValidateSet('sol','luna')][string]$Model='sol', [ValidateSet('first','resume')][string]$Phase='first', [string]$RunRoot)
$ErrorActionPreference='Stop'
$project=(Resolve-Path -LiteralPath (Join-Path $PSScriptRoot '..')).ProviderPath
$configuredRoot=[Environment]::GetEnvironmentVariable('CLAUDE_CONFIG_DIR')
$globalRoot=if($configuredRoot){[IO.Path]::GetFullPath($configuredRoot)}else{Join-Path ([Environment]::GetFolderPath('UserProfile')) '.claude'}
try{
 $setting=Get-Item -LiteralPath (Join-Path $globalRoot 'settings.json') -Force
 if($setting.Length -gt 256KB -or ($setting.Attributes -band [IO.FileAttributes]::ReparsePoint)){throw 'BOUNDARY'}
 $value=[IO.File]::ReadAllText($setting.FullName)|ConvertFrom-Json -AsHashtable
 $modeMatches=$value.permissions.defaultMode -is [string] -and $value.permissions.defaultMode -ceq 'bypassPermissions';$value=$null
 if(-not $modeMatches){throw 'MODE'}
}catch{throw 'GLOBAL_BYPASS_SETTINGS_UNVERIFIED'}
foreach($path in @($project,(Join-Path $project '.tmp'))){
 if((Get-Item -LiteralPath $path -Force).Attributes -band [IO.FileAttributes]::ReparsePoint){throw 'WORKFLOW_ROOT_BOUNDARY'}
}
$sourceFiles=@('src/clauduct.mjs','src/agent-route.mjs','src/agent-selection.mjs','src/workflow-selection.mjs',
 'src/native-gateway.mjs','src/native-protocol.mjs','src/models.mjs','verification/native-workflow-resume-entry.mjs',
 'verification/verify-native-workflow-resume.ps1','verification/verify-workflow-resume-evidence.mjs','verification/stop-owned-native-tree.ps1')
$sourceHashes=@{}
foreach($file in $sourceFiles){$sourceHashes[$file]=(Get-FileHash -LiteralPath (Join-Path $project $file)).Hash.ToLowerInvariant()}
if($Phase -eq 'first'){
 if($RunRoot){throw 'WORKFLOW_FIRST_ROOT'}
 $run=Join-Path $project ('.tmp/native-workflow-stored-'+[Guid]::NewGuid().ToString('N'))
 foreach($name in @('config','work','temp')){[void][IO.Directory]::CreateDirectory((Join-Path $run $name))}
 [IO.File]::WriteAllText((Join-Path $run 'config/settings.json'),'{"permissions":{"defaultMode":"bypassPermissions"}}'+"`n")
 $manifest=@{model=$Model;sessionId=[Guid]::NewGuid().ToString();createdAtMs=[DateTimeOffset]::UtcNow.ToUnixTimeMilliseconds();requestLimitEachPhase=12;phaseMs=30000;wrapperMs=50000;actualBackendRequests=0;credentialReads=0;fullPersonalProfileLoaded=$false;sourceHashes=$sourceHashes}
 [IO.File]::WriteAllText((Join-Path $run 'manifest.json'),($manifest|ConvertTo-Json -Compress)+"`n")
}else{
 $run=[IO.Path]::GetFullPath($RunRoot)
 if((Split-Path -Parent $run) -ine (Join-Path $project '.tmp') -or (Split-Path -Leaf $run) -notmatch '^native-workflow-stored-[a-f0-9]{32}$' -or
   ((Get-Item -LiteralPath $run -Force).Attributes -band [IO.FileAttributes]::ReparsePoint)){throw 'WORKFLOW_RESUME_ROOT'}
 $first=Get-Content -LiteralPath (Join-Path $run 'first-result.json') -Raw|ConvertFrom-Json
 $manifest=Get-Content -LiteralPath (Join-Path $run 'manifest.json') -Raw|ConvertFrom-Json -AsHashtable
 foreach($file in $sourceFiles){if($manifest.sourceHashes[$file] -cne $sourceHashes[$file]){throw 'WORKFLOW_SOURCE_CHANGED'}}
 if(-not $first.passed -or -not $first.tree.stopped -or $first.model -cne $Model -or (Test-Path -LiteralPath (Join-Path $run 'resume-result.json'))){throw 'WORKFLOW_RESUME_STATE'}
 $recovered=& node (Join-Path $PSScriptRoot 'verify-workflow-resume-evidence.mjs') $run --first
 if($LASTEXITCODE -ne 0 -or -not ($recovered|ConvertFrom-Json).passed){throw 'WORKFLOW_FIRST_EVIDENCE_UNVERIFIED'}
 if(Test-Path -LiteralPath (Join-Path $run 'first-verified.json')){throw 'WORKFLOW_FIRST_EVIDENCE_EXISTS'}
 [IO.File]::WriteAllText((Join-Path $run 'first-verified.json'),$recovered+"`n")
 $owner=& (Join-Path $project 'verification/stop-owned-native-tree.ps1') -RootPid $first.pid -StartedAfterMs $first.startedMs -RunRoot $run
 if(-not $? -or -not ($owner|ConvertFrom-Json).stopped){throw 'WORKFLOW_PREVIOUS_WORKER_NOT_STOPPED'}
}
$info=[Diagnostics.ProcessStartInfo]::new((Get-Command node.exe -CommandType Application|Select-Object -First 1).Source)
$info.WorkingDirectory=Join-Path $run 'work';$info.UseShellExecute=$false;$info.CreateNoWindow=$true
$info.RedirectStandardInput=$true;$info.RedirectStandardOutput=$true;$info.RedirectStandardError=$true
$info.Environment.Clear()
foreach($name in @('SystemRoot','WINDIR','SystemDrive','ComSpec','PATH','PATHEXT','USERPROFILE','HOMEDRIVE','HOMEPATH','APPDATA','LOCALAPPDATA','ProgramData','ProgramFiles','ProgramFiles(x86)','OS','PROCESSOR_ARCHITECTURE')){
 $value=[Environment]::GetEnvironmentVariable($name);if($value){$info.Environment[$name]=$value}
}
$info.Environment['CLAUDE_CONFIG_DIR']=Join-Path $run 'config'
foreach($name in @('CLAUDE_CODE_TMPDIR','TEMP','TMP')){$info.Environment[$name]=Join-Path $run 'temp'}
$info.Environment['CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC']='1'
$info.Environment['CLAUDE_CODE_POWERSHELL_RESPECT_EXECUTION_POLICY']='1'
$info.ArgumentList.Add((Join-Path $PSScriptRoot 'native-workflow-resume-entry.mjs'));$info.ArgumentList.Add($run);$info.ArgumentList.Add($Phase)
$startedMs=[DateTimeOffset]::UtcNow.ToUnixTimeMilliseconds();$process=[Diagnostics.Process]::Start($info)
try{
 $process.StandardInput.Close();$out=$process.StandardOutput.ReadToEndAsync();$err=$process.StandardError.ReadToEndAsync()
 $timedOut=-not $process.WaitForExit(30000)
 $treeArgs=@{RootPid=$process.Id;StartedAfterMs=$startedMs;RunRoot=$run};if($timedOut){$treeArgs.Stop=$true}
 $treeOutput=& (Join-Path $project 'verification/stop-owned-native-tree.ps1') @treeArgs
 if(-not $?){throw 'WORKFLOW_TREE_UNVERIFIED'}
 $tree=$treeOutput|ConvertFrom-Json
 if(-not $tree.stopped -or -not $process.WaitForExit(5000)){throw 'WORKFLOW_TREE_UNVERIFIED'}
 $stdout=$out.GetAwaiter().GetResult();$stderr=$err.GetAwaiter().GetResult()
 if($stdout.Length+$stderr.Length -gt 1MB){throw 'WORKFLOW_OUTPUT_LIMIT'}
 [IO.File]::WriteAllText((Join-Path $run ($Phase+'-stdout.txt')),$stdout);[IO.File]::WriteAllText((Join-Path $run ($Phase+'-stderr.txt')),$stderr)
 $entry=Get-Content -LiteralPath (Join-Path $run ($Phase+'-entry.json')) -Raw|ConvertFrom-Json
 $lines=@($stderr -split "`n"|Where-Object {$_.StartsWith('CLAUDUCT_REQUEST_STATUS ')})
 $status=$null;if($lines.Count -eq 1){$status=$lines[0].Substring(24)|ConvertFrom-Json}
 $cleanup=$status -and @($status.cleanup.PSObject.Properties).Count -eq 9 -and @($status.cleanup.PSObject.Properties|Where-Object {$_.Value -ne $true}).Count -eq 0
 $init=@($stdout -split "`n"|Where-Object {$_}|ForEach-Object {$_|ConvertFrom-Json}|Where-Object {$_.type -eq 'system' -and $_.subtype -eq 'init'})
 $bypass=$init.Count -ge 1 -and $init.Count -le 4 -and @($init|Where-Object {$_.permissionMode -cne 'bypassPermissions'}).Count -eq 0
 $result=[ordered]@{root=$run;model=$Model;phase=$Phase;entry=$entry;pid=$process.Id;startedMs=$startedMs;tree=$tree;exitCode=$process.ExitCode;timedOut=$timedOut;
  elapsedMs=([DateTimeOffset]::UtcNow.ToUnixTimeMilliseconds()-$startedMs);status=$status;cleanupComplete=[bool]$cleanup;bypassObserved=$bypass;
  passed=($process.ExitCode -eq 0 -and -not $timedOut -and $entry.completed -and $cleanup -and $bypass)}
 $result|ConvertTo-Json -Depth 12|Set-Content -LiteralPath (Join-Path $run ($Phase+'-result.json')) -Encoding utf8
 [ordered]@{root=$run;model=$Model;phase=$Phase;passed=$result.passed;entry=$entry;exitCode=$process.ExitCode;remaining=$tree.remaining;cleanupComplete=[bool]$cleanup;bypassObserved=$bypass}|ConvertTo-Json -Depth 5
}finally{$process.Dispose()}
