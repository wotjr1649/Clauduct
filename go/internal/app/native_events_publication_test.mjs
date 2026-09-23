import assert from 'node:assert/strict';
import * as fs from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';

const root=await fs.mkdtemp(path.join(os.tmpdir(),'clauduct-publication-test-'));
try {
  const source=(await fs.readFile(new URL('./native-events.mjs',import.meta.url),'utf8'))
    .replace('__CLAUDUCT_EVENT_ROOT__',JSON.stringify(root.replaceAll('\\','/')));
  const {register}=await import('data:text/javascript;base64,'+Buffer.from(source).toString('base64'));
  const handlers=new Map();register((name,fn)=>handlers.set(name,fn));
  const $={session:{id:async()=>'public'},clock:{now:async()=>Date.now()},fs:{
    write:async(p,s)=>{await fs.mkdir(path.dirname(p),{recursive:true});await fs.writeFile(p,s);},read:p=>fs.readFile(p,'utf8'),
    exists:async p=>{try{await fs.stat(p);return true;}catch(e){if(e.code==='ENOENT')return false;throw e;}}
  }};
  async function* next(){return {toolUses:[],answer:'ok',stopReason:'end_turn'};}
  async function step(turn,agent){for await(const _ of handlers.get('turn.step')($,{turnId:turn,agentId:agent,index:0,model:'gpt-5.6-sol',effort:'high'},next)){} }
  // Terminal failure survives the next turn's progress publication. Successful
  // completion must never produce an independent cancellation instruction.
  for (const agent of ['', 'child']) {
    for (const reason of ['aborted','error','refusal','answer']) {
      const turn='terminal_'+(agent||'root')+'_'+reason;
      await handlers.get('turn.complete')($,{agentId:agent,turnId:turn,reason},async e=>e);
      const file=path.join(root,'cancel-'+turn+'.json');
      if(reason==='answer') { assert.equal(await $.fs.exists(file),false);continue; }
      const receipt=await fs.readFile(file,'utf8');
      assert.deepEqual(JSON.parse(receipt),{session:'public',agent,turn,reason});
      await handlers.get('turn.complete')($,{agentId:agent,turnId:turn,reason:'aborted'},async e=>e);
      assert.equal(await fs.readFile(file,'utf8'),receipt,'duplicate terminal event replaced original cause');
    }
  }
  console.log('PASS: exact root/child terminal receipts persist; success never cancels');
  // Real filesystem failure, then retry of the same native turn.
  const journal=path.join(root,'active','root');
  await fs.mkdir(journal,{recursive:true});
  await fs.mkdir(path.join(journal,'1-retry.json'));
  await assert.rejects(step('retry'));
  await fs.rmdir(path.join(journal,'1-retry.json'));
  await assert.doesNotReject(step('retry'));
  const committed=await fs.readFile(path.join(journal,'2-retry.json'),'utf8');
  assert.equal(JSON.parse(committed.trim()).turn,'retry');
  assert.equal(await fs.readFile(path.join(journal,'2-retry.ready'),'utf8'),'');
  await step('retry');
  assert.equal((await fs.readdir(journal)).length,2,'same turn was published twice');
  await step('second');
  assert.equal(await fs.readFile(path.join(journal,'2-retry.json'),'utf8'),committed,'previous receipt was overwritten');
  await fs.mkdir(path.join(journal,'4-marker.ready'));
  await assert.rejects(step('marker'));
  const uncommitted=await fs.readFile(path.join(journal,'4-marker.json'),'utf8');
  await fs.rmdir(path.join(journal,'4-marker.ready'));
  await step('marker');
  assert.equal(await fs.readFile(path.join(journal,'4-marker.json'),'utf8'),uncommitted,'retry overwrote its previous attempt');
  assert.equal(await fs.readFile(path.join(journal,'5-marker.ready'),'utf8'),'');
  // Capacity is in-memory bookkeeping, so a memory-backed filesystem keeps this long
  // session fast. Over its life it outgrows the former 4096 turn, child and
  // cancellation limits and the 8192 sequence cap: finished turns release their slots.
  const memory=new Map();
  const $long={...$,fs:{write:async(p,s)=>{memory.set(p,s);},read:async p=>memory.get(p),exists:async p=>memory.has(p)}};
  async function longStep(turn,agent){for await(const _ of handlers.get('turn.step')($long,{turnId:turn,agentId:agent,index:0,model:'gpt-5.6-sol',effort:'high'},next)){} }
  for(let i=0;i<4200;i++) {
    await longStep('done_turn_'+i,'done_'+i);
    await handlers.get('turn.complete')($long,{agentId:'done_'+i,turnId:'done_turn_'+i,reason:'aborted'},async e=>e);
  }
  for(let i=0;i<4100;i++) await longStep('long_'+i);
  const newest=[...memory.keys()].map(k=>k.match(/\/active\/root\/(\d+)-long_4099\.ready$/)).find(Boolean);
  assert.ok(newest && Number(newest[1])>8192,'publication past the former sequence cap');
  // Concurrently active agents stay bounded; root holds one slot, and at most one
  // concurrent publisher may take the final one.
  for(let i=0;i<4094;i++) await longStep('active_turn_'+i,'active_'+i);
  const last=await Promise.allSettled([longStep('last','active_last'),longStep('other_last','active_other')]);
  assert.equal(last.filter(r=>r.status==='fulfilled').length,1);
  assert.match(last.find(r=>r.status==='rejected').reason.message,/CLAUDUCT_NATIVE_EVENT_LIMIT/);
  await assert.rejects(longStep('overflow','another'),/CLAUDUCT_NATIVE_EVENT_LIMIT/);
  await longStep('long_4099'); // An active agent's current turn still works at capacity.
  // Exercise the real event module with file-backed decisions. A completed
  // notification owes no child and must not leave lifecycle drain waiting.
  handlers.clear();register((name,fn)=>handlers.set(name,fn));
  for(const waiting of [true,false]) {
    const turn=waiting?'waiting_notification':'finished_notification';
    await handlers.get('prompt.submit')($,{origin:{kind:'composer'}},async e=>e);
    await handlers.get('prompt.submit')($,{origin:{kind:'task-notification'}},async e=>e);
    await handlers.get('turn.start')($,{turnId:turn},async e=>e);
    await fs.writeFile(path.join(root,'decision-root.json'),JSON.stringify({turn,index:0,hold:true,wait:waiting}));
    const result={turnId:turn,index:0,toolUses:[],answer:'',stopReason:'end_turn',usage:null};
    function response() {const stream=(async function*(){yield {kind:'stop',stopReason:'end_turn',usage:null};return result;})();stream.result=Promise.resolve(result);return stream;}
    for await(const _ of handlers.get('turn.step')($,{turnId:turn,index:0},response)){}
    await handlers.get('turn.complete')($,{turnId:turn,reason:'answer'},async e=>e);
    const progress=JSON.parse(await fs.readFile(path.join(root,'progress-root.json'),'utf8'));
    assert.equal(progress.phase,waiting?'awaiting_children':'turn_ended','completed notification left lifecycle waiting');
  }
  console.log('PASS: failed write retry, immutable earlier receipts, long session under a 4096 active-agent cap');
  console.log('PASS: pending child wait and completed notification lifecycle are distinct');
} finally {
  await fs.rm(root,{recursive:true,force:true});
}
