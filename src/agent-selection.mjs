import { open, realpath } from 'node:fs/promises';
import { resolve, relative, isAbsolute, dirname, basename, join } from 'node:path';
import { setTimeout as delay } from 'node:timers/promises';
import { selectModel, ROLE_MODELS } from './models.mjs';

const validId = value => typeof value === 'string' && /^[A-Za-z0-9_-]{1,200}$/.test(value);
export const SELECTION_FAILURES = Object.freeze(['IDENTITY', 'PATH', 'SIZE', 'ACCESS', 'IO', 'MISSING', 'PARSE', 'CALL', 'ROLE', 'PARENT', 'MODEL', 'UNKNOWN']);
export const COMPLETION_FAILURES = Object.freeze(['PARENT_UNVERIFIED', 'PARENT_IDENTITY', 'REGISTRATION',
  'PARENT_COMPLETION', 'TRANSCRIPT_BINDING', 'PARENT_TRANSCRIPT_READ', 'NOTIFICATION_ORIGIN',
  'NOTIFICATION_TIME', 'NOTIFICATION_HEADER', 'CHILD_COMPLETION', 'CHILD_RELATIONSHIP',
  'CHILD_METADATA_READ', 'CHILD_IDENTITY', 'CHILD_TRANSCRIPT_READ', 'CHILD_FINAL_RESPONSE',
  'CHILD_FINAL_TIME', 'PARENT_METADATA_READ', 'IDENTITY_RECHECK', 'EVIDENCE_CHANGED']);
export const COMPLETION_STATES = Object.freeze(['UNRECORDED', 'REQUEST_STARTED', 'RECORDED', 'CONSUMED', 'NONTERMINAL', 'INVALID_RESPONSE']);
const fail = (reason = 'UNKNOWN') => { throw Object.assign(new Error('AGENT_SELECTION_UNVERIFIED'), { selectionReason: reason }); };
export const SELECTION_IO_CODES = Object.freeze(['ENOTDIR', 'EISDIR', 'EMFILE', 'ENFILE', 'EBUSY', 'EINVAL', 'ERR_ENCODING_INVALID_ENCODED_DATA', 'OTHER']);
const failIO = error => { throw Object.assign(new Error('AGENT_SELECTION_UNVERIFIED'), {
  selectionReason: 'IO', selectionIoCode: SELECTION_IO_CODES.includes(error.code) ? error.code : 'OTHER'
}); };
const aliases = Object.freeze({ haiku: 'luna', sonnet: 'luna', opus: 'sol' });
const model = value => selectModel(Object.hasOwn(aliases, value) ? aliases[value] : value);
const within = (root, path) => { const rel = relative(root, path); return rel !== '' && !isAbsolute(rel) && rel !== '..' && !rel.startsWith('..\\') && !rel.startsWith('../'); };
const sameIdentity = (previous, metadata) => previous && metadata && metadata.stoppedByUser !== true
  && previous.role === metadata.agentType && previous.origin === metadata.toolUseId
  && previous.model === metadata.model && previous.name === metadata.name
  && previous.parent === (metadata.parentAgentId ?? undefined);

// Native metadata writes are not awaited before SubagentStart. Only a pending,
// validated parent tool call can make a metadata snapshot usable for a new start.
export function createAgentSelection({ projectsRoot, readMetadata, timeoutMs = 1500 } = {}) {
  const pending = new Map();
  const verified = new Map();
  const startedAt = Date.now();
  const key = (session, call) => `${session}:${call}`;
  async function read(binding, suffix = 'meta.json', fresh = false) {
    if (readMetadata && suffix === 'meta.json') return readMetadata(binding);
    if (!validId(binding.sessionId) || typeof binding.transcriptPath !== 'string') fail('IDENTITY');
    const root = await realpath(projectsRoot);
    const transcript = resolve(binding.transcriptPath);
    if (basename(transcript) !== `${binding.sessionId}.jsonl` || !within(root, transcript)) fail('PATH');
    const path = join(dirname(transcript), binding.sessionId, 'subagents', `agent-${binding.id}.${suffix}`);
    const canonical = await realpath(path);
    // Do not follow metadata symlinks outside the native project tree.
    if (!within(root, canonical) || canonical.toLowerCase() !== path.toLowerCase()) fail('PATH');
    const file = await open(canonical, 'r');
    try {
      const stat = await file.stat();
      if (!stat.isFile()) fail();
      if (fresh && stat.birthtimeMs < startedAt) fail('IDENTITY');
      const tail = suffix === 'jsonl', limit = tail ? 1048576 : 16384;
      const offset = tail ? Math.max(0, stat.size - limit) : 0;
      const buffer = Buffer.alloc(limit + Number(!tail));
      const { bytesRead } = await file.read(buffer, 0, buffer.length, offset);
      if (bytesRead > limit) fail('SIZE');
      // Discard only the partial first line before decoding a bounded UTF-8 tail.
      const start = offset ? buffer.indexOf(10) + 1 : 0;
      if (offset && !start) fail('SIZE');
      const text = new TextDecoder('utf-8', { fatal: true }).decode(buffer.subarray(start, bytesRead));
      return tail ? text.trimEnd().split('\n').filter(Boolean).map(line => JSON.parse(line)) : JSON.parse(text);
    } finally { await file.close(); }
  }
  async function nativeFork(binding, metadata) {
    // Native writes both sidecars before launching a fork. A live authenticated
    // Start plus exact, fresh sidecars is an origin independent of model tool IDs.
    if (!projectsRoot || binding.nativeRegistered !== true || metadata.toolUseId !== undefined
      || metadata.parentAgentId != null || metadata.spawnDepth !== 1
      || metadata.stoppedByUser === true
      || binding.role !== 'general-purpose' || !validId(metadata.name)
      || verified.has(key(binding.sessionId, binding.id))) return false;
    // A model-originated Skill still requires its exact PostToolUse link.
    if ([...pending.values()].some(call => call.session === binding.sessionId && call.tool === 'Skill'
      && call.parent === undefined && call.skill === metadata.name)) return false;
    const marker = await read(binding, 'forked-skill.marker.json', true);
    const scope = await read(binding, 'forked-skill.json', true);
    if (marker?.forkedSkill !== true || marker.skillName !== metadata.name
      || scope?.skillName !== metadata.name || scope.attributionName !== metadata.name) fail('IDENTITY');
    return true;
  }
  function remember(message, session, parent, parentRoute) {
    if (!validId(session)) return;
    const now = Date.now();
    for (const [id, call] of pending) if (now - call.created > 300000) pending.delete(id);
    const additions = new Map();
    for (const block of message.content) {
      if (block.type !== 'tool_use' || !['Agent', 'Task', 'Skill', 'SendMessage', 'Workflow'].includes(block.name)) continue;
      if (!validId(block.id)) fail();
      const input = block.input ?? {};
      if (input.model !== undefined) {
        if (typeof input.model !== 'string') fail();
        if (input.model !== 'inherit') model(input.model);
      }
      if (input.subagent_type !== undefined && (typeof input.subagent_type !== 'string'
        || input.subagent_type.length === 0 || input.subagent_type.length > 200)) fail();
      let inheritedRoute;
      if (input.model === 'inherit') {
        if (typeof parentRoute?.model !== 'string' || typeof parentRoute?.effort !== 'string') fail('MODEL');
        inheritedRoute = Object.freeze(selectModel(parentRoute.model, parentRoute.effort));
      }
      const call = { parent, tool: block.name, role: input.subagent_type,
        selection: input.model, inheritedRoute, skill: typeof input.skill === 'string' && input.skill.length <= 200 ? input.skill : undefined,
        target: block.name === 'SendMessage' && validId(input.to) && typeof input.message === 'string' && input.message.trim() ? input.to : undefined,
        session, created: now };
      // Native peers may address their parent by name; resolve only that verified relationship.
      if (call.target) {
        call.recipient = call.target;
        const sender = verified.get(key(session, parent));
        const owner = sender?.parent && verified.get(key(session, sender.parent));
        if (owner?.name === call.target) call.target = sender.parent;
      }
      // Bounded outstanding metadata, never a cumulative session execution limit.
      const id = key(session, block.id);
      if (pending.size + additions.size >= 1024 || pending.has(id) || additions.has(id)) fail();
      additions.set(id, call);
    }
    // Validate the whole response before publishing any routing evidence.
    for (const [id, call] of additions) pending.set(id, call);
  }
  function linkSkill(link) {
    const call = pending.get(key(link.sessionId, link.toolUseId));
    if (!validId(link.id) || !call || call.tool !== 'Skill'
      || typeof call.skill !== 'string' || !call.skill || call.skill !== link.skill
      || call.parent !== link.parent || (call.child !== undefined && call.child !== link.id)) fail('CALL');
    if ([...pending.values()].some(other => other !== call && other.session === link.sessionId && other.child === link.id)) fail('CALL');
    call.child = link.id;
  }
  function linkResume(link) {
    const call = pending.get(key(link.sessionId, link.toolUseId));
    if (!call || call.tool !== 'SendMessage' || call.recipient !== link.id || call.parent !== link.parent) fail('CALL');
    const previous = verified.get(key(link.sessionId, call.target));
    const sender = verified.get(key(link.sessionId, link.parent));
    const peer = previous && sender?.parent === call.target;
    // Require the verified parent-child relationship in either direction, not a name search.
    if (!previous || (previous.parent !== link.parent && !peer)) return false;
    call.resumeConfirmed = true;
    call.resumeParent = previous.parent;
    call.peerResume = Boolean(peer);
    return true;
  }
  function begin(session, id) {
    const state = verified.get(key(session, id));
    if (!state) return;
    state.completion = undefined;
    state.completionState = 'REQUEST_STARTED';
    return state.request = {};
  }
  function delivered(session, id, request, message) {
    const state = verified.get(key(session, id));
    if (state && request && state.request === request) {
      state.completionState = message.stop_reason !== 'end_turn' ? 'NONTERMINAL' : !validId(message.id) ? 'INVALID_RESPONSE' : 'RECORDED';
      if (state.completionState === 'RECORDED') state.completion = { id: message.id, at: Date.now() };
    }
  }
  async function completionResume(binding, previous, signal, evidence) {
    evidence.parent = previous.completionState ?? 'UNRECORDED';
    evidence.stage = 'REGISTRATION';
    if (!projectsRoot || binding.nativeRegistered !== true) return;
    evidence.stage = 'PARENT_COMPLETION';
    if (!previous.completion) return;
    evidence.stage = 'TRANSCRIPT_BINDING';
    if (previous.transcriptPath !== binding.transcriptPath) return;
    const parentCompletion = previous.completion;
    evidence.stage = 'PARENT_TRANSCRIPT_READ';
    const records = await read(binding, 'jsonl');
    const latest = records.findLast(row => ['user', 'assistant'].includes(row.type));
    evidence.stage = 'NOTIFICATION_ORIGIN';
    if (latest?.type !== 'user' || latest.isMeta !== true || latest.origin?.kind !== 'task-notification'
      || latest.sessionId !== binding.sessionId || latest.agentId !== binding.id || !validId(latest.uuid)
      || latest.uuid === previous.notification || typeof latest.message?.content !== 'string') return;
    const at = Date.parse(latest.timestamp);
    evidence.stage = 'NOTIFICATION_TIME';
    if (!Number.isFinite(at) || at < parentCompletion.at || at > Date.now() || Date.now() - at > 300000) return;
    // Native 2.1.266 prepends a harness notice. Only the first outer header is
    // parsed; quoted result text cannot provide task identity or completion state.
    const text = latest.message.content;
    const header = /<task-notification>\s*<task-id>([A-Za-z0-9_-]{1,200})<\/task-id>\s*(?:<tool-use-id>([A-Za-z0-9_-]{1,200})<\/tool-use-id>\s*)?(?:<output-file>[^<]*<\/output-file>\s*)?<status>completed<\/status>\s*<summary>/.exec(text);
    evidence.stage = 'NOTIFICATION_HEADER';
    if (!header || text.indexOf('<task-notification>') !== header.index
      || text.indexOf('<task-notification>', header.index + 1) !== -1
      || !text.trimEnd().endsWith('</task-notification>')) return;
    const child = verified.get(key(binding.sessionId, header[1]));
    const completion = child?.completion;
    evidence.child = child?.completionState ?? 'UNRECORDED';
    evidence.stage = 'CHILD_COMPLETION';
    if (!completion) return;
    evidence.stage = 'CHILD_RELATIONSHIP';
    if (child.parent !== binding.id || child.transcriptPath !== binding.transcriptPath
      || (header[2] !== undefined && header[2] !== child.origin) || completion.at > at) return;
    const childBinding = { ...binding, id: header[1], role: child.role };
    evidence.stage = 'CHILD_METADATA_READ';
    const childMetadata = await read(childBinding);
    evidence.stage = 'CHILD_IDENTITY';
    if (!sameIdentity(child, childMetadata)) return;
    evidence.stage = 'CHILD_TRANSCRIPT_READ';
    const childRecords = await read(childBinding, 'jsonl');
    const final = childRecords.findLast(row => ['user', 'assistant'].includes(row.type));
    evidence.stage = 'CHILD_FINAL_RESPONSE';
    if (final?.type !== 'assistant' || final.isApiErrorMessage === true
      || final.sessionId !== binding.sessionId || final.agentId !== header[1]
      || final.message?.id !== completion.id || final.message.stop_reason !== 'end_turn') return;
    const finalAt = Date.parse(final.timestamp);
    evidence.stage = 'CHILD_FINAL_TIME';
    if (!Number.isFinite(finalAt) || finalAt < startedAt || finalAt > at) return;
    // Recheck identities after asynchronous reads; cancellation consumes nothing.
    evidence.stage = 'PARENT_METADATA_READ';
    const parentRecheck = await read(binding);
    evidence.stage = 'CHILD_METADATA_READ';
    const childRecheck = await read(childBinding);
    evidence.stage = 'IDENTITY_RECHECK';
    if (!sameIdentity(previous, parentRecheck) || !sameIdentity(child, childRecheck)) return;
    signal?.throwIfAborted();
    evidence.stage = 'EVIDENCE_CHANGED';
    if (verified.get(key(binding.sessionId, binding.id)) !== previous
      || previous.completion !== parentCompletion
      || verified.get(key(binding.sessionId, header[1])) !== child || child.completion !== completion) return;
    return { child, completion, parentCompletion, notification: latest.uuid };
  }
  async function resolveSelection(binding, signal, evidence) {
    if (!validId(binding.sessionId) || !validId(binding.id)) fail('IDENTITY');
    const deadline = Date.now() + timeoutMs;
    let reason = 'MISSING';
    do {
      signal?.throwIfAborted();
      evidence.stage = verified.has(key(binding.sessionId, binding.id)) ? 'PARENT_METADATA_READ' : null;
      let metadata;
      try { metadata = await read(binding); }
      catch (error) {
        if (error.selectionReason) throw error;
        if (['EACCES', 'EPERM'].includes(error.code)) fail('ACCESS');
        if (error.code !== 'ENOENT' && !(error instanceof SyntaxError)) failIO(error);
        reason = error instanceof SyntaxError ? 'PARSE' : 'MISSING';
      }
      signal?.throwIfAborted();
      let nativeEntry = false;
      if (metadata && metadata.agentType === binding.role) {
        try { nativeEntry = await nativeFork(binding, metadata); }
        catch (error) {
          if (error.selectionReason) throw error;
          if (['EACCES', 'EPERM'].includes(error.code)) fail('ACCESS');
          if (error.code !== 'ENOENT' && !(error instanceof SyntaxError)) failIO(error);
        }
      }
      signal?.throwIfAborted();
      const skillEntry = metadata && metadata.toolUseId === undefined
        ? [...pending.entries()].find(([, call]) => call.session === binding.sessionId && call.child === binding.id
          && call.tool === 'Skill' && call.skill === metadata.name) : undefined;
      const previous = verified.get(key(binding.sessionId, binding.id));
      evidence.stage = previous ? 'PARENT_IDENTITY' : 'PARENT_UNVERIFIED';
      evidence.parent = previous?.completionState ?? 'UNRECORDED';
      evidence.child = null;
      const resumeEntry = sameIdentity(previous, metadata) && previous.role === binding.role
        ? [...pending.entries()].find(([, call]) => call.session === binding.sessionId && call.target === binding.id
          && call.resumeConfirmed && call.resumeParent === previous.parent) : undefined;
      if (!resumeEntry && sameIdentity(previous, metadata) && previous.role === binding.role) {
        let completion;
        try { completion = await completionResume(binding, previous, signal, evidence); }
        catch (error) {
          if (error.selectionReason) throw error;
          if (['EACCES', 'EPERM'].includes(error.code)) fail('ACCESS');
          if (error.code !== 'ENOENT' && !(error instanceof SyntaxError)) {
            signal?.throwIfAborted(); failIO(error);
          }
        }
        signal?.throwIfAborted();
        if (completion && completion.child.completion === completion.completion
          && previous.completion === completion.parentCompletion) {
          completion.child.completion = undefined;
          completion.child.completionState = 'CONSUMED';
          previous.completion = undefined;
          previous.completionState = 'CONSUMED';
          previous.notification = completion.notification;
          return { ...previous.selection, source: 'verified-completion-resume' };
        }
      }
      if (metadata && metadata.agentType === binding.role && (validId(metadata.toolUseId) || skillEntry || resumeEntry || nativeEntry)) {
        const id = skillEntry?.[0] ?? (pending.has(key(binding.sessionId, metadata.toolUseId))
          ? key(binding.sessionId, metadata.toolUseId) : resumeEntry?.[0] ?? key(binding.sessionId, metadata.toolUseId));
        const call = pending.get(id) ?? (nativeEntry ? { parent: undefined, tool: 'NativeFork', role: binding.role } : undefined);
        const parent = resumeEntry?.[0] === id ? call.resumeParent : call?.parent;
        reason = !call ? 'CALL' : (metadata.parentAgentId ?? undefined) !== parent ? 'PARENT' : 'ROLE';
        if (call && (metadata.parentAgentId ?? undefined) === parent
          && (!call.role || call.role === binding.role)) {
          const selected = metadata.model;
          if ((['Agent', 'Task'].includes(call.tool) || call.selection !== undefined) && call.selection !== selected) fail('MODEL');
          if (selected !== undefined && typeof selected !== 'string') fail('MODEL');
          const route = selected !== undefined && selected !== 'inherit' ? model(selected) : undefined;
          const inheritedRoute = selected === 'inherit'
            ? (resumeEntry?.[0] === id ? previous.selection.route : call.inheritedRoute) : undefined;
          if (selected === 'inherit' && !inheritedRoute) fail('MODEL');
          pending.delete(id);
          const selection = { route: inheritedRoute ?? route ?? (Object.hasOwn(ROLE_MODELS, binding.role) ? ROLE_MODELS[binding.role] : undefined),
            source: nativeEntry ? 'native-fork' : resumeEntry?.[0] === id ? (call.peerResume ? 'verified-peer-resume' : 'verified-resume') : selected === 'inherit' ? 'native-inherit' : route ? 'explicit-metadata' : skillEntry ? 'skill-result' : 'role-default',
            sessionId: binding.sessionId, parent,
            ...(metadata.name === 'code-review' && (nativeEntry || skillEntry || resumeEntry) && { review: true }) };
          selection.reviewContext = selection.review === true || (resumeEntry?.[0] === id
            ? previous.reviewContext === true
            : ['Agent', 'Task'].includes(call.tool) && verified.get(key(binding.sessionId, parent))?.reviewContext === true);
          const agentKey = key(binding.sessionId, binding.id);
          verified.delete(agentKey);
          if (verified.size >= 1024) verified.delete(verified.keys().next().value);
          verified.set(agentKey, { role: binding.role, origin: metadata.toolUseId, model: metadata.model,
            name: metadata.name, parent: metadata.parentAgentId ?? undefined, reviewContext: selection.reviewContext,
            transcriptPath: binding.transcriptPath, selection, notification: previous?.notification });
          return selection;
        }
      } else if (metadata) reason = metadata.agentType !== binding.role ? 'ROLE' : 'IDENTITY';
      if (Date.now() >= deadline) break;
      await delay(Math.min(25, deadline - Date.now()), undefined, { signal });
    } while (true);
    fail(reason);
  }
  async function resolveWithDiagnostics(binding, signal) {
    const evidence = { stage: null, parent: null, child: null };
    try {
      return await resolveSelection(binding, signal, evidence);
    } catch (error) {
      error.completionFailure = evidence.stage;
      error.completionParentState = evidence.parent;
      error.completionChildState = evidence.child;
      throw error;
    }
  }
  return { remember, linkSkill, linkResume, begin, delivered, resolve: resolveWithDiagnostics };
}
