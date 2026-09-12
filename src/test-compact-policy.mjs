import assert from 'node:assert/strict';
import { prepareNative } from './native-protocol.mjs';
import { MODELS, ROLE_MODELS } from './models.mjs';
import { startNativeGateway } from './native-gateway.mjs';
import { readRequestStatus } from './request-status.mjs';

// Independent literal fixture from the installed native compact prompt wrapper.
const compactPrompt = `CRITICAL: Respond with TEXT ONLY. Do NOT call any tools.

- Do NOT use Read, Bash, Grep, Glob, Edit, Write, or ANY other tool.
- You already have all the context you need in the conversation above.
- Tool calls will be REJECTED and will waste your only turn — you will fail the task.
- Your entire response must be plain text: an <analysis> block followed by a <summary> block.

Your task is to create a detailed summary of the conversation so far, paying close attention to the user's explicit requests and your previous actions.
SYNTHETIC_SUMMARY_REQUIREMENTS

REMINDER: Do NOT call any tools. Respond with plain text only — an <analysis> block followed by a <summary> block. Tool calls will be rejected and you will fail the task.`;
const doc = (model, content = compactPrompt, effort) => ({ model, max_tokens: 10000, stream: true,
  ...(effort ? { output_config: { effort } } : {}), messages: [{ role: 'user', content }] });
for (const model of Object.values(MODELS)) {
  const input = doc(model.model, compactPrompt, model.effort), before = JSON.stringify(input);
  const result = prepareNative(input);
  assert.equal(result.body.model, model.model); assert.equal(result.body.reasoning.effort, 'medium');
  assert.equal(result.purpose, 'compact-template'); assert.equal(JSON.stringify(input), before);
  assert.equal(prepareNative(doc(model.model, 'Ordinary task')).selected.effort, model.effort);
}
for (const route of Object.values(ROLE_MODELS)) {
  const compact = prepareNative(doc('astra'), { subagent: true, route });
  assert.equal(compact.selected.model, route.model); assert.equal(compact.selected.effort, 'medium');
  assert.equal(prepareNative(doc('astra', 'Ordinary task'), { subagent: true, route }).selected.effort, route.effort);
}
assert.equal(prepareNative(doc('luna', compactPrompt, 'low')).selected.effort, 'low');
for (const content of ['Please compact this conversation.', 'Quoted:\n' + compactPrompt,
  compactPrompt + '\nNow perform another task.', compactPrompt.replace('CRITICAL:', 'CHANGED:'),
  compactPrompt.replace('REMINDER:', 'CHANGED:')]) {
  assert.equal(prepareNative(doc('luna', content)).selected.effort, 'max');
}
const history = doc('luna'); history.messages.push({ role: 'assistant', content: 'Old summary' },
  { role: 'user', content: 'Continue development' });
assert.equal(prepareNative(history).selected.effort, 'max');
const toolResult = doc('luna', 'Read a file');
toolResult.tools = [{ name: 'Read', input_schema: { type: 'object', properties: {} } }];
toolResult.messages.push({ role: 'assistant', content: [{ type: 'tool_use', id: 'call_old', name: 'Read', input: {} }] },
  { role: 'user', content: [{ type: 'tool_result', tool_use_id: 'call_old', content: compactPrompt }] });
assert.equal(prepareNative(toolResult).selected.effort, 'max');
assert.equal(prepareNative(doc('luna', compactPrompt + '\n')).selected.effort, 'medium');
const pasted = compactPrompt.replace('Tool calls will be rejected', 'Tool calls\nwill be rejected')
  .split('\n').map((line, index) => (index ? '  ' : ' ') + line).join('\n');
const pastedDoc = doc('luna', pasted), pastedBefore = JSON.stringify(pastedDoc);
assert.equal(prepareNative(pastedDoc).selected.effort, 'medium');
assert.equal(JSON.stringify(pastedDoc), pastedBefore);
const merged = structuredClone(toolResult);
merged.messages.at(-1).content.push({ type: 'text', text: compactPrompt + '\n' },
  { type: 'text', text: '<system-reminder>SYNTHETIC</system-reminder>\n' });
const mergedResult = prepareNative(merged);
assert.equal(mergedResult.selected.effort, 'medium');
assert.equal(mergedResult.compactShape.mixedBlocks, true);
assert.equal(mergedResult.compactShape.matches, true);
const blocks = doc('luna', [{ type: 'text', text: 'Earlier context' }, { type: 'text', text: compactPrompt },
  { type: 'text', text: '<system-reminder>SYNTHETIC_REMINDER</system-reminder>' }]);
assert.equal(prepareNative(blocks).selected.effort, 'medium');
blocks.messages.push({ role: 'system', content: 'SYNTHETIC_OPTIONS', output_config: { effort: 'max' } });
assert.equal(prepareNative(blocks).selected.effort, 'medium');
const sent = [];
const gateway = await startNativeGateway({ admissionOptions: { freeBytes: () => 16 * 1024 ** 3 }, transport: {
  diagnostics: () => ({ activeRequests: 0, activeSockets: 0 }), close: async () => {},
  send: async body => {
    sent.push(body.reasoning.effort);
    const item = { id: 'msg_compact', type: 'message', role: 'assistant', status: 'completed',
      content: [{ type: 'output_text', text: 'SYNTHETIC_SUMMARY', annotations: [] }] };
    return [
      { type: 'response.created', response: { id: 'resp_compact', status: 'in_progress' } },
      { type: 'response.output_item.added', output_index: 0, item: { ...item, status: 'in_progress', content: [] } },
      { type: 'response.output_text.delta', output_index: 0, item_id: item.id, content_index: 0, delta: 'SYNTHETIC_SUMMARY' },
      { type: 'response.output_text.done', output_index: 0, item_id: item.id, content_index: 0, text: 'SYNTHETIC_SUMMARY' },
      { type: 'response.output_item.done', output_index: 0, item },
      { type: 'response.completed', response: { id: 'resp_compact', status: 'completed', model: body.model,
        reasoning: body.reasoning, output: [item], usage: { input_tokens: 5, output_tokens: 2, total_tokens: 7 } } }
    ];
  }
} });
try {
  for (const content of [compactPrompt, 'Continue normal work']) {
    const response = await fetch(`http://127.0.0.1:${gateway.port}/v1/messages`, {
      method: 'POST', signal: AbortSignal.timeout(5000), headers: { ...gateway.clientHeaders(),
        'anthropic-version': '2023-06-01', 'content-type': 'application/json' }, body: JSON.stringify(doc('luna', content)) });
    assert.equal(response.status, 200); assert.match(await response.text(), /message_stop/);
  }
  assert.deepEqual(sent, ['medium', 'max']);
  const status = await readRequestStatus({ ANTHROPIC_BASE_URL: `http://127.0.0.1:${gateway.port}`,
    ANTHROPIC_AUTH_TOKEN: gateway.clientHeaders().Authorization.slice(7), CLAUDE_CODE_MAX_CONTEXT_TOKENS: '500000',
    CLAUDE_CODE_AUTO_COMPACT_WINDOW: '100000', CLAUDE_AUTOCOMPACT_PCT_OVERRIDE: '83.33333333333334' });
  assert.equal(status.clientContextPolicy.window, 500000);
  assert.equal(status.clientContextPolicy.autoCompactWindow, 100000);
  assert.equal(status.recentRequests[0].purpose, 'compact-template');
  assert.equal(status.recentRequests[0].requestedEffort, 'max');
  assert.equal(status.recentRequests[0].effort, 'medium');
  assert.equal(status.recentRequests[1].purpose, 'conversation');
  assert.equal(status.recentRequests[1].effort, 'max');
} catch (error) {
  const state = gateway.diagnostics();
  console.error(JSON.stringify({ fixture: 'original-compact-baseline', sent: sent.length,
    requests: state.requests, transportRejections: state.lifetime.transportRejections,
    failures: state.recentRequests.map(row => ({ stage: row.failureStage, category: row.failureCategory })) }));
  throw error;
} finally { await gateway.close(); }
// F21: the opt-in verification route preserves compact effort; all existing
// default-model/role assertions above retain the production contract.
for (const selected of [{ model: 'gpt-5.6-luna', effort: 'max' }, { model: 'gpt-5.6-sol', effort: 'low' }]) {
  for (const subagent of [false, true]) {
    const input = doc(selected.model, compactPrompt, selected.effort);
    const result = prepareNative(input, { subagent, route: subagent ? selected : undefined, verificationSelection: selected });
    assert.equal(result.purpose, 'compact-template');
    assert.deepEqual(result.selected, selected);
    assert.equal(result.body.reasoning.effort, selected.effort);
  }
  const observed = [];
  const locked = await startNativeGateway({ verificationSelection: selected,
    admissionOptions: { freeBytes: () => 16 * 1024 ** 3 }, transport: {
      diagnostics: () => ({ activeRequests: 0, activeSockets: 0 }), close: async () => {},
      send: async body => {
        observed.push({ model: body.model, effort: body.reasoning.effort });
        const item = { id: 'msg_route', type: 'message', role: 'assistant', status: 'completed',
          content: [{ type: 'output_text', text: 'PUBLIC_ROUTE_OK', annotations: [] }] };
        return [
          { type: 'response.created', response: { id: 'resp_route', status: 'in_progress' } },
          { type: 'response.output_item.added', output_index: 0, item: { ...item, status: 'in_progress', content: [] } },
          { type: 'response.output_text.delta', output_index: 0, item_id: item.id, content_index: 0, delta: 'PUBLIC_ROUTE_OK' },
          { type: 'response.output_text.done', output_index: 0, item_id: item.id, content_index: 0, text: 'PUBLIC_ROUTE_OK' },
          { type: 'response.output_item.done', output_index: 0, item },
          { type: 'response.completed', response: { id: 'resp_route', status: 'completed', model: body.model,
            reasoning: body.reasoning, output: [item], usage: { input_tokens: 5, output_tokens: 2, total_tokens: 7 } } }
        ];
      }
    } });
  try {
    for (const [model, effort, prompt, allowed] of [
      [selected.model, selected.effort, 'Public task', true],
      [selected.model, selected.effort, compactPrompt, true],
      [selected.model, 'medium', 'Public task', false],
      ['gpt-6-astra', selected.effort, 'Public task', false]
    ]) {
      const count = observed.length;
      const response = await fetch(`http://127.0.0.1:${locked.port}/v1/messages`, { method: 'POST',
        signal: AbortSignal.timeout(5000), headers: { ...locked.clientHeaders(),
          'anthropic-version': '2023-06-01', 'content-type': 'application/json' }, body: JSON.stringify(doc(model, prompt, effort)) })
        .catch(error => { throw new Error(JSON.stringify({ model, effort, allowed, observed: observed.length,
          failure: locked.diagnostics().recentRequests.at(-1)?.failureCategory, code: error.cause?.code })); });
      const text = await response.text();
      if (allowed) { assert.equal(response.status, 200); assert.match(text, /message_stop/); }
      assert.equal(observed.length, count + Number(allowed));
      if (!allowed) {
        const failure = locked.diagnostics().recentRequests.at(-1);
        assert.equal(failure.failureCategory, 'VERIFICATION_ROUTE_MISMATCH');
        assert.equal(failure.attempts.length, 0);
      }
    }
    assert.deepEqual(observed, [selected, selected]);
  } finally { await locked.close(); }
}
console.log(JSON.stringify({ suite: 'compact-policy', passed: true, externalRequests: 0, credentialReads: 0 }));
