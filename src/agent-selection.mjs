import { open, realpath } from 'node:fs/promises';
import { resolve, relative, isAbsolute, dirname, basename, join } from 'node:path';
import { setTimeout as delay } from 'node:timers/promises';
import { selectModel, ROLE_MODELS } from './models.mjs';

const validId = value => typeof value === 'string' && /^[A-Za-z0-9_-]{1,200}$/.test(value);
export const SELECTION_FAILURES = Object.freeze(['IDENTITY', 'PATH', 'SIZE', 'ACCESS', 'IO', 'MISSING', 'PARSE', 'CALL', 'ROLE', 'PARENT', 'MODEL', 'UNKNOWN']);
const fail = (reason = 'UNKNOWN') => { throw Object.assign(new Error('AGENT_SELECTION_UNVERIFIED'), { selectionReason: reason }); };
const aliases = Object.freeze({ haiku: 'luna', sonnet: 'luna', opus: 'sol' });
const model = value => selectModel(Object.hasOwn(aliases, value) ? aliases[value] : value);
const within = (root, path) => { const rel = relative(root, path); return rel !== '' && !isAbsolute(rel) && rel !== '..' && !rel.startsWith('..\\') && !rel.startsWith('../'); };

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
      const buffer = Buffer.alloc(16385);
      const { bytesRead } = await file.read(buffer, 0, buffer.length, 0);
      if (bytesRead > 16384) fail('SIZE');
      return JSON.parse(new TextDecoder('utf-8', { fatal: true }).decode(buffer.subarray(0, bytesRead)));
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
  function remember(message, session, parent) {
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
      const call = { parent, tool: block.name, role: input.subagent_type,
        selection: input.model, skill: typeof input.skill === 'string' && input.skill.length <= 200 ? input.skill : undefined,
        target: block.name === 'SendMessage' && validId(input.to) && typeof input.message === 'string' && input.message.trim() ? input.to : undefined,
        session, created: now };
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
    const previous = verified.get(key(link.sessionId, link.id));
    if (!call || call.tool !== 'SendMessage' || call.target !== link.id || call.parent !== link.parent) fail('CALL');
    // Parent notifications and messages to other sessions are not child resumes.
    if (!previous || previous.parent !== link.parent) return false;
    call.resumeConfirmed = true;
    return true;
  }
  async function resolveSelection(binding, signal) {
    if (!validId(binding.sessionId) || !validId(binding.id)) fail('IDENTITY');
    const deadline = Date.now() + timeoutMs;
    let reason = 'MISSING';
    do {
      signal?.throwIfAborted();
      let metadata;
      try { metadata = await read(binding); }
      catch (error) {
        if (error.selectionReason) throw error;
        if (['EACCES', 'EPERM'].includes(error.code)) fail('ACCESS');
        if (error.code !== 'ENOENT' && !(error instanceof SyntaxError)) fail('IO');
        reason = error instanceof SyntaxError ? 'PARSE' : 'MISSING';
      }
      signal?.throwIfAborted();
      let nativeEntry = false;
      if (metadata && metadata.agentType === binding.role) {
        try { nativeEntry = await nativeFork(binding, metadata); }
        catch (error) {
          if (error.selectionReason) throw error;
          if (['EACCES', 'EPERM'].includes(error.code)) fail('ACCESS');
          if (error.code !== 'ENOENT' && !(error instanceof SyntaxError)) fail('IO');
        }
      }
      signal?.throwIfAborted();
      const skillEntry = metadata && metadata.toolUseId === undefined
        ? [...pending.entries()].find(([, call]) => call.session === binding.sessionId && call.child === binding.id
          && call.tool === 'Skill' && call.skill === metadata.name) : undefined;
      const previous = verified.get(key(binding.sessionId, binding.id));
      const resumeEntry = metadata && previous && previous.role === binding.role
        && previous.origin === metadata.toolUseId && previous.model === metadata.model
        && previous.name === metadata.name && previous.parent === (metadata.parentAgentId ?? undefined)
        ? [...pending.entries()].find(([, call]) => call.session === binding.sessionId && call.target === binding.id
          && call.resumeConfirmed && call.parent === previous.parent) : undefined;
      if (metadata && metadata.agentType === binding.role && (validId(metadata.toolUseId) || skillEntry || resumeEntry || nativeEntry)) {
        const id = skillEntry?.[0] ?? (pending.has(key(binding.sessionId, metadata.toolUseId))
          ? key(binding.sessionId, metadata.toolUseId) : resumeEntry?.[0] ?? key(binding.sessionId, metadata.toolUseId));
        const call = pending.get(id) ?? (nativeEntry ? { parent: undefined, tool: 'NativeFork', role: binding.role } : undefined);
        reason = !call ? 'CALL' : (metadata.parentAgentId ?? undefined) !== call.parent ? 'PARENT' : 'ROLE';
        if (call && (metadata.parentAgentId ?? undefined) === call.parent
          && (!call.role || call.role === binding.role)) {
          const selected = metadata.model;
          if ((['Agent', 'Task'].includes(call.tool) || call.selection !== undefined) && call.selection !== selected) fail('MODEL');
          if (selected !== undefined && typeof selected !== 'string') fail('MODEL');
          const route = selected !== undefined && selected !== 'inherit' ? model(selected) : undefined;
          pending.delete(id);
          const selection = { route: selected === 'inherit' ? undefined : route ?? (Object.hasOwn(ROLE_MODELS, binding.role) ? ROLE_MODELS[binding.role] : undefined),
            source: nativeEntry ? 'native-fork' : resumeEntry?.[0] === id ? 'verified-resume' : selected === 'inherit' ? 'native-inherit' : route ? 'explicit-metadata' : skillEntry ? 'skill-result' : 'role-default',
            sessionId: binding.sessionId, parent: call.parent,
            ...(metadata.name === 'code-review' && (nativeEntry || skillEntry || resumeEntry) && { review: true }) };
          const agentKey = key(binding.sessionId, binding.id);
          verified.delete(agentKey);
          if (verified.size >= 1024) verified.delete(verified.keys().next().value);
          verified.set(agentKey, { role: binding.role, origin: metadata.toolUseId, model: metadata.model,
            name: metadata.name, parent: metadata.parentAgentId ?? undefined });
          return selection;
        }
      } else if (metadata) reason = metadata.agentType !== binding.role ? 'ROLE' : 'IDENTITY';
      if (Date.now() >= deadline) break;
      await delay(Math.min(25, deadline - Date.now()), undefined, { signal });
    } while (true);
    fail(reason);
  }
  return { remember, linkSkill, linkResume, resolve: resolveSelection };
}
