import { createHash } from 'node:crypto';
import { open, realpath } from 'node:fs/promises';
import { basename, dirname, isAbsolute, join, relative, resolve } from 'node:path';
import { selectModel } from './models.mjs';
import { setTimeout as delay } from 'node:timers/promises';
import { isDeepStrictEqual } from 'node:util';

const fail = reason => { throw Object.assign(new Error('AGENT_SELECTION_UNVERIFIED'), { selectionReason: reason }); };
const within = (root, path) => { const rel = relative(root, path); return rel !== '' && !isAbsolute(rel) && rel !== '..' && !rel.startsWith('../') && !rel.startsWith('..\\'); };
export const workflowDigest = text => createHash('sha256').update(text).digest('hex');
const id = value => typeof value === 'string' && /^[A-Za-z0-9_-]{1,200}$/.test(value);

// A live native registration plus an authenticated Workflow result binds the
// exact run directory. Never discover children by scanning arbitrary folders.
export function createWorkflowSelection(projectsRoot) {
  const runs = new Map(), children = new Map(), linking = new Set();
  const key = (session, value) => `${session}:${value}`;
  function location(link, call) {
    if (!call?.workflow || call.parent !== link.parent || call.session !== link.sessionId
      || !/^wf_[a-z0-9-]{6,}$/.test(link.runId)
      || !/^[A-Za-z0-9_-]{1,100}$/.test(link.workflowName) || !id(link.taskId)
      || !id(link.sessionId) || typeof link.transcriptPath !== 'string'
      || basename(link.transcriptPath) !== `${link.sessionId}.jsonl`) fail('CALL');
    const root = join(dirname(resolve(link.transcriptPath)), link.sessionId);
    const directory = join(root, 'subagents', 'workflows', link.runId);
    const script = join(root, 'workflows', 'scripts', `${link.workflowName}-${link.runId}.js`);
    if (resolve(link.transcriptDir) !== directory || resolve(link.scriptPath) !== script) fail('PATH');
    const runKey = key(link.sessionId, link.runId);
    return { directory, script, runKey };
  }
  function link(link, call) {
    const { directory, script, runKey } = location(link, call);
    if (call.workflow.digest !== link.scriptDigest || link.resumeFromRunId !== undefined
      || runs.has(runKey) || linking.has(runKey) || runs.size >= 1024) fail('CALL');
    runs.set(runKey, { ...link, directory, script, route: call.workflow.route, created: call.created });
  }
  async function openEvidence(path) {
    const root = await realpath(projectsRoot), canonical = await realpath(path);
    if (!within(root, canonical) || canonical.toLowerCase() !== path.toLowerCase()) fail('PATH');
    return open(canonical, 'r');
  }
  async function read(path, limit, firstLine = false, snapshot = false) {
    const file = await openEvidence(path);
    try {
      const stat = await file.stat();
      if (!stat.isFile() || (!firstLine && stat.size > limit)) fail('SIZE');
      const buffer = Buffer.alloc(limit + 1);
      const { bytesRead } = await file.read(buffer, 0, buffer.length, 0);
      const newline = firstLine ? buffer.subarray(0, bytesRead).indexOf(10) : -1;
      const end = newline < 0 ? bytesRead : newline;
      if (end > limit || (firstLine && newline < 0 && stat.size > bytesRead)) fail('SIZE');
      const text = new TextDecoder('utf-8', { fatal: true }).decode(buffer.subarray(0, end));
      return snapshot ? { text, bytes: bytesRead, identity: `${stat.dev}:${stat.ino}:${stat.birthtimeMs}` } : text;
    } finally { await file.close(); }
  }
  async function readJournal(path, agentId, signal, deadline, previous, inspectResume = false) {
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
      const states = inspectResume ? new Map() : null;
      function row(bytes) {
        check();
        if (bytes.length > 131072 || ++records > 65536) fail('SIZE');
        const value = JSON.parse(decoder.decode(bytes));
        if (!value || typeof value !== 'object' || Array.isArray(value)) fail('PARSE');
        if (records === 1 && value.type !== 'launched') fail('IDENTITY');
        if (inspectResume && records > 1) {
          if (!id(value.agentId) || typeof value.key !== 'string' || !/^v2:[a-f0-9]{64}$/.test(value.key)) fail('IDENTITY');
          const state = states.get(value.agentId);
          if (value.type === 'started') {
            if (state || typeof value.label !== 'string') fail('IDENTITY');
            states.set(value.agentId, { key: value.key, label: value.label, terminal: null });
          } else if (['result', 'failed'].includes(value.type)) {
            if (!state || state.terminal || state.key !== value.key || value.type === 'result' && !Object.hasOwn(value, 'result')) fail('IDENTITY');
            state.terminal = value.type;
            if (value.type === 'result') state.result = value.result;
          } else fail('IDENTITY');
        }
        if (agentId === undefined) return;
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
      return { entry, identity, bytes: stat.size, digest: digest.digest('hex'), ...(states && { states }) };
    } finally { await file.close(); }
  }
  async function checkOrigin(path, prior) {
    const next = await read(path, 16777216, false, true);
    if (next.identity !== prior.identity || next.bytes < prior.bytes
      || workflowDigest(Buffer.from(next.text).subarray(0, prior.bytes)) !== prior.digest) fail('IDENTITY');
  }
  async function checkSavedResult(directory, agentId, state, sessionId, before, signal, deadline) {
    const check = () => { signal?.throwIfAborted(); if (performance.now() > deadline) fail('SIZE'); };
    check();
    const metaPath = join(directory, `agent-${agentId}.meta.json`), path = join(directory, `agent-${agentId}.jsonl`);
    const metadataRaw = await read(metaPath, 16384), metadata = JSON.parse(metadataRaw);
    if (metadata.agentType !== 'workflow-subagent' || metadata.parentAgentId != null || metadata.toolUseId !== undefined
      || metadata.spawnDepth !== 1 || metadata.stoppedByUser === true || metadata.description !== state.label) fail('IDENTITY');
    const raw = await read(path, 1048576), records = raw.trimEnd().split('\n').map(JSON.parse);
    if (records.length > 65536) fail('SIZE');
    const first = records[0];
    if (first?.type !== 'user' || first.sessionId !== sessionId || first.agentId !== agentId
      || !Number.isFinite(Date.parse(first.timestamp)) || Date.parse(first.timestamp) > before) fail('IDENTITY');
    const outputs = new Map(); let terminal, structured;
    for (const row of records) {
      check();
      if (!['user', 'assistant'].includes(row.type)) continue;
      const at = Date.parse(row.timestamp);
      if (row.sessionId !== sessionId || row.agentId !== agentId || !Number.isFinite(at)
        || at < Date.parse(first.timestamp) || at > before
        || !Array.isArray(row.message?.content) && !(row.type === 'user' && typeof row.message?.content === 'string')) fail('IDENTITY');
      if (row.type === 'assistant') {
        terminal = row;
        for (const block of row.message.content) if (block.type === 'tool_use' && block.name === 'StructuredOutput') {
          if (!id(block.id) || outputs.has(block.id)) fail('IDENTITY');
          outputs.set(block.id, { input: block.input, at });
        }
      } else for (const block of Array.isArray(row.message.content) ? row.message.content : []) {
        const output = outputs.get(block.tool_use_id);
        if (block.type === 'tool_result' && output && block.is_error !== true && at >= output.at) structured = { id: block.tool_use_id, input: output.input };
      }
    }
    if (!terminal || terminal.isApiErrorMessage === true) fail('IDENTITY');
    if (terminal.message.stop_reason === 'tool_use') {
      const finalOutputs = terminal.message.content.filter(block => block.type === 'tool_use');
      if (finalOutputs.length !== 1 || finalOutputs[0].name !== 'StructuredOutput' || finalOutputs[0].id !== structured?.id
        || !isDeepStrictEqual(structured.input, state.result)) fail('IDENTITY');
    } else if (terminal.message.stop_reason === 'end_turn' && typeof state.result === 'string') {
      if (terminal.message.content.some(block => block.type !== 'text' && block.type !== 'redacted_thinking' || block.type === 'text' && typeof block.text !== 'string')
        || terminal.message.content.filter(block => block.type === 'text').map(block => block.text).join('') !== state.result) fail('IDENTITY');
    } else fail('IDENTITY');
    if (await read(metaPath, 16384) !== metadataRaw || await read(path, 1048576) !== raw) fail('IDENTITY');
    check();
  }
  async function resume(link, call, signal) {
    const { directory, script, runKey } = location(link, call);
    if (call.parent !== undefined || !id(link.toolUseId) || link.scriptDigest !== undefined
      || typeof link.resumedScriptDigest !== 'string' || !/^[a-f0-9]{64}$/.test(link.resumedScriptDigest)
      || call.workflow.resumeFromRunId !== link.runId || link.resumeFromRunId !== link.runId
      || call.workflow.scriptPath !== link.scriptPath || !Number.isFinite(call.created)
      || linking.has(runKey) || !runs.has(runKey) && runs.size >= 1024) fail('CALL');
    const previous = runs.get(runKey);
    if (previous && (call.created < previous.created || previous.toolUseId === link.toolUseId)) fail('CALL');
    linking.add(runKey);
    const deadline = performance.now() + 1000;
    const check = () => { signal?.throwIfAborted(); if (performance.now() > deadline) fail('SIZE'); };
    try {
      check();
      const history = await read(link.transcriptPath, 16777216, false, true);
      const rows = history.text.trimEnd().split('\n');
      if (rows.length > 65536) fail('SIZE');
      const calls = new Map(); let origin;
      for (const raw of rows) {
        check(); if (Buffer.byteLength(raw) > 2097152) fail('SIZE');
        const row = JSON.parse(raw);
        if (!row || typeof row !== 'object' || Array.isArray(row)) fail('PARSE');
        if (row.sessionId !== link.sessionId || !Array.isArray(row.message?.content)) continue;
        const timestamp = Date.parse(row.timestamp);
        if (!Number.isFinite(timestamp) || timestamp > call.created) continue;
        if (row.type === 'assistant') for (const block of row.message.content) {
          const input = block.input;
          if (block.type !== 'tool_use' || block.name !== 'Workflow' || typeof input?.script !== 'string'
            || input.scriptPath !== undefined || input.resumeFromRunId !== undefined || input.name !== undefined) continue;
          if (!id(block.id) || calls.has(block.id) || Buffer.byteLength(input.script) > 524288 || calls.size >= 1024) fail('CALL');
          calls.set(block.id, workflowDigest(input.script));
        }
        const result = row.toolUseResult;
        if (row.type !== 'user' || result?.runId !== link.runId) continue;
        const matched = row.message.content.filter(block => block.type === 'tool_result' && calls.has(block.tool_use_id));
        if (!matched.length) continue;
        if (origin || matched.length !== 1 || matched[0].is_error === true || result.status !== 'async_launched'
          || result.taskType !== 'local_workflow' || result.workflowName !== link.workflowName
          || result.scriptPath !== link.scriptPath || result.transcriptDir !== link.transcriptDir || !id(result.taskId)) fail('IDENTITY');
        origin = calls.get(matched[0].tool_use_id);
      }
      if (!origin) fail('MISSING');
      if (link.resumedScriptDigest !== origin || workflowDigest(await read(script, 524288)) !== origin) fail('IDENTITY');
      const journalPath = join(directory, 'journal.jsonl');
      const journal = await readJournal(journalPath, undefined, signal, deadline, undefined, true);
      if (![...journal.states.values()].some(state => state.terminal === 'result')) fail('MISSING');
      for (const [agentId, state] of journal.states) if (state.terminal === 'result')
        await checkSavedResult(directory, agentId, state, link.sessionId, call.created, signal, deadline);
      for (const [agentId, state] of journal.states) if (!state.terminal) {
        let text;
        for (;;) {
          check();
          try { text = await read(join(directory, `agent-${agentId}.jsonl`), 1048576, true); break; }
          catch (error) { if (error.code !== 'ENOENT') throw error; await delay(10, undefined, { signal }); }
        }
        const first = JSON.parse(text);
        if (first.type !== 'user' || first.sessionId !== link.sessionId || first.agentId !== agentId
          || !Number.isFinite(Date.parse(first.timestamp)) || Date.parse(first.timestamp) < call.created
          || Date.parse(first.timestamp) > Date.now()) fail('IDENTITY');
      }
      const originProof = { identity: history.identity, bytes: history.bytes, digest: workflowDigest(history.text) };
      await checkOrigin(link.transcriptPath, originProof);
      await readJournal(journalPath, undefined, signal, deadline, journal, true);
      if (workflowDigest(await read(script, 524288)) !== origin) fail('IDENTITY');
      check();
      if (runs.get(runKey) !== previous) fail('CALL');
      const resumeJournal = { identity: journal.identity, bytes: journal.bytes, digest: journal.digest };
      runs.set(runKey, { ...link, scriptDigest: origin, directory, script, route: call.workflow.route,
        created: call.created, originProof, resumeJournal });
    } finally { linking.delete(runKey); }
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
      const journal = await readJournal(journalPath, binding.id, signal, deadline, run.resumeJournal);
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
      if (run.originProof) await checkOrigin(run.transcriptPath, run.originProof);
      if (runs.get(key(run.sessionId, run.runId)) !== run) fail('IDENTITY');
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
  return { link, resume, resolve: resolveChild };
}
