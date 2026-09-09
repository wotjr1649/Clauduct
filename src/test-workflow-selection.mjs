import assert from 'node:assert/strict';
import { mkdtemp, mkdir, writeFile, rm } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import { join } from 'node:path';
import { createAgentSelection } from './agent-selection.mjs';
import { bindingFrom, registerBinding } from './agent-route.mjs';
import { startNativeGateway } from './native-gateway.mjs';
import { readRequestStatus } from './request-status.mjs';

const root = await mkdtemp(fileURLToPath(new URL('./workflow-fixture-', import.meta.url)));
const sessionId = 'synthetic-session', runId = 'wf_synthetic-01', workflowName = 'synthetic-workflow';
const transcriptPath = join(root, `${sessionId}.jsonl`);
const directory = join(root, sessionId, 'subagents', 'workflows', runId);
const scriptPath = join(root, sessionId, 'workflows', 'scripts', `${workflowName}-${runId}.js`);
const script = "export const meta = { name: 'synthetic-workflow', description: 'SYNTHETIC_PRIVATE' }; return null;";
const parentRoute = { model: 'gpt-5.6-sol', effort: 'medium' };
const invocation = { content: [{ type: 'tool_use', id: 'workflow-call', name: 'Workflow', input: { script } }] };
const event = { hook_event_name: 'PostToolUse', tool_name: 'Workflow', session_id: sessionId,
  transcript_path: transcriptPath, tool_use_id: 'workflow-call', tool_input: { script },
  tool_response: { status: 'async_launched', taskType: 'local_workflow', runId, workflowName,
    taskId: 'workflow-task', transcriptDir: directory, scriptPath } };
const link = bindingFrom(event);
const binding = id => ({ id, sessionId, transcriptPath, role: 'workflow-subagent', nativeRegistered: true,
  requestedModel: parentRoute.model });
const row = id => ({ type: 'started', key: `v2:${'a'.repeat(64)}`, agentId: id, label: id });
const journal = rows => writeFile(join(directory, 'journal.jsonl'), [{ type: 'launched' }, ...rows].map(JSON.stringify).join('\n') + '\n');
const metadata = id => ({ agentType: 'workflow-subagent', description: id, spawnDepth: 1,
  requestShape: 'foreground', requestNonInteractive: false });
async function child(id, extra = {}) {
  await writeFile(join(directory, `agent-${id}.meta.json`), JSON.stringify({ ...metadata(id), ...extra }));
  await writeFile(join(directory, `agent-${id}.jsonl`), JSON.stringify({ type: 'user', sessionId, agentId: id,
    timestamp: new Date().toISOString(), message: { role: 'user', content: 'SYNTHETIC_PRIVATE' } }) + '\n');
}
function selection() {
  const value = createAgentSelection({ projectsRoot: root, timeoutMs: 30 });
  value.remember(invocation, sessionId, undefined, parentRoute);
  return value;
}
let passed = 0;
try {
  await mkdir(directory, { recursive: true });
  await mkdir(join(root, sessionId, 'workflows', 'scripts'), { recursive: true });
  await writeFile(transcriptPath, ''); await writeFile(scriptPath, script);
  assert.ok(!JSON.stringify(link).includes('SYNTHETIC_PRIVATE'));
  assert.equal(bindingFrom({ ...event, tool_response: { ...event.tool_response, taskType: 'remote_agent' } }), null);
  assert.equal(bindingFrom({ ...event, tool_input: { script, resumeFromRunId: runId } }), null);
  assert.throws(() => bindingFrom({ ...event, tool_response: { ...event.tool_response, runId: '../escape' } })); passed++;

  const routes = selection();
  for (const change of [{ sessionId: 'other' }, { toolUseId: 'other' }, { parent: 'other' },
    { scriptDigest: '0'.repeat(64) }, { transcriptDir: root }, { scriptPath: transcriptPath }]) {
    assert.throws(() => routes.linkWorkflow({ ...link, ...change })); passed++;
  }
  routes.linkWorkflow(link);
  assert.throws(() => routes.linkWorkflow(link)); passed++;
  await child('one'); await child('two'); await journal([row('one'), row('two')]);
  const [one, two] = await Promise.all([routes.resolve(binding('one')), routes.resolve(binding('two'))]);
  assert.deepEqual(one.route, parentRoute); assert.deepEqual(two.route, parentRoute);
  assert.equal(one.source, 'workflow-result'); passed++;
  await assert.rejects(routes.resolve({ ...binding('one'), nativeRegistered: false }));
  await assert.rejects(routes.resolve({ ...binding('one'), requestedModel: 'unknown' }));
  await assert.rejects(routes.resolve({ ...binding('one'), requestedEffort: 'max' })); passed++;

  for (const [rows, changes] of [
    [[row('one'), row('one')], {}],
    [[row('one'), { type: 'failed', agentId: 'one' }], {}],
    [[{ ...row('one'), key: 'SYNTHETIC_PRIVATE' }], {}],
    [[row('one')], { stoppedByUser: true }],
    [[row('one')], { agentType: 'general-purpose' }],
    [[row('one')], { toolUseId: 'forged' }],
    [[row('one')], { parentAgentId: 'forged' }],
    [[row('one')], { description: 'wrong' }],
    [[row('one')], { model: 'gpt-5.6-luna' }],
    [[row('one')], { model: 'unknown' }],
    [[row('one')], { model: null }],
    [[row('one')], { model: { model: parentRoute.model } }],
    [[row('one')], { effort: 'high' }]
  ]) {
    const current = selection(); current.linkWorkflow(link);
    await child('one', changes); await journal(rows);
    await assert.rejects(current.resolve(binding('one'))); passed++;
  }
  {
    const current = selection(); current.linkWorkflow(link); await child('one'); await journal([row('one')]);
    await writeFile(scriptPath, script + ' ');
    await assert.rejects(current.resolve(binding('one')));
    await writeFile(scriptPath, script); passed++;
  }
  {
    const current = selection(); current.linkWorkflow(link); await child('one'); await journal([row('one')]);
    await writeFile(join(directory, 'agent-one.meta.json'), 'x'.repeat(16385));
    await assert.rejects(current.resolve(binding('one')), error => error.selectionReason === 'SIZE'); passed++;
  }
  // Real loopback hook registration -> selector -> gateway -> filtered status.
  {
    const current = createAgentSelection({ projectsRoot: root, timeoutMs: 1000 });
    current.remember(invocation, sessionId, undefined, parentRoute);
    await child('late'); await journal([row('late')]);
    const waiting = current.resolve({ ...binding('late'), requestedModel: 'sol' });
    current.linkWorkflow(link);
    assert.deepEqual((await waiting).route, parentRoute); passed++;
    await child('explicit'); await journal([row('late'), row('explicit')]);
    assert.deepEqual((await current.resolve({ ...binding('explicit'), requestedModel: 'luna', requestedEffort: 'high' })).route,
      { model: 'gpt-5.6-luna', effort: 'high' }); passed++;
  }
  {
    const current = selection(); current.linkWorkflow(link); await child('one'); await journal([row('one')]);
    await writeFile(join(directory, 'agent-one.jsonl'), JSON.stringify({ type: 'user', sessionId: 'wrong', agentId: 'one', timestamp: new Date().toISOString() }));
    await assert.rejects(current.resolve(binding('one')), error => error.selectionReason === 'IDENTITY'); passed++;
    await child('one');
    await writeFile(join(directory, 'journal.jsonl'), 'x'.repeat(131073));
    await assert.rejects(current.resolve(binding('one')), error => error.selectionReason === 'SIZE'); passed++;
  }
  for (const scenario of [
    { model: parentRoute.model, effort: undefined, expected: parentRoute },
    { model: 'gpt-5.6-luna', effort: 'high', expected: { model: 'gpt-5.6-luna', effort: 'high' } },
    { model: 'gpt-5.6-terra', effort: 'xhigh', expected: { model: 'gpt-5.6-terra', effort: 'xhigh' } }
  ]) {
    const current = selection();
    await child('one', scenario.effort ? { model: scenario.model } : {}); await journal([row('one')]);
    const seen = [];
    const gateway = await startNativeGateway({ agentSelection: current, admissionOptions: { freeBytes: () => 16 * 1024 ** 3 },
      transport: { send: async body => { seen.push(body); return [
        { type: 'response.created', response: { id: 'r', status: 'in_progress' } },
        { type: 'response.output_item.added', output_index: 0, item: { type: 'message', id: 'm', role: 'assistant', content: [] } },
        { type: 'response.output_text.delta', output_index: 0, item_id: 'm', content_index: 0, delta: 'OK' },
        { type: 'response.output_text.done', output_index: 0, item_id: 'm', content_index: 0, text: 'OK' },
        { type: 'response.output_item.done', output_index: 0, item: { type: 'message', id: 'm', role: 'assistant', content: [{ type: 'output_text', text: 'OK' }] } },
        { type: 'response.output_item.added', output_index: 1, item: { type: 'reasoning', id: 'rs_workflow', summary: [], encrypted_content: 'SYNTHETIC_OPAQUE' } },
        { type: 'response.output_item.done', output_index: 1, item: { type: 'reasoning', id: 'rs_workflow', summary: [], encrypted_content: 'SYNTHETIC_OPAQUE' } },
        { type: 'response.completed', response: { id: 'r', status: 'completed', model: body.model, output: [],
          usage: { input_tokens: 1, output_tokens: 1, total_tokens: 2 } } }
      ]; }, close: async () => {}, diagnostics: () => ({}) } });
    const env = { ANTHROPIC_BASE_URL: `http://127.0.0.1:${gateway.port}`, ANTHROPIC_AUTH_TOKEN: gateway.clientHeaders().Authorization.slice(7) };
    try {
      await registerBinding(link, env);
      await registerBinding({ id: 'one', role: 'workflow-subagent', stop: false, sessionId, transcriptPath }, env);
      const response = await fetch(`${env.ANTHROPIC_BASE_URL}/v1/messages`, { method: 'POST', signal: AbortSignal.timeout(5000),
        headers: { ...gateway.clientHeaders(), 'anthropic-version': '2023-06-01', 'content-type': 'application/json',
          'x-claude-code-session-id': sessionId, 'x-claude-code-agent-id': 'one' },
        body: JSON.stringify({ model: scenario.model, ...(scenario.effort && { output_config: { effort: scenario.effort } }),
          stream: true, max_tokens: 100, messages: [{ role: 'user', content: 'SYNTHETIC_PRIVATE' }] }) });
      const wire = await response.text();
      assert.equal(response.status, 200, wire);
      const frames = wire.split('\n\n').filter(frame => frame.startsWith('event:')).map(frame => JSON.parse(frame.split('\ndata: ')[1]));
      const blocks = [], yielded = [];
      for (const frame of frames) {
        if (frame.type === 'content_block_start') blocks[frame.index] = structuredClone(frame.content_block);
        if (frame.type === 'content_block_delta' && frame.delta.type === 'text_delta') blocks[frame.index].text += frame.delta.text;
        if (frame.type === 'content_block_stop') yielded.push(blocks[frame.index]);
      }
      assert.deepEqual(yielded.map(block => block.type), ['redacted_thinking', 'text']);
      assert.equal(yielded.at(-1).text, 'OK');
      assert.equal(frames.at(-2).delta.stop_reason, 'end_turn');
      assert.equal(frames.at(-1).type, 'message_stop');
      assert.equal(seen.length, 1);
      assert.equal(seen[0].model, scenario.expected.model); assert.equal(seen[0].reasoning.effort, scenario.expected.effort);
      const status = await readRequestStatus(env), record = status.recentRequests.at(-1);
      assert.equal(record.selectionSource, 'workflow-result'); assert.equal(record.success, true);
      assert.equal(record.role, 'workflow-subagent');
      assert.equal(record.model, scenario.expected.model); assert.equal(record.effort, scenario.expected.effort);
      assert.equal(record.roleRegistered, true); assert.ok(!JSON.stringify(status).includes('SYNTHETIC_PRIVATE')); passed++;
    } finally { await gateway.close(); }
  }
  console.log(JSON.stringify({ suite: 'workflow-selection', passed, actualClaude: 0, externalRequests: 0,
    notRun: ['native Workflow execution', 'symlink permission-dependent checks'] }));
} finally { await rm(root, { recursive: true, force: true }); }
