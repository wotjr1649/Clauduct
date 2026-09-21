// Read this explicitly supplied S44 session only; never execute recorded scripts.
import fs from 'node:fs';import path from 'node:path';import {fileURLToPath} from 'node:url';
import assert from 'node:assert/strict';import crypto from 'node:crypto';
const here=path.dirname(fileURLToPath(import.meta.url)),id='97c86a99-71c3-4a4b-b639-05f8fbe30423';
const run=path.join(here,'tui-'+id),meta=JSON.parse(fs.readFileSync(path.join(run,'session.json'),'utf8'));
if(meta.id!==id)throw Error('SESSION_IDENTITY');
const approved='D:/AIDEV/clauduct-s36-build/repair-20260919/verification/policy-repair-20260919/interactive-e268b95d-2048-47e3-b36b-5483000541a7';
if(path.resolve(meta.profile)!==path.resolve(approved,'profile')||path.resolve(meta.project)!==path.resolve(approved,'project'))throw Error('SESSION_SCOPE');
const projects=path.join(meta.profile,'projects'),parents=fs.readdirSync(projects).filter(n=>fs.existsSync(path.join(projects,n,id+'.jsonl')));
if(parents.length!==1)throw Error('TRANSCRIPT_IDENTITY');
const base=path.join(projects,parents[0],id);
const read=file=>{if(fs.statSync(file).size>32<<20)throw Error('INPUT_BOUND');return fs.readFileSync(file,'utf8');};
const mode=process.argv[2]??'root',start=Number(process.argv[3]??0),end=Number(process.argv[4]??100000);
const emit=value=>console.log(JSON.stringify(value).replace(/clauduct-compact:[A-Za-z0-9_-]+/g,'clauduct-compact:[REDACTED]'));
function show(file){const rows=read(file).trim().split(/\r?\n/).filter(Boolean).map(s=>JSON.parse(s));emit({file,rows:rows.length});for(let i=start;i<Math.min(end,rows.length);i++){const r=rows[i];let content=r.message?.content??r.content;if(Array.isArray(content))content=content.filter(b=>!['thinking','redacted_thinking'].includes(b.type));emit({n:i+1,time:r.timestamp,type:r.type,subtype:r.subtype,role:r.message?.role,content,attachmentType:r.attachment?.type,toolUseResult:r.toolUseResult,compactMetadata:r.compactMetadata,durationMs:r.durationMs});}}
if(mode==='root')show(base+'.jsonl');
else if(mode==='children'){for(const n of fs.readdirSync(path.join(base,'subagents')).filter(n=>/^agent-[a-z0-9]+\.jsonl$/.test(n)))show(path.join(base,'subagents',n));}
else if(mode==='workflows'){for(const n of fs.readdirSync(path.join(base,'workflows')).filter(n=>/^wf_[a-z0-9-]+\.json$/.test(n))){const w=JSON.parse(read(path.join(base,'workflows',n)));console.log(JSON.stringify({runId:w.runId,taskId:w.taskId,status:w.status,startTime:w.startTime,durationMs:w.durationMs,agentCount:w.agentCount,result:w.result}));const dir=path.join(base,'subagents','workflows',w.runId);if(fs.existsSync(dir))for(const f of fs.readdirSync(dir).filter(f=>f==='journal.jsonl'||/^agent-[a-z0-9]+\.jsonl$/.test(f))){if(f==='journal.jsonl')console.log(read(path.join(dir,f)));else show(path.join(dir,f));}}}
else if(mode==='receipts'){for(const phase of fs.readdirSync(run).filter(n=>n==='first'||/^resume-\d+$/.test(n))){const tmp=path.join(run,phase,'tmp');for(const plugin of fs.readdirSync(tmp).filter(n=>/^clauduct-native-events-\d+$/.test(n))){const receipts=path.join(tmp,plugin,'receipts');for(const name of fs.readdirSync(receipts).filter(n=>/^(active-|progress-|cancel-|stopped-|workflow-(?:read-|wf_))/.test(n)))console.log(JSON.stringify({phase,name,receipt:JSON.parse(read(path.join(receipts,name)))}));}}}
else if(mode==='status'){for(const phase of fs.readdirSync(run).filter(n=>n==='first'||/^resume-\d+$/.test(n))){const dir=path.join(run,phase,'tmp','clauduct');for(const n of fs.readdirSync(dir).filter(n=>/^status-\d+\.json$/.test(n)))emit({phase,status:JSON.parse(read(path.join(dir,n)))});}}
else if(mode==='shapes'){const rows=read(base+'.jsonl').trim().split(/\r?\n/).map(JSON.parse);rows.forEach((r,i)=>{if(!r.message)emit({n:i+1,type:r.type,subtype:r.subtype,keys:Object.keys(r),attachmentKeys:r.attachment&&Object.keys(r.attachment)});});}
else if(mode==='audit'){
 const json=file=>JSON.parse(read(file)),jsonl=file=>read(file).trim().split(/\r?\n/).filter(Boolean).map(JSON.parse);
 const digest=file=>crypto.createHash('sha256').update(fs.readFileSync(file)).digest('hex');
 const proof=json(path.join(here,'inspection-'+id+'.json')),rows=jsonl(base+'.jsonl');
 assert.equal(proof.transcript.sha256,digest(base+'.jsonl'));
 const phases=proof.phases;assert.equal(phases.length,2);
 for(const p of phases){assert.equal(p.statusHash,digest(p.statusFile));assert.equal(p.exit,0);assert.equal(p.watchdogForced,false);assert.equal(p.lifecycle.nativeReaped,true);assert.equal(p.completion.apiFailures,0);assert.equal(p.completion.nativeToolFailures,0);assert.equal(p.completion.unacquiredResultsInRecent,0);assert.equal(p.persistence.failed,0);assert.equal(p.sha256,meta.sha256);}
 const first=phases.find(p=>p.phase==='first'),second=phases.find(p=>p.phase.startsWith('resume-'));
 assert.equal(first.requests.received+second.requests.received,142);
 assert.equal(second.persistence.restored,2);assert.equal(second.completion.rejectedWorkflowCalls,1);
 const wf=runId=>json(path.join(base,'workflows',runId+'.json'));
 const journal=runId=>jsonl(path.join(base,'subagents','workflows',runId,'journal.jsonl'));
 assert.deepEqual(wf('wf_d99eac77-3dd').result,{a:'43',b:['180']});
 const parallel=journal('wf_46291b6d-602');assert.deepEqual(parallel.map(r=>r.type),['launched','started','started','result','result']);assert.deepEqual(wf('wf_46291b6d-602').result,['56',['81']]);
 const sourceRecovery=wf('wf_9ccb2c8a-67c');assert.equal(sourceRecovery.agentCount,0);assert.equal(sourceRecovery.result.newAgentExecutions,0);assert.deepEqual(sourceRecovery.result.results.map(r=>r.body).sort(),['180','43']);
 const plan=wf('wf_dc42ba37-87b');assert.equal(plan.agentCount,1);assert.equal(plan.result.complete,false);
 assert.deepEqual(plan.result.results.map(r=>[r.step,r.state,r.body??r.value??null]),[['A','completed_result_reused','S44_A=303'],['B','started_not_reexecuted',null],['C','completed','S44_C=144']]);
 assert.equal(journal('wf_0b6bb6ef-b30').filter(r=>r.type==='started').length,2);
 assert.equal(journal('wf_dc42ba37-87b').filter(r=>r.type==='started').length,1);
 const oldIDs=['a79f754b8453603b7','a605349b6b3db64db','a2e220915335eccb1','a873df4d8f8de1df3'];
 for(const agent of oldIDs){const s=second.selections.recent.find(s=>s.agent===agent);assert.ok(s);assert.equal(s.backendResponses,0);}
 const select=agent=>first.selections.recent.find(s=>s.agent===agent);
 const role=select(oldIDs[0]);assert.equal(role.modelProvided,false);assert.equal(role.effortProvided,false);assert.equal(role.effectiveModel,'gpt-5.6-sol');assert.equal(role.effectiveEffort,'high');
 const modelOnly=select(oldIDs[1]);assert.equal(modelOnly.modelProvided,true);assert.equal(modelOnly.effortProvided,false);assert.equal(modelOnly.effectiveEffort,'max');
 const grand=select('a2c287b1305e60298');assert.equal(grand.parent,'a27b2f629c105edda');assert.equal(grand.modelProvided,false);assert.equal(grand.effortProvided,false);assert.equal(grand.source,'delegation-inherited');assert.equal(grand.effectiveModel,'gpt-5.6-terra');assert.equal(grand.effectiveEffort,'medium');
 const blocks=r=>Array.isArray(r.message?.content)?r.message.content:[];
 const answer=(rs,text)=>rs.some(r=>r.type==='assistant'&&blocks(r).some(b=>b.type==='text'&&b.text.trim()===text));
 const child=jsonl(path.join(base,'subagents','agent-a2c287b1305e60298.jsonl'));assert.ok(answer(child,'144'));assert.equal(child.flatMap(blocks).filter(b=>b.type==='tool_use').length,0);
 const middle=jsonl(path.join(base,'subagents','agent-a27b2f629c105edda.jsonl'));assert.equal(middle.flatMap(blocks).filter(b=>b.type==='tool_use').length,1);assert.ok(middle.some(r=>r.type==='user'&&typeof r.message?.content==='string'&&r.message.content.includes('<result>144</result>')));assert.ok(middle.some(r=>r.type==='assistant'&&blocks(r).some(b=>b.type==='text'&&b.text.includes('145'))));
 const stops=rows.flatMap(blocks).filter(b=>b.type==='tool_use'&&b.name==='TaskStop');assert.equal(stops.length,1);assert.equal(stops[0].input.task_id,'wtpj1d1e2');assert.ok(rows.some(r=>r.toolUseResult?.taskId==='wtpj1d1e2'&&r.toolUseResult?.success===true)||rows.some(r=>typeof r.toolUseResult==='object'&&JSON.stringify(r.toolUseResult).includes('Successfully stopped task: wtpj1d1e2')));
 const bRows=jsonl(path.join(base,'subagents','workflows','wf_0b6bb6ef-b30','agent-a873df4d8f8de1df3.jsonl'));
 assert.equal(bRows.flatMap(blocks).filter(b=>b.type==='tool_use'&&b.name==='Bash').length,1);assert.ok(bRows.some(r=>r.toolUseResult==='User rejected tool use'));
 const compacts=rows.map((r,i)=>({r,i})).filter(({r})=>r.subtype==='compact_boundary');assert.equal(compacts.length,2);
 const between=rows.slice(compacts[0].i+1,compacts[1].i);
 const ordinaryUser=r=>r.type==='user'&&!r.isCompactSummary&&typeof r.message?.content==='string'&&!r.message.content.startsWith('<')&&r.message.content!=='/compact';
 assert.equal(between.filter(ordinaryUser).length,0);assert.ok(between.some(r=>r.message?.content?.includes?.('Set model to `gpt-5.6-terra`')));
 for(const text of ['S44_AFTER_DENIAL=47','S44_AFTER_COMPACT=29','S44_RECOVERY_A=31'])assert.ok(answer(rows,text));
 const esc=rows.filter(r=>r.type==='user'&&typeof r.message?.content==='string'&&r.message.content.startsWith('S44_ESC.'));assert.equal(esc.length,1);assert.equal(answer(rows,'S44_RECOVERY_B=37'),false);
 const context=rows.filter(r=>typeof r.message?.content==='string'&&r.message.content.startsWith('## Context Usage\n')).map(r=>({time:r.timestamp,tokens:r.message.content.match(/\*\*Tokens:\*\* ([^\n]+)/)?.[1],messages:r.message.content.match(/\| Messages \| ([^|]+)\|/)?.[1].trim()}));assert.equal(context.length,6);assert.deepEqual(context.slice(3).map(c=>c.tokens),['13k / 500k (3%)','13k / 500k (3%)','13k / 500k (3%)']);assert.deepEqual(context.slice(3).map(c=>c.messages),['3.9k','5.2k','6.5k']);
 const evidence={id,sha256:meta.sha256,transcriptSHA256:digest(base+'.jsonl'),statusHashes:phases.map(p=>({phase:p.phase,sha256:p.statusHash})),scope:'historical_artifact_audit_no_inference_rerun',observedCore:'pass',strictS44Acceptance:'incomplete',checks:['zero_api_failures_both_phases','normal_exit_native_reaped_both','custom_role_sol_high','model_only_luna_max','scriptPath_43_180','nested_parallel_56_81','native_grandchild_inherits_terra_medium_144_then_145','source_restore_zero_new_children','plan_A_reuse_B_no_reexecution_C_only_144','duplicate_rejected_next_47','two_manual_compacts_no_intervening_generation_next_29','partial_text_cancel_next_31'],unverified:['B_OS_command_started_before_TaskStop','Esc_before_first_text_and_draft_restoration','second_Esc_and_recovery_37','screen_paint_latency'],compactionMs:compacts.map(({r})=>r.compactMetadata.durationMs),context,sourceRun:'wf_d99eac77-3dd',sourceRecovery:'wf_9ccb2c8a-67c',planRun:'wf_0b6bb6ef-b30',planRecovery:'wf_dc42ba37-87b'};
 fs.writeFileSync(path.join(here,'s44-evidence.json'),JSON.stringify(evidence,null,2)+'\n');emit(evidence);
}
else if(mode==='focus'){
 const jsonl=file=>read(file).trim().split(/\r?\n/).filter(Boolean).map(JSON.parse);
 const rows=jsonl(base+'.jsonl'),blocks=r=>Array.isArray(r.message?.content)?r.message.content:[];
 const markers=rows.filter(r=>r.subtype==='compact_boundary'),summaries=rows.filter(r=>r.isCompactSummary);
 assert.equal(markers.length,2);assert.equal(summaries.length,2);
 const compaction=markers.map((r,i)=>({line:rows.indexOf(r)+1,time:r.timestamp,trigger:r.compactMetadata.trigger,preTokens:r.compactMetadata.preTokens,postTokens:r.compactMetadata.postTokens,durationMs:r.compactMetadata.durationMs,summaryChars:summaries[i].message.content.length,summaryUtf8Bytes:Buffer.byteLength(summaries[i].message.content)}));
 assert.deepEqual(compaction.map(c=>[c.preTokens,c.postTokens]),[[29933,2516],[1392,2603]]);
 const answer=text=>rows.find(r=>r.type==='assistant'&&blocks(r).some(b=>b.type==='text'&&b.text===text));
 const before=answer('S44_BEFORE_COMPACT=17'),after=answer('S44_AFTER_COMPACT=29');
 assert.ok(before&&after);assert.equal(before.message.usage.input_tokens,29921);assert.equal(after.message.usage.input_tokens,12833);
 const bEvidence=[];
 for(const [session,workflow,agent] of [[id,'wf_0b6bb6ef-b30','a873df4d8f8de1df3'],['263ef248-4675-4de2-b905-6d0f2bc4dc1d','wf_3d8c5523-f5e','af36add66b56f9b40']]){
  const childFile=path.join(projects,parents[0],session,'subagents','workflows',workflow,'agent-'+agent+'.jsonl');
  const child=jsonl(childFile),calls=child.flatMap(r=>blocks(r).filter(b=>b.type==='tool_use'&&b.name==='Bash').map(b=>({time:r.timestamp,call:b.id})));
  const results=child.filter(r=>r.toolUseResult).map(r=>({time:r.timestamp,kind:r.toolDenialKind,result:r.toolUseResult}));
  assert.equal(calls.length,1);assert.equal(results.length,1);assert.equal(results[0].kind,'user-rejected');assert.equal(results[0].result,'User rejected tool use');
  bEvidence.push({session,workflow,agent,childFile,calls,results,commandStart:'unverified',reason:'no_OS_start_or_process_exit_receipt_in_retained_evidence'});
 }
 const program='C:/Users/js/.local/share/claude/versions/2.1.278',bytes=fs.readFileSync(program),source=bytes.toString('latin1');
 const nativeChecks=[['usage_or_estimate','function Lg(e,n){let r=XFt(e);if(!r)return ig(e,n);return dI(r.usage)+ig(e.slice(r.anchorIndex+1),n)}'],['compaction_pre','async function PYn(e,n,r){let s=Lg(e)'],['compaction_post','yt=ig(mI(pt));if(gt.compactMetadata.postTokens=yt'],['preserved_usage_reset','function Rie(e){if(e.type!=="assistant")return e;return{...e,message:{...e.message,usage:{...e.message.usage,input_tokens:0,output_tokens:0,cache_creation_input_tokens:0,cache_read_input_tokens:0}}}}'],['synthetic_abort_classification','createSyntheticErrorMessage(e,n,r){if(n==="user_interrupted")']].map(([name,needle])=>{const offset=source.indexOf(needle,195000000);assert.ok(offset>=195000000&&offset<232000000,name);return{name,offset};});
 const evidence={id,scope:'read_only_historical_and_installed_source_analysis',productSHA256:meta.sha256,native:{version:'2.1.278',sha256:crypto.createHash('sha256').update(bytes).digest('hex'),checks:nativeChecks,executedExtractedCode:false},userClarification:{esc:'user_reported_no_problem_and_intentionally_stopped_before_all_recovery_steps',mechanicallyObserved:'one_submitted_S44_ESC_and_recovery_31',missingRecovery37:'not_claimed_verified'},bEvidence,compaction,summaryGrowthChars:compaction[1].summaryChars-compaction[0].summaryChars,nativePostEstimateGrowth:compaction[1].postTokens-compaction[0].postTokens,ordinaryUsage:{before:{model:before.message.model,input:before.message.usage.input_tokens},after:{model:after.message.model,input:after.message.usage.input_tokens},reduction:before.message.usage.input_tokens-after.message.usage.input_tokens,meaning:'different_requests_and_models_not_controlled_compression_ratio'},conclusion:{compaction:'first_reduced_context_second_resummary_slightly_expanded_not_exact_total_usage_comparison',B:'started_command_cancellation_not_proven',strictS44:'incomplete',v030Release:'final_approval_withheld'}};
 fs.writeFileSync(path.join(here,'s44-focus-evidence.json'),JSON.stringify(evidence,null,2)+'\n');emit(evidence);
}
else throw Error('MODE');
