import { createHash } from 'node:crypto';
import { open, realpath } from 'node:fs/promises';
import { basename, dirname, isAbsolute, join, relative, resolve } from 'node:path';
import { selectModel } from './models.mjs';

const fail = reason => { throw Object.assign(new Error('AGENT_SELECTION_UNVERIFIED'), { selectionReason: reason }); };
const within = (root, path) => { const rel = relative(root, path); return rel !== '' && !isAbsolute(rel) && rel !== '..' && !rel.startsWith('../') && !rel.startsWith('..\\'); };
export const workflowDigest = text => createHash('sha256').update(text).digest('hex');
const id = value => typeof value === 'string' && /^[A-Za-z0-9_-]{1,200}$/.test(value);

// A live native registration plus an authenticated Workflow result binds the
// exact run directory. Never discover children by scanning arbitrary folders.
export function createWorkflowSelection(projectsRoot) {
  const runs = new Map(), children = new Map();
  const key = (session, value) => `${session}:${value}`;
  function link(link, call) {
    if (!call?.workflow || call.parent !== link.parent || call.session !== link.sessionId
      || call.workflow.digest !== link.scriptDigest || !/^wf_[a-z0-9-]{6,}$/.test(link.runId)
      || !/^[A-Za-z0-9_-]{1,100}$/.test(link.workflowName) || !id(link.taskId)
      || !id(link.sessionId) || typeof link.transcriptPath !== 'string'
      || basename(link.transcriptPath) !== `${link.sessionId}.jsonl`) fail('CALL');
    const root = join(dirname(resolve(link.transcriptPath)), link.sessionId);
    const directory = join(root, 'subagents', 'workflows', link.runId);
    const script = join(root, 'workflows', 'scripts', `${link.workflowName}-${link.runId}.js`);
    if (resolve(link.transcriptDir) !== directory || resolve(link.scriptPath) !== script) fail('PATH');
    const runKey = key(link.sessionId, link.runId);
    if (runs.has(runKey) || runs.size >= 1024) fail('CALL');
    runs.set(runKey, { ...link, directory, script, route: call.workflow.route, created: call.created });
  }
  async function read(path, limit) {
    const root = await realpath(projectsRoot), canonical = await realpath(path);
    if (!within(root, canonical) || canonical.toLowerCase() !== path.toLowerCase()) fail('PATH');
    const file = await open(canonical, 'r');
    try {
      const stat = await file.stat();
      if (!stat.isFile() || stat.size > limit) fail('SIZE');
      const buffer = Buffer.alloc(limit + 1);
      const { bytesRead } = await file.read(buffer, 0, buffer.length, 0);
      if (bytesRead > limit) fail('SIZE');
      return new TextDecoder('utf-8', { fatal: true }).decode(buffer.subarray(0, bytesRead));
    } finally { await file.close(); }
  }
  async function resolveChild(binding, signal) {
    if (!projectsRoot || !binding.nativeRegistered || binding.role !== 'workflow-subagent'
      || !id(binding.id) || !id(binding.sessionId)) fail('IDENTITY');
    const candidates = [...runs.values()].filter(run => run.sessionId === binding.sessionId
      && run.transcriptPath === binding.transcriptPath);
    let match;
    for (const run of candidates) {
      signal?.throwIfAborted();
      const script = await read(run.script, 524288);
      if (workflowDigest(script) !== run.scriptDigest) fail('IDENTITY');
      const raw = await read(join(run.directory, 'journal.jsonl'), 131072);
      const journal = raw.trimEnd().split('\n').map(line => JSON.parse(line));
      if (journal[0]?.type !== 'launched') fail('IDENTITY');
      const entries = journal.filter(row => row.agentId === binding.id);
      if (!entries.length) continue;
      if (match || entries.length !== 1 || entries[0].type !== 'started'
        || !/^v2:[a-f0-9]{64}$/.test(entries[0].key) || typeof entries[0].label !== 'string') fail('IDENTITY');
      const path = join(run.directory, `agent-${binding.id}.meta.json`);
      const metadataRaw = await read(path, 16384), metadata = JSON.parse(metadataRaw);
      if (metadata.agentType !== 'workflow-subagent' || metadata.toolUseId !== undefined
        || metadata.parentAgentId != null || metadata.spawnDepth !== 1
        || metadata.model !== undefined || metadata.effort !== undefined || metadata.stoppedByUser === true
        || metadata.description !== entries[0].label || run.parent !== undefined) fail('IDENTITY');
      // Require a fresh native child transcript with matching session and identity.
      const transcript = await read(join(run.directory, `agent-${binding.id}.jsonl`), 1048576);
      const first = JSON.parse(transcript.split('\n')[0]);
      if (first.type !== 'user' || first.agentId !== binding.id || first.sessionId !== binding.sessionId
        || !Number.isFinite(Date.parse(first.timestamp)) || Date.parse(first.timestamp) < run.created
        || Date.parse(first.timestamp) > Date.now()) fail('IDENTITY');
      if (await read(path, 16384) !== metadataRaw) fail('IDENTITY');
      const latest = (await read(join(run.directory, 'journal.jsonl'), 131072)).trimEnd().split('\n').map(line => JSON.parse(line));
      if (latest[0]?.type !== 'launched'
        || JSON.stringify(latest.filter(row => row.agentId === binding.id)) !== JSON.stringify(entries)) fail('IDENTITY');
      if (workflowDigest(await read(run.script, 524288)) !== run.scriptDigest) fail('IDENTITY');
      match = { run, metadata };
    }
    if (!match) fail('MISSING');
    signal?.throwIfAborted();
    const childKey = key(binding.sessionId, binding.id), previous = children.get(childKey);
    if (previous && previous.runId !== match.run.runId) fail('IDENTITY');
    if (!previous && children.size >= 1024) fail('SIZE');
    const parent = match.run.route;
    if (typeof binding.requestedModel !== 'string') fail('MODEL');
    // Workflow native requests may carry an explicit model/effort. Only the
    // verified Workflow path uses them; ordinary Agent role defaults are unchanged.
    let selected;
    try { selected = selectModel(binding.requestedModel, binding.requestedEffort
      ?? (selectModel(binding.requestedModel).model === parent.model ? parent.effort : undefined)); }
    catch { fail('MODEL'); }
    const route = Object.freeze(selected);
    if (previous && (previous.route.model !== route.model || previous.route.effort !== route.effort)) fail('MODEL');
    children.set(childKey, { runId: match.run.runId, route });
    return { route, source: 'workflow-result', sessionId: binding.sessionId, parent: undefined };
  }
  return { link, resolve: resolveChild };
}
