// Test-only native control probe. No response, usage, result or success is invented.
const output=__PROBE_OUTPUT__;
async function record($, state, event) {
  if(state.events.length>=100)throw Error('PROBE_EVENT_LIMIT');
  state.events.push(event);
  await $.fs.write(output,JSON.stringify(state.events));
}
export const register=on=>{
  const state={events:[]}, previous=new Map();
  on('turn.step',async function*($,e,next){
    const id=e.agentId||'',last=previous.get(id);
    if(last&&last.turn===e.turnId&&e.index===last.index+1&&last.empty){
      const children=(await $.agent.list()).filter(a=>a.parentId===id&&a.status==='running');
      if(id&&children.length){
        previous.delete(id);
        await record($,state,{kind:'suppressed',agent:id,turn:e.turnId,index:e.index,children:children.map(a=>a.id)});
        return {turnId:e.turnId,index:e.index,answer:'',toolUses:[],stopReason:null,usage:null};
      }
    }
    const result=yield* next(e);
    const empty=result.answer.trim()===''&&result.toolUses.length===0&&result.stopReason==='end_turn';
    previous.set(id,{turn:e.turnId,index:e.index,empty});
    await record($,state,{kind:'response',agent:id,turn:e.turnId,index:e.index,empty});
    return result;
  });
};
