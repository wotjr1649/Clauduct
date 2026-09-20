// Fixed, local metadata and public test markers only. Never persist message
// bodies, reasoning, credentials or compaction tickets in the audit artifact.
import fs from 'node:fs';
import path from 'node:path';
import crypto from 'node:crypto';
import assert from 'node:assert/strict';
import {fileURLToPath} from 'node:url';
const here=path.dirname(fileURLToPath(import.meta.url));
const base='D:/AIDEV/clauduct-s36-build/repair-20260919/verification/policy-repair-20260919';
const approved=path.join(base,'interactive-e268b95d-2048-47e3-b36b-5483000541a7');
const project=path.join(approved,'profile/projects/D--AIDEV-clauduct-s36-build-repair-20260919-verification-policy-repair-20260919-interactive-e268b95d-2048-47e3-b36b-5483000541a7-project');
const id='432ae808-ff52-435e-aed2-e0ca5c8c983a',run=path.join(base,'interactive-'+id);
const read=f=>{assert(fs.statSync(f).size<32*1024*1024);return fs.readFileSync(f,'utf8').replace(/^\uFEFF/,'');};
const json=f=>JSON.parse(read(f)),rows=f=>read(f).trim().split(/\r?\n/).map(JSON.parse);
const sha=f=>crypto.createHash('sha256').update(fs.readFileSync(f)).digest('hex');
const manifest=json(path.join(run,'run.json')),build=json(path.join(here,'build-53482a4.json'));
assert.equal(manifest.sha256,build.artifacts['clauduct.exe']);
assert.equal(path.resolve(manifest.profile),path.resolve(approved,'profile'));
assert.equal(manifest.state,'exited');assert.equal(manifest.exitCode,0);assert(!manifest.watchdogForced);
const files=fs.readdirSync(path.join(run,'tmp/clauduct')).filter(n=>/^status-\d+\.json$/.test(n));assert.equal(files.length,1);
const statusFile=path.join(run,'tmp/clauduct',files[0]),s=json(statusFile);
assert(s.lifecycle.final&&s.lifecycle.nativeReaped);assert.equal(s.gateway.requests.active,0);
assert.equal(s.completion.apiFailures,0);assert.equal(s.completion.unacquiredResultsInRecent,0);
assert.equal(s.completion.rejectedWorkflowCalls,2);assert.equal(s.completion.nativeToolFailures,1);
assert.equal(s.gateway.nativeToolFailures.recent[0].tool,'TaskStop');
assert.equal(s.gateway.nativeToolFailures.recent[0].source,'tool_result');
const blocks=r=>Array.isArray(r.message?.content)?r.message.content:typeof r.message?.content==='string'?[{type:'text',text:r.message.content}]:[];
const text=r=>blocks(r).filter(b=>b.type==='text').map(b=>b.text).join('\n');
const rootFile=path.join(project,id+'.jsonl'),root=rows(rootFile);
assert(root.some(r=>r.type==='assistant'&&text(r).trim()==='S44_FAILURE_RECOVERED_31'));
assert(root.some(r=>r.type==='assistant'&&text(r).trim()==='S44_FINAL_ALIVE_37'));
const dir=path.join(project,id,'workflows');
const workflows=fs.readdirSync(dir).filter(n=>/^wf_[\w-]+\.json$/.test(n)).map(n=>json(path.join(dir,n)));
assert.equal(workflows.length,2,'UNEXPECTED_WORKFLOW_EXECUTION');
const original=workflows.find(w=>w.runId==='wf_1fe400a8-48d');assert(original);assert.equal(original.status,'killed');
const resumed=workflows.find(w=>w!==original);assert.equal(resumed.status,'completed');
assert.equal(resumed.result.complete,false);
assert.deepEqual(resumed.result.results.map(r=>[r.step,r.state]),[['A','completed_result_reused'],['B','started_not_reexecuted'],['C','completed']]);
assert.equal(resumed.result.results[0].body,'S44_G_17');assert.equal(resumed.result.results[2].value,'S44_H_23');
const summarize=w=>{
 const d=path.join(project,id,'subagents/workflows',w.runId),journalFile=path.join(d,'journal.jsonl'),journal=rows(journalFile),started=journal.filter(r=>r.type==='started');
 const children=started.map(r=>{
  const file=path.join(d,'agent-'+r.agentId+'.jsonl'),rr=rows(file);
  const choice=s.gateway.agentSelections.recent.find(a=>a.agent===r.agentId);assert(choice?.state==='selection_verified');
  return {id:r.agentId,model:choice.effectiveModel,effort:choice.effectiveEffort,transcriptSha256:sha(file),tools:rr.flatMap(blocks).filter(b=>b.type==='tool_use').map(b=>b.name)};
 });
 return {run:w.runId,task:w.taskId,status:w.status,journalSha256:sha(journalFile),children};
};
const before=summarize(original),after=summarize(resumed);
assert.equal(before.children.length,2);assert.equal(after.children.length,1);
assert.deepEqual(before.children.map(c=>[c.model,c.effort]),[['gpt-5.6-sol','high'],['gpt-5.6-terra','medium']]);
assert.deepEqual(before.children[1].tools,['Bash']);
assert.deepEqual(after.children[0].tools,[]);assert.equal(after.children[0].model,'gpt-5.6-luna');assert.equal(after.children[0].effort,'max');
const processBefore=json(path.join(here,'b3-process-before-stop.json')),processAfter=json(path.join(here,'b3-process-after-stop.json'));
assert.equal(processBefore.pid,processAfter.pid);assert.equal(processBefore.start,processAfter.start);
assert.equal(processBefore.exited,false);assert.equal(processAfter.exited,true);assert(processAfter.runtimeMs<180000);
const counts=s.gateway.modelContexts.reduce((a,m)=>({matched:a.matched+m.observed.countMatches,mismatched:a.mismatched+m.observed.countMismatches}),{matched:0,mismatched:0});
assert.equal(counts.mismatched,0);
const report={id,commit:build.commit,binarySha256:manifest.sha256,native:s.gateway.client,statusSha256:sha(statusFile),transcriptSha256:sha(rootFile),
 completion:s.completion,lifecycle:s.lifecycle,counts,original:before,resumed:after,process:{before:processBefore,after:processAfter},
 toolFailure:s.gateway.nativeToolFailures.recent[0],
 acceptance:{deadline:'separate_native_fixture_evidence',workflowStopResume:'pass_for_observed_plan',toolFailureRecovery:'pass',
 workerRestrictionAdversarial:'not_reached: first call rejected; second attempt declined by parent',universal:false}};
fs.writeFileSync(path.join(here,'candidate-evidence.json'),JSON.stringify(report,null,2)+'\n');
console.log(JSON.stringify({id,counts,completion:s.completion,acceptance:report.acceptance}));
