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
  async function openEvidence(path) {
    const root = await realpath(projectsRoot), canonical = await realpath(path);
    if (!within(root, canonical) || canonical.toLowerCase() !== path.toLowerCase()) fail('PATH');
    return open(canonical, 'r');
  }
  async function read(path, limit, firstLine = false) {
    const file = await openEvidence(path);
    try {
      const stat = await file.stat();
      if (!stat.isFile() || (!firstLine && stat.size > limit)) fail('SIZE');
      const buffer = Buffer.alloc(limit + 1);
      const { bytesRead } = await file.read(buffer, 0, buffer.length, 0);
      const newline = firstLine ? buffer.subarray(0, bytesRead).indexOf(10) : -1;
      const end = newline < 0 ? bytesRead : newline;
      if (end > limit || (firstLine && newline < 0 && stat.size > bytesRead)) fail('SIZE');
      return new TextDecoder('utf-8', { fatal: true }).decode(buffer.subarray(0, end));
    } finally { await file.close(); }
  }
  async function readJournal(path, agentId, signal, deadline, previous) {
    // Bound retained memory per record, total scan work and time independently.
    // A new child may occur anywhere in a long journal; a tail alone cannot prove
    // that its origin is unique or that an earlier stop/failure was not omitted.
    const check = () => { signal?.throwIfAborted(); if (performance.now() > deadline) fail('SIZE'); };
    check();
    const file = await openEvidence(path);
    try {
      const stat = await file.stat();
      check();
      if (!stat.isFile() || stat.size > 16777216) fail('SIZE');
      const identity = `${stat.dev}:${stat.ino}:${stat.birthtimeMs}`;
      if (previous && (identity !== previous.identity || stat.size < previous.bytes)) fail('IDENTITY');
      const digest = createHash('sha256'), prefix = previous ? createHash('sha256') : undefined;
      const decoder = new TextDecoder('utf-8', { fatal: true });
      const chunk = Buffer.alloc(65536);
      let pending = Buffer.alloc(0), position = 0, records = 0, entry;
      function row(bytes) {
        check();
        if (bytes.length > 131072 || ++records > 65536) fail('SIZE');
        const value = JSON.parse(decoder.decode(bytes));
        if (!value || typeof value !== 'object' || Array.isArray(value)) fail('PARSE');
        if (records === 1 && value.type !== 'launched') fail('IDENTITY');
        if (value.agentId !== agentId) return;
        if (entry || value.type !== 'started' || typeof value.key !== 'string' || !/^v2:[a-f0-9]{64}$/.test(value.key)
          || typeof value.label !== 'string') fail('IDENTITY');
        entry = { key: value.key, label: value.label };
      }
      while (position < stat.size) {
        check();
        const { bytesRead } = await file.read(chunk, 0, Math.min(chunk.length, stat.size - position), position);
        check();
        if (!bytesRead) fail('IDENTITY');
        const bytes = chunk.subarray(0, bytesRead);
        digest.update(bytes);
        if (previous && position < previous.bytes) prefix.update(bytes.subarray(0, Math.min(bytesRead, previous.bytes - position)));
        position += bytesRead;
        pending = Buffer.concat([pending, bytes]);
        let start = 0, newline;
        while ((newline = pending.indexOf(10, start)) !== -1) {
          row(pending.subarray(start, newline)); start = newline + 1;
        }
        pending = pending.subarray(start);
        if (pending.length > 131072) fail('SIZE');
      }
      if (pending.length) row(pending);
      if (!records) fail('IDENTITY');
      const after = await file.stat();
      check();
      if (`${after.dev}:${after.ino}:${after.birthtimeMs}` !== identity || after.size < stat.size) fail('IDENTITY');
      if (previous && prefix.digest('hex') !== previous.digest) fail('IDENTITY');
      return { entry, identity, bytes: stat.size, digest: digest.digest('hex') };
    } finally { await file.close(); }
  }
  async function resolveChild(binding, signal) {
    if (!projectsRoot || !binding.nativeRegistered || binding.role !== 'workflow-subagent'
      || !id(binding.id) || !id(binding.sessionId)) fail('IDENTITY');
    const deadline = performance.now() + 1000;
    const candidates = [...runs.values()].filter(run => run.sessionId === binding.sessionId
      && run.transcriptPath === binding.transcriptPath);
    let match;
    for (const run of candidates) {
      signal?.throwIfAborted();
      const journalPath = join(run.directory, 'journal.jsonl');
      const journal = await readJournal(journalPath, binding.id, signal, deadline);
      if (!journal.entry) continue;
      if (match) fail('IDENTITY');
      // A completed run's script can be edited before an independent run starts.
      // Check every journal for duplicate origins, then validate the script of
      // the run that actually owns this child. Its digest must still match.
      const script = await read(run.script, 524288);
      if (workflowDigest(script) !== run.scriptDigest) fail('IDENTITY');
      const path = join(run.directory, `agent-${binding.id}.meta.json`);
      const metadataRaw = await read(path, 16384), metadata = JSON.parse(metadataRaw);
      if (metadata.agentType !== 'workflow-subagent' || metadata.toolUseId !== undefined
        || metadata.parentAgentId != null || metadata.spawnDepth !== 1
        || metadata.effort !== undefined || metadata.stoppedByUser === true
        || metadata.description !== journal.entry.label || run.parent !== undefined) fail('IDENTITY');
      // Require a fresh native child transcript with matching session and identity.
      const transcriptPath = join(run.directory, `agent-${binding.id}.jsonl`);
      const transcript = await read(transcriptPath, 1048576, true);
      const first = JSON.parse(transcript);
      if (first.type !== 'user' || first.agentId !== binding.id || first.sessionId !== binding.sessionId
        || !Number.isFinite(Date.parse(first.timestamp)) || Date.parse(first.timestamp) < run.created
        || Date.parse(first.timestamp) > Date.now()) fail('IDENTITY');
      if (await read(path, 16384) !== metadataRaw) fail('IDENTITY');
      await readJournal(journalPath, binding.id, signal, deadline, journal);
      if (await read(transcriptPath, 1048576, true) !== transcript) fail('IDENTITY');
      if (workflowDigest(await read(run.script, 524288)) !== run.scriptDigest) fail('IDENTITY');
      match = { run, metadata };
    }
    if (!match) fail('MISSING');
    signal?.throwIfAborted();
    if (performance.now() > deadline) fail('SIZE');
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
    // Native persists an explicit Workflow model in the sidecar, but not effort.
    // It is corroborating evidence, not permission to override the request.
    if (match.metadata.model !== undefined) {
      try {
        if (typeof match.metadata.model !== 'string'
          || selectModel(match.metadata.model).model !== selected.model) fail('MODEL');
      } catch { fail('MODEL'); }
    }
    const route = Object.freeze(selected);
    if (previous && (previous.route.model !== route.model || previous.route.effort !== route.effort)) fail('MODEL');
    children.set(childKey, { runId: match.run.runId, route });
    return { route, source: 'workflow-result', sessionId: binding.sessionId, parent: undefined };
  }
  return { link, resolve: resolveChild };
}
