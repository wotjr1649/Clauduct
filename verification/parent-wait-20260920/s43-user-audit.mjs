// Fixed approved public-fixture run. Read inert JSON only; no command execution,
// network, credential access, profile changes or copying of transcript bodies.
import fs from 'node:fs';
import path from 'node:path';
import crypto from 'node:crypto';
import assert from 'node:assert/strict';
import {fileURLToPath} from 'node:url';
const root=path.dirname(fileURLToPath(import.meta.url)),id='8544df8b-bc32-4df0-bffe-fce86176e3e1';
const base='D:/AIDEV/clauduct-s36-build/repair-20260919/verification/policy-repair-20260919';
const approved=path.join(base,'interactive-e268b95d-2048-47e3-b36b-5483000541a7'),runDir=path.join(base,'interactive-'+id);
const read=f=>{assert(fs.statSync(f).size<16*1024*1024);return fs.readFileSync(f,'utf8');},json=f=>JSON.parse(read(f)),rows=f=>read(f).split('\n').filter(Boolean).map(JSON.parse);
const hash=f=>crypto.createHash('sha256').update(read(f)).digest('hex');
const m=json(path.join(runDir,'run.json')),build=json(path.join(root,'build.json'));
assert.equal(path.resolve(m.profile),path.resolve(approved,'profile'));assert.equal(path.resolve(m.project),path.resolve(approved,'project'));assert.equal(m.id,id);assert.equal(m.sha256,build.artifacts['clauduct.exe']);assert.deepEqual(m.artifacts,build.artifacts);assert.equal(m.state,'exited');
const sf=path.join(runDir,'tmp/clauduct/status-15756.json'),status=json(sf);assert(status.lifecycle.final);assert.equal(status.exitCode,0);
const project=path.join(approved,'profile/projects/D--AIDEV-clauduct-s36-build-repair-20260919-verification-policy-repair-20260919-interactive-e268b95d-2048-47e3-b36b-5483000541a7-project');
const file=path.join(project,id+'.jsonl'),tr=rows(file),dir=path.join(project,id);
const text=r=>typeof r.message?.content==='string'?r.message.content:(r.message?.content??[]).filter(b=>b.type==='text').map(b=>b.text).join('\n');
const calls=r=>Array.isArray(r.message?.content)?r.message.content.filter(b=>b.type==='tool_use'):[];
const results=r=>Array.isArray(r.message?.content)?r.message.content.filter(b=>b.type==='tool_result'):[];
const markers=t=>[...new Set((t??'').match(/(?:S43_[A-Z0-9_]+|PUBLIC_[A-Z0-9_]+|ORCHID-63)/g)??[])];
const summary=(rr)=>rr.map((r,i)=>({line:i+1,time:r.timestamp,type:r.type,markers:markers(text(r)),command:text(r).match(/<command-name>(.*?)<\/command-name>/s)?.[1],compact:!!r.isCompactSummary,
 calls:calls(r).map(c=>({name:c.name,id:c.id,role:c.input.subagent_type,model:c.input.model,effort:c.input.effort,background:c.input.run_in_background,to:c.input.to,task:c.input.task_id,plan:c.name==='Workflow'&&c.input.script==='clauduct:plan-v1',resume:c.input.resumeFromRunId})),
 results:results(r).map(c=>({call:c.tool_use_id,error:!!c.is_error,markers:markers(JSON.stringify(c.content)),denied:/denied|denial|rejected|not authorized|PreToolUse:Workflow hook error/i.test(JSON.stringify(c.content)),nativeRejection:r.toolUseResult==='User rejected tool use'}))}));
const checkpoints=fs.readdirSync(path.join(runDir,'checkpoints-s43')).filter(n=>/^[a-z0-9-]+\.json$/.test(n)).map(n=>{const f=path.join(runDir,'checkpoints-s43',n),c=json(f);assert.equal(c.id,id);return {name:n,sha256:hash(f),status:c.status};}).sort((a,b)=>a.status.gateway.uptimeMs-b.status.gateway.uptimeMs);
const records=new Map();for(const s of [...checkpoints.map(c=>c.status),status])for(const r of s.gateway.recent)records.set(r.seq,r);
const p={id,commit:build.commit,binarySha256:m.sha256,native:status.gateway.client,completion:status.completion,lifecycle:status.lifecycle,requests:status.gateway.requests.received,inferences:status.inferences,rootRows:tr.length,rootSha256:hash(file),root:summary(tr),
 checkpoints:checkpoints.map(c=>({name:c.name,sha256:c.sha256,uptimeMs:c.status.gateway.uptimeMs,context:c.status.gateway.contextDisplay})),records:[...records.values()].sort((a,b)=>a.seq-b.seq),
 features:status.gateway.features,modelContexts:status.gateway.modelContexts,agentSelections:status.gateway.agentSelections,children:[],workflows:[],contextReports:[],compactions:[]};
for(let i=0;i<tr.length;i++){const r=tr[i],t=text(r);if(t.includes('Context Usage')&&t.includes('Estimated usage by category'))p.contextReports.push({line:i+1,time:r.timestamp,model:t.match(/(?:gpt-6-astra|gpt-5\.6-(?:sol|terra|luna))/)?.[0],usage:t.match(/\*\*Tokens:\*\*\s*([^\n]+)/)?.[1]?.trim(),messages:t.match(/\| Messages\s*\|\s*([^|]+)/)?.[1]?.trim()});if(r.isCompactSummary)p.compactions.push({line:i+1,time:r.timestamp,chars:t.length,markers:markers(t)});}
const sub=path.join(dir,'subagents');
for(const n of fs.readdirSync(sub).filter(n=>/^agent-[a-f0-9]+\.jsonl$/.test(n))){const f=path.join(sub,n),rr=rows(f);p.children.push({agent:n.slice(6,-6),sha256:hash(f),rows:rr.length,events:summary(rr)});}
const wd=path.join(dir,'workflows');
for(const n of fs.readdirSync(wd).filter(n=>/^wf_[A-Za-z0-9_-]+\.json$/.test(n))){const f=path.join(wd,n),w=json(f),jd=path.join(sub,'workflows',w.runId),jf=path.join(jd,'journal.jsonl'),jr=rows(jf);
 p.workflows.push({run:w.runId,task:w.taskId,status:w.status,start:w.startTime,durationMs:w.durationMs,sha256:hash(f),journalSha256:hash(jf),complete:w.result?.complete,completeMeaning:w.result?.completeMeaning,results:w.result?.results?.map(r=>({step:r.step,state:r.state,agent:r.agentId,markers:markers(typeof r.value==='string'?r.value:r.body)})),journal:jr.map(r=>({type:r.type,key:r.key,agent:r.agentId,step:r.label?.match(/^clauduct-step:([^ ]+)/)?.[1],markers:markers(typeof r.result==='string'?r.result:undefined)})),children:fs.readdirSync(jd).filter(n=>/^agent-[a-f0-9]+\.jsonl$/.test(n)).map(n=>{const f=path.join(jd,n);return {agent:n.slice(6,-6),sha256:hash(f),events:summary(rows(f))};})});
}
// Assertions pin this human run; they do not manufacture missing test conditions.
const row=n=>{assert(tr[n-1]);return tr[n-1];},ms=(a,b)=>Date.parse(b)-Date.parse(a);
const child=id=>{const c=p.children.find(c=>c.agent===id);assert(c);return c;};
const rec=n=>{const r=records.get(n);assert(r);return r;};
const hasAnswer=(n,marker)=>{assert.equal(row(n).type,'assistant');assert(!row(n).isCompactSummary);assert(text(row(n)).includes(marker));};
assert.equal(p.rootRows,258);assert.equal(p.checkpoints.length,11);
assert.deepEqual(p.contextReports.slice(0,3).map(c=>c.usage),Array(3).fill('10.1k / 500k (2%)'));
assert.deepEqual(p.contextReports.slice(0,3).map(c=>c.messages),['19','14','14']);
const leaf=child('a3aefbeedb6fa78ad');
const leafResult=leaf.events.find(e=>e.results.some(r=>r.markers.includes('S43_H_LEAF_41')));assert(leafResult);
const newInput=row(39);assert.equal(newInput.type,'user');assert(text(newInput).includes('S43_H_INPUT'));
const newInputBeforeLeafResultMs=ms(newInput.timestamp,leafResult.time);assert.equal(newInputBeforeLeafResultMs,3038);
for(const n of [65,67]){assert(rec(n).parentReadiness.pending.length);assert.equal(rec(n).parentReadiness.replyWithheld,false);}
hasAnswer(46,'S43_H_INPUT_OK');hasAnswer(51,'S43_H_ROOT_43');hasAnswer(77,'S43_H_PARALLEL_OK');assert(text(row(77)).includes('180'));
const inherited=[['a3aefbeedb6fa78ad','abdb30f0520c339ab','gpt-5.6-terra','medium'],['a0a5b3a6c6e690f2f','a6febef2c21fb620c','gpt-5.6-sol','high'],['aabfa5c7c8ee06c2d','ad95955fff50d8061','gpt-5.6-terra','medium']];
for(const [id,parent,model,effort] of inherited){const s=p.agentSelections.recent.find(s=>s.agent===id);assert(s);assert.equal(s.parent,parent);assert.equal(s.requestPresenceVerified,true);assert.equal(s.modelProvided,false);assert.equal(s.effortProvided,false);assert.equal(s.effectiveModel,model);assert.equal(s.effectiveEffort,effort);assert.equal(child(parent).events.flatMap(e=>e.calls).filter(c=>c.name==='Agent').length,1);}
assert.equal(ms(row(56).timestamp,row(57).timestamp),22);
assert.equal(p.workflows.length,2);
const original=p.workflows.find(w=>w.run==='wf_4059a300-94c'),resumed=p.workflows.find(w=>w.run==='wf_9200ee3c-ba3');assert(original&&resumed);
assert.deepEqual(original.journal.filter(j=>j.type==='started').map(j=>j.step),['A','B']);
assert.deepEqual(resumed.journal.filter(j=>j.type==='started').map(j=>j.step),['C']);
assert.deepEqual(resumed.results.map(r=>r.state),['completed_result_reused','started_not_reexecuted','completed']);assert.equal(resumed.complete,false);
const workerB=original.children.find(c=>c.agent==='ae6308c37098eab46'),workerC=resumed.children[0];
assert.equal(workerB.events.flatMap(e=>e.calls).filter(c=>c.name==='Bash').length,1);
assert(workerB.events.some(e=>e.results.some(r=>r.nativeRejection&&r.error)));
assert.equal(workerC.events.flatMap(e=>e.calls).length,0);assert(workerC.events.some(e=>e.type==='assistant'&&e.markers.includes('S43_H_C_23')));
assert.equal(p.root.flatMap(e=>e.calls).filter(c=>c.name==='Workflow').length,3);
assert.equal(status.completion.rejectedWorkflowCalls,1);hasAnswer(151,'S43_H_AFTER_REJECT_OK');
const compactEvidence=[[154,163,127],[195,204,132]].map(([start,end,seq])=>{const r=rec(seq);assert.equal(r.kind,'compaction');assert.equal(r.model,'gpt-5.6-sol');assert.equal(r.effort,'high');assert.equal(r.countAgreement,'matched');return {seq,requested:r.requested,model:r.model,effort:r.effort,transcriptMs:ms(row(start).timestamp,row(end).timestamp),requestMs:r.endedMs-r.startedMs};});
assert.equal(p.compactions.length,2);assert(text(row(193)).includes('terra'));assert.equal(p.root.slice(193,194).filter(r=>r.type==='user').length,0);
hasAnswer(181,'S43_H_AFTER_COMPACT_OK');hasAnswer(222,'S43_H_SECOND_COMPACT_OK');
const cancellations=[[137,232,234,'S43_H_EARLY_RECOVERED'],[141,244,250,'S43_H_FINAL_RECOVERED']].map(([seq,start,end,marker])=>{const r=rec(seq);assert.equal(r.category,'CANCELLED');assert.equal(r.streamEnd.clientContext,'cancelled');assert(r.backendProgress.textDeltas>0);assert.equal(r.parentReadiness.replyWithheld,false);hasAnswer(end,marker);assert(text(row(end)).includes('ORCHID-63'));return {seq,textDeltas:r.backendProgress.textDeltas,firstByteBeforeCancellationMs:r.endedMs-r.firstByteMs,recoveryMs:ms(row(start).timestamp,row(end).timestamp),preTextCancellationTested:false};});
const noPoll=[...p.root,...p.children.flatMap(c=>c.events),...p.workflows.flatMap(w=>w.children.flatMap(c=>c.events))].flatMap(e=>e.calls).filter(c=>c.name==='TaskOutput');assert.equal(noPoll.length,0);
const matches=p.modelContexts.reduce((n,c)=>n+c.observed.countMatches,0),mismatches=p.modelContexts.reduce((n,c)=>n+c.observed.countMismatches,0);assert.equal(matches,59);assert.equal(mismatches,0);assert.equal(status.completion.apiFailures,0);assert.equal(status.completion.unacquiredResultsInRecent,0);
p.findings={auditAssertions:'passed_for_this_recorded_run',newInputBeforeLeafResultMs,parallelAgentCallGapMs:22,compactEvidence,cancellations,usageAgreement:{matches,mismatches},workflowBOSCommandStartedBeforeStop:'unverified_native_tool_rejection',workflowCToolCalls:0,taskOutputCalls:0,naturalEmptyReplyTested:false,firstPaintTimingMeasured:false};
fs.writeFileSync(path.join(root,'s43-user-audit.json'),JSON.stringify(p,null,2)+'\n');
if(process.argv[2]==='root')for(let i=0;i<tr.length;i++){const r=tr[i],t=text(r);if((r.type==='assistant'||r.type==='user')&&t&&!r.isCompactSummary&&!t.startsWith('# Workflow authoring reference'))console.log(JSON.stringify({line:i+1,time:r.timestamp,type:r.type,text:t.replace(/\[clauduct-compact:[^\]]+\]/g,'[REDACTED_CONTROL]').replace(/[A-Za-z0-9_+-]{80,}/g,'[REDACTED_LONG_VALUE]').slice(0,process.argv[3]==='all'?10000:1600)}));}
else console.log(JSON.stringify({id,rootRows:p.rootRows,checkpoints:p.checkpoints,contexts:p.contextReports,compactions:p.compactions,workflows:p.workflows.map(w=>({run:w.run,task:w.task,status:w.status,complete:w.complete,results:w.results,journal:w.journal})),counts:status.gateway.modelContexts.map(c=>({model:c.model,...c.observed}))}));
