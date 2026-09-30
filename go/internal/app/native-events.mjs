// No environment mutation, network, process or report-body access. Native identity,
// progress and terminal receipts are recorded. Workflow source files use native Read
// with its permission checks. A verified dependency wait consumes only the gateway's
// empty control response. Native commands are untouched.
const root = __CLAUDUCT_EVENT_ROOT__;
const requiredConfirmations = __CLAUDUCT_REQUIRED_PERMISSIONS__.ask;
async function confirmationsVerified($) {
  try {
    let settings = await $.settings.read();
    if (settings.allowManagedPermissionRulesOnly === true) settings = await $.settings.read({source:'policy'});
    const ask = settings.permissions?.ask, deny = settings.permissions?.deny;
    return requiredConfirmations.every(tool => [ask,deny].some(rules => Array.isArray(rules) && (rules.includes(tool) || rules.includes('*'))));
  } catch { return false; }
}
async function preparePermissions($, state) {
  if (state.permissionFailed || !await confirmationsVerified($)) {
    state.permissionFailed=true;
    throw new Error('NATIVE_CONFIRMATION_UNVERIFIED');
  }
  state.permissionReady=true;
}
async function answerConfirmations($, state) {
  if (state.permissionBusy || state.permissionFailed || !state.requests.size) return;
  state.permissionBusy=true;
  try {
    const file=root+'/confirmation-request.json';
    if (!await $.fs.exists(file)) return;
    const request=JSON.parse(await $.fs.read(file));
    if (!/^[A-Za-z0-9_-]{43}$/.test(request.nonce) || !ident(request.session) || typeof request.search!=='boolean' || request.auxiliary!==undefined && typeof request.auxiliary!=='boolean') throw new Error('NATIVE_CONFIRMATION_UNVERIFIED');
    if (request.nonce===state.permissionNonce) {
      const body=await $.fs.read(root+'/confirmation-reply.json');
      const ready=await $.fs.read(root+'/confirmation-ready.json');
      let published=false;
      try { published=JSON.parse(body).nonce===request.nonce && JSON.parse(ready).nonce===request.nonce; } catch {}
      if (published) return; // A late prior module may have displaced either file.
    }
    state.permissionNonce=request.nonce;
    const verified=await confirmationsVerified($);
    const step=await session($)===request.session && [...state.requests].find(p=>p.session===request.session && p.agent===request.agent && p.turn===request.turn && p.index===request.index && p.search===request.search && !!p.auxiliary===!!request.auxiliary);
    const confirmations=verified && !!step && !state.permissionFailed;
    const unmatched=verified && !step && !state.permissionFailed;
    const reply={...request,agent:step?.agent||'',turn:step?.turn||'',index:step?.index??-1,confirmations,unmatched};
    if (JSON.parse(await $.fs.read(file)).nonce!==request.nonce) return;
    await $.fs.write(root+'/confirmation-reply.json',JSON.stringify(reply));
    if (JSON.parse(await $.fs.read(file)).nonce!==request.nonce) return;
    await $.fs.write(root+'/confirmation-ready.json',JSON.stringify({nonce:request.nonce}));
  } catch { state.permissionFailed=true; }
  finally { state.permissionBusy=false; }
}
function beginConfirmation($, state, active) {
  if (state.requests.size>=4096) throw new Error('CLAUDUCT_NATIVE_EVENT_LIMIT');
  state.requests.add(active);
  if (!state.permissionTimer) state.permissionTimer=$.clock.every(25,()=>{void answerConfirmations($,state);});
}
function endConfirmation(state, active) {
  state.requests.delete(active);
  if (!state.requests.size) {
    state.permissionTimer?.cancel();
    state.permissionTimer=null;
  }
}
const ident = value => typeof value === 'string' && /^[A-Za-z0-9_-]{1,200}$/.test(value) ? value : '';
// /clear starts a new native session in this process without registering this module
// again. Read the id for every receipt: a cached first id signed later turns for the
// cleared session, and the gateway refused every request after /clear.
async function session($) {
  const id = ident(await $.session.id());
  if (!id) throw new Error('CLAUDUCT_NATIVE_SESSION_INVALID');
  return id;
}
async function observe($, progress, p, signal) {
    const receipt = {agent:p.agent,turn:p.turn,
      phase:p.phase,signal,sequence:++p.sequence,
      pendingTools:p.pendingTools,permissionRequests:p.permissionRequests};
    const prior = p.write;
    p.write = (async () => {
      if (prior) await prior;
      receipt.session=await session($);receipt.at=await $.clock.now();
      if (progress.get(p.agent)!==p) return; // a late prior turn cannot replace current progress
      const name=p.agent?'progress-child-'+p.agent:'progress-root';
      await $.fs.write(root+'/'+name+'.json',JSON.stringify(receipt));
    })();
    await p.write;
}
export const register = on => {
  const state = {mode:'unclassified',peerMode:'unclassified',permissionReady:false,requests:new Set()};
  const turns = new Map();
  let turnSequence=0;
  const cancelledTurns = new Set();
  const progress = new Map();
  let origin='unclassified';
  on('agent.spawn', async ($, e, next) => {
    const sid=await session($),call=ident(e.tool_use_id),parent=e.parentAgentId?ident(e.parentAgentId):'';
    if (!call || e.parentAgentId && !parent) throw new Error('CLAUDUCT_NATIVE_ID_INVALID');
    const file=root+'/selection-'+sid+'-'+call+'.json';
    if (!await $.fs.exists(file)) return next(e); // native/plugin-owned choices retain their own route
    let choice;
    try { choice=JSON.parse(await $.fs.read(file)); }
    catch { throw new Error('CLAUDUCT_NATIVE_SELECTION_UNVERIFIED'); }
    if (choice.session!==sid || choice.call!==call || choice.parent!==parent || choice.role!==e.subagentType || !__CLAUDUCT_MODELS__.includes(choice.model)) throw new Error('CLAUDUCT_NATIVE_SELECTION_UNVERIFIED');
    if (e.fork) return next(e); // native owns fork inheritance
    return next({...e,model:choice.model});
  });
  on('prompt.submit', async ($, e, next) => {
    origin=e.origin?.kind || 'unclassified';
    if (origin==='composer') state.mode='native_tui';
    else if (origin==='sdk') state.mode='sdk';
    else if (origin==='peer' && state.mode==='unclassified') state.mode=state.peerMode;
    const p=progress.get('');
    if (e.turnId && p?.turn===e.turnId && origin!=='task-notification') p.intervened=true;
    return next(e);
  });
  on('turn.start', async ($, e, next) => {
    state.rootTurn=e.turnId;state.rootOrigin=origin;origin='unclassified';
    return next(e);
  });
  on('session.start', async ($, e, next) => {
    // Native 2.1.282 delivers SendMessage as peer on both surfaces. Its
    // session evidence selects the response contract; other origins stay unknown.
    state.peerMode=e.isInteractive===true?'native_tui':e.isInteractive===false?'sdk':'unclassified';
    await preparePermissions($,state);
    await $.fs.write(root + '/ready.json', JSON.stringify({session:await session($)}));
    return next(e);
  });
  on('turn.step', async function* ($, e, next) {
    await preparePermissions($,state);
    const agent=e.agentId?ident(e.agentId):'', turn=ident(e.turnId);
    if (!turn || (e.agentId && !agent)) throw new Error('CLAUDUCT_NATIVE_ID_INVALID');
    let p=progress.get(agent);
    if (!p || p.turn!==turn) {
      if (progress.size>=4096 && !p) throw new Error('CLAUDUCT_NATIVE_EVENT_LIMIT');
      p={agent,turn,sequence:0,pendingTools:0,permissionRequests:0,phase:'request',write:p?.write};
      progress.set(agent,p);
    }
    p.phase='request';p.index=e.index;
    // One publication per agent's current turn: completed turns release their slot,
    // so 4096 bounds concurrently active agents, not turns over a session's life.
    let current=turns.get(agent);
    if (current?.turn!==turn) {
      if (turns.size>=4096 && !current) throw new Error('CLAUDUCT_NATIVE_EVENT_LIMIT');
      const name=agent?'child-'+agent:'root';
      const directory=root+'/active/'+name, prior=current?.publication;
      const publication=(async () => {
        // Reload/respawn loses module state, but not the journal. Include unfinished
        // bodies too: reusing their number could publish a partial earlier attempt.
        // Serialize one agent's publications; different agents keep running in parallel.
        if (prior) await prior;
        const entries=await $.fs.exists(directory)?await $.fs.list(directory):[];
        if (entries.length>16384) throw new Error('CLAUDUCT_NATIVE_EVENT_LIMIT');
        for (const entry of entries) {
          if (!entry.name.endsWith('.json')) continue;
          const match=/^([1-9][0-9]*)-([A-Za-z0-9_-]{1,200})\.json$/.exec(entry.name);
          const number=match?Number(match[1]):NaN;
          if (entry.kind!=='file' || !Number.isSafeInteger(number)) throw new Error('CLAUDUCT_NATIVE_SEQUENCE_INVALID');
          turnSequence=Math.max(turnSequence,number);
        }
        if (turnSequence>=Number.MAX_SAFE_INTEGER) throw new Error('CLAUDUCT_NATIVE_EVENT_LIMIT');
        const file=directory+'/'+(++turnSequence)+'-'+turn;
        const receipt={session:await session($),agent,turn};
        if (agent) {
          receipt.model=__CLAUDUCT_MODELS__.includes(e.model)?e.model:'unlisted';
          receipt.effort=__CLAUDUCT_EFFORTS__.includes(e.effort)?e.effort:'unlisted';
        }
        // Native has write (including mkdir), but no rename/append. A new filename
        // records each attempt; only the empty marker publishes its finished body.
        // A reader seeing the newer body without its marker refuses the old turn.
        await $.fs.write(file+'.json',JSON.stringify(receipt));
        await $.fs.write(file+'.ready','');
      })();
      const reserved={turn,publication};
      turns.set(agent,reserved); // Reserve in-flight capacity, not successful completion.
      publication.catch(() => { if (turns.get(agent)===reserved) turns.delete(agent); }); // Failed turns remain retryable.
      current=reserved;
    }
    await current.publication;
    await observe($,progress,p,'request_started');
    const name=agent?'child-'+agent:'root';
    // index 0 of an explicit new input is never withheld. Child wakeups without
    // ingress provenance remain outside this control until a later tool step.
    const handback=agent && e.index>0 && !p.intervened ? p.handback || '' : '';
    const eligible=(state.mode==='native_tui' || state.mode==='sdk') && !p.intervened && (e.index>0 && (p.delegated || handback!=='') || !agent && state.rootTurn===turn && state.rootOrigin==='task-notification');
    const sid=await session($);
    await $.fs.write(root+'/step-'+name+'.json',JSON.stringify({session:sid,agent,turn,index:e.index,eligible,mode:state.mode,handback}));
    const active={session:sid,agent,turn,index:e.index,search:false};
    let held=false;
    try {
      beginConfirmation($,state,active);
      if (state.mode!=='native_tui' && state.mode!=='sdk') return yield* next(e);
      const stream=next(e);
      let decision;
      const prefix=[];
      for await (const chunk of stream) {
        // Retry/envelope engine chunks may precede the HTTP response. Resolve
        // the control only at its first semantic chunk, after gateway admission.
        if (eligible && !decision) {
          prefix.push(chunk);
          if (prefix.length>64) throw new Error('CLAUDUCT_PARENT_WAIT_PREFIX_LIMIT');
          if (chunk.kind==='engine') continue;
        }
        if (!decision) {
          const file=root+'/decision-'+name+'.json';
          // A decision this build cannot parse is an unverified decision, not a crash. The
          // refusal three lines down is the deliberate answer for that; a bare JSON.parse
          // threw a SyntaxError that escaped this generator instead, past the only handler
          // that knows what to do about it.
          decision={hold:false};
          if (await $.fs.exists(file)) {
            try { decision=JSON.parse(await $.fs.read(file)); }
            catch { throw new Error('CLAUDUCT_PARENT_WAIT_UNVERIFIED'); }
          }
          if (decision.turn!==turn || decision.index!==e.index) decision={hold:false};
          if (typeof decision.hold!=='boolean' || decision.hold && (!eligible || typeof decision.wait!=='boolean')) throw new Error('CLAUDUCT_PARENT_WAIT_UNVERIFIED');
        }
        if (prefix.length) {if (!decision.hold) yield* prefix;prefix.length=0;}
        else if (!decision.hold) yield chunk;
      }
      if (!decision) yield* prefix;
      const result=await stream.result;
      if (!decision?.hold) return result;
      if (result.toolUses.length || result.answer!=='' || result.stopReason!=='end_turn') throw new Error('CLAUDUCT_PARENT_WAIT_CONTROL_INVALID');
      if (state.mode==='sdk' || handback && !decision.wait) {
        // SDK retries an empty assistant block and rejects a dropped response
        // after a notification. Return an attributed status, never a child result
        // or a claim of completion. A TUI hand-back needs a terminal answer only
        // when its children are done; an interim report keeps the native wait.
        const answer=handback
          ? '[Clauduct] Subagent report handed back; no additional response.'
          : !agent && e.index===0
          ? '[Clauduct] Background task notification received; no additional response.'
          : '[Clauduct] Waiting for background task notification.';
        yield {kind:'text',index:0,text:answer};
        yield {kind:'stop',stopReason:'end_turn',usage:result.usage};
        return {...result,answer,stopReason:'end_turn'};
      }
      held=decision.wait;p.phase=held?'awaiting_children':'progress_unconfirmed';
      await observe($,progress,p,held?'children_wait':'notification_consumed');
      return {...result,answer:'',stopReason:null};
    }
    finally {
      endConfirmation(state,active);
      if (!held) {
        p.phase=p.pendingTools?'tool_pending':'progress_unconfirmed';
        await observe($,progress,p,'request_returned');
      }
    }
  }).catch(async function* ($, e, next) {
    // Before next, any unverified step must refuse instead of invoking native's
    // default fallback. Once called, preserve unrelated upstream failures.
    if (!state.permissionFailed && next.called) return undefined;
    state.permissionFailed=true;
    const answer='[Clauduct] NATIVE_CONFIRMATION_UNVERIFIED: native step could not be verified; restart this session.';
    yield {kind:'text',index:0,text:answer};
    yield {kind:'stop',stopReason:'refusal',usage:null};
    return {turnId:e.turnId,index:e.index,answer,toolUses:[],stopReason:'refusal',usage:null};
  });
  on('tool.call', async ($, e, next) => {
	if (!state.permissionReady || state.permissionFailed) return {deny:'NATIVE_CONFIRMATION_UNVERIFIED'};
	// Settings can change while inference is in flight. Recheck at execution too.
	if ((requiredConfirmations.includes(e.tool) || e.tool.startsWith('mcp__')) && !await confirmationsVerified($)) return {deny:'NATIVE_CONFIRMATION_UNVERIFIED'};
	if (e.tool==='Workflow' && (e.scriptPath!==undefined || e.script===undefined && e.name!==undefined)) {
	  const call=ident(e.tool_use_id),sid=await session($);
	  if (!call) return {deny:'CLAUDUCT_WORKFLOW_SOURCE_UNVERIFIED'};
	  const file=root+'/workflow-source-'+call+'.json';
	  if (!await $.fs.exists(file)) return {deny:'CLAUDUCT_WORKFLOW_SOURCE_UNVERIFIED'};
	  const source=JSON.parse(await $.fs.read(file));
	  if (source.session!==sid || source.call!==call || !Array.isArray(source.paths) || !source.paths.length || source.paths.length>64 || typeof source.trailer!=='string') return {deny:'CLAUDUCT_WORKFLOW_SOURCE_UNVERIFIED'};
	  let selected;
	  for (const path of source.paths) {
	    if (typeof path!=='string' || path.length>4096) return {deny:'CLAUDUCT_WORKFLOW_SOURCE_UNVERIFIED'};
	    if (await $.fs.exists(path)) {selected=path;break;}
	  }
	  if (!selected) return {deny:'CLAUDUCT_WORKFLOW_SOURCE_MISSING: no agent started; provide an existing scriptPath or inline script'};
	  // This is the actual native Read tool. A denial/partial read never falls
	  // back to $.fs.read(source), nor to the original unadapted Workflow.
	  let read;
	  try { read=await $.tool.call({tool:'Read',file_path:selected}); }
	  catch { return {deny:'CLAUDUCT_WORKFLOW_SOURCE_READ_UNVERIFIED'}; }
	  const result=read.result;
	  if (read.deny || read.isError || result?.type!=='text' || result.file?.startLine!==1 || result.file.numLines!==result.file.totalLines || result.file.truncatedByTokenCap || typeof result.file.content!=='string' || result.file.content.includes('__CLAUDUCT_')) return {deny:'CLAUDUCT_WORKFLOW_SOURCE_READ_UNVERIFIED'};
	  let text=result.file.content;
	  if (source.previousTrailer!==undefined) {
	    if (typeof source.previousTrailer!=='string' || !source.previousTrailer) return {deny:'CLAUDUCT_WORKFLOW_SOURCE_UNVERIFIED'};
	    if (text.endsWith(source.previousTrailer)) text=text.slice(0,-source.previousTrailer.length);
	  }
	  const bytes=new TextEncoder().encode(text);
	  if (bytes.length>500*1024) return {deny:'CLAUDUCT_WORKFLOW_SOURCE_TOO_LARGE'};
	  const digest=Array.from(new Uint8Array(await crypto.subtle.digest('SHA-256',bytes)),v=>v.toString(16).padStart(2,'0')).join('');
	  await $.fs.write(root+'/workflow-read-'+call+'.json',JSON.stringify({session:sid,call,path:selected,digest}));
	  e={...e,script:text+source.trailer};delete e.scriptPath;delete e.name;
	}
    const p=progress.get(e.agentId?ident(e.agentId):'');
    if (!p) return next(e);
    // Native can request a long tool's background decision after inference ended.
    // Its auxiliary scope cannot authorize ordinary inference or hosted search.
    const active={session:await session($),agent:p.agent,turn:p.turn,index:p.index,search:e.tool==='WebSearch',auxiliary:e.tool!=='WebSearch'};
    p.handback=''; // Any later tool invalidates the preceding hand-back step.
    // Skill: a forked skill runs in the background in the TUI, like an Agent or Workflow.
    if (e.tool==='Agent' || e.tool==='SendMessage' || e.tool==='Workflow' || e.tool==='Skill') p.delegated=true;
    // Counted inside the guarded region. A throwing observe left the counter raised with
    // nothing to lower it, and session_lifecycle waits for it to reach zero: the session
    // then burned the whole deadline grace and was force-stopped instead of drained.
    p.pendingTools++;p.phase='tool_pending';
    try {
      if (active) beginConfirmation($,state,active);
      await observe($,progress,p,'tool_started');
      const out=await next(e), value=out.result;
      // The native classifier has already accepted and delivered this report.
      // Record identity only; an attempted/denied hand-back grants nothing.
      if (p.agent && e.tool==='SubagentHandback' && !out.deny && !out.isError && value?.success===true) {
        p.handback=ident(e.tool_use_id);
      }
      if (!out.deny && !out.isError && value && e.tool==='Workflow' && value.status==='async_launched' && value.taskType==='local_workflow') {
        const run=ident(value.runId), task=ident(value.taskId), call=ident(e.tool_use_id);
        if (run && task && call) await $.fs.write(root+'/workflow-'+run+'.json',JSON.stringify({session:await session($),run,task,call}));
      }
      if (!out.deny && !out.isError && value && e.tool==='TaskStop') {
        const task=ident(value.task_id), call=ident(e.tool_use_id);
        if (task && call && value.task_type==='local_workflow') await $.fs.write(root+'/stopped-'+task+'.json',JSON.stringify({session:await session($),task,call,type:value.task_type}));
      }
      return out;
    }
    finally {
      if (active) endConfirmation(state,active);
      p.pendingTools--;p.phase=p.pendingTools?'tool_pending':'progress_unconfirmed';
      await observe($,progress,p,'tool_returned');
    }
  }).catch(($, e, next) => {
    if (next.called) return undefined;
    state.permissionFailed=true;
    return {deny:'NATIVE_CONFIRMATION_UNVERIFIED'};
  });
  on('classic.PermissionRequest', async ($, e, next) => {
    const p=progress.get(e.agent_id?ident(e.agent_id):'');
    if (p) { p.permissionRequests++;await observe($,progress,p,'permission_requested'); }
    return next(e);
  });
  on('turn.complete', async ($, e, next) => {
    if (['aborted','error','refusal'].includes(e.reason)) {
      const agent=e.agentId?ident(e.agentId):'',turn=ident(e.turnId);
      if (!turn || e.agentId && !agent) throw new Error('CLAUDUCT_NATIVE_ID_INVALID');
      if (!cancelledTurns.has(turn)) {
        await $.fs.write(root+'/cancel-'+turn+'.json',JSON.stringify({session:await session($),agent,turn,reason:e.reason}));
        cancelledTurns.add(turn);
        // Duplicates follow the turn that just ended; forget the oldest, not the session.
        if (cancelledTurns.size>4096) cancelledTurns.delete(cancelledTurns.values().next().value);
      }
    }
    if (e.agentId) {
      const agent=ident(e.agentId),turn=ident(e.turnId);
      if (!agent || !turn || !['answer','aborted','refusal','error'].includes(e.reason)) throw new Error('CLAUDUCT_NATIVE_END_INVALID');
      await $.fs.write(root+'/end-'+agent+'-'+turn+'.json',JSON.stringify({session:await session($),agent,turn,reason:e.reason}));
    }
    const agent=e.agentId?ident(e.agentId):'';
    const p=progress.get(agent);
    if (p && p.turn===e.turnId && ['answer','aborted','refusal','error'].includes(e.reason)) {
      if (p.phase!=='awaiting_children' || e.reason!=='answer') p.phase='turn_ended';
      await observe($,progress,p,'turn_'+e.reason);
      if (agent && progress.get(agent)===p) progress.delete(agent); // Its receipt file stays.
    }
    // A later step of the same turn would publish it again under a newer sequence.
    if (turns.get(agent)?.turn===ident(e.turnId)) turns.delete(agent);
    return next(e);
  });
};
