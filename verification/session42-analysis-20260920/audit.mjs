// Read only this user-submitted run and its already approved public fixture profile.
// Persist derived facts and source hashes, never reasoning, image data or raw profiles.
import fs from 'node:fs';
import path from 'node:path';
import crypto from 'node:crypto';
import assert from 'node:assert/strict';
import {fileURLToPath} from 'node:url';
const output=path.dirname(fileURLToPath(import.meta.url));
const id='df320410-f4cd-443d-86f6-290ac649f634';
const base='D:/AIDEV/clauduct-s36-build/repair-20260919/verification/policy-repair-20260919';
const runDir=path.join(base,'interactive-'+id), approved=path.join(base,'interactive-e268b95d-2048-47e3-b36b-5483000541a7');
const sources=[];
function read(file){const size=fs.statSync(file).size;assert(size<32*1024*1024,'READ_LIMIT');const raw=fs.readFileSync(file);sources.push({file,bytes:size,sha256:crypto.createHash('sha256').update(raw).digest('hex')});return raw.toString('utf8').replace(/^\uFEFF/,'');}
const json=file=>JSON.parse(read(file));
const manifest=json(path.join(runDir,'run.json'));
assert.equal(manifest.id,id);assert.equal(path.resolve(manifest.profile),path.resolve(approved,'profile'));assert.equal(path.resolve(manifest.project),path.resolve(approved,'project'));
assert.equal(manifest.sha256,'bbc498a694fbfa39c74c9d6f2339d289b34ace6a51660247e0485c58cb94633b');assert.equal(manifest.state,'exited');
const project=path.join(approved,'profile/projects/D--AIDEV-clauduct-s36-build-repair-20260919-verification-policy-repair-20260919-interactive-e268b95d-2048-47e3-b36b-5483000541a7-project');
function transcript(file){return read(file).split(/\r?\n/).filter(Boolean).map((line,index)=>({line:index+1,row:JSON.parse(line)}));}
const root=transcript(path.join(project,id+'.jsonl'));
const children=fs.readdirSync(path.join(project,id,'subagents')).filter(n=>/^agent-[a-f0-9]+\.jsonl$/.test(n)).map(n=>({file:n,rows:transcript(path.join(project,id,'subagents',n))}));
const blocks=r=>typeof r.message?.content==='string'?[{type:'text',text:r.message.content}]:Array.isArray(r.message?.content)?r.message.content:[];
const texts=r=>blocks(r).filter(b=>b.type==='text'&&typeof b.text==='string').map(b=>b.text);
const plain=r=>texts(r).join('\n');
const status=json(path.join(runDir,'tmp/clauduct/status-18872.json'));
const snapshots=fs.readdirSync(path.join(runDir,'checkpoints-s42')).filter(n=>/^[A-Z0-9-]+\.json$/.test(n)).map(n=>({name:n,status:json(path.join(runDir,'checkpoints-s42',n))}));
const requests=new Map();
for(const s of [status,...snapshots.map(s=>s.status)])for(const r of [...s.gateway.recent,...s.gateway.recentFailures]){
 const previous=requests.get(r.seq);if(!previous||(r.endedMs||0)>(previous.endedMs||0))requests.set(r.seq,r);
}
const observed=[...requests.values()].sort((a,b)=>a.seq-b.seq), request=seq=>{assert(requests.has(seq),'REQUEST_NOT_RETAINED '+seq);return requests.get(seq);};
const userPrompts=root.filter(({row:r})=>r.type==='user'&&!r.isCompactSummary&&/^S42_[A-Z_]+\./.test(plain(r))).map(({line,row:r})=>({line,at:r.timestamp,marker:plain(r).match(/^S42_[A-Z_]+/)[0],uuid:r.uuid,sha256:crypto.createHash('sha256').update(plain(r)).digest('hex')}));
const answers=root.filter(({row:r})=>r.type==='assistant').flatMap(({line,row:r})=>texts(r).map(text=>({line,at:r.timestamp,text})));
const markerEvidence=(marker,prompt)=>{
 const start=userPrompts.find(p=>p.marker===prompt);assert(start,'RECOVERY_PROMPT_MISSING');
 const end=userPrompts.find(p=>p.line>start.line)?.line??Infinity;
 return answers.filter(a=>a.line>start.line&&a.line<end&&a.text.includes(marker)).map(a=>({line:a.line,at:a.at}));
};
const skill=root.find(({row:r})=>r.type==='user'&&plain(r).startsWith('# Workflow authoring reference'));assert(skill);
const contract=plain(skill.row), optionLine=contract.split('\n').find(l=>l.startsWith('- agent(prompt:'));assert(optionLine);
const compactionNative=root.filter(({row:r})=>r.subtype==='compact_boundary').map(({line,row:r})=>({line,at:r.timestamp,trigger:r.compactMetadata.trigger,durationMs:r.compactMetadata.durationMs,preTokens:r.compactMetadata.preTokens,postTokens:r.compactMetadata.postTokens}));
const compactions=observed.filter(r=>r.kind==='compaction').map((r,i)=>({seq:r.seq,model:r.model,effort:r.effort,input:r.backendInputTokens,counted:r.countedInputTokens,countMs:r.countMs,totalMs:r.endedMs-r.startedMs,toFirstByteMs:r.firstByteMs-r.startedMs,afterFirstByteMs:r.endedMs-r.firstByteMs,textDeltas:r.backendProgress.textDeltas,native:compactionNative[i],nextMain:observed.find(x=>x.seq>r.seq&&x.nativeRequestClass==='main'&&x.outcome==='ok')}));
assert.equal(compactions.length,2);assert.equal(compactionNative.length,2);for(const c of compactions){assert.equal(c.model,'gpt-5.6-sol');assert.equal(c.effort,'high');assert.equal(c.input,c.counted);assert.equal(c.native.trigger,'manual');}
const summaries=root.filter(({row:r})=>r.isCompactSummary).map(({line,row:r})=>({line,characters:plain(r).length,ledgerIDs:[...new Set(plain(r).match(/\bI\d{2}\b/g)||[])].sort(),rootMarker:plain(r).includes('CEDAR-58'),forkMarker:plain(r).includes('INDIGO-76'),forkID:plain(r).includes('a4607dbae2a1a6689')}));
const commands=root.filter(({row:r})=>r.type==='user'&&(plain(r).startsWith('<local-command-stdout>Set model')||plain(r).startsWith('/compact'))).map(({line,row:r})=>({line,at:r.timestamp,text:plain(r)}));
const contextReports=root.filter(({row:r})=>r.type==='user'&&plain(r).startsWith('## Context Usage')).map(({line,row:r})=>({line,at:r.timestamp,model:plain(r).match(/\*\*Model:\*\* ([^ \n]+)/)?.[1],tokens:plain(r).match(/\*\*Tokens:\*\* ([^\n]+)/)?.[1],messages:plain(r).split('\n').find(l=>/^\| Messages /.test(l))}));
const recovery=Object.fromEntries([['S42_EARLY_RECOVERED','S42_RECOVER_EARLY'],['S42_TEXT_RECOVERED','S42_RECOVER_TEXT'],['S42_REFUSAL_RECOVERED','S42_AFTER_REFUSAL']].map(([m,p])=>[m,markerEvidence(m,p)]));
assert(recovery.S42_EARLY_RECOVERED.length);assert(recovery.S42_TEXT_RECOVERED.length);assert.equal(recovery.S42_REFUSAL_RECOVERED.length,0);
assert.equal(request(270).category,'WORKFLOW_RECOVERY_UNVERIFIED');assert.equal(request(272).category,'WORKFLOW_RECOVERY_UNVERIFIED');
const readCall=root.flatMap(({line,row:r})=>blocks(r).filter(b=>r.type==='assistant'&&b.type==='tool_use'&&b.id==='call_VMfGQcfBn2fmB5pOOcbitaq2').map(b=>({line,at:r.timestamp,id:b.id,name:b.name})))[0];
const readResult=root.find(({row:r})=>blocks(r).some(b=>b.type==='tool_result'&&b.tool_use_id===readCall.id));assert(readResult);const cachedRead=blocks(readResult.row).find(b=>b.type==='tool_result'&&b.tool_use_id===readCall.id);assert.equal(typeof cachedRead.content,'string');
const cancellationGroups=Object.groupBy(status.gateway.recentFailures.filter(r=>r.category==='CANCELLED'),r=>r.nativeRequestClass);
assert.equal(cancellationGroups.auxiliary.length,3);assert.equal(cancellationGroups.subagent.length,1);assert.equal(cancellationGroups.main.length,5);
const matches=status.gateway.modelContexts.reduce((s,m)=>s+m.observed.countMatches,0),mismatches=status.gateway.modelContexts.reduce((s,m)=>s+m.observed.countMismatches,0),preflights=status.gateway.modelContexts.reduce((s,m)=>s+m.observed.preflights,0),cacheHits=status.gateway.modelContexts.reduce((s,m)=>s+m.observed.countCacheHits,0);
const warmups=Object.entries(status.gateway.totals.kindRoutes).filter(([k])=>k.startsWith('count_tokens/')&&k.endsWith('/backend-count-warmup')).reduce((s,[,n])=>s+n,0);
assert.equal(status.attempts,status.inferences+warmups+preflights-cacheHits);
const toolCalls=root.flatMap(({line,row:r})=>blocks(r).filter(b=>r.type==='assistant'&&b.type==='tool_use').map(b=>({line,at:r.timestamp,name:b.name,id:b.id,role:b.input?.subagent_type,to:b.input?.to||b.input?.recipient})));
const requestsCompact=observed.map(r=>({seq:r.seq,kind:r.kind,class:r.nativeRequestClass,model:r.model,effort:r.effort,agent:r.agentId,outcome:r.outcome,category:r.category,input:r.backendInputTokens,counted:r.countedInputTokens,countMs:r.countMs,startedMs:r.startedMs,firstByteMs:r.firstByteMs,endedMs:r.endedMs,checks:r.verifiedChecks,backendProgress:r.backendProgress}));
const result={id,productCommit:'34ebcb1608e0dc712a9ae9f887427fdc48a3bcfd',binarySha256:manifest.sha256,nativeVersion:status.gateway.client.version,rootRows:root.length,childFiles:children.length,childRows:children.reduce((n,c)=>n+c.rows.length,0),retainedRequests:observed.length,totalRequests:status.gateway.requests.received,completion:status.completion,lifecycle:status.lifecycle,commands,userPrompts,toolCalls,workflowContract:{line:skill.line,signature:optionLine.split(' — ')[0],toolsOptionMentioned:/\btools\??\s*:/.test(optionLine),maxTurnsMentioned:contract.includes('maxTurns'),workflowToolCalls:toolCalls.filter(t=>t.name==='Workflow').length,workflowAgentRequests:status.gateway.features.find(f=>f.name==='workflow_agent').requests},compactions,summaries,contextReports,recovery,cachedReadRecovery:{call:readCall,resultLine:readResult.line,result:cachedRead.content},cancellations:Object.fromEntries(Object.entries(cancellationGroups).map(([k,v])=>[k,v.map(r=>r.seq)])),counting:{matches,mismatches,preflights,cacheHits,warmups,totalAttempts:status.attempts,inferences:status.inferences,reconciledAttempts:status.inferences+warmups+preflights-cacheHits},selections:status.gateway.agentSelections.recent,features:status.gateway.features,checkpoints:snapshots.map(({name,status:s})=>({name,at:s.lifecycle.observedAt,requests:s.gateway.requests.received,completion:s.completion,kinds:s.gateway.totals.kinds,contextDisplay:s.gateway.contextDisplay})),requests:requestsCompact,sources,limitations:['No saved raw backend output arguments for rejected Workflow calls; model motive and exact second-call input remain unverified.','Actual offered child Agent role schema was not saved in this user run.','Native context estimates are separate from backend exact preflight counts.','User keypresses are not recorded; repeated input origin needs user confirmation.']};
result.userConfirmation={source:'current user reply to S42 repeated-submission question',earlyEscRepeatedSubmissions:'manually submitted and cancelled multiple times'};
result.limitations[result.limitations.length-1]='Individual keypress timestamps are not recorded; the user confirmed the repeated early-Esc submissions were manual.';
fs.writeFileSync(path.join(output,'evidence.json'),JSON.stringify(result,null,2)+'\n');
console.log(JSON.stringify({id,rootRows:result.rootRows,childFiles:result.childFiles,childRows:result.childRows,retainedRequests:result.retainedRequests,workflowContract:result.workflowContract,compactions:compactions.map(({nextMain,...c})=>({...c,nextMain:{seq:nextMain.seq,model:nextMain.model,effort:nextMain.effort,input:nextMain.backendInputTokens}})),recovery,cancellations:result.cancellations,counting:result.counting,summaries,contextReports,commands},null,2));
