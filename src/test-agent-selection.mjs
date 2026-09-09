import assert from 'node:assert/strict';
import { mkdtemp, mkdir, writeFile, unlink, rmdir } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import { join } from 'node:path';
import { createAgentSelection } from './agent-selection.mjs';
import { startNativeGateway } from './native-gateway.mjs';
import { registerBinding, bindingFrom } from './agent-route.mjs';
import { readRequestStatus } from './request-status.mjs';

const binding = (id, extra = {}) => ({ id, role: 'general-purpose', sessionId: 'session', stop: false, ...extra });
const call = (id, model, role = 'general-purpose') => ({ content: [{ type: 'tool_use', id, name: 'Agent',
  input: { subagent_type: role, ...(model === undefined ? {} : { model }) } }] });
const metadata = (toolUseId, model, extra = {}) => ({ agentType: 'general-purpose', toolUseId,
  ...(model === undefined ? {} : { model }), ...extra });
let snapshots = new Map();
const selection = createAgentSelection({ timeoutMs: 80, readMetadata: async b => snapshots.get(b.id) });
selection.remember(call('call_A'), 'session');
selection.remember(call('call_B', 'opus'), 'session');
snapshots.set('A', metadata('call_A')); snapshots.set('B', metadata('call_B', 'opus'));
const [b, a] = await Promise.all([selection.resolve(binding('B')), selection.resolve(binding('A'))]);
assert.deepEqual(a.route, { model: 'gpt-5.6-luna', effort: 'max' });
assert.deepEqual(b.route, { model: 'gpt-5.6-sol', effort: 'xhigh' });
await assert.rejects(selection.resolve(binding('B')), /AGENT_SELECTION_UNVERIFIED/);

// A reused agent initially sees its old snapshot: it cannot reuse a consumed call.
selection.remember(call('call_resume', 'terra'), 'session');
const resumeTimer = setTimeout(() => snapshots.set('B', metadata('call_resume', 'terra')), 30);
try { assert.equal((await selection.resolve(binding('B'))).route.model, 'gpt-5.6-terra'); }
finally { clearTimeout(resumeTimer); }
// Native SendMessage resume retains the original creation toolUseId in metadata.
const send = { content: [{ type: 'tool_use', id: 'send_B', name: 'SendMessage', input: { to: 'B', message: 'SYNTHETIC_RESUME' } }] };
selection.remember(send, 'session');
await assert.rejects(selection.resolve(binding('B')), /AGENT_SELECTION_UNVERIFIED/);
const resumeHook = { hook_event_name: 'PostToolUse', tool_name: 'SendMessage', session_id: 'session', tool_use_id: 'send_B',
  tool_input: { to: 'B', message: 'SYNTHETIC_RESUME' }, tool_response: { success: true } };
assert.equal(bindingFrom({ ...resumeHook, tool_response: { success: false } }), null);
assert.equal(JSON.stringify(bindingFrom(resumeHook)).includes('SYNTHETIC_RESUME'), false);
assert.throws(() => selection.linkResume({ ...bindingFrom(resumeHook), parent: 'wrong' }), /AGENT_SELECTION_UNVERIFIED/);
assert.throws(() => selection.linkResume({ ...bindingFrom(resumeHook), sessionId: 'other' }), /AGENT_SELECTION_UNVERIFIED/);
assert.throws(() => selection.linkResume({ ...bindingFrom(resumeHook), id: 'A' }), /AGENT_SELECTION_UNVERIFIED/);
selection.linkResume(bindingFrom(resumeHook));
snapshots.set('B', metadata('call_resume', 'opus'));
await assert.rejects(selection.resolve(binding('B')), /AGENT_SELECTION_UNVERIFIED/);
snapshots.set('B', metadata('call_resume', 'terra'));
const resumed = await selection.resolve(binding('B'));
assert.equal(resumed.source, 'verified-resume');
assert.equal(resumed.route.model, 'gpt-5.6-terra');
await assert.rejects(selection.resolve(binding('B')), /AGENT_SELECTION_UNVERIFIED/);
selection.remember(call('call_bad', 'opus'), 'session');
snapshots.set('bad', metadata('call_bad', 'haiku'));
await assert.rejects(selection.resolve(binding('bad')), /AGENT_SELECTION_UNVERIFIED/);
selection.remember(call('call_nested', 'sol'), 'session', 'parent');
snapshots.set('nested', metadata('call_nested', 'sol', { parentAgentId: 'wrong' }));
await assert.rejects(selection.resolve(binding('nested')), /AGENT_SELECTION_UNVERIFIED/);
snapshots.set('nested', metadata('call_nested', 'sol', { parentAgentId: 'parent' }));
assert.equal((await selection.resolve(binding('nested'))).parent, 'parent');
// Both native representations of no parent must survive a verified resume.
selection.remember(call('null_origin', 'opus'), 'session');
snapshots.set('null_parent', metadata('null_origin', 'opus', { parentAgentId: null }));
assert.equal((await selection.resolve(binding('null_parent'))).source, 'explicit-metadata');
for (const [index, parentAgentId] of [null, undefined].entries()) {
  const toolUseId = `null_resume_${index}`;
  selection.remember({ content: [{ type: 'tool_use', id: toolUseId, name: 'SendMessage',
    input: { to: 'null_parent', message: 'SYNTHETIC' } }] }, 'session');
  assert.equal(selection.linkResume({ sessionId: 'session', toolUseId, id: 'null_parent' }), true);
  snapshots.set('null_parent', metadata('null_origin', 'opus', { parentAgentId: 'wrong' }));
  await assert.rejects(selection.resolve(binding('null_parent')), /AGENT_SELECTION_UNVERIFIED/);
  snapshots.set('null_parent', metadata('null_origin', 'opus', { parentAgentId }));
  assert.equal((await selection.resolve(binding('null_parent'))).source, 'verified-resume');
}
selection.remember(call('call_inherit', 'inherit'), 'session');
snapshots.set('inherit', metadata('call_inherit', 'inherit'));
assert.equal((await selection.resolve(binding('inherit'))).source, 'native-inherit');
const abort = new AbortController(); abort.abort();
await assert.rejects(selection.resolve(binding('missing'), abort.signal));
let deniedReads = 0;
const denied = createAgentSelection({ readMetadata: async () => {
  deniedReads++; throw Object.assign(new Error('SYNTHETIC_DENIAL'), { code: 'EACCES' });
} });
await assert.rejects(denied.resolve(binding('denied')));
assert.equal(deniedReads, 1);
for (const code of ['ENOTDIR', 'ERR_ENCODING_INVALID_ENCODED_DATA', 'SYNTHETIC_PRIVATE_CODE']) {
  const io = createAgentSelection({ readMetadata: async () => { throw Object.assign(new Error('SYNTHETIC_PRIVATE_PATH'), { code }); } });
  await assert.rejects(io.resolve(binding('io')), error => error.selectionReason === 'IO'
    && error.selectionIoCode === (code === 'SYNTHETIC_PRIVATE_CODE' ? 'OTHER' : code)
    && !JSON.stringify(error).includes('SYNTHETIC_PRIVATE'));
}
selection.remember(call('call_role', undefined, 'Plan'), 'session');
snapshots.set('role', metadata('call_role'));
await assert.rejects(selection.resolve(binding('role')), /AGENT_SELECTION_UNVERIFIED/);
assert.throws(() => selection.remember(call('call_unknown', 'unknown'), 'session'), /UNSUPPORTED_MODEL_OR_EFFORT/);
assert.throws(() => selection.remember(call('call_object', { private: 'SYNTHETIC' }), 'session'), /AGENT_SELECTION_UNVERIFIED/);
// A later invalid call must not leave an earlier, undelivered call usable.
for (const duplicate of [false, true]) {
  const routes = createAgentSelection({ timeoutMs: 10, readMetadata: async () => metadata('batch_first', 'opus') });
  const first = call('batch_first', 'opus').content[0];
  const second = duplicate ? first : call('batch_invalid', 'unsupported').content[0];
  assert.throws(() => routes.remember({ content: [first, second] }, 'session'));
  await assert.rejects(routes.resolve(binding('batch_child')), /AGENT_SELECTION_UNVERIFIED/);
  routes.remember({ content: [first] }, 'session');
  assert.equal((await routes.resolve(binding('batch_child'))).route.model, 'gpt-5.6-sol');
}

const skillCall = id => ({ content: [{ type: 'tool_use', id, name: 'Skill', input: { skill: 'code-review' } }] });
// Missing names must not become routing evidence through undefined equality.
for (const skill of [undefined, null, 42, '', 'x'.repeat(201)]) {
  const invalid = createAgentSelection({ timeoutMs: 1, readMetadata: async () => ({ agentType: 'general-purpose' }) });
  invalid.remember({ content: [{ type: 'tool_use', id: 'bad_skill', name: 'Skill', input: { skill } }] }, 'session');
  for (const name of [undefined, skill]) {
    assert.throws(() => invalid.linkSkill({ sessionId: 'session', toolUseId: 'bad_skill', id: 'bad_child', skill: name }), /AGENT_SELECTION_UNVERIFIED/);
  }
  await assert.rejects(invalid.resolve(binding('bad_child')), /AGENT_SELECTION_UNVERIFIED/);
}
const skillHook = (id, child) => ({ hook_event_name: 'PostToolUse', tool_name: 'Skill', session_id: 'session', tool_use_id: id,
  tool_response: { success: true, status: 'forked', background: true, agentId: child, commandName: 'code-review', result: 'SYNTHETIC_PRIVATE' } });
selection.remember(skillCall('skill_one'), 'session');
selection.remember(skillCall('skill_two'), 'session');
snapshots.set('skillA', { agentType: 'general-purpose', name: 'code-review', spawnDepth: 1 });
snapshots.set('skillB', { agentType: 'general-purpose', name: 'code-review', spawnDepth: 1 });
// A matching name alone never authorizes correlation.
await assert.rejects(selection.resolve(binding('skillA')), /AGENT_SELECTION_UNVERIFIED/);
assert.equal(bindingFrom({ ...skillHook('skill_one', 'skillA'), tool_response: { status: 'inline' } }), null);
const linkA = bindingFrom(skillHook('skill_one', 'skillA'));
assert.equal(JSON.stringify(linkA).includes('SYNTHETIC_PRIVATE'), false);
assert.throws(() => selection.linkSkill({ ...linkA, skill: 'different' }), /AGENT_SELECTION_UNVERIFIED/);
const linkTimer = setTimeout(() => {
  selection.linkSkill(bindingFrom(skillHook('skill_two', 'skillB')));
  selection.linkSkill(linkA);
}, 25);
try {
  const linked = await Promise.all([selection.resolve(binding('skillA')), selection.resolve(binding('skillB'))]);
  assert.ok(linked.every(result => result.route.model === 'gpt-5.6-luna'));
  assert.ok(linked.every(result => result.source === 'skill-result'));
} finally { clearTimeout(linkTimer); }
assert.throws(() => selection.linkSkill(linkA), /AGENT_SELECTION_UNVERIFIED/);

// A verified child can wake its existing forked parent by ID or its exact native name.
selection.remember(call('peer_child_origin'), 'session', 'skillA');
snapshots.set('peer_child', metadata('peer_child_origin', undefined, { parentAgentId: 'skillA' }));
await selection.resolve(binding('peer_child'));
for (const [index, recipient] of ['skillA', 'code-review'].entries()) {
  const toolUseId = `peer_send_${index}`;
  selection.remember({ content: [{ type: 'tool_use', id: toolUseId, name: 'SendMessage',
    input: { to: recipient, message: 'SYNTHETIC_PEER' } }] }, 'session', 'peer_child');
  await assert.rejects(selection.resolve(binding('skillA'))); // no successful delivery proof yet
  assert.throws(() => selection.linkResume({ sessionId: 'other', toolUseId, id: recipient, parent: 'peer_child' }));
  assert.throws(() => selection.linkResume({ sessionId: 'session', toolUseId, id: recipient, parent: 'unknown' }));
  assert.equal(selection.linkResume({ sessionId: 'session', toolUseId, id: recipient, parent: 'peer_child' }), true);
  for (const change of [{ name: 'different' }, { model: 'opus' }, { parentAgentId: 'wrong' }, { stoppedByUser: true }]) {
    snapshots.set('skillA', { agentType: 'general-purpose', name: 'code-review', spawnDepth: 1, ...change });
    await assert.rejects(selection.resolve(binding('skillA')));
  }
  snapshots.set('skillA', { agentType: 'general-purpose', name: 'code-review', spawnDepth: 1 });
  const parent = await selection.resolve(binding('skillA'));
  assert.equal(parent.source, 'verified-peer-resume');
  assert.equal(parent.parent, undefined); assert.equal(parent.review, true);
  assert.equal(parent.route.model, 'gpt-5.6-luna');
  await assert.rejects(selection.resolve(binding('skillA'))); // delivery proof consumed once
}
selection.remember({ content: [{ type: 'tool_use', id: 'unrelated_send', name: 'SendMessage',
  input: { to: 'skillA', message: 'SYNTHETIC' } }] }, 'session', 'A');
assert.equal(selection.linkResume({ sessionId: 'session', toolUseId: 'unrelated_send', id: 'skillA', parent: 'A' }), false);
await assert.rejects(selection.resolve(binding('skillA')));

// Exercise the real bounded filesystem reader with task-owned synthetic files.
const root = await mkdtemp(fileURLToPath(new URL('./selection-fixture-', import.meta.url)));
const sessionDir = join(root, 'session'), agentsDir = join(sessionDir, 'subagents');
const path = join(agentsDir, 'agent-disk.meta.json');
const markerPath = join(agentsDir, 'agent-disk.forked-skill.marker.json');
const scopePath = join(agentsDir, 'agent-disk.forked-skill.json');
try {
  await mkdir(sessionDir); await mkdir(agentsDir);
  const disk = createAgentSelection({ projectsRoot: root, timeoutMs: 60 });
  disk.remember(call('call_disk', 'opus'), 'session');
  await writeFile(path, JSON.stringify(metadata('call_disk', 'opus')));
  const location = binding('disk', { transcriptPath: join(root, 'session.jsonl') });
  assert.equal((await disk.resolve(location)).route.effort, 'xhigh');
  await assert.rejects(disk.resolve({ ...location, transcriptPath: join(root, '..', 'session.jsonl') }));
  await writeFile(path, ' '.repeat(16385));
  await assert.rejects(disk.resolve(location), /AGENT_SELECTION_UNVERIFIED/);
  const native = createAgentSelection({ projectsRoot: root, timeoutMs: 30 });
  const modelFork = createAgentSelection({ projectsRoot: root, timeoutMs: 30 });
  await new Promise(resolve => setTimeout(resolve, 10));
  const nativeMeta = { agentType: 'general-purpose', name: 'code-review', spawnDepth: 1 };
  await writeFile(path, JSON.stringify(nativeMeta));
  const nativeLocation = { ...location, nativeRegistered: true };
  await assert.rejects(native.resolve(nativeLocation)); // name alone is not proof
  await writeFile(markerPath, JSON.stringify({ forkedSkill: true, skillName: 'code-review' }));
  await writeFile(scopePath, JSON.stringify({ skillName: 'wrong', attributionName: 'code-review' }));
  await assert.rejects(native.resolve(nativeLocation));
  await writeFile(scopePath, JSON.stringify({ skillName: 'code-review', attributionName: 'code-review', effort: 'low' }));
  await assert.rejects(native.resolve(location)); // no live registration
  await writeFile(path, JSON.stringify({ ...nativeMeta, parentAgentId: 'wrong' }));
  await assert.rejects(native.resolve(nativeLocation));
  await writeFile(path, JSON.stringify(nativeMeta));
  modelFork.remember({ content: [{ type: 'tool_use', id: 'model_skill', name: 'Skill', input: { skill: 'code-review' } }] }, 'session');
  await assert.rejects(modelFork.resolve(nativeLocation));
  modelFork.linkSkill({ sessionId: 'session', toolUseId: 'model_skill', id: 'disk', skill: 'code-review' });
  assert.equal((await modelFork.resolve(nativeLocation)).source, 'skill-result');
  const ng = await startNativeGateway({ agentSelection: native,
    admissionOptions: { freeBytes: () => 16 * 1024 ** 3 }, transport: {
      close: async () => {}, diagnostics: () => ({}), send: async body => {
        assert.equal(body.model, 'gpt-5.6-luna'); assert.equal(body.reasoning.effort, 'max');
        const item = { id: 'msg_fork', type: 'message', role: 'assistant', status: 'completed',
          content: [{ type: 'output_text', text: 'OK', annotations: [] }] };
        return [{ type: 'response.created', response: { id: 'resp_fork', status: 'in_progress' } },
          { type: 'response.output_item.added', output_index: 0, item: { ...item, content: [], status: 'in_progress' } },
          { type: 'response.output_text.delta', output_index: 0, item_id: item.id, content_index: 0, delta: 'OK' },
          { type: 'response.output_text.done', output_index: 0, item_id: item.id, content_index: 0, text: 'OK' },
          { type: 'response.output_item.done', output_index: 0, item },
          { type: 'response.completed', response: { id: 'resp_fork', status: 'completed', model: body.model,
            output: [item], usage: { input_tokens: 1, output_tokens: 1, total_tokens: 2 } } }];
      } } });
  try {
    const env = { ANTHROPIC_BASE_URL: `http://127.0.0.1:${ng.port}`, ANTHROPIC_AUTH_TOKEN: ng.clientHeaders().Authorization.slice(7) };
    await registerBinding(location, env);
    const response = await fetch(`${env.ANTHROPIC_BASE_URL}/v1/messages`, { method: 'POST', signal: AbortSignal.timeout(5000),
      headers: { ...ng.clientHeaders(), 'content-type': 'application/json', 'anthropic-version': '2023-06-01',
        'x-claude-code-session-id': 'session', 'x-claude-code-agent-id': 'disk' },
      body: JSON.stringify({ model: 'luna', stream: true, max_tokens: 100, messages: [{ role: 'user', content: 'SYNTHETIC' }] }) });
    assert.equal(response.status, 200, await response.text());
    assert.equal((await readRequestStatus(env)).recentRequests.at(-1).selectionSource, 'native-fork');
  } finally { await ng.close(); }
  await assert.rejects(native.resolve(nativeLocation)); // consumed creation cannot replay
  await new Promise(resolve => setTimeout(resolve, 10));
  const stale = createAgentSelection({ projectsRoot: root, timeoutMs: 30 });
  await assert.rejects(stale.resolve(nativeLocation)); // earlier gateway's sidecars
} finally {
  for (const extra of [markerPath, scopePath]) await unlink(extra).catch(error => { if (error.code !== 'ENOENT') throw error; });
  await unlink(path).catch(error => { if (error.code !== 'ENOENT') throw error; });
  await rmdir(agentsDir); await rmdir(sessionDir); await rmdir(root);
}

const received = [];
let onCancelRead;
const routes = createAgentSelection({ timeoutMs: 100, readMetadata: async b => {
  if (b.id === 'cancel_test') onCancelRead?.();
  return snapshots.get(b.id);
} });
snapshots.delete('gateway');
const gateway = await startNativeGateway({ agentSelection: routes,
  admissionOptions: { freeBytes: () => 16 * 1024 ** 3 }, transport: {
    close: async () => {}, diagnostics: () => ({}), send: async body => {
      received.push(body);
      if (body.tools?.length) {
        const args = JSON.stringify({ subagent_type: 'general-purpose', model: 'opus' });
        const item = { type: 'function_call', id: 'fc_parent', call_id: 'call_gateway',
          name: body.tools[0].name, arguments: args, status: 'completed' };
        return [{ type: 'response.created', response: { id: 'resp_parent', status: 'in_progress' } },
          { type: 'response.output_item.added', output_index: 0, item: { ...item, arguments: '', status: 'in_progress' } },
          { type: 'response.function_call_arguments.delta', output_index: 0, item_id: item.id, delta: args },
          { type: 'response.function_call_arguments.done', output_index: 0, item_id: item.id, arguments: args },
          { type: 'response.output_item.done', output_index: 0, item },
          { type: 'response.completed', response: { id: 'resp_parent', status: 'completed', model: body.model,
            reasoning: body.reasoning, output: [item], usage: { input_tokens: 1, output_tokens: 1, total_tokens: 2 } } }];
      }
      const item = { type: 'message', id: 'msg_test', role: 'assistant', status: 'completed',
        content: [{ type: 'output_text', text: 'OK', annotations: [] }] };
      return [{ type: 'response.created', response: { id: 'resp_test', status: 'in_progress' } },
        { type: 'response.output_item.added', output_index: 0, item: { ...item, status: 'in_progress', content: [] } },
        { type: 'response.output_text.delta', output_index: 0, item_id: item.id, content_index: 0, delta: 'OK' },
        { type: 'response.output_text.done', output_index: 0, item_id: item.id, content_index: 0, text: 'OK' },
        { type: 'response.output_item.done', output_index: 0, item },
        { type: 'response.completed', response: { id: 'resp_test', status: 'completed', model: body.model,
          reasoning: body.reasoning, output: [item], usage: { input_tokens: 1, output_tokens: 1, total_tokens: 2 } } }];
    }
  } });
try {
  const source = { ANTHROPIC_BASE_URL: `http://127.0.0.1:${gateway.port}`,
    ANTHROPIC_AUTH_TOKEN: gateway.clientHeaders().Authorization.slice(7) };
  const parent = await fetch(`${source.ANTHROPIC_BASE_URL}/v1/messages`, { method: 'POST', signal: AbortSignal.timeout(5000),
    headers: { ...gateway.clientHeaders(), 'anthropic-version': '2023-06-01', 'content-type': 'application/json',
      'x-claude-code-session-id': 'session' },
    body: JSON.stringify({ model: 'luna', stream: true, max_tokens: 100, messages: [{ role: 'user', content: 'SYNTHETIC' }],
      tools: [{ name: 'Agent', input_schema: { type: 'object', properties: {
        subagent_type: { type: 'string' }, model: { type: 'string' } }, required: ['subagent_type'] } }] }) });
  assert.equal(parent.status, 200); assert.match(await parent.text(), /call_gateway/);
  await registerBinding(binding('gateway'), source);
  // Match native ordering: the hook must return before the sidecar is written.
  const post = (id, signal = AbortSignal.timeout(5000)) => fetch(`${source.ANTHROPIC_BASE_URL}/v1/messages`, { method: 'POST', signal,
    headers: { ...gateway.clientHeaders(), 'anthropic-version': '2023-06-01', 'content-type': 'application/json',
      'x-claude-code-session-id': 'session', 'x-claude-code-agent-id': id },
    body: JSON.stringify({ model: 'luna', stream: true, max_tokens: 100, messages: [{ role: 'user', content: 'SYNTHETIC' }] }) });
  const metadataTimer = setTimeout(() => snapshots.set('gateway', metadata('call_gateway', 'opus')), 30);
  try {
    const results = await Promise.all([post('gateway'), post('gateway')]);
    for (const ok of results) { const body = await ok.text(); assert.equal(ok.status, 200, body); }
  } finally { clearTimeout(metadataTimer); }
  assert.equal(received[1].model, 'gpt-5.6-sol'); assert.equal(received[1].reasoning.effort, 'xhigh');
  const modelStatus = await readRequestStatus(source);
  assert.equal(modelStatus.recentRequests.at(-1).requestedModel, 'gpt-5.6-luna');
  assert.equal(modelStatus.recentRequests.at(-1).model, 'gpt-5.6-sol');
  assert.equal(gateway.diagnostics().recentRequests.at(-1).selectionSource, 'explicit-metadata');
  const missing = await post('unknown'); assert.equal(missing.status, 400); await missing.text();
  await registerBinding(binding('not_ready'), source);
  const notReady = await post('not_ready');
  assert.equal(notReady.status, 400);
  assert.match(await notReady.text(), /AGENT_SELECTION_UNVERIFIED_MISSING/);
  assert.equal(received.length, 3);
  routes.remember(skillCall('skill_gateway'), 'session');
  snapshots.set('fork_gateway', { agentType: 'general-purpose', name: 'code-review', spawnDepth: 1 });
  await registerBinding(binding('fork_gateway'), source);
  const waitingFork = post('fork_gateway');
  await registerBinding(bindingFrom(skillHook('skill_gateway', 'fork_gateway')), source);
  const fork = await waitingFork;
  const forkBody = await fork.text(); assert.equal(fork.status, 200, forkBody);
  assert.equal(received[3].model, 'gpt-5.6-luna');
  assert.equal(gateway.diagnostics().recentRequests.at(-1).selectionSource, 'skill-result');
  await registerBinding({ ...binding('gateway'), stop: true }, source);
  routes.remember({ content: [{ type: 'tool_use', id: 'resume_gateway', name: 'SendMessage',
    input: { to: 'gateway', message: 'SYNTHETIC_CONTINUE' } }] }, 'session');
  await registerBinding(binding('gateway'), source);
  const resumedRequest = post('gateway');
  await registerBinding(bindingFrom({ ...resumeHook, tool_use_id: 'resume_gateway',
    tool_input: { to: 'gateway', message: 'SYNTHETIC_CONTINUE' } }), source);
  const resumedResponse = await resumedRequest;
  assert.equal(resumedResponse.status, 200, await resumedResponse.text());
  assert.equal(received[4].model, 'gpt-5.6-sol');
  assert.equal(gateway.diagnostics().recentRequests.at(-1).selectionSource, 'verified-resume');

  routes.remember(call('peer_gateway_origin'), 'session', 'fork_gateway');
  snapshots.set('peer_gateway', metadata('peer_gateway_origin', undefined, { parentAgentId: 'fork_gateway' }));
  // Resolve the sender before exercising its authenticated successful PostToolUse hook.
  await routes.resolve(binding('peer_gateway'));
  routes.remember({ content: [{ type: 'tool_use', id: 'peer_gateway_send', name: 'SendMessage',
    input: { to: 'code-review', message: 'SYNTHETIC_PEER' } }] }, 'session', 'peer_gateway');
  await registerBinding({ ...binding('fork_gateway'), stop: true }, source);
  await registerBinding(binding('fork_gateway'), source);
  const peerWaiting = post('fork_gateway');
  await registerBinding(bindingFrom({ ...resumeHook, agent_id: 'peer_gateway', tool_use_id: 'peer_gateway_send',
    tool_input: { to: 'code-review', message: 'SYNTHETIC_PEER' } }), source);
  const peerResponse = await peerWaiting;
  assert.equal(peerResponse.status, 200, await peerResponse.text());
  assert.equal((await readRequestStatus(source)).recentRequests.at(-1).selectionSource, 'verified-peer-resume');

  routes.remember(call('cancel_origin', 'opus'), 'session');
  await registerBinding(binding('cancel_test'), source);
  const reading = new Promise(resolve => { onCancelRead = resolve; });
  const cancellation = new AbortController();
  const cancelled = post('cancel_test', cancellation.signal).then(async r => { await r.text(); return 'completed'; }, e => e.name);
  await reading; cancellation.abort();
  const survivor = post('cancel_test');
  snapshots.set('cancel_test', metadata('cancel_origin', 'opus'));
  const survivingResponse = await survivor;
  assert.equal(survivingResponse.status, 200, await survivingResponse.text());
  assert.equal(await cancelled, 'AbortError');
  assert.equal(received.length, 7);
} finally { await gateway.close(); }
process.stdout.write(JSON.stringify({ suite: 'agent-selection', passed: true, actualClaude: 0, externalRequests: 0 }) + '\n');
