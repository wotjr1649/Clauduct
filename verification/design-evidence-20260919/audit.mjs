// Read only these four public-fixture sessions; retain shapes/counts, never bodies.
import fs from 'node:fs';
import path from 'node:path';
import crypto from 'node:crypto';
import assert from 'node:assert/strict';
import {fileURLToPath} from 'node:url';
const dir=path.dirname(fileURLToPath(import.meta.url));
const ids=['77805b16-e954-414b-99ba-74763eaa97f2','ed9b4683-3fcb-4a7b-8fbc-1407ac7a7e3a','45defe83-2b79-40ff-a908-82e77418b0eb','fb99d281-ed62-4357-9ca0-fd6457997d99'];
const profile='D:/AIDEV/clauduct-s36-build/repair-20260919/verification/policy-repair-20260919/interactive-e268b95d-2048-47e3-b36b-5483000541a7/profile';
const slug='D--AIDEV-clauduct-s36-build-repair-20260919-verification-policy-repair-20260919-interactive-e268b95d-2048-47e3-b36b-5483000541a7-project';
const base=path.join(profile,'projects',slug);
const nudge='[Your previous response had no visible output. Please continue and produce a user-visible response.]';
function read(file){const stat=fs.statSync(file);assert(stat.isFile()&&stat.size<4*1024*1024);return fs.readFileSync(file,'utf8').split('\n').filter(Boolean).map(s=>JSON.parse(s));}
const runs=ids.map(id=>{
  const run=JSON.parse(fs.readFileSync(path.join(dir,'run-'+id,'result.json')));
  assert.equal(run.id,id);assert.equal(run.profile.replaceAll('\\','/'),profile);assert.equal(run.realBackend,false);assert.equal(run.exitCode,0);
  assert.match(run.middle,/^[a-z0-9]+$/);assert.match(run.leaf,/^[a-z0-9]+$/);
  const root=read(path.join(base,id+'.jsonl'));
  const middle=read(path.join(base,id,'subagents','agent-'+run.middle+'.jsonl'));
  const leaf=read(path.join(base,id,'subagents','agent-'+run.leaf+'.jsonl'));
  const has=(rows,needle)=>rows.filter(r=>r.type==='assistant'&&Array.isArray(r.message?.content)&&r.message.content.some(c=>c.type==='text'&&c.text===needle)).length;
  const nativeError=root.some(r=>r.type==='user'&&JSON.stringify(r.message?.content).includes('ECONNRESET'));
  const leafRelease=run.records.find(r=>r.event==='leaf-release');
  const woke=run.records.find(r=>r.kind==='middle-woke');
  return {id,variant:run.variant??'zero-content',version:run.version,nativeSha256:run.sha256,exitCode:run.exitCode,deadlineExpired:!!run.deadlineExpired,requests:run.records.filter(r=>r.event==='request').length,middleRequests:run.records.filter(r=>r.agentId===run.middle&&r.event==='request').length,nativeEmptyNudges:middle.filter(r=>r.type==='user'&&r.isMeta===true&&r.message?.content===nudge).length,leafAnswers:has(leaf,'LEAF_NATIVE_WAIT_OK'),middleAnswers:has(middle,'MIDDLE_NATIVE_WAIT_OK'),rootAnswers:has(root,'ROOT_NATIVE_WAIT_OK'),rootFailureNotificationECONNRESET:nativeError,wakeDelayMs:woke&&leafRelease?woke.ms-leafRelease.ms:null};
});
assert.equal(runs[0].nativeEmptyNudges,1);
assert.equal(runs[0].rootAnswers,0);assert.equal(runs[0].rootFailureNotificationECONNRESET,true);
assert.equal(runs[1].rootAnswers,0);assert.equal(runs[1].rootFailureNotificationECONNRESET,true);
assert.equal(runs[2].nativeEmptyNudges,0);assert.equal(runs[2].rootAnswers,1);
assert.equal(runs[3].nativeEmptyNudges,1);assert.equal(runs[3].rootAnswers,1);
const native=fs.readFileSync('C:/Users/js/.local/bin/claude.exe');
assert(runs.every(r=>r.nativeSha256===crypto.createHash('sha256').update(native).digest('hex')));
const staticChecks=[
 ['emptyStreamRejection','Stream completed with message_start but no content blocks completed - triggering non-streaming fallback'],
 ['nativeEmptyNudge',nudge],
 ['nudgeTransition','transition:{reason:"thinking_only_retry"}'],
 ['nudgeGuard','st==="end_turn"||st==="stop_sequence"'],
];
const binaryEvidence=staticChecks.map(([name,text])=>{const offset=native.indexOf(Buffer.from(text),190000000);assert(offset>=0);return {name,offset};});
const report={scope:'native TUI and local synthetic transport; not product/backend acceptance',runs,binaryEvidence,auditConsistencyChecksPassed:true,limitations:['ECONNRESET was observed in both an empty run and an initial nonempty control; root cause not established.','Later success does not remove those failures.','No network/model inference billing, counting, or latency guarantee follows from synthetic usage.','No Clauduct waiting implementation was changed or verified.']};
fs.writeFileSync(path.join(dir,'native-wait-audit.json'),JSON.stringify(report,null,2)+'\n');
console.log(JSON.stringify(report));
