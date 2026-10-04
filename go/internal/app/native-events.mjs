// No environment mutation, arbitrary process or report-body access. Native identity,
// progress and terminal receipts are recorded. Workflow source files use native Read
// with its permission checks. A verified dependency wait consumes only the gateway's
// empty control response. Native commands are untouched.
const root = __CLAUDUCT_EVENT_ROOT__;
const confirmationHelper = __CLAUDUCT_CONFIRMATION_HELPER__;
const confirmationArgs = __CLAUDUCT_CONFIRMATION_ARGS__;
// Permission rules are native's own (v0.6.4): this module adds and checks none. What it
// keeps is the origin proof -- a fresh nonce answered for the live session/Agent/turn/step.
async function preparePermissions($, state) {
  if (state.permissionFailed) {
    state.permissionFailed=true;
    throw new Error('NATIVE_CONFIRMATION_UNVERIFIED');
  }
  // Reload keeps the gateway's last challenge. Consume its nonce before this
  // module forwards any new request, so a finished helper cannot poison it.
  state.permissionBaseline ??= (async()=>{
    const file=root+'/confirmation-request.json';
    if (await $.fs.exists(file)) {
      const previous=JSON.parse(await $.fs.read(file));
      if (!/^[A-Za-z0-9_-]{43}$/.test(previous.nonce)) throw new Error('NATIVE_CONFIRMATION_UNVERIFIED');
      state.permissionNonce=previous.nonce;
    }
  })();
  await state.permissionBaseline;
  state.permissionReady=true;
}
async function answerConfirmations($, state) {
  if (state.permissionJob || state.permissionFailed || state.permissionClosed) return;
  const job={};state.permissionJob=job;
  try {
    const file=root+'/confirmation-request.json';
    if (!await $.fs.exists(file)) return;
    const request=JSON.parse(await $.fs.read(file));
    if (!/^[A-Za-z0-9_-]{43}$/.test(request.nonce) || !ident(request.session) || typeof request.search!=='boolean' || request.auxiliary!==undefined && typeof request.auxiliary!=='boolean') throw new Error('NATIVE_CONFIRMATION_UNVERIFIED');
    if (request.nonce===state.permissionNonce) return;
    state.permissionNonce=request.nonce;
    if (state.cancelledTurns.has(request.turn)) return;
    let step=await session($)===request.session && [...state.requests].find(p=>p.session===request.session && p.agent===request.agent && p.turn===request.turn && p.index===request.index && p.search===request.search && !!p.auxiliary===!!request.auxiliary);
    if (JSON.parse(await $.fs.read(file)).nonce!==request.nonce) return;
    if (step && !state.requests.has(step)) {
      // Its scope ended during the awaits above; an identical twin may still own
      // a live request. Without one the request is answered unmatched: silence
      // would time out the handshake and latch every later request.
      step=[...state.requests].find(p=>p.session===step.session && p.agent===step.agent && p.turn===step.turn && p.index===step.index && p.search===step.search && !!p.auxiliary===!!step.auxiliary);
    }
    const confirmations=!!step && !state.permissionFailed;
    const unmatched=!step && !state.permissionFailed;
    const reply={...request,agent:step?.agent||'',turn:step?.turn||'',index:step?.index??-1,confirmations,unmatched};
    if (state.permissionClosed || state.permissionJob!==job) return;
    if (state.permissionLeases.size>=64) throw new Error('CLAUDUCT_NATIVE_EVENT_LIMIT');
    const stream=$.process.spawn({argv:[confirmationHelper,...confirmationArgs],input:JSON.stringify(reply)});
    const lease={stream,active:step,stopped:false};state.permissionLeases.add(lease);job.lease=lease;
    let marker='';
    do {
      let chunk;
      try {chunk=await stream.next();}
      catch {await closeConfirmation(state,lease);return;}
      if (chunk.done && chunk.value?.code===3) {await closeConfirmation(state,lease);return;} // stale: the request already ended
      if (chunk.done || chunk.value.stream!=='stdout') throw new Error('NATIVE_CONFIRMATION_UNVERIFIED');
      marker+=chunk.value.text;
      if (marker.length>64 || state.permissionClosed || lease.stopped) throw new Error('NATIVE_CONFIRMATION_UNVERIFIED');
    } while(!marker.includes('\n'));
    if (marker!=='CLAUDUCT_CONFIRMATION_CONNECTED\n') throw new Error('NATIVE_CONFIRMATION_UNVERIFIED');
    void (async()=>{
      try {
        const ended=await stream.next();
        if (!ended.done || ended.value.code!==0) {
          if (!lease.stopped && !state.permissionClosed && !state.cancelledTurns.has(lease.active?.turn)) state.permissionFailed=true;
          throw new Error('NATIVE_CONFIRMATION_UNVERIFIED');
        }
        state.permissionLeases.delete(lease);
      } catch {
        // Native abort can interrupt this pull before turn cleanup runs. Close
        // this request's proof; a cancelled connection is not a policy failure.
        await closeConfirmation(state,lease);
      }

    })().catch(()=>{state.permissionFailed=true;});
  } catch {
    if (state.permissionJob===job && !job.lease?.stopped && !state.permissionClosed) state.permissionFailed=true;
    if (job.lease) await closeConfirmation(state,job.lease);
  }
  finally {if(state.permissionJob===job) state.permissionJob=null;}
}
async function settleConfirmations(promises) {
  const results=await Promise.allSettled(promises);
  const failed=results.find(result=>result.status==='rejected');
  if (failed) throw failed.reason;
}
async function closeConfirmation(state, lease) {
  lease.stopped=true;
  try {await lease.stream.return();state.permissionLeases.delete(lease);}
  catch (error) {state.permissionFailed=true;throw error;}
}
function beginConfirmation($, state, active) {
  if (state.permissionClosed) throw new Error('NATIVE_CONFIRMATION_UNVERIFIED');
  if (state.requests.size>=4096) throw new Error('CLAUDUCT_NATIVE_EVENT_LIMIT');
  if (!state.requests.size) {
    state.permissionTimer?.cancel();
    state.permissionTimer=$.clock.every(25,()=>{void answerConfirmations($,state).catch(()=>{state.permissionFailed=true;});});
  }
  state.requests.add(active);
}
// A scope that ends normally leaves an admitted request alone: its proof closes
// with that HTTP request. Cutting it would fail work native still awaits
// (measured: a child's progress check outlived its step by seconds and native
// ended it itself). Cancellation closes the turn's proofs instead (closeTurn).
async function endConfirmation($, state, active) {
  state.requests.delete(active);
  if (!state.requests.size) {
    state.permissionTimer?.cancel();
    // A late request needs an explicit unmatched reply, not a storage-failure latch.
    state.permissionTimer=state.permissionClosed?null:$.clock.every(250,()=>{void answerConfirmations($,state).catch(()=>{state.permissionFailed=true;});});
  }
}
async function closeTurn(state, agent, turn) {
  if (state.permissionJob?.lease?.active?.agent===agent && state.permissionJob.lease.active.turn===turn) state.permissionJob=null;
  await settleConfirmations([...state.permissionLeases].filter(lease=>lease.active?.agent===agent && lease.active?.turn===turn).map(lease=>closeConfirmation(state,lease)));
}
async function cancelTurn($, state, scope, reason) {
    if (state.cancelledTurns.has(scope.turn)) return;
    state.cancelledTurns.add(scope.turn);
    try {await $.fs.write(root+'/cancel-'+scope.turn+'.json',JSON.stringify({session:scope.session||await session($),agent:scope.agent,turn:scope.turn,reason}));}
    catch (error) {state.cancelledTurns.delete(scope.turn);state.permissionFailed=true;throw error;}
    if (state.cancelledTurns.size>4096) state.cancelledTurns.delete(state.cancelledTurns.values().next().value);
  }
const ident = value => typeof value === 'string' && /^[A-Za-z0-9_-]{1,200}$/.test(value) ? value : '';
// Same tag as the gateway's nativeToolUseID: one native step, not a secret.
const toolMark = /^[A-Za-z0-9_-]+__cdt([0-9a-f]{12})$/;
async function stepTag(p) {
  const bytes=new TextEncoder().encode([p.session,p.agent,p.turn,String(p.index)].join('\n'));
  return Array.from(new Uint8Array(await crypto.subtle.digest('SHA-256',bytes)).slice(0,6),v=>v.toString(16).padStart(2,'0')).join('');
}
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
  const state = {mode:'unclassified',peerMode:'unclassified',permissionReady:false,requests:new Set(),permissionLeases:new Set()};
  const turns = new Map();
  let turnSequence=0;
  const cancelledTurns = state.cancelledTurns=new Set();
  const progress = new Map();
  on('agent.spawn', async ($, e, next) => {
    const sid=await session($),call=ident(e.tool_use_id),parent=e.parentAgentId?ident(e.parentAgentId):'';
    if (!call || e.parentAgentId && !parent) throw new Error('CLAUDUCT_NATIVE_ID_INVALID');
    const file=root+'/selection-'+sid+'-'+call+'.json';
    let forwarded=e; // native/plugin-owned choices retain their own route
    if (await $.fs.exists(file)) {
      let choice;
      try { choice=JSON.parse(await $.fs.read(file)); }
      catch { throw new Error('CLAUDUCT_NATIVE_SELECTION_UNVERIFIED'); }
      if (choice.session!==sid || choice.call!==call || choice.parent!==parent || choice.role!==e.subagentType || !__CLAUDUCT_MODELS__.includes(choice.model)) throw new Error('CLAUDUCT_NATIVE_SELECTION_UNVERIFIED');
      if (!e.fork) forwarded={...e,model:choice.model}; // native owns fork inheritance
    }
    const answer=await next(forwarded);
    // An Agent teams teammate's HTTP requests carry its address (<name>@<team>) while its
    // hook events carry the loop id (native 2.1.289). Record the pair for the gateway (#269).
    if (e.isTeammate===true && answer?.deny===undefined) {
      const agent=ident(answer?.agentId), address=answer?.teammateId;
      if (!agent || typeof address!=='string' || !/^[A-Za-z0-9_-]{1,100}@[A-Za-z0-9_-]{1,100}$/.test(address)) throw new Error('CLAUDUCT_NATIVE_ID_INVALID');
      await $.fs.write(root+'/teammate-'+sid+'-'+address+'.json',JSON.stringify({session:sid,agent,address,call,role:e.subagentType}));
    }
    return answer;
  });
  on('prompt.submit', async ($, e, next) => {
    const origin=e.origin?.kind || 'unclassified';
    if (origin==='composer') state.mode='native_tui';
    else if (origin==='sdk') state.mode='sdk';
    else if (origin==='peer' && state.mode==='unclassified') state.mode=state.peerMode;
    const p=progress.get('');
    if (e.turnId && p?.turn===e.turnId && origin!=='task-notification') p.intervened=true;
    return next(e);
  });
  on('turn.start', async ($, e, next) => {
    const restart=state.permissionRestart;
    // Native /clear continues under another session ID without session.start.
    // Only a fresh native turn can reopen it, never a delayed old tool hook.
    if (state.permissionClosed && restart && ident(e.turnId) && e.turnId!==restart.turn) {
      const sid=await session($);
      if (sid!==restart.session) {
        await preparePermissions($,state);
        if (state.permissionClosed && state.permissionRestart===restart) {
          state.permissionClosed=false;state.permissionRestart=null;
        }
      }
    }
    state.rootTurn=e.turnId;
    return next(e);
  });
  on('session.start', async ($, e, next) => {
    // Native 2.1.282 delivers SendMessage as peer on both surfaces. Its
    // session evidence selects the response contract; other origins stay unknown.
    state.peerMode=e.isInteractive===true?'native_tui':e.isInteractive===false?'sdk':'unclassified';
    await preparePermissions($,state);
    state.permissionClosed=false;state.permissionRestart=null;
    if (!state.permissionTimer) state.permissionTimer=$.clock.every(250,()=>{void answerConfirmations($,state).catch(()=>{state.permissionFailed=true;});});
    await $.fs.write(root + '/ready.json', JSON.stringify({session:await session($)}));
    return next(e);
  });
  on('session.end', async ($, e, next) => {
    state.permissionClosed=true;
    state.permissionRestart=['clear','resume'].includes(e.reason) && ident(e.sessionId)?{session:e.sessionId,turn:state.rootTurn}:null;
    const scopes=[...progress.values()].filter(p=>p.session && p.phase!=='turn_ended');
    for (const p of progress.values()) p.phase='turn_ended';
    state.permissionTimer?.cancel();state.permissionTimer=null;
    state.requests.clear();
    state.permissionJob=null;
    await settleConfirmations([...state.permissionLeases].map(lease=>closeConfirmation(state,lease)).concat(scopes.map(p=>cancelTurn($,state,p,'aborted'))));
    return next(e);
  });
  on('turn.step', async function* ($, e, next) {
    await preparePermissions($,state);
    const agent=e.agentId?ident(e.agentId):'', turn=ident(e.turnId);
    if (!turn || (e.agentId && !agent)) throw new Error('CLAUDUCT_NATIVE_ID_INVALID');
    if (state.permissionClosed && state.permissionRestart) {
      // An anonymous typed fork has no turn.start to prove the replacement
      // session. Refuse that scope without latching later verified user turns.
      const answer='[Clauduct] NATIVE_REQUEST_ORIGIN_UNVERIFIED: no live native turn proved this request; this request was not sent.';
      yield {kind:'text',index:0,text:answer};
      yield {kind:'stop',stopReason:'refusal',usage:null};
      return {turnId:e.turnId,index:e.index,answer,toolUses:[],stopReason:'refusal',usage:null};
    }
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
    // prompt.submit has the previous turn's ID; turn.start mints a new ID
    // without origin. Their ordering cannot attest a first-step peer input.
    const eligible=(state.mode==='native_tui' || state.mode==='sdk') && !p.intervened && e.index>0 && (p.delegated || handback!=='');
    const sid=await session($);
    p.session=sid;
    await $.fs.write(root+'/step-'+name+'.json',JSON.stringify({session:sid,agent,turn,index:e.index,eligible,mode:state.mode,handback,peer:false}));
    const active={session:sid,agent,turn,index:e.index,search:false};
    // Native also asks auxiliary questions about a step while it is still inferring
    // (a child's progress check after ~30 s). Same Agent/turn/step proof unit; the
    // ordinary inference request cannot borrow it (auxiliary must match).
    const asking={...active,auxiliary:true};
    let held=false;
    try {
      beginConfirmation($,state,active);beginConfirmation($,state,asking);
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
      await endConfirmation($,state,active);await endConfirmation($,state,asking);
      if (!held && p.phase!=='turn_ended') {
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
    // Model calls carry the gateway's mark of the step they were issued to.
    // An unmarked call is a hook module's direct $.tool.call: native rules
    // still apply, but it gets no turn scope and cannot delegate (#214).
    const mark=toolMark.exec(typeof e.tool_use_id==='string'?e.tool_use_id:'');
    const agent=e.agentId?ident(e.agentId):'',p=mark?progress.get(agent):undefined;
    if (state.permissionClosed || mark && (e.agentId && !agent || !p || p.phase==='turn_ended' || cancelledTurns.has(p.turn) || mark[1]!==await stepTag(p))) return {deny:'NATIVE_REQUEST_ORIGIN_UNVERIFIED'};
	if (!state.permissionReady || state.permissionFailed) return {deny:'NATIVE_CONFIRMATION_UNVERIFIED'};
    if (!mark) return ['Agent','SendMessage','Workflow','Skill'].includes(e.tool)?{deny:'NATIVE_DIRECT_DELEGATION_UNSUPPORTED'}:next(e);
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
    // Native can request a long tool's background decision after inference ended.
    // Its auxiliary scope cannot authorize ordinary inference or hosted search.
    const active={session:await session($),agent:p.agent,turn:p.turn,index:p.index,search:e.tool==='WebSearch',auxiliary:e.tool!=='WebSearch'};
    if (state.permissionClosed || progress.get(p.agent)!==p || p.phase==='turn_ended' || cancelledTurns.has(p.turn)) return {deny:'NATIVE_REQUEST_ORIGIN_UNVERIFIED'};
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
      try { if (active) await endConfirmation($,state,active); }
      finally {
        p.pendingTools--;if (p.phase!=='turn_ended') p.phase=p.pendingTools?'tool_pending':'progress_unconfirmed';
        await observe($,progress,p,'tool_returned');
      }
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
      const p=progress.get(agent);if (p?.turn===turn) p.phase='turn_ended';
      await settleConfirmations([...state.requests].filter(active=>active.agent===agent && active.turn===turn).map(active=>endConfirmation($,state,active)).concat(closeTurn(state,agent,turn),cancelTurn($,state,{session:p?.turn===turn?p.session:'',agent,turn},e.reason)));
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
