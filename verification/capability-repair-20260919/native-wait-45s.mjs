// Native-only protocol experiment, not a Clauduct or real-backend acceptance test.
// Public fixtures, loopback transport and a validated test-only wait-control hook.
import fs from 'node:fs';
import path from 'node:path';
import http from 'node:http';
import crypto from 'node:crypto';
import {spawn, spawnSync} from 'node:child_process';
import {fileURLToPath} from 'node:url';

const root=path.dirname(fileURLToPath(import.meta.url));
const workspace='D:/AIDEV/clauduct-s36-build/repair-20260919/verification/policy-repair-20260919/interactive-e268b95d-2048-47e3-b36b-5483000541a7';
const project=path.join(workspace,'project'), profile=path.join(workspace,'profile');
const native='C:/Users/js/.local/bin/claude.exe';
const probe=process.argv.includes('--probe');
const leafFailure=process.argv.includes('--leaf-failure');
const cancelCase=process.argv.includes('--cancel-case');
const variant=process.argv[2]||'zero-content';
if(!['zero-content','nonempty-control','empty-text'].includes(variant))throw Error('UNKNOWN_VARIANT');
const id=crypto.randomUUID(), dir=path.join(root,'run-'+id);
fs.mkdirSync(path.join(dir,'tmp'),{recursive:true});
const env=Object.fromEntries(['SystemRoot','WINDIR','COMSPEC','PATHEXT','PATH','NUMBER_OF_PROCESSORS','PROCESSOR_ARCHITECTURE'].filter(k=>process.env[k]).map(k=>[k,process.env[k]]));
Object.assign(env,{HOME:profile,USERPROFILE:profile,APPDATA:profile,LOCALAPPDATA:profile,TEMP:path.join(dir,'tmp'),TMP:path.join(dir,'tmp'),CLAUDE_CONFIG_DIR:profile,DISABLE_AUTOUPDATER:'1',DISABLE_TELEMETRY:'1',DISABLE_ERROR_REPORTING:'1',CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC:'1',CLAUDE_CODE_MAX_RETRIES:'0',CLAUDE_CODE_RETRY_WATCHDOG:'0',CLAUDE_CODE_DISABLE_AUTO_MEMORY:'1',ENABLE_TOOL_SEARCH:'false',ANTHROPIC_AUTH_TOKEN:'synthetic-local-only'});
const version=spawnSync(native,['--version'],{env,cwd:project,encoding:'utf8',timeout:10000,windowsHide:true});
if(version.status!==0||!/^\d+\.\d+\.\d+/.test(version.stdout.trim()))throw Error('NATIVE_VERSION_NOT_OBSERVED');
const manifest={id,native,version:version.stdout.trim(),sha256:crypto.createHash('sha256').update(fs.readFileSync(native)).digest('hex'),harnessSha256:crypto.createHash('sha256').update(fs.readFileSync(fileURLToPath(import.meta.url))).digest('hex'),project,profile,variant,fixture:true,realBackend:false,mode:'interactive-native-empty-wait',deadlineSeconds:180,started:new Date().toISOString(),records:[]};
const save=()=>fs.writeFileSync(path.join(dir,'result.json'),JSON.stringify(manifest,null,2)+'\n');
const plugin=path.join(dir,'plugin');
if(probe){
  fs.mkdirSync(path.join(plugin,'.claude-plugin'),{recursive:true});fs.mkdirSync(path.join(plugin,'hooks'));
  fs.writeFileSync(path.join(plugin,'.claude-plugin/plugin.json'),JSON.stringify({name:'clauduct-wait-probe',version:'1.0.0',description:'Bounded native control experiment',author:{name:'Clauduct'}}));
  fs.writeFileSync(path.join(plugin,'hooks/hooks.json'),'{"modules":["./probe.mjs"]}');
  fs.writeFileSync(path.join(plugin,'hooks/probe.mjs'),fs.readFileSync(path.join(root,'wait-probe.mjs'),'utf8').replace('__PROBE_OUTPUT__',JSON.stringify(path.join(dir,'hook-events.json').replaceAll('\\','/'))));
  env.CLAUDE_CODE_ENABLE_FUNCTION_HOOKS='1';
  const check=spawnSync(native,['plugin','validate',plugin],{env,cwd:project,encoding:'utf8',timeout:20000,windowsHide:true});
  manifest.probe=true;manifest.validatorExit=check.status;save();
  if(check.status!==0){console.error(check.stdout,check.stderr);throw Error('PROBE_VALIDATION_FAILED');}
}
manifest.leafFailure=leafFailure;manifest.cancelCase=cancelCase;
const begun=Date.now(), turns=new Map();
let middle,leaf,generations=0,leafReleased=false,rootSpawned=false,child;
const timers=new Set();
const schedule=(fn,ms)=>{const t=setTimeout(()=>{timers.delete(t);fn();},ms);timers.add(t);};
function emit(res,model,blocks){
  if(res.destroyed){manifest.records.push({event:'response-destroyed',ms:Date.now()-begun});save();return;}
  res.writeHead(200,{'Content-Type':'text/event-stream'});
  const ev=(type,x)=>res.write(`event: ${type}\ndata: ${JSON.stringify({type,...x})}\n\n`);
  ev('message_start',{message:{id:'msg_'+crypto.randomUUID(),type:'message',role:'assistant',model,content:[],stop_reason:null,stop_sequence:null,usage:{input_tokens:1000,output_tokens:1}}});
  blocks.forEach((b,index)=>{
    ev('content_block_start',{index,content_block:b.type==='tool_use'?{...b,input:{}}:{type:'text',text:''}});
    ev('content_block_delta',{index,delta:b.type==='tool_use'?{type:'input_json_delta',partial_json:JSON.stringify(b.input)}:{type:'text_delta',text:b.text}});
    ev('content_block_stop',{index});
  });
  ev('message_delta',{delta:{stop_reason:blocks.some(b=>b.type==='tool_use')?'tool_use':'end_turn',stop_sequence:null},usage:{output_tokens:10}});
  ev('message_stop',{});res.end();
}
const text=text=>({type:'text',text});
const agent=(id,prompt)=>({type:'tool_use',id,name:'Agent',input:{subagent_type:'general-purpose',description:prompt,prompt,run_in_background:true}});
const server=http.createServer(async(req,res)=>{
  try{
    let raw='';for await(const chunk of req){raw+=chunk;if(raw.length>4*1024*1024){res.writeHead(413);res.end();return;}}
    const body=raw?JSON.parse(raw):{};
    if(req.url.includes('count_tokens')){res.setHeader('Content-Type','application/json');res.end('{"input_tokens":1000}');return;}
    if(!req.url.startsWith('/v1/messages')){res.writeHead(404);res.end();return;}
    if(++generations>18){manifest.requestCapReached=true;save();res.writeHead(429);res.end();return;}
    const agentId=req.headers['x-claude-code-agent-id'];
    const key=agentId||'root', n=(turns.get(key)||0)+1;turns.set(key,n);
    const history=JSON.stringify(body.messages||[]);
    const rec={event:'request',seq:generations,ms:Date.now()-begun,path:req.url,stream:body.stream,agentId,parentAgentId:req.headers['x-claude-code-parent-agent-id'],sessionId:req.headers['x-claude-code-session-id'],turn:n,tools:(body.tools||[]).length,hasLeaf:history.includes('LEAF_NATIVE_WAIT_OK'),hasMiddle:history.includes('MIDDLE_NATIVE_WAIT_OK')};
    rec.hasFailure=history.includes('PUBLIC_LEAF_FAILURE');
    manifest.records.push(rec);
    res.on('finish',()=>{rec.finishMs=Date.now()-begun;save();});
    res.on('close',()=>{rec.closeMs=Date.now()-begun;rec.finished=res.writableFinished;save();});
    req.on('aborted',()=>{rec.aborted=true;save();});
    if(!agentId&&!(body.tools||[]).some(t=>t.name==='Agent')){rec.kind='auxiliary';save();emit(res,body.model,[text('Local fixture')]);return;}
    if(!agentId&&cancelCase&&history.includes('PUBLIC_AFTER_ESC')){rec.kind='root-recovery';save();emit(res,body.model,[text('PUBLIC_AFTER_ESC_OK')]);return;}
    if(agentId&&!middle)middle=agentId;
    else if(agentId!==middle&&agentId&&!leaf)leaf=agentId;
    if(leaf&&agentId===leaf){
      rec.kind='leaf-held';save();
      schedule(()=>{leafReleased=true;manifest.records.push({event:'leaf-release',agentId,ms:Date.now()-begun});save();
        if(leafFailure){if(!res.destroyed){res.writeHead(400,{'Content-Type':'application/json'});res.end(JSON.stringify({type:'error',error:{type:'invalid_request_error',message:'PUBLIC_LEAF_FAILURE'}}));}}
        else emit(res,body.model,[text('LEAF_NATIVE_WAIT_OK')]);
      },cancelCase?45000:12000);
    }else if(middle&&agentId===middle){
      if(n===1){rec.kind='middle-spawn';emit(res,body.model,[agent('call_native_wait_leaf','NATIVE_WAIT_LEAF')]);}
      else if(!leafReleased){rec.kind='middle-wait-response';rec.variant=variant;emit(res,body.model,variant==='zero-content'?[]:[text(variant==='empty-text'?'':'CONTROL_WAITING_FOR_CHILD')]);}
      else{rec.kind='middle-woke';emit(res,body.model,[text(rec.hasLeaf?'MIDDLE_NATIVE_WAIT_OK':rec.hasFailure?'MIDDLE_FAILURE_OBSERVED':'MIDDLE_MISSING_LEAF')]);}
      save();
    }else{
      if(!rootSpawned){rootSpawned=true;rec.kind='root-spawn';emit(res,body.model,[agent('call_native_wait_middle','NATIVE_WAIT_MIDDLE')]);}
      else{rec.kind=rec.hasMiddle?'root-received':'root-idle';emit(res,body.model,[text(rec.hasMiddle?'ROOT_NATIVE_WAIT_OK':'ROOT_WAITING_ACK')]);}
      save();
    }
  }catch(error){manifest.handlerError=error.name;save();if(!res.headersSent)res.writeHead(500);res.end();}
});
await new Promise(resolve=>server.listen(0,'127.0.0.1',resolve));
env.ANTHROPIC_BASE_URL=`http://127.0.0.1:${server.address().port}`;
manifest.loopbackPort=server.address().port;save();
console.log(JSON.stringify({id,version:manifest.version,fixture:true,result:path.join(dir,'result.json')}));
child=spawn(native,['--session-id',id,'--model','gpt-6-astra','--effort','low','--strict-mcp-config',...(probe?['--plugin-dir',plugin]:[])],{cwd:project,env,stdio:'inherit',windowsHide:true});
manifest.pid=child.pid;save();
const deadline=setTimeout(()=>{manifest.deadlineExpired=true;save();child.kill('SIGKILL');},180000);
child.on('error',error=>{manifest.spawnError=error.code;save();});
child.on('close',(code,signal)=>{
  clearTimeout(deadline);for(const t of timers)clearTimeout(t);
  manifest.exitCode=code;manifest.signal=signal;manifest.finished=new Date().toISOString();
  manifest.middle=middle;manifest.leaf=leaf;save();
  server.closeAllConnections();server.close();
  console.log(JSON.stringify({id,exitCode:code,signal,result:path.join(dir,'result.json')}));
  process.exitCode=code??1;
});
