// Read only a candidate run in the previously approved public fixture profile.
import fs from 'node:fs';
import path from 'node:path';
import {fileURLToPath} from 'node:url';
const root=path.dirname(fileURLToPath(import.meta.url)),id=process.argv[2];
if(!/^[a-f0-9-]{36}$/.test(id||''))throw Error('INVALID_ID');
const base='D:/AIDEV/clauduct-s36-build/repair-20260919/verification/policy-repair-20260919';
const run=path.join(base,'interactive-'+id),m=JSON.parse(fs.readFileSync(path.join(run,'run.json'),'utf8'));
const approved=path.join(base,'interactive-e268b95d-2048-47e3-b36b-5483000541a7');
if(path.resolve(m.profile)!==path.resolve(approved,'profile')||path.resolve(m.project)!==path.resolve(approved,'project')||path.resolve(m.binary)!==path.resolve(root,'bin','clauduct.exe'))throw Error('UNREVIEWED_RUN');
const dir=path.join(run,'tmp','clauduct'),names=fs.readdirSync(dir).filter(n=>/^status-\d+\.json$/.test(n));
if(names.length!==1)throw Error('STATUS_NOT_UNIQUE');
const status=JSON.parse(fs.readFileSync(path.join(dir,names[0]),'utf8'));
if(!status.lifecycle?.final||m.state!=='exited')throw Error('NOT_FINAL');
const result={run:m,status};
fs.writeFileSync(path.join(root,'run-'+id+'.json'),JSON.stringify(result,null,2)+'\n');
console.log(JSON.stringify({id,sha256:m.sha256,completion:status.completion,features:status.gateway.features.filter(f=>f.requests>0),nativeEvents:status.gateway.nativeEvents}));
