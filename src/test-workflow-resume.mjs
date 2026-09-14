import assert from 'node:assert/strict';
import { mkdtemp, mkdir, writeFile, readFile, rm } from 'node:fs/promises';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { createAgentSelection } from './agent-selection.mjs';
import { bindingFrom } from './agent-route.mjs';

await mkdir(new URL('../.tmp/', import.meta.url), { recursive: true });
const root = await mkdtemp(fileURLToPath(new URL('../.tmp/workflow-resume-', import.meta.url)));
const sessionId = 'public-saved-session', runId = 'wf_public-resume', workflowName = 'public-workflow';
const transcriptPath = join(root, `${sessionId}.jsonl`), directory = join(root, sessionId, 'subagents/workflows', runId);
const scriptPath = join(root, sessionId, 'workflows/scripts', `${workflowName}-${runId}.js`);
const script = "export const meta = { name: 'public-workflow' }; return { total: 12 };";
const route = { model: 'gpt-5.6-sol', effort: 'low' }, oldTime = Date.now() - 10000;
const oldResult = { status: 'async_launched', taskType: 'local_workflow', taskId: 'old-task', runId, workflowName,
  transcriptDir: directory, scriptPath };
const historyRows = [
  { type: 'assistant', sessionId, timestamp: new Date(oldTime).toISOString(), message: { role: 'assistant', content: [
    { type: 'tool_use', id: 'original-call', name: 'Workflow', input: { script } }] } },
  { type: 'user', sessionId, timestamp: new Date(oldTime + 1).toISOString(), message: { role: 'user', content: [
    { type: 'tool_result', tool_use_id: 'original-call', content: 'PUBLIC_ORIGINAL_LAUNCH' }] }, toolUseResult: oldResult }
];
const encode = rows => rows.map(JSON.stringify).join('\n') + '\n';
const key = letter => `v2:${letter.repeat(64)}`;
const start = (agentId, value) => ({ type: 'started', key: key(value), agentId, label: agentId });
const priorRows = [{ type: 'launched' }, start('cached', 'a'), { type: 'result', key: key('a'), agentId: 'cached', result: { sum: 5 } },
  start('failed', 'b'), { type: 'failed', key: key('b'), agentId: 'failed' }];
const input = { scriptPath, resumeFromRunId: runId };
const event = { hook_event_name: 'PostToolUse', tool_name: 'Workflow', session_id: sessionId, transcript_path: transcriptPath,
  tool_use_id: 'resume-call', tool_input: { ...input, script }, tool_response: { ...oldResult, taskId: 'new-task' } };
const link = bindingFrom(event);
const binding = (id = 'fresh') => ({ id, sessionId, transcriptPath, role: 'workflow-subagent', nativeRegistered: true,
  requestedModel: 'sol', requestedEffort: 'low' });
async function child(id, timestamp = Date.now(), changes = {}) {
  await writeFile(join(directory, `agent-${id}.meta.json`), JSON.stringify({ agentType: 'workflow-subagent', description: id, spawnDepth: 1, model: 'sol', ...changes }));
  await writeFile(join(directory, `agent-${id}.jsonl`), JSON.stringify({ type: 'user', sessionId, agentId: id,
    timestamp: new Date(timestamp).toISOString(), message: { role: 'user', content: 'PUBLIC_CHILD_TASK' } }) + '\n'
    + (id === 'cached' ? encode([
      { type: 'assistant', sessionId, agentId: id, timestamp: new Date(timestamp + 1).toISOString(), message: { role: 'assistant', stop_reason: 'tool_use',
        content: [{ type: 'tool_use', id: 'public-structured', name: 'StructuredOutput', input: { sum: 5 } }] } },
      { type: 'user', sessionId, agentId: id, timestamp: new Date(timestamp + 2).toISOString(), message: { role: 'user',
        content: [{ type: 'tool_result', tool_use_id: 'public-structured', content: 'PUBLIC_OUTPUT_ACCEPTED' }] } }
    ]) : ''));
}
async function setup({ journal = [...priorRows, start('fresh', 'b')], history = historyRows, freshTime, metadata } = {}) {
  await writeFile(transcriptPath, encode(history)); await writeFile(scriptPath, script);
  const selection = createAgentSelection({ projectsRoot: root, timeoutMs: 20 });
  selection.remember({ content: [{ type: 'tool_use', id: 'resume-call', name: 'Workflow', input }] }, sessionId, undefined, route);
  await child('cached', oldTime); await child('failed', oldTime); await child('fresh', freshTime ?? Date.now(), metadata);
  await writeFile(join(directory, 'journal.jsonl'), encode(journal));
  return selection;
}
let checks = 0;
try {
  await mkdir(directory, { recursive: true }); await mkdir(join(root, sessionId, 'workflows/scripts'), { recursive: true });
  assert.equal(link.resumeFromRunId, runId); assert.equal(link.scriptDigest, undefined); assert.match(link.resumedScriptDigest, /^[a-f0-9]{64}$/); checks++;
  for (const tool_input of [{ ...input, script, scriptPath: scriptPath + '.other' }, { ...input, script, resumeFromRunId: 'wf_other-run' }]) {
    assert.throws(() => bindingFrom({ ...event, tool_input })); checks++;
  }
  assert.equal(bindingFrom({ ...event, tool_input: input }), null); checks++;
  {
    const selection = createAgentSelection({ projectsRoot: root });
    selection.remember({ content: [{ type: 'tool_use', id: 'resume-call', name: 'Workflow', input: { ...input, script } }] }, sessionId, undefined, route);
    await assert.rejects(selection.linkWorkflow(link)); checks++;
  }
  {
    const selection = await setup(); await selection.linkWorkflow(link);
    const result = await selection.resolve(binding()); assert.deepEqual(result.route, route); assert.equal(result.source, 'workflow-result'); checks++;
    await assert.rejects(async () => selection.linkWorkflow(link)); checks++;
    for (const value of [binding('cached'), binding('failed'), { ...binding(), sessionId: 'other-session' },
      { ...binding(), nativeRegistered: false }, { ...binding(), requestedModel: 'luna' }]) {
      await assert.rejects(selection.resolve(value)); checks++;
    }
  }
  for (const mutation of [
    { toolUseId: 'not-current' }, { parent: 'foreign-parent' }, { runId: 'wf_other-run' },
    { resumeFromRunId: 'wf_other-run' }, { scriptPath: transcriptPath }, { transcriptPath: scriptPath },
    { transcriptDir: root }, { scriptDigest: '0'.repeat(64) }, { resumedScriptDigest: '0'.repeat(64) }, { resumedScriptDigest: undefined }
  ]) {
    const selection = await setup(); await assert.rejects(async () => selection.linkWorkflow({ ...link, ...mutation })); checks++;
  }
  for (const journal of [
    [{ type: 'launched' }, start('failed', 'b'), { type: 'failed', key: key('b'), agentId: 'failed' }, start('fresh', 'b')],
    [...priorRows.slice(0, 4), start('fresh', 'b')],
    [...priorRows, start('failed', 'b')],
    [...priorRows, { type: 'result', key: key('a'), agentId: 'cached', result: { sum: 5 } }, start('fresh', 'b')],
    [...priorRows, { ...start('fresh', 'b'), key: 'invalid' }]
  ]) {
    const selection = await setup({ journal }); await assert.rejects(selection.linkWorkflow(link)); checks++;
  }
  for (const history of [[], historyRows.map(row => ({ ...row, sessionId: 'other' })), [...historyRows, historyRows[1]],
    [historyRows[0], { ...historyRows[1], toolUseResult: { ...oldResult, scriptPath: transcriptPath } }]]) {
    const selection = await setup({ history }); await assert.rejects(selection.linkWorkflow(link)); checks++;
  }
  {
    const selection = await setup({ freshTime: oldTime }); await assert.rejects(selection.linkWorkflow(link)); checks++;
  }
  {
    const selection = await setup({ metadata: { stoppedByUser: true } }); await selection.linkWorkflow(link);
    await assert.rejects(selection.resolve(binding())); checks++;
  }
  {
    const selection = await setup(); await writeFile(scriptPath, script + '// changed');
    await assert.rejects(selection.linkWorkflow(link)); checks++;
  }
  for (const path of [join(directory, 'journal.jsonl'), join(directory, 'agent-cached.jsonl')]) {
    const selection = await setup(); await writeFile(path, (await readFile(path, 'utf8')).replace('"sum":5', '"sum":6'));
    await assert.rejects(selection.linkWorkflow(link)); checks++;
  }
  for (const mutation of [rows => { rows[2].message.content[0].is_error = true; },
    rows => { rows[1].isApiErrorMessage = true; }, rows => { rows[0].sessionId = 'foreign'; }]) {
    const selection = await setup(), path = join(directory, 'agent-cached.jsonl');
    const rows = (await readFile(path, 'utf8')).trimEnd().split('\n').map(JSON.parse); mutation(rows);
    await writeFile(path, encode(rows)); await assert.rejects(selection.linkWorkflow(link)); checks++;
  }
  for (const malformed of [false, true]) {
    const selection = await setup(), text = malformed ? '[object Object]' : 'PUBLIC_TEXT_RESULT';
    const journal = [...priorRows, start('fresh', 'b')].map(row => row.type === 'result' ? { ...row, result: text } : row);
    await writeFile(join(directory, 'journal.jsonl'), encode(journal));
    await writeFile(join(directory, 'agent-cached.jsonl'), encode([
      { type: 'user', sessionId, agentId: 'cached', timestamp: new Date(oldTime).toISOString(), message: { role: 'user', content: 'PUBLIC_TEXT_TASK' } },
      { type: 'assistant', sessionId, agentId: 'cached', timestamp: new Date(oldTime + 1).toISOString(), message: { role: 'assistant', stop_reason: 'end_turn',
        content: [{ type: 'text', text: malformed ? {} : text }] } }
    ]));
    if (malformed) await assert.rejects(selection.linkWorkflow(link));
    else { await selection.linkWorkflow(link); assert.deepEqual((await selection.resolve(binding())).route, route); }
    checks++;
  }
  {
    const selection = await setup(); await selection.linkWorkflow(link);
    await writeFile(transcriptPath, encode([...historyRows, { type: 'system', event: 'PUBLIC_APPEND' }]));
    assert.deepEqual((await selection.resolve(binding())).route, route); checks++;
    await writeFile(transcriptPath, encode(historyRows).replace('PUBLIC_ORIGINAL_LAUNCH', 'PUBLIC_REPLACED_LAUNCH'));
    await assert.rejects(selection.resolve(binding())); checks++;
  }
  {
    const selection = await setup(); await selection.linkWorkflow(link);
    const journalPath = join(directory, 'journal.jsonl');
    await writeFile(journalPath, (await readFile(journalPath, 'utf8')).replace('"sum":5', '"sum":6'));
    await assert.rejects(selection.resolve(binding())); checks++;
  }
  {
    const selection = await setup(); const cancel = new AbortController(), reason = new Error('PUBLIC_CANCEL'); cancel.abort(reason);
    await assert.rejects(selection.linkWorkflow(link, cancel.signal), error => error === reason);
    await assert.rejects(selection.resolve(binding())); checks++;
  }
  {
    const selection = await setup(); const results = await Promise.allSettled([selection.linkWorkflow(link), selection.linkWorkflow(link)]);
    assert.equal(results.filter(row => row.status === 'fulfilled').length, 1);
    assert.equal(results.filter(row => row.status === 'rejected').length, 1);
    assert.deepEqual((await selection.resolve(binding())).route, route); checks++;
  }
  console.log(JSON.stringify({ suite: 'workflow-resume', checks, passed: true, actualNativeExecutions: 0, actualBackendRequests: 0,
    savedResultAndNewChildVerified: true, priorActiveChildRejected: true, originalRecordTamperingRejected: true }));
} finally { await rm(root, { recursive: true, force: true }); }
