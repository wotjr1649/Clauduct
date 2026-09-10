import assert from 'node:assert/strict';
import { mkdtemp, mkdir, writeFile, unlink, rmdir } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import { join } from 'node:path';
import { createAgentSelection } from './agent-selection.mjs';
import { startNativeGateway } from './native-gateway.mjs';
import { registerBinding, bindingFrom } from './agent-route.mjs';
import { readRequestStatus } from './request-status.mjs';
import { MODELS, EFFORTS } from './models.mjs';
import { interactiveLaunch } from './clauduct.mjs';

const binding = (id, extra = {}) => ({ id, role: 'general-purpose', sessionId: 'session', stop: false, ...extra });
const call = (id, model, role = 'general-purpose') => ({ content: [{ type: 'tool_use', id, name: 'Agent',
  input: { subagent_type: role, ...(model === undefined ? {} : { model }) } }] });
const metadata = (toolUseId, model, extra = {}) => ({ agentType: 'general-purpose', toolUseId,
  ...(model === undefined ? {} : { model }), ...extra });
let snapshots = new Map();
const probeLaunch = interactiveLaunch({ port: 12345, clientHeaders: () => ({ Authorization: 'Bearer SYNTHETIC' }) },
  {}, 'D:/SYNTHETIC_PROJECT', MODELS.luna, [], { verifyAgentModels: true, gptAgents: true });
const agentDefinitions = JSON.parse(probeLaunch.args[probeLaunch.args.indexOf('--agents') + 1]);
// A delayed first request must not start a child already stopped by the user.
for (const tool of ['Agent', 'Task']) for (const role of ['general-purpose', 'clauduct-sol', 'clauduct-inherit']) {
  const stopped = createAgentSelection({ agentDefinitions, timeoutMs: 10,
    readMetadata: async () => metadata('stopped_creation', undefined, { agentType: role, stoppedByUser: true }) });
  const creation = call('stopped_creation', undefined, role);
  creation.content[0].name = tool;
  stopped.remember(creation, 'session', undefined, { model: MODELS.luna.model, effort: 'high' });
  await assert.rejects(stopped.resolve(binding('stopped', { role })), error => error.selectionReason === 'IDENTITY');
}
for (const definition of [{}, { model: null }, { model: 'unknown' }, { model: MODELS.sol.model, effort: 'invalid' }]) {
  assert.throws(() => createAgentSelection({ agentDefinitions: { invalid: definition } }));
}
const probeRole = 'clauduct-probe-inherit';
const definitionSelection = createAgentSelection({ agentDefinitions, timeoutMs: 10, readMetadata: async b => snapshots.get(b.id) });
for (const [name, expected] of Object.entries(MODELS)) {
  const id = `general_${name}`, role = `clauduct-${name}`;
  definitionSelection.remember(call(id, undefined, role), 'session', undefined, { model: MODELS.luna.model, effort: 'low' });
  snapshots.set(id, metadata(id, undefined, { agentType: role }));
  const chosen = await definitionSelection.resolve(binding(id, { role }));
  assert.deepEqual(chosen.route, expected); assert.equal(chosen.source, 'definition-model');
  assert.throws(() => { chosen.route.effort = 'low'; }, TypeError);
}
definitionSelection.remember(call('general_leaf', undefined, 'clauduct-inherit'), 'session', 'general_sol', MODELS.sol);
snapshots.set('general_leaf', metadata('general_leaf', undefined, { agentType: 'clauduct-inherit', parentAgentId: 'general_sol' }));
assert.deepEqual((await definitionSelection.resolve(binding('general_leaf', { role: 'clauduct-inherit' }))).route, MODELS.sol);
definitionSelection.remember({ content: [{ type: 'tool_use', id: 'general_resume', name: 'SendMessage',
  input: { to: 'general_sol', message: 'SYNTHETIC' } }] }, 'session', undefined, MODELS.luna);
definitionSelection.linkResume({ sessionId: 'session', toolUseId: 'general_resume', id: 'general_sol' });
assert.deepEqual((await definitionSelection.resolve(binding('general_sol', { role: 'clauduct-sol' }))).route, MODELS.sol);
definitionSelection.remember(call('definition_origin', undefined, probeRole), 'session', undefined,
  { model: 'gpt-5.6-luna', effort: 'high' });
snapshots.set('definition_child', metadata('definition_origin', undefined, { agentType: probeRole }));
const definitionInherited = await definitionSelection.resolve(binding('definition_child', { role: probeRole }));
assert.deepEqual(definitionInherited.route, { model: 'gpt-5.6-luna', effort: 'high' });
assert.equal(definitionInherited.source, 'definition-inherit');
// A name alone is not evidence.
const unregisteredDefinition = createAgentSelection({ timeoutMs: 10, readMetadata: async b => snapshots.get(b.id) });
unregisteredDefinition.remember(call('definition_origin', undefined, probeRole), 'session', undefined,
  { model: 'gpt-5.6-luna', effort: 'high' });
assert.equal((await unregisteredDefinition.resolve(binding('definition_child', { role: probeRole }))).route, undefined);
assert.throws(() => definitionSelection.remember(call('missing_parent', undefined, probeRole), 'session'));
for (const change of [{ toolUseId: 'wrong' }, { model: 'inherit' }, { parentAgentId: 'wrong' }, { agentType: 'Plan' }]) {
  definitionSelection.remember(call('definition_bad', undefined, probeRole), 'other-session', undefined,
    { model: 'gpt-5.6-luna', effort: 'high' });
  snapshots.set('definition_bad', metadata('definition_bad', undefined, { agentType: probeRole, ...change }));
  await assert.rejects(definitionSelection.resolve(binding('definition_bad', { role: probeRole, sessionId: 'other-session' })));
  // The valid metadata can still consume the original, unmodified pending call.
  snapshots.set('definition_bad', metadata('definition_bad', undefined, { agentType: probeRole }));
  assert.equal((await definitionSelection.resolve(binding('definition_bad', { role: probeRole, sessionId: 'other-session' }))).route.effort, 'high');
}
for (const [name, selected] of Object.entries(MODELS)) for (const effort of EFFORTS) {
  const id = `definition_${name}_${effort}`, parent = { model: selected.model, effort };
  definitionSelection.remember(call(id, undefined, probeRole), 'session', undefined, parent);
  snapshots.set(id, metadata(id, undefined, { agentType: probeRole }));
  assert.deepEqual((await definitionSelection.resolve(binding(id, { role: probeRole }))).route, parent);
}
definitionSelection.remember({ content: [{ type: 'tool_use', id: 'definition_resume', name: 'SendMessage',
  input: { to: 'definition_child', message: 'SYNTHETIC' } }] }, 'session', undefined, MODELS.astra);
definitionSelection.linkResume({ sessionId: 'session', toolUseId: 'definition_resume', id: 'definition_child' });
assert.deepEqual((await definitionSelection.resolve(binding('definition_child', { role: probeRole }))).route,
  { model: 'gpt-5.6-luna', effort: 'high' });
const mutableDefinitions = { [probeRole]: { model: 'inherit' } };
const fixedDefinitions = createAgentSelection({ agentDefinitions: mutableDefinitions, timeoutMs: 10,
  readMetadata: async b => snapshots.get(b.id) });
mutableDefinitions[probeRole].model = 'sol'; mutableDefinitions.injected = { model: 'inherit' };
for (const [id, role] of [['definition_frozen', probeRole], ['definition_injected', 'injected']]) {
  fixedDefinitions.remember(call(id, undefined, role), 'session', undefined, { model: 'gpt-5.6-luna', effort: 'high' });
  snapshots.set(id, metadata(id, undefined, { agentType: role }));
  assert.deepEqual((await fixedDefinitions.resolve(binding(id, { role }))).route,
    role === probeRole ? { model: 'gpt-5.6-luna', effort: 'high' } : undefined);
}
definitionSelection.remember(call('definition_override', 'terra', probeRole), 'session');
snapshots.set('definition_override', metadata('definition_override', 'terra', { agentType: probeRole }));
assert.deepEqual((await definitionSelection.resolve(binding('definition_override', { role: probeRole }))).route, MODELS.terra);
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
const parentRoute = { model: 'gpt-6-astra', effort: 'max' };
// Each session's actual parent route is the source; no fixed main model/effort.
for (const [name, selected] of Object.entries(MODELS)) for (const effort of EFFORTS) {
  const id = `inherit_${name}_${effort}`, parent = { model: selected.model, effort };
  selection.remember(call(id, 'inherit'), 'session', undefined, parent);
  snapshots.set(id, metadata(id, 'inherit'));
  assert.deepEqual((await selection.resolve(binding(id))).route, parent);
}
selection.remember(call('call_inherit', 'inherit'), 'session', undefined, parentRoute);
parentRoute.effort = 'low'; // A later parent change must not rewrite creation evidence.
snapshots.set('inherit', metadata('call_inherit', 'inherit'));
const inherited = await selection.resolve(binding('inherit'));
assert.equal(inherited.source, 'native-inherit');
assert.deepEqual(inherited.route, { model: 'gpt-6-astra', effort: 'max' });
assert.throws(() => { inherited.route.effort = 'low'; }, TypeError);
for (const invalidParent of [undefined, {}, { model: 'gpt-6-astra' }, { effort: 'max' },
  { model: 'unknown', effort: 'max' }, { model: 'gpt-6-astra', effort: 'invalid' }]) {
  assert.throws(() => selection.remember(call('invalid_inherit', 'inherit'), 'session', undefined, invalidParent));
}
// Direct parent snapshot, not the top-level parent or a model's default effort.
selection.remember(call('grandchild_inherit', 'inherit'), 'session', 'inherit', { model: 'gpt-5.6-sol', effort: 'low' });
snapshots.set('grandchild', metadata('grandchild_inherit', 'inherit', { parentAgentId: 'wrong' }));
await assert.rejects(selection.resolve(binding('grandchild')), /AGENT_SELECTION_UNVERIFIED/);
snapshots.set('grandchild', metadata('grandchild_inherit', 'inherit', { parentAgentId: 'inherit' }));
assert.deepEqual((await selection.resolve(binding('grandchild'))).route, { model: 'gpt-5.6-sol', effort: 'low' });
selection.remember({ content: [{ type: 'tool_use', id: 'inherit_resume', name: 'SendMessage',
  input: { to: 'inherit', message: 'SYNTHETIC' } }] }, 'session', undefined, parentRoute);
selection.linkResume({ sessionId: 'session', toolUseId: 'inherit_resume', id: 'inherit' });
assert.deepEqual((await selection.resolve(binding('inherit'))).route, { model: 'gpt-6-astra', effort: 'max' });
for (const [id, chosen, expected] of [
  ['inherit_plan', 'inherit', { model: 'gpt-6-astra', effort: 'low' }],
  ['explicit_astra', 'astra', { model: 'gpt-6-astra', effort: 'medium' }],
  ['explicit_sol', 'sol', { model: 'gpt-5.6-sol', effort: 'xhigh' }],
  ['explicit_terra', 'terra', { model: 'gpt-5.6-terra', effort: 'high' }],
  ['explicit_plan', 'luna', { model: 'gpt-5.6-luna', effort: 'max' }],
  ['default_plan', undefined, { model: 'gpt-5.6-sol', effort: 'xhigh' }]
]) {
  selection.remember(call(id, chosen, 'Plan'), 'session', undefined, parentRoute);
  snapshots.set(id, metadata(id, chosen, { agentType: 'Plan' }));
  await assert.rejects(selection.resolve(binding(id, { role: 'Plan', sessionId: 'other' })), /AGENT_SELECTION_UNVERIFIED/);
  assert.deepEqual((await selection.resolve(binding(id, { role: 'Plan' }))).route, expected);
}
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
assert.throws(() => selection.remember(call('call_unknown', 'unknown'), 'session'),
  error => error.message === 'AGENT_SELECTION_UNVERIFIED' && error.selectionReason === 'MODEL');
// Every Claude alias and its full model id route to the same GPT model; an unmapped
// name still fails closed instead of ending the turn without a reason.
for (const [name, expected] of [['fable', MODELS.astra], ['claude-fable-5-1', MODELS.astra],
  ['opus', MODELS.sol], ['claude-opus-5', MODELS.sol], ['sonnet', MODELS.luna], ['claude-sonnet-5', MODELS.luna],
  ['haiku', MODELS.luna], ['claude-haiku-4-5-20251001', MODELS.luna]]) {
  const id = `alias_${name.replaceAll(/[^a-z0-9]/g, '_')}`;
  selection.remember(call(id, name), 'session');
  snapshots.set(id, metadata(id, name));
  assert.deepEqual((await selection.resolve(binding(id))).route, expected);
}
for (const name of ['claude-3-5-sonnet-20241022', '__proto__', 'claude-', 'fable-5']) {
  assert.throws(() => selection.remember(call(`bad_${name}`, name), 'session'),
    error => error.selectionReason === 'MODEL');
}
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
const stoppedSkill = createAgentSelection({ timeoutMs: 10, readMetadata: async () => ({
  agentType: 'general-purpose', name: 'code-review', spawnDepth: 1, stoppedByUser: true }) });
stoppedSkill.remember(skillCall('stopped_skill'), 'session');
stoppedSkill.linkSkill({ sessionId: 'session', toolUseId: 'stopped_skill', id: 'stopped_skill_child', skill: 'code-review' });
await assert.rejects(stoppedSkill.resolve(binding('stopped_skill_child')), error => error.selectionReason === 'IDENTITY');
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
const reviewMember = await selection.resolve(binding('peer_child'));
assert.equal(reviewMember.reviewContext, true);
assert.equal(reviewMember.review, undefined); // member does not force root diff collection
assert.equal(a.reviewContext, false);
selection.remember(call('review_grandchild'), 'session', 'peer_child');
snapshots.set('review_leaf', metadata('review_grandchild', undefined, { parentAgentId: 'peer_child' }));
assert.equal((await selection.resolve(binding('review_leaf'))).reviewContext, true);
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
  assert.equal(parent.reviewContext, true);
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
        assert.ok(body.input.some(item => item.role === 'developer' && item.content.includes('already executing native code-review')));
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
let gatewayCall = { id: 'call_gateway', model: 'opus' };
let onCancelRead;
const routes = createAgentSelection({ agentDefinitions, timeoutMs: 100, readMetadata: async b => {
  if (b.id === 'cancel_test') onCancelRead?.();
  return snapshots.get(b.id);
} });
snapshots.delete('gateway');
const gateway = await startNativeGateway({ agentSelection: routes,
  admissionOptions: { freeBytes: () => 16 * 1024 ** 3 }, transport: {
    close: async () => {}, diagnostics: () => ({}), send: async body => {
      received.push(body);
      if (body.tools?.length) {
        const args = JSON.stringify({ subagent_type: gatewayCall.role ?? 'general-purpose', model: gatewayCall.model });
        const item = { type: 'function_call', id: 'fc_parent', call_id: gatewayCall.id,
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

  // Gateway contract only: this synthetic schema does not claim native Agent accepts inherit.
  gatewayCall = { id: 'gateway_inherit_origin', model: 'inherit' };
  const inheritParent = await fetch(`${source.ANTHROPIC_BASE_URL}/v1/messages`, { method: 'POST', signal: AbortSignal.timeout(5000),
    headers: { ...gateway.clientHeaders(), 'anthropic-version': '2023-06-01', 'content-type': 'application/json',
      'x-claude-code-session-id': 'session' },
    body: JSON.stringify({ model: 'astra', output_config: { effort: 'max' }, stream: true, max_tokens: 100,
      messages: [{ role: 'user', content: 'SYNTHETIC' }],
      tools: [{ name: 'Agent', input_schema: { type: 'object', properties: {
        subagent_type: { type: 'string' }, model: { type: 'string', enum: ['inherit'] } } } }] }) });
  assert.equal(inheritParent.status, 200, await inheritParent.text());
  snapshots.set('gateway_inherit', metadata('gateway_inherit_origin', 'inherit'));
  await registerBinding(binding('gateway_inherit'), source);
  for (const result of await Promise.all([post('gateway_inherit'), post('gateway_inherit')])) {
    assert.equal(result.status, 200, await result.text());
  }
  for (const request of received.slice(-2)) {
    assert.equal(request.model, 'gpt-6-astra'); assert.equal(request.reasoning.effort, 'max');
  }
  const inheritStatus = (await readRequestStatus(source)).recentRequests.at(-1);
  assert.equal(inheritStatus.selectionSource, 'native-inherit');
  assert.equal(inheritStatus.requestedModel, 'gpt-5.6-luna');
  assert.equal(inheritStatus.model, 'gpt-6-astra'); assert.equal(inheritStatus.effort, 'max');

  // Real native enum, omitted model: definition inheritance vs unchanged built-in defaults.
  for (const [index, role, expected, effort, selectionSource] of [
    [0, probeRole, 'gpt-5.6-luna', 'high', 'definition-inherit'],
    [1, 'general-purpose', 'gpt-5.6-luna', 'max', 'role-default'],
    [2, 'Plan', 'gpt-5.6-sol', 'xhigh', 'role-default'],
    [3, 'clauduct-inherit', 'gpt-5.6-luna', 'high', 'definition-inherit'],
    ...Object.entries(MODELS).map(([name, model], i) => [i + 4, `clauduct-${name}`, model.model, model.effort, 'definition-model'])
  ]) {
    const id = `definition_gateway_${index}`;
    gatewayCall = { id, role };
    const parentResponse = await fetch(`${source.ANTHROPIC_BASE_URL}/v1/messages`, { method: 'POST', signal: AbortSignal.timeout(5000),
      headers: { ...gateway.clientHeaders(), 'anthropic-version': '2023-06-01', 'content-type': 'application/json',
        'x-claude-code-session-id': 'session' },
      body: JSON.stringify({ model: 'luna', output_config: { effort: 'high' }, stream: true, max_tokens: 100,
        messages: [{ role: 'user', content: 'SYNTHETIC' }],
        tools: [{ name: 'Agent', input_schema: { type: 'object', properties: {
          subagent_type: { type: 'string' }, model: { type: 'string', enum: ['sonnet', 'opus', 'haiku', 'fable'] }
        }, required: ['subagent_type'] } }] }) });
    assert.equal(parentResponse.status, 200, await parentResponse.text());
    snapshots.set(id, metadata(id, undefined, { agentType: role, stoppedByUser: true }));
    await registerBinding(binding(id, { role }), source);
    const beforeStopped = received.length;
    const stoppedResponse = await post(id);
    assert.equal(stoppedResponse.status, 400);
    assert.match(await stoppedResponse.text(), /AGENT_SELECTION_UNVERIFIED_IDENTITY/);
    assert.equal(received.length, beforeStopped);
    const stoppedStatus = (await readRequestStatus(source)).recentRequests.at(-1);
    assert.equal(stoppedStatus.failureStage, 'selection');
    assert.equal(stoppedStatus.selectionFailure, 'IDENTITY');
    // Synthetic native state replacement and fresh registration, not a user-stop bypass.
    snapshots.set(id, metadata(id, undefined, { agentType: role }));
    await registerBinding(binding(id, { role }), source);
    for (const response of await Promise.all([post(id), post(id)])) assert.equal(response.status, 200, await response.text());
    const status = (await readRequestStatus(source)).recentRequests.at(-1);
    assert.equal(status.model, expected); assert.equal(status.effort, effort); assert.equal(status.selectionSource, selectionSource);
    for (const request of received.slice(-2)) {
      assert.equal(request.model, expected); assert.equal(request.reasoning.effort, effort);
    }
  }
  // Creation must snapshot the actual direct-parent route, not the main
  // request's current model or the model claimed in a nested request body.
  const directParent = 'definition_gateway_3'; // already verified luna/high
  const nestedRole = 'clauduct-inherit', nestedChild = 'nested_definition_child';
  gatewayCall = { id: 'nested_definition_origin', role: nestedRole };
  const nestedPost = (agent, parent, createsChild = false) => fetch(`${source.ANTHROPIC_BASE_URL}/v1/messages`, {
    method: 'POST', signal: AbortSignal.timeout(5000),
    headers: { ...gateway.clientHeaders(), 'anthropic-version': '2023-06-01', 'content-type': 'application/json',
      'x-claude-code-session-id': 'session', ...(agent && { 'x-claude-code-agent-id': agent }),
      ...(parent && { 'x-claude-code-parent-agent-id': parent }) },
    body: JSON.stringify({ model: 'astra', output_config: { effort: 'max' }, stream: true, max_tokens: 100,
      messages: [{ role: 'user', content: 'SYNTHETIC_NESTED' }], ...(createsChild && {
        tools: [{ name: 'Agent', input_schema: { type: 'object', properties: {
          subagent_type: { type: 'string' }, model: { type: 'string', enum: ['sonnet', 'opus', 'haiku', 'fable'] }
        }, required: ['subagent_type'] } }]
      }) })
  });
  const switchedMain = await nestedPost();
  assert.equal(switchedMain.status, 200, await switchedMain.text());
  assert.equal(received.at(-1).model, 'gpt-6-astra');
  assert.equal(received.at(-1).reasoning.effort, 'max');
  const nestedCreation = await nestedPost(directParent, undefined, true);
  assert.equal(nestedCreation.status, 200, await nestedCreation.text());
  assert.equal(received.at(-1).model, 'gpt-5.6-luna'); assert.equal(received.at(-1).reasoning.effort, 'high');
  const createdBy = (await readRequestStatus(source)).recentRequests.at(-1);
  snapshots.set(nestedChild, metadata(gatewayCall.id, undefined, { agentType: nestedRole, parentAgentId: directParent }));
  await registerBinding(binding(nestedChild, { role: nestedRole }), source);
  const beforeWrongParent = received.length;
  const wrongParent = await nestedPost(nestedChild, 'wrong-parent');
  assert.equal(wrongParent.status, 400); await wrongParent.text();
  assert.equal(received.length, beforeWrongParent);
  const grandchildResponse = await nestedPost(nestedChild, directParent);
  assert.equal(grandchildResponse.status, 200, await grandchildResponse.text());
  assert.equal(received.at(-1).model, 'gpt-5.6-luna'); assert.equal(received.at(-1).reasoning.effort, 'high');
  const nestedStatus = (await readRequestStatus(source)).recentRequests;
  assert.equal(nestedStatus.at(-1).selectionSource, 'definition-inherit');
  assert.equal(nestedStatus.at(-1).parentRef, createdBy.agentRef);
  assert.equal(nestedStatus.at(-1).success, true);
} finally { await gateway.close(); }
process.stdout.write(JSON.stringify({ suite: 'agent-selection', passed: true, actualClaude: 0, externalRequests: 0 }) + '\n');
