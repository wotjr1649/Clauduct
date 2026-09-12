import assert from 'node:assert/strict';
import { resolve } from 'node:path';
import { createFixtureToolPolicy, guardFixtureTransport } from '../verification/fixture-tool-policy.mjs';
import { searchRequestBody } from './native-search.mjs';
import { prepareNative } from './native-protocol.mjs';

let checks = 0;
const workingRoot = resolve('SYNTHETIC_WORK'), readPath = resolve(workingRoot, 'models.mjs');
const workflowScript = "return await agent('Compute 2 + 3', { model: 'luna' });";
const agent = createFixtureToolPolicy({ version: 1, kind: 'agent', workingRoot, readPath });
const workflow = createFixtureToolPolicy({ version: 1, kind: 'workflow', workingRoot, workflowScript });
const image = createFixtureToolPolicy({ version: 1, kind: 'image', workingRoot, readPath: resolve(workingRoot, 'square.png') });
const webfetch = createFixtureToolPolicy({ version: 1, kind: 'webfetch', workingRoot });
const searchPolicy = { version: 1, kind: 'websearch', workingRoot };
const websearch = createFixtureToolPolicy(searchPolicy);
const event = (name, input) => ({ type: 'response.completed', response: { output: [{ type: 'function_call', name, arguments: JSON.stringify(input) }] } });
for (const [policy, name, input] of [
  [agent, 'Read', { file_path: readPath }], [agent, 'Read', { file_path: 'models.mjs', offset: 1, limit: 100 }],
  [agent, 'Agent', { prompt: 'Public probe', description: 'Probe', subagent_type: 'clauduct-probe-inherit', run_in_background: false }],
  [workflow, 'Workflow', { script: workflowScript }], [workflow, 'StructuredOutput', { sum: 5 }],
  [workflow, 'TaskOutput', { task_id: 'public-task-1', block: true, timeout: 1000 }],
  [image, 'Read', { file_path: 'square.png' }],
  [webfetch, 'WebFetch', { url: 'https://example.com', prompt: 'Extract the title.' }],
  [websearch, 'WebSearch', { query: 'Node.js documentation', allowed_domains: ['nodejs.org'] }]
]) { assert.doesNotThrow(() => policy(event(name, input))); checks++; }
for (const [policy, name, input] of [
  [agent, 'Read', { file_path: '../private-profile.json' }], [agent, 'Read', { file_path: 'models.mjs:stream' }],
  [agent, 'Read', { file_path: readPath, command: 'anything' }], [agent, 'Bash', { command: 'anything' }],
  [agent, 'Agent', { prompt: 'Probe', description: 'Probe', subagent_type: 'clauduct-probe-inherit', model: 'astra' }],
  [agent, 'Agent', { prompt: 'Probe', description: 'Probe', subagent_type: 'clauduct-probe-inherit', run_in_background: true }],
  [agent, 'Agent', { prompt: 'Probe', description: 'Probe', subagent_type: 'clauduct-probe-inherit', run_in_background: 'true' }],
  [workflow, 'Workflow', { script: workflowScript + ' arbitrary();' }], [workflow, 'Workflow', { script: workflowScript, scriptPath: 'anything' }],
  [workflow, 'StructuredOutput', { sum: 5, command: 'anything' }], [workflow, 'TaskOutput', { task_id: '../private' }],
  [image, 'Read', { file_path: '../private.png' }], [image, 'TaskOutput', { task_id: 'public-task-1' }],
  [webfetch, 'WebFetch', { url: 'https://example.com.evil.invalid', prompt: 'Title' }],
  [webfetch, 'WebFetch', { url: 'https://example.com/private', prompt: 'Title' }],
  [webfetch, 'WebFetch', { url: 'https://example.com', prompt: 'Title', headers: {} }],
  [websearch, 'WebSearch', { query: 'SYNTHETIC_PRIVATE', allowed_domains: ['nodejs.org'] }],
  [websearch, 'WebSearch', { query: 'Node.js documentation', allowed_domains: ['elsewhere.invalid'] }]
]) { assert.throws(() => policy(event(name, input)), error => error.code === 'VERIFICATION_TOOL_INPUT_REJECTED'); checks++; }
// A rejected completion never reaches the downstream callback that releases native tools.
let delivered = 0;
const unsafe = event('Workflow', { script: 'arbitrary();' });
const transport = { send: async (_body, _signal, options) => { await options.onEvent(unsafe); }, close: async () => {}, diagnostics: () => ({}) };
const guarded = guardFixtureTransport(transport, { version: 1, kind: 'workflow', workingRoot, workflowScript });
await assert.rejects(guarded.send({}, new AbortController().signal, { onEvent: () => { delivered++; } }),
  error => error.code === 'VERIFICATION_TOOL_INPUT_REJECTED');
assert.equal(delivered, 0); checks++;
let sends = 0;
const limited = guardFixtureTransport({ send: async () => { sends++; return [{ type: 'response.completed', response: { output: [],
  usage: { input_tokens: 131072, output_tokens: 1 } } }]; } }, { version: 1, kind: 'workflow', workingRoot, workflowScript });
await limited.send({ input: [{ role: 'user', content: [{ type: 'input_image', image_url: 'data:image/webp;base64,PUBLIC' }] }] }, new AbortController().signal);
await assert.rejects(limited.send({}, new AbortController().signal), error => error.code === 'REQUEST_BUDGET');
assert.equal(sends, 1); assert.equal(limited.fixtureUsage().completions, 1); checks++;
assert.equal(limited.fixtureUsage().imageFormatMask, 8); checks++;
for (const [format, mask] of [['png', 1], ['jpeg', 2], ['gif', 4], ['webp', 8]]) {
  const imageBlock = { type: 'image', source: { type: 'base64', media_type: `image/${format}`, data: 'UFVCTElD' } };
  const prepared = prepareNative({ model: 'luna', max_tokens: 100, stream: true, messages: [
    { role: 'user', content: 'Read the public image.' },
    { role: 'assistant', content: [{ type: 'tool_use', id: 'image_read', name: 'Read', input: { file_path: 'square.png' } }] },
    { role: 'user', content: [{ type: 'tool_result', tool_use_id: 'image_read', content: [imageBlock] }] }
  ] });
  let observed;
  const imageTransport = guardFixtureTransport({ send: async body => { observed = body; return []; } },
    { version: 1, kind: 'image', workingRoot, readPath });
  await imageTransport.send(prepared.body, new AbortController().signal);
  assert.equal(observed, prepared.body);
  assert.equal(imageTransport.fixtureUsage().imageFormatMask, mask, `native Read result ${format}`); checks++;
}
await assert.rejects(limited.search({}, new AbortController().signal), error => error.code === 'REQUEST_BUDGET'); checks++;
let searches = 0;
const searchGuard = guardFixtureTransport({ search: async () => { searches++; return {}; },
  diagnostics: () => ({ requestAttempts: searches }) }, searchPolicy);
const allowedBody = searchRequestBody(null, 'gpt-5.6-luna', { query: 'Node.js documentation', allowed: ['nodejs.org'], blocked: null });
await searchGuard.search(allowedBody, new AbortController().signal); checks++;
for (const body of [{ ...allowedBody, model: 'gpt-6-astra' }, { ...allowedBody, extra: 'SYNTHETIC_PRIVATE' },
  { ...allowedBody, commands: { search_query: [{ q: 'SYNTHETIC_PRIVATE' }] } }]) {
  await assert.rejects(searchGuard.search(body, new AbortController().signal), error => error.code === 'VERIFICATION_TOOL_INPUT_REJECTED'); checks++;
}
assert.equal(searches, 1); assert.equal(searchGuard.fixtureUsage().requestAttempts, 1); checks++;
const noTools = createFixtureToolPolicy({ version: 1, kind: 'none', workingRoot });
assert.throws(() => noTools(event('TaskOutput', { task_id: 'public' })), error => error.code === 'VERIFICATION_TOOL_INPUT_REJECTED');
assert.doesNotThrow(() => noTools({ type: 'response.completed', response: { output: [{ type: 'message' }] } })); checks++;
console.log(JSON.stringify({ suite: 'fixture-tool-policy', checks, unreviewedToolDeliveries: delivered,
  externalRequests: 0, actualCredentialReads: 0 }));
