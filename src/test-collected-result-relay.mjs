import assert from 'node:assert/strict';
import { mkdtemp, mkdir, writeFile, rm } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import { join } from 'node:path';
import { createAgentSelection } from './agent-selection.mjs';
import { bindingFrom } from './agent-route.mjs';

const root = await mkdtemp(fileURLToPath(new URL('../.tmp/collected-relay-', import.meta.url)));
const directory = join(root, 'session', 'subagents');
const route = { model: 'gpt-5.6-sol', effort: 'low' };
const binding = id => ({ id, role: 'claude', sessionId: 'session', transcriptPath: join(root, 'session.jsonl'), nativeRegistered: true });
const meta = id => ({ agentType: 'claude', toolUseId: `call_${id}`, model: 'inherit',
  ...(['child', 'sibling'].includes(id) && { parentAgentId: 'parent' }), ...(id === 'foreign' && { parentAgentId: 'other' }) });
const final = (id, status = 'completed') => ({ type: 'assistant', sessionId: 'session', agentId: id,
  timestamp: new Date().toISOString(), ...(status === 'failed' && { isApiErrorMessage: true }),
  message: { id: `response_${id}`, role: 'assistant', stop_reason: status === 'failed' ? 'stop_sequence' : 'end_turn', content: [] } });
const save = (id, value) => writeFile(join(directory, `agent-${id}.jsonl`), JSON.stringify(value) + '\n');
const tool = (id, name, input) => ({ content: [{ type: 'tool_use', id, name, input }] });
let serial = 0, checks = 0;
async function setup() {
  const selection = createAgentSelection({ projectsRoot: root, timeoutMs: 5 });
  for (const id of ['parent', 'other', 'child', 'sibling', 'foreign']) {
    selection.remember(tool(`call_${id}`, 'Agent', { subagent_type: 'claude', model: 'inherit' }), 'session', meta(id).parentAgentId, route);
    await writeFile(join(directory, `agent-${id}.meta.json`), JSON.stringify(meta(id)));
    await selection.resolve(binding(id));
    const request = selection.begin('session', id);
    if (id === 'child') selection.failed('session', id, request);
    else selection.delivered('session', id, request, final(id).message);
    await save(id, final(id, id === 'child' ? 'failed' : 'completed'));
  }
  return selection;
}
function collection(selection, id, status = id === 'child' ? 'failed' : 'completed', parent) {
  const toolUseId = `collect_${++serial}`;
  selection.remember(tool(toolUseId, 'TaskOutput', { task_id: id, block: true }), 'session', parent, route);
  return { kind: 'task-result', sessionId: 'session', toolUseId, id, status, ...(parent && { parent }) };
}
function relay(selection, target = 'parent') {
  const toolUseId = `send_${++serial}`;
  selection.remember(tool(toolUseId, 'SendMessage', { to: target, message: 'Continue the original task using the collected child results.' }), 'session', undefined, route);
  assert.equal(selection.linkResume({ sessionId: 'session', toolUseId, id: target }), true);
}
try {
  await mkdir(directory, { recursive: true });
  const hook = { hook_event_name: 'PostToolUse', tool_name: 'TaskOutput', session_id: 'session', tool_use_id: 'collect',
    tool_input: { task_id: 'child' }, tool_response: { retrieval_status: 'success', task: { task_id: 'child', task_type: 'local_agent', status: 'failed', output: 'PUBLIC_FAILURE' } } };
  assert.deepEqual(bindingFrom(hook), { kind: 'task-result', sessionId: 'session', toolUseId: 'collect', id: 'child', status: 'failed' }); checks++;
  assert.equal(bindingFrom({ ...hook, tool_response: { ...hook.tool_response, retrieval_status: 'timeout' } }), null); checks++;
  for (const status of ['running', 'killed', 'cancelled']) {
    assert.equal(bindingFrom({ ...hook, tool_response: { ...hook.tool_response, task: { ...hook.tool_response.task, status } } }), null); checks++;
  }
  assert.throws(() => bindingFrom({ ...hook, tool_input: { task_id: 'sibling' } })); checks++;
  {
    const s = await setup();
    assert.equal(await s.linkTaskResult(collection(s, 'child')), true);
    assert.equal(await s.linkTaskResult(collection(s, 'sibling')), true);
    // Repeated collection before delivery is idempotent.
    assert.equal(await s.linkTaskResult(collection(s, 'child')), false);
    relay(s);
    const selected = await s.resolve(binding('parent'));
    assert.equal(selected.source, 'verified-result-relay'); assert.deepEqual(selected.route, route); checks++;
    s.delivered('session', 'parent', s.begin('session', 'parent'), final('parent').message);
    await save('parent', final('parent'));
    assert.equal(await s.linkTaskResult(collection(s, 'child')), false);
    relay(s);
    await assert.rejects(s.resolve(binding('parent'))); checks++;
  }
  for (const fault of ['wrong-origin', 'wrong-collector', 'stale-request', 'wrong-status', 'wrong-session', 'wrong-final-id',
    'wrong-final-child', 'nonterminal', 'cancelled-child', 'changed-child-model']) {
    const s = await setup(), link = collection(s, 'sibling');
    if (fault === 'wrong-origin') link.toolUseId = 'unknown';
    if (fault === 'wrong-collector') link.parent = 'other';
    if (fault === 'stale-request') s.begin('session', 'sibling');
    if (fault === 'wrong-status') link.status = 'failed';
    if (fault === 'wrong-session') link.sessionId = 'other';
    if (fault === 'wrong-final-id') await save('sibling', { ...final('sibling'), message: { ...final('sibling').message, id: 'old' } });
    if (fault === 'wrong-final-child') await save('sibling', final('child'));
    if (fault === 'nonterminal') await save('sibling', { ...final('sibling'), message: { ...final('sibling').message, stop_reason: 'tool_use' } });
    if (fault === 'cancelled-child' || fault === 'changed-child-model') await writeFile(join(directory, 'agent-sibling.meta.json'),
      JSON.stringify({ ...meta('sibling'), ...(fault === 'cancelled-child' ? { stoppedByUser: true } : { model: 'sol' }) }));
    await assert.rejects(s.linkTaskResult(link), undefined, fault); checks++;
  }
  for (const fault of ['child-restarted', 'parent-restarted', 'parent-cancelled', 'child-cancelled', 'changed-final', 'abort']) {
    const s = await setup(); await s.linkTaskResult(collection(s, 'child')); await s.linkTaskResult(collection(s, 'sibling')); relay(s);
    if (fault === 'child-restarted') s.begin('session', 'sibling');
    if (fault === 'parent-restarted') s.begin('session', 'parent');
    if (fault.endsWith('-cancelled')) {
      const id = fault === 'parent-cancelled' ? 'parent' : 'child';
      await writeFile(join(directory, `agent-${id}.meta.json`), JSON.stringify({ ...meta(id), stoppedByUser: true }));
    }
    if (fault === 'changed-final') await save('child', final('child'));
    const signal = fault === 'abort' ? AbortSignal.abort() : undefined;
    await assert.rejects(s.resolve(binding('parent'), signal), undefined, fault); checks++;
  }
  {
    const s = await setup();
    assert.equal(await s.linkTaskResult(collection(s, 'sibling', 'completed', 'parent')), false); checks++;
    await assert.rejects(s.linkTaskResult(collection(s, 'child', 'failed', 'other'))); checks++;
    await s.linkTaskResult(collection(s, 'foreign'));
    relay(s, 'parent');
    assert.equal((await s.resolve(binding('parent'))).source, 'verified-resume'); checks++;
    relay(s, 'other');
    assert.equal((await s.resolve(binding('other'))).source, 'verified-result-relay'); checks++;
  }
  {
    const s = await setup(); await s.linkTaskResult(collection(s, 'child')); relay(s);
    const results = await Promise.allSettled([s.resolve(binding('parent')), s.resolve(binding('parent'))]);
    assert.equal(results.filter(r => r.status === 'fulfilled').length, 1); checks++;
  }
  process.stdout.write(JSON.stringify({ checks, passed: true, actualBackendRequests: 0 }) + '\n');
} finally { await rm(root, { recursive: true, force: true }); }
