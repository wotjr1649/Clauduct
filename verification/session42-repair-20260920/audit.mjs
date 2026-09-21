// Local, data-only audit of two public-fixture TUI runs. No transcript bodies copied.
import fs from 'node:fs';
import path from 'node:path';
import crypto from 'node:crypto';
import assert from 'node:assert/strict';
import {fileURLToPath} from 'node:url';
const root=path.dirname(fileURLToPath(import.meta.url));
const base='D:/AIDEV/clauduct-s36-build/repair-20260919/verification/policy-repair-20260919';
const approved=path.join(base,'interactive-e268b95d-2048-47e3-b36b-5483000541a7');
const slug='D--AIDEV-clauduct-s36-build-repair-20260919-verification-policy-repair-20260919-interactive-e268b95d-2048-47e3-b36b-5483000541a7-project';
const cases=[['4d465e2d-370e-4f6a-a5ae-532e416ea656','build-632bb6e.json'],['16dfd222-e9b6-44a7-94d6-27f383d84850','build.json']];
const read=f=>{assert(fs.statSync(f).size<16*1024*1024);return fs.readFileSync(f,'utf8');};
const json=f=>JSON.parse(read(f)),rows=f=>read(f).split('\n').filter(Boolean).map(JSON.parse);
const text=r=>typeof r.message?.content==='string'?r.message.content:(r.message?.content??[]).filter(b=>b.type==='text').map(b=>b.text).join('\n');
const calls=r=>Array.isArray(r.message?.content)?r.message.content.filter(b=>b.type==='tool_use'):[];
const digest=f=>crypto.createHash('sha256').update(read(f)).digest('hex');
const reports=[];
for(const [id,buildName] of cases){
  const savedFile=path.join(root,'run-'+id+'.json');if(!fs.existsSync(savedFile))continue;
  const {run:m,status:s}=json(savedFile),build=json(path.join(root,buildName));
  assert.equal(path.resolve(m.profile),path.resolve(approved,'profile'));assert.equal(path.resolve(m.project),path.resolve(approved,'project'));
  assert.equal(path.resolve(m.binary),path.resolve(root,'bin/clauduct.exe'));assert.equal(m.sha256,build.artifacts['clauduct.exe']);
  assert.equal(m.state,'exited');assert.equal(m.exitCode,0);assert(s.lifecycle.final&&s.lifecycle.nativeReaped);assert(!m.watchdogForced);
  const project=path.join(m.profile,'projects',slug),file=path.join(project,id+'.jsonl'),tr=rows(file),directory=path.join(project,id);
  const proof={id,commit:build.commit,binarySha256:m.sha256,transcriptSha256:digest(file),rootRows:tr.length,completion:s.completion,
    requests:s.gateway.requests.received,attempts:s.attempts,inferences:s.inferences,modelContexts:s.gateway.modelContexts,
    matches:s.gateway.modelContexts.reduce((a,c)=>a+c.observed.countMatches,0),mismatches:s.gateway.modelContexts.reduce((a,c)=>a+c.observed.countMismatches,0),
    children:[],workflowRecoveries:[],compactions:[],commands:[],recoveryAnswers:[],sendMessages:[],stageAnswers:[],denialResults:[]};
  assert.equal(s.completion.apiFailures,0);assert.equal(s.completion.unacquiredResultsInRecent,0);assert.equal(proof.mismatches,0);
  for(let i=0;i<tr.length;i++){
    const r=tr[i],t=text(r);
    if(r.type==='user'&&t.includes('<command-name>'))proof.commands.push({line:i+1,time:r.timestamp,name:t.match(/<command-name>(.*?)<\/command-name>/s)?.[1]});
    if(r.isCompactSummary)proof.compactions.push({line:i+1,time:r.timestamp,summaryChars:t.length,words:t.trim().split(/\s+/).length,retained:['CEDAR-84','R1','R2','a1eb4f6ba346dd0c4','ac94e0c902d03b318','AMBER-17','INDIGO-28','wf_6f48df99-04f'].filter(x=>t.includes(x))});
    if(r.type==='assistant'&&/PUBLIC_S43_(RECOVERED|FINAL_RECOVERED)/.test(t))proof.recoveryAnswers.push({line:i+1,time:r.timestamp,markers:t.match(/PUBLIC_S43_(?:FINAL_)?RECOVERED/g),fir:t.includes('FIR-62'),cedar:t.includes('CEDAR-84'),answer43:/\b43\b/.test(t)});
    if(r.type==='assistant'&&/PUBLIC_S43_(BOTH_RESUMED|POST_COMPACT_OK)/.test(t))proof.stageAnswers.push({line:i+1,time:r.timestamp,combined:t.includes('PUBLIC_S43_BOTH_RESUMED'),postCompact:t.includes('PUBLIC_S43_POST_COMPACT_OK'),cedar:t.includes('CEDAR-84'),r2:t.includes('R2'),amber46:/AMBER-17[\s*`|]*46/.test(t),indigo64:/INDIGO-28[\s*`|]*64/.test(t)});
    if(r.type==='user'&&Array.isArray(r.message?.content))for(const c of r.message.content){if(c.type==='tool_result'&&c.is_error&&JSON.stringify(c.content).includes('PreToolUse:Workflow hook error: WORKFLOW_RECOVERY_UNVERIFIED'))proof.denialResults.push({line:i+1,time:r.timestamp,call:c.tool_use_id});}
    for(const c of calls(r))if(c.name==='SendMessage')proof.sendMessages.push({line:i+1,time:r.timestamp,message:r.message.id,call:c.id,to:c.input.to});
  }
  const childDir=path.join(directory,'subagents');
  for(const n of fs.readdirSync(childDir).filter(n=>/^agent-[a-f0-9]+\.jsonl$/.test(n))){
    const f=path.join(childDir,n),ch=rows(f),agent=n.slice(6,-6);
    proof.children.push({agent,sha256:digest(f),rows:ch.length,selection:s.gateway.agentSelections.recent.find(x=>x.agent===agent),
      agentCalls:ch.flatMap((r,i)=>calls(r).filter(c=>c.name==='Agent').map(c=>({line:i+1,role:c.input.subagent_type,call:c.id}))),
      discoveries:ch.flatMap((r,i)=>calls(r).filter(c=>c.name==='ToolSearch').map(c=>({line:i+1}))),
      listingLines:ch.flatMap((r,i)=>JSON.stringify(r).includes('agent_listing_delta')?[i+1]:[]),
      arithmeticAnswers:ch.flatMap((r,i)=>r.type==='assistant'&&/^(?:(?:AMBER-17|INDIGO-28) )?(?:37|53|46|64|100)$/.test(text(r).trim())?[{line:i+1,time:r.timestamp,value:Number(text(r).trim().split(' ').at(-1))}]:[])});
  }
  const scripts=path.join(directory,'workflows/scripts');
  if(fs.existsSync(scripts))for(const n of fs.readdirSync(scripts).filter(n=>n.startsWith('clauduct-recovered-results-')&&n.endsWith('.js'))){
    const script=read(path.join(scripts,n)),prefix='\nreturn JSON.parse(',start=script.indexOf(prefix);assert(start>0&&script.endsWith(');'));
    const data=JSON.parse(JSON.parse(script.slice(start+prefix.length,-2))),recovery=n.slice('clauduct-recovered-results-'.length,-3);
    const journal=rows(path.join(directory,'subagents/workflows',recovery,'journal.jsonl'));
    assert.equal(data.newAgentExecutions,0);assert.equal(journal.filter(e=>e.type==='started').length,0);
    const original=rows(path.join(directory,'subagents/workflows',data.sourceRunId,'journal.jsonl'));
    proof.workflowRecoveries.push({source:data.sourceRunId,recovery,newStarted:0,originalOrder:original.map(e=>({type:e.type,agent:e.agentId})),
      results:data.results.map(r=>({agent:r.agentId,state:r.state,value:JSON.parse(r.body).value}))});
  }
  const records=new Map(s.gateway.recent.map(r=>[r.seq,r]));
  for(const n of fs.readdirSync(root).filter(n=>/^checkpoint-.*\.json$/.test(n))){const c=json(path.join(root,n));if(c.id===id)for(const r of c.status.gateway.recent)records.set(r.seq,r);}
  proof.detailedCompactionRequests=[...records.values()].filter(r=>r.kind==='compaction');
  proof.resumedRequests=[...records.values()].filter(r=>r.source==='verified-resume');
  if(id==='16dfd222-e9b6-44a7-94d6-27f383d84850'){
    const a=proof.resumedRequests.find(r=>r.agentId==='a1eb4f6ba346dd0c4'),b=proof.resumedRequests.find(r=>r.agentId==='ac94e0c902d03b318');
    assert(a&&b);assert.equal(a.model,'gpt-5.6-sol');assert.equal(a.effort,'high');assert.equal(b.model,'gpt-5.6-terra');assert.equal(b.effort,'medium');
    proof.resumedOverlapMs=Math.min(a.endedMs,b.endedMs)-Math.max(a.startedMs,b.startedMs);assert(proof.resumedOverlapMs>0);
    assert(proof.stageAnswers.some(r=>r.combined&&r.cedar&&r.r2&&r.amber46&&r.indigo64));
    assert(proof.stageAnswers.some(r=>r.postCompact&&r.cedar&&r.r2&&r.amber46&&r.indigo64));
    assert.equal(proof.detailedCompactionRequests.length,2);assert(proof.detailedCompactionRequests.every(r=>r.outcome==='ok'&&r.model==='gpt-5.6-sol'&&r.effort==='high'&&r.countAgreement==='matched'));
    assert(s.gateway.agentContexts.some(c=>!c.agentId&&c.model==='gpt-5.6-terra'&&c.effort==='medium'));
    proof.prematureReport={claimLine:44,completionNoticeLine:46,correctionLine:47};assert(text(tr[46]).includes('premature'));assert(text(tr[45]).includes('task-notification'));
  }
  proof.rejectedWorkflowCalls=s.gateway.totals.rejectedWorkflowCalls??0;
  assert.equal(proof.denialResults.length,1);assert.equal(proof.recoveryAnswers.length,1);assert(proof.denialResults[0].line<proof.recoveryAnswers[0].line);
  reports.push(proof);
}
assert(reports.length>0);
fs.writeFileSync(path.join(root,'tui-audit.json'),JSON.stringify({runs:reports,scope:'Observed public-fixture native TUI and subscription backend; no universal acceptance claim'},null,2)+'\n');
console.log(JSON.stringify(reports.map(r=>({id:r.id,completion:r.completion,matches:r.matches,compactions:r.compactions,workflows:r.workflowRecoveries,recoveries:r.recoveryAnswers}))));
