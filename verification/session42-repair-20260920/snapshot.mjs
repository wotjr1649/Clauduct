// Capture status only from this approved, public-fixture verification run.
import fs from 'node:fs';
import path from 'node:path';
import assert from 'node:assert/strict';
import {fileURLToPath} from 'node:url';
const root=path.dirname(fileURLToPath(import.meta.url)),phase=process.argv[2];
assert(['seed','first-compact','second-compact'].includes(phase));
const id='16dfd222-e9b6-44a7-94d6-27f383d84850';
const base='D:/AIDEV/clauduct-s36-build/repair-20260919/verification/policy-repair-20260919';
const run=path.join(base,'interactive-'+id),read=f=>JSON.parse(fs.readFileSync(f,'utf8'));
const m=read(path.join(run,'run.json'));
assert.equal(path.resolve(m.profile),path.resolve(base,'interactive-e268b95d-2048-47e3-b36b-5483000541a7/profile'));
assert.equal(path.resolve(m.binary),path.resolve(root,'bin/clauduct.exe'));
assert.equal(m.sha256,read(path.join(root,'build.json')).artifacts['clauduct.exe']);
const dir=path.join(run,'tmp/clauduct'),names=fs.readdirSync(dir).filter(n=>/^status-\d+\.json$/.test(n));assert.equal(names.length,1);
const status=read(path.join(dir,names[0]));
fs.writeFileSync(path.join(root,'checkpoint-'+phase+'.json'),JSON.stringify({id,phase,status},null,2)+'\n',{flag:'wx'});
console.log(JSON.stringify({phase,completion:status.completion,recent:status.gateway.recent.filter(r=>r.kind==='compaction'),contexts:status.gateway.agentContexts.filter(c=>!c.agentId)}));
