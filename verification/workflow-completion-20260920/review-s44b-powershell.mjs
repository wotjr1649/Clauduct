// Recheck local evidence; never rerun a workload or change an execution policy.
import fs from 'node:fs';
import path from 'node:path';
import crypto from 'node:crypto';
import assert from 'node:assert/strict';
import {fileURLToPath} from 'node:url';
const here=path.dirname(fileURLToPath(import.meta.url)),id='7055dbfa-bec9-4880-98d6-a32d6f35ecca';
const evidence=path.join(here,'powershell-20260921'),session=path.join(here,'tui-'+id);
const read=p=>{assert.ok(fs.statSync(p).size<32<<20);return fs.readFileSync(p);};
const json=p=>JSON.parse(read(p).toString().replace(/^\uFEFF/,''));
const hash=p=>crypto.createHash('sha256').update(read(p)).digest('hex');
const meta=json(path.join(session,'session.json')),run=json(path.join(session,'first','run.json'));
const approved='D:/AIDEV/clauduct-s36-build/repair-20260919/verification/policy-repair-20260919/interactive-e268b95d-2048-47e3-b36b-5483000541a7';
assert.equal(meta.id,id);assert.equal(path.resolve(meta.profile),path.resolve(approved,'profile'));
assert.equal(path.resolve(meta.project),path.resolve(approved,'project'));
const statusDir=path.join(session,'first','tmp','clauduct'),names=fs.readdirSync(statusDir).filter(n=>/^status-\d+\.json$/.test(n));
assert.equal(names.length,1);const status=json(path.join(statusDir,names[0]));
const projects=path.join(meta.profile,'projects'),matches=fs.readdirSync(projects).filter(n=>fs.existsSync(path.join(projects,n,id+'.jsonl')));
assert.equal(matches.length,1);const base=path.join(projects,matches[0],id);
const calls=[],results=[],texts=[];
function transcript(file,agent){for(const line of read(file).toString().trim().split(/\r?\n/)){
 const row=JSON.parse(line);for(const b of Array.isArray(row.message?.content)?row.message.content:[]){
  if(b.type==='tool_use'&&['TaskStop','Bash'].includes(b.name))calls.push({agent,time:row.timestamp,id:b.id,name:b.name,task:b.input.task_id,command:b.input.command,timeout:b.input.timeout,background:b.input.run_in_background});
  if(b.type==='tool_result'){const content=typeof b.content==='string'?b.content:JSON.stringify(b.content);results.push({call:b.tool_use_id,time:row.timestamp,error:b.is_error===true,stopped:content?.includes('Successfully stopped task:')});}
  if(b.type==='text'&&row.type==='assistant'&&agent==='parent')texts.push({time:row.timestamp,values:b.text.match(/PS(?:51|7)_(?:STOPPED|RECOVERY)=\d+/g)??[]});
 }
}}
transcript(base+'.jsonl','parent');
const workflows=fs.readdirSync(path.join(base,'workflows')).filter(n=>/^wf_[a-f0-9-]+\.json$/.test(n)).map(n=>json(path.join(base,'workflows',n)));
for(const wf of workflows){const dir=path.join(base,'subagents','workflows',wf.runId);for(const name of fs.readdirSync(dir).filter(n=>/^agent-[a-z0-9]+\.jsonl$/.test(n)))transcript(path.join(dir,name),name.slice(6,-6));}
const checks=[];const check=(name,fn)=>{fn();checks.push(name);};
check('same_development_binaries_and_native',()=>{const build=json(path.join(here,'build.json'));for(const n of ['clauduct.exe','clauduct-hook.exe','clauduct-dev.exe'])assert.equal(hash(path.join('D:/AIDEV/clauduct-s36-build',n)),build.artifacts[n]);assert.equal(meta.sha256,build.artifacts['clauduct.exe']);assert.equal(status.gateway.client.version,'2.1.278');});
check('both_engine_preflights_and_invalid_input_rejection',()=>{const pre=json(path.join(evidence,'preflight.json'));assert.equal(pre.length,2);for(const p of pre){assert.equal(p.syntax,'pass');assert.equal(p.invalidProbeRejected,true);assert.notEqual(p.exit,0);}});
check('all_persistent_and_default_policies_unchanged',()=>assert.deepEqual(json(path.join(evidence,'policy-after.json')),json(path.join(evidence,'policy-before.json'))));
check('reviewed_fixture_matches_deployed_copy',()=>{assert.equal(hash(path.join(here,'public-s44b-probe.ps1')),hash(path.join(meta.project,'public-s44b-probe.ps1')));assert.equal(hash(path.join(here,'public-s44b-probe.ps1')),'56c86408a8cda29cab9ae89fcc53a6757e8c88d98f60f01b771b5b99cc04a5f3');});
check('normal_exit_without_unexpected_failures',()=>{assert.equal(run.exit,0);assert.ok(!run.watchdogForced);assert.equal(status.lifecycle.reason,'native_exit');assert.equal(status.lifecycle.nativeReaped,true);assert.equal(status.lifecycle.final,true);for(const k of ['apiFailures','cancelledRequests','nativeToolFailures','unacquiredResultsInRecent','rejectedWorkflowCalls'])assert.equal(status.completion[k],0);assert.equal(status.completion.nativeCancellations,2);});
const stops=calls.filter(x=>x.name==='TaskStop'),bash=calls.filter(x=>x.name==='Bash');
check('exactly_two_single_step_workloads_and_stops',()=>{assert.equal(workflows.length,2);assert.equal(stops.length,2);assert.equal(bash.length,2);for(const wf of workflows){assert.equal(wf.agentCount,1);assert.equal(wf.status,'killed');assert.equal(stops.filter(x=>x.task===wf.taskId).length,1);}});
const observations=[];
for(const [probe,version,task,engine,prefix] of [
 ['s44b-510000000001','5.1.26100.8870','wzbff9l19','C:/Windows/System32/WindowsPowerShell/v1.0/powershell.exe','PS51'],
 ['s44b-700000000001','7.6.6','wwxvifxt5','C:/Program Files/PowerShell/7/pwsh.exe','PS7']]){
 const before=json(path.join(here,probe,'before.json')),after=json(path.join(here,probe,'after.json'));
 const stop=stops.find(x=>x.task===task),call=bash.find(x=>x.command?.endsWith('-ProbeId '+probe));
 check(prefix+'_exact_command_and_TaskStop',()=>{assert.ok(stop&&call);assert.equal(call.command,(prefix==='PS7'?`"${engine}"`:engine)+' -NoProfile -ExecutionPolicy RemoteSigned -File ./public-s44b-probe.ps1 -ProbeId '+probe);assert.equal(call.timeout,240000);assert.ok(!call.background);assert.ok(results.some(x=>x.call===stop.id&&x.stopped&&!x.error));});
 check(prefix+'_real_parent_and_child_with_temporary_policy',()=>{assert.equal(before.method,'held_OS_process_handles_and_start_time');assert.equal(before.observerKillsProcesses,false);assert.equal(before.identities.length,2);assert.equal(before.childParentPid,before.identities.find(x=>x.role==='root').pid);assert.equal(new Set(before.identities.map(x=>x.pid)).size,2);for(const p of before.identities){assert.equal(p.probe,probe);assert.equal(p.version,version);assert.equal(p.policy,'RemoteSigned');assert.equal(p.processPolicy,'RemoteSigned');assert.equal(path.resolve(p.engine).toLowerCase(),path.resolve(engine).toLowerCase());}});
 let maximumStopToExitMs=0;
 check(prefix+'_held_handles_confirm_reap_after_stop',()=>{assert.ok(Date.parse(before.atUtc)<Date.parse(stop.time));assert.equal(after.observerKillsProcesses,false);assert.equal(after.stoppedBeforeNaturalDeadline,true);assert.deepEqual(after.remaining,[]);assert.deepEqual(after.finishedMarkers,[]);assert.equal(after.exits.length,2);for(const exited of after.exits){const p=before.identities.find(x=>x.pid===exited.pid);assert.ok(p);assert.equal(p.startedTicks,exited.startedTicks);const delta=Date.parse(exited.exitedUtc)-Date.parse(stop.time);assert.ok(delta>=0&&delta<5000);maximumStopToExitMs=Math.max(maximumStopToExitMs,delta);assert.ok(Date.parse(exited.exitedUtc)-Date.parse(p.observedUtc)<180000);}});
 check(prefix+'_stop_reply_and_separate_input_recovery',()=>{for(const v of [prefix+'_STOPPED=53',prefix+'_RECOVERY=71'])assert.ok(texts.some(x=>Date.parse(x.time)>Date.parse(stop.time)&&x.values.includes(v)));});
 observations.push({probe,version,task,workflow:workflows.find(x=>x.taskId===task).runId,call,stop,before,after,maximumStopToExitMs});
}
check('both_workflow_agents_use_terra_medium_once',()=>{const selections=status.gateway.agentSelections.recent;assert.equal(selections.length,2);for(const s of selections){assert.equal(s.effectiveModel,'gpt-5.6-terra');assert.equal(s.effectiveEffort,'medium');assert.equal(s.backendResponses,1);assert.equal(s.backendCompletions,1);assert.equal(s.source,'workflow-selection');}});
const out={id,result:'pass',scope:'PowerShell 5.1/7 public script execution, temporary policy inheritance, native Workflow cancellation and recovery',checks,sha256:meta.sha256,fixtureHash:hash(path.join(here,'public-s44b-probe.ps1')),transcriptHash:hash(base+'.jsonl'),completion:status.completion,requests:status.gateway.requests.received,policies:json(path.join(evidence,'policy-after.json')),observations};
fs.writeFileSync(path.join(evidence,'acceptance.json'),JSON.stringify(out,null,2)+'\n');
console.log(JSON.stringify({id,result:out.result,checks:checks.length,completion:out.completion,observations:observations.map(x=>({version:x.version,task:x.task,workflow:x.workflow,maximumStopToExitMs:x.maximumStopToExitMs}))}));
