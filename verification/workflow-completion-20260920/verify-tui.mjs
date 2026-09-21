// Assert the observed public TUI session, not a simulated backend or model claim.
import fs from 'node:fs';import path from 'node:path';import assert from 'node:assert/strict';import crypto from 'node:crypto';import {fileURLToPath} from 'node:url';
const here=path.dirname(fileURLToPath(import.meta.url)),id='263ef248-4675-4de2-b905-6d0f2bc4dc1d';
const proof=JSON.parse(fs.readFileSync(path.join(here,'inspection-'+id+'.json'),'utf8'));
const meta=JSON.parse(fs.readFileSync(path.join(here,'tui-'+id,'session.json'),'utf8'));
const first=proof.phases.find(p=>p.phase==='first'),second=proof.phases.find(p=>p.phase.startsWith('resume-'));
for(const p of [first,second]){assert.equal(p.exit,0);assert.equal(p.watchdogForced,false);assert.equal(p.lifecycle.nativeReaped,true);assert.equal(p.completion.apiFailures,0);assert.equal(p.completion.nativeToolFailures,0);assert.equal(p.completion.unacquiredResultsInRecent,0);assert.equal(p.persistence.failed,0);}
assert.equal(first.persistence.saved,2);assert.equal(second.persistence.restored,2);assert.equal(second.completion.rejectedWorkflowCalls,1);
assert.equal(second.features.find(f=>f.name==='workflow_agent').requests,1);
const oldIDs=first.selections.recent.map(s=>s.agent);assert.equal(oldIDs.length,4);
for(const s of second.selections.recent.filter(s=>oldIDs.includes(s.agent)))assert.equal(s.backendResponses,0);
const role=first.selections.recent.find(s=>s.role==='public-math');assert.equal(role.modelProvided,false);assert.equal(role.effortProvided,false);assert.equal(role.effectiveModel,'gpt-5.6-sol');assert.equal(role.effectiveEffort,'high');
const modelOnly=first.selections.recent.find(s=>s.requestedModel==='gpt-5.6-luna');assert.equal(modelOnly.effortProvided,false);assert.equal(modelOnly.effectiveEffort,'max');
const projects=path.join(meta.profile,'projects');const parent=fs.readdirSync(projects).find(n=>fs.existsSync(path.join(projects,n,id+'.jsonl')));assert.ok(parent);
const base=path.join(projects,parent,id),load=run=>JSON.parse(fs.readFileSync(path.join(base,'workflows',run+'.json'),'utf8'));
const recovered=load('wf_c46373ab-a6e');assert.deepEqual(recovered.result.results.map(r=>r.body).sort(),['180','43']);assert.equal(recovered.agentCount,0);assert.equal(recovered.result.newAgentExecutions,0);
const continued=load('wf_c56e9820-7fd');assert.equal(continued.agentCount,1);assert.equal(continued.result.complete,false);
const states=continued.result.results;assert.equal(states.find(r=>r.step==='A').body,'S49_A=303');assert.equal(states.find(r=>r.step==='A').state,'completed_result_reused');assert.equal(states.find(r=>r.step==='B').state,'started_not_reexecuted');assert.equal(states.find(r=>r.step==='C').value,'S49_C=144');
for(const [run,hash]of [['wf_3d8c5523-f5e','3864b5164d31331486eaf6d5e4a85cd7eda73dd571060c79a34e60024c8692d2'],['wf_d6781e46-358','571dda3a2d511e2e9deb6829edb4886b95f129a6351e0b5cce63406559835473']])assert.equal(proof.workflows.find(w=>w.runId===run).journalHash,hash);
assert.equal(proof.workflows.find(w=>w.runId==='wf_3d8c5523-f5e').claimed,true);
const raw=fs.readFileSync(base+'.jsonl'),rows=raw.toString('utf8').trim().split(/\r?\n/).map(s=>JSON.parse(s));
assert.ok(rows.some(r=>r.type==='assistant'&&Array.isArray(r.message?.content)&&r.message.content.some(b=>b.type==='text'&&b.text.trim()==='S49_AFTER_DENIAL=53')));
const evidence={id,productSource:'31ff1184c21d7dac0fccd03394081aacd78b9db5',sha256:meta.sha256,transcriptSHA256:crypto.createHash('sha256').update(raw).digest('hex'),checks:['source_native_Read','custom_role_defaults_sol_high','model_only_luna_max','normal_exit_checkpoint','same_session_new_launcher_restore','completed_results_43_180_without_child_reexecution','plan_A_reused_B_not_reexecuted_C_only_144','original_journals_unchanged','duplicate_tool_denial','next_answer_53','zero_api_failures','native_reaped_both_phases'],humanAcceptance:'not_assessed'};
fs.writeFileSync(path.join(here,'final-evidence.json'),JSON.stringify(evidence,null,2)+'\n');console.log(JSON.stringify(evidence));
