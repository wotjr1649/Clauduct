import assert from 'node:assert/strict';
import { mkdtempSync, mkdirSync, writeFileSync, rmSync } from 'node:fs';
import { join, resolve, sep } from 'node:path';
import { fileURLToPath } from 'node:url';
import { verifyCompletionRelayTarget } from '../verification/completion-relay-target.mjs';
import { createFixtureToolPolicy } from '../verification/fixture-tool-policy.mjs';

const temporaryRoot = fileURLToPath(new URL('../.tmp/', import.meta.url));
const root = mkdtempSync(join(temporaryRoot, 'completion-relay-'));
const workingRoot = join(root, 'work'), agents = join(root, 'config', 'projects', 'public', 'session', 'subagents');
mkdirSync(workingRoot); mkdirSync(agents, { recursive: true });
const args = { workingRoot, target: 'parent', parentCall: 'call_parent', childCalls: ['call_first', 'call_second'] };
const metadata = id => ({ agentType: id === 'parent' ? 'clauduct-inherit' : 'clauduct-probe-inherit',
  toolUseId: `call_${id}`, ...(id !== 'parent' && { parentAgentId: 'parent' }) });
const row = id => ({ type: 'assistant', agentId: id, sessionId: 'session', message: { stop_reason: 'end_turn',
  content: [{ type: 'text', text: 'MODEL-PROBE-COMPLETED' }] } });
const metaPath = id => join(agents, `agent-${id}.meta.json`);
const transcript = (id, value) => writeFileSync(join(agents, `agent-${id}.jsonl`), JSON.stringify(value) + '\n');
function seed() { for (const id of ['parent', 'first', 'second']) { writeFileSync(metaPath(id), JSON.stringify(metadata(id))); transcript(id, row(id)); } }
const rejected = action => assert.throws(action, error => error.code === 'VERIFICATION_TOOL_INPUT_REJECTED');
let checks = 0;
try {
  seed(); assert.equal(verifyCompletionRelayTarget(args), 'parent'); checks++;
  const configuration = { version: 1, kind: 'completion', completionMode: 'relay', workingRoot,
    readPath: join(workingRoot, 'models.mjs'), parentPrompt: 'PUBLIC_PARENT', childPrompts: ['PUBLIC_FIRST', 'PUBLIC_SECOND'] };
  const event = (name, input, call_id) => ({ type: 'response.completed', response: { output: [{ type: 'function_call', name, call_id, arguments: JSON.stringify(input) }] } });
  const parentEvent = event('Agent', { subagent_type: 'clauduct-inherit', prompt: 'PUBLIC_PARENT', run_in_background: true, description: 'Public' }, 'call_parent');
  const sendEvent = event('SendMessage', { to: 'parent', message: 'PUBLIC_CHILDREN_COMPLETED' }, 'call_send');
  const relay = createFixtureToolPolicy(configuration);
  rejected(() => relay(sendEvent)); checks++;
  relay(parentEvent);
  for (const id of ['first', 'second']) relay(event('Agent', { subagent_type: 'clauduct-probe-inherit', prompt: `PUBLIC_${id.toUpperCase()}`, run_in_background: true, description: 'Public' }, `call_${id}`));
  for (const change of [{ to: 'different-session' }, { message: 'UNREVIEWED_MESSAGE' }, { extra: true }]) {
    rejected(() => relay(event('SendMessage', { to: 'parent', message: 'PUBLIC_CHILDREN_COMPLETED', ...change }, 'send'))); checks++;
  }
  assert.doesNotThrow(() => relay(sendEvent)); checks++;
  rejected(() => relay(sendEvent)); checks++;
  rejected(() => relay(event('TaskOutput', { task_id: 'unrelated-session', block: true, timeout: 60000 }, 'collect'))); checks++;
  const collect = event('TaskOutput', { task_id: 'parent', block: true, timeout: 60000 }, 'collect');
  rejected(() => createFixtureToolPolicy(configuration)(collect)); checks++;
  assert.doesNotThrow(() => relay(collect)); checks++;
  rejected(() => relay(collect)); checks++;
  rejected(() => createFixtureToolPolicy(configuration)({ ...parentEvent, response: { output: [{ ...parentEvent.response.output[0], call_id: undefined }] } })); checks++;
  for (const change of [{ target: '../parent' }, { target: 'first' }, { target: 'unrelated-session' },
    { parentCall: 'different_call' }, { childCalls: ['call_first', 'call_first'] }, { childCalls: ['call_first', 'unknown'] }]) {
    rejected(() => verifyCompletionRelayTarget({ ...args, ...change })); checks++;
  }
  for (const [id, change] of [['parent', { stoppedByUser: true }], ['parent', { parentAgentId: 'other' }],
    ['first', { stoppedByUser: true }], ['first', { parentAgentId: 'other' }], ['second', { toolUseId: 'call_first' }]]) {
    seed(); writeFileSync(metaPath(id), JSON.stringify({ ...metadata(id), ...change }));
    rejected(() => verifyCompletionRelayTarget(args)); checks++;
  }
  for (const change of [{ type: 'user' }, { isApiErrorMessage: true }, { sessionId: 'other' },
    { agentId: 'other' }, { message: { stop_reason: 'tool_use', content: [] } },
    { message: { stop_reason: 'end_turn', content: [{ type: 'text', text: 'UNFINISHED' }] } }]) {
    seed(); transcript('second', { ...row('second'), ...change });
    rejected(() => verifyCompletionRelayTarget(args)); checks++;
  }
  seed(); writeFileSync(join(agents, 'agent-second.jsonl'), '{"unfinished":');
  rejected(() => verifyCompletionRelayTarget(args)); checks++;
  seed(); writeFileSync(metaPath('second'), 'x'.repeat(16385));
  rejected(() => verifyCompletionRelayTarget(args)); checks++;
  seed(); assert.equal(verifyCompletionRelayTarget(args), 'parent'); checks++;
  console.log(JSON.stringify({ suite: 'completion-relay-target', checks, externalMessages: 0, externalRequests: 0, actualCredentialReads: 0 }));
} finally {
  assert.ok(resolve(root).startsWith(resolve(temporaryRoot) + sep));
  assert.ok(root.startsWith(join(temporaryRoot, 'completion-relay-')));
  rmSync(root, { recursive: true });
}
