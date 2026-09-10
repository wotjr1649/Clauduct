import assert from 'node:assert/strict';
import { request } from 'node:http';
import { prepareNative } from './native-protocol.mjs';
import { startNativeGateway } from './native-gateway.mjs';
import { readRequestStatus } from './request-status.mjs';

const base = () => ({ model: 'sol', stream: true, max_tokens: 100,
  messages: [{ role: 'user', content: 'synthetic' }] });
const cases = [
  ['REQUEST_FIELDS', d => { d.SYNTHETIC_PRIVATE_FIELD = 'SYNTHETIC_PRIVATE_VALUE'; }],
  ['REQUEST_SHAPE', d => { d.stream = false; }],
  ['OUTPUT_CONFIG_FIELDS', d => { d.output_config = { private: true }; }],
  ['CONTEXT_FIELDS', d => { d.context_management = { private: true }; }],
  ['THINKING_FIELDS', d => { d.thinking = { type: 'adaptive', private: true }; }],
  ['THINKING_TYPE', d => { d.thinking = { type: 'unknown' }; }],
  ['THINKING_BUDGET', d => { d.thinking = { type: 'enabled', budget_tokens: -1 }; }],
  ['MESSAGE_FIELDS', d => { d.messages[0].private = true; }],
  ['MESSAGE_EFFORT_ROLE', d => { d.messages[0].output_config = { effort: 'high' }; }],
  ['TEXT_SHAPE', d => { d.system = {}; }],
  ['TEXT_VALUE', d => { d.messages[0].content = [{ type: 'text', text: 7 }]; }],
  ['TEXT_FIELDS', d => { d.messages[0].content = [{ type: 'text', text: 'x', private: true }]; }],
  ['CACHE_FIELDS', d => { d.messages[0].content = [{ type: 'text', text: 'x', cache_control: { private: true } }]; }],
  ['CACHE_VALUE', d => { d.messages[0].content = [{ type: 'text', text: 'x', cache_control: { type: 'unknown' } }]; }],
  ['TOOLS_SHAPE', d => { d.tools = {}; }],
  ['TOOL_FIELDS', d => { d.tools = [{ name: 'Read', input_schema: { type: 'object' }, private: true }]; }],
  ['TOOL_CHOICE_FIELDS', d => { d.tool_choice = { type: 'auto', private: true }; }],
  ['TOOL_RESULT_FIELDS', d => { d.messages[0].content = [{ type: 'tool_result', private: true }]; }],
  ['TOOL_USE_FIELDS', d => { d.messages[0] = { role: 'assistant', content: [{ type: 'tool_use', private: true }] }; }],
  ['IMAGE_FIELDS', d => { d.messages[0].content = [{ type: 'image', private: true }]; }],
  ['IMAGE_SOURCE_FIELDS', d => { d.messages[0].content = [{ type: 'image', source: { private: true } }]; }],
  ['REDACTED_FIELDS', d => { d.messages[0].content = [{ type: 'redacted_thinking', private: true }]; }],
];
for (const [expected, mutate] of cases) {
  const doc = base(); mutate(doc);
  assert.throws(() => prepareNative(doc), error => {
    assert.equal(error.code, 'UNSUPPORTED_REQUEST');
    assert.equal(error.requestFailure, expected);
    assert.ok(!JSON.stringify(error).includes('SYNTHETIC_PRIVATE'));
    return true;
  });
}

// RED is valid tool output, not itself an unsupported request.
const red = base();
red.tools = [{ name: 'Bash', input_schema: { type: 'object' } }];
red.messages = [
  { role: 'assistant', content: [{ type: 'tool_use', id: 'call_1', name: 'Bash', input: {} }] },
  { role: 'user', content: [{ type: 'tool_result', tool_use_id: 'call_1', is_error: true, content: '5 tests failed' }] },
];
assert.ok(prepareNative(red).body.input.some(item => item.type === 'function_call_output'));

let sends = 0;
const gateway = await startNativeGateway({ admissionOptions: { freeBytes: () => 16 * 1024 ** 3 },
  transport: { send: async () => { sends++; throw Error('must not send'); }, close: async () => {}, diagnostics: () => ({}) } });
try {
  const body = base(); body.SYNTHETIC_PRIVATE_FIELD = 'SYNTHETIC_PRIVATE_VALUE';
  const response = await new Promise((resolve, reject) => {
    const req = request({ hostname: '127.0.0.1', port: gateway.port, path: '/v1/messages', method: 'POST',
      signal: AbortSignal.timeout(5000), agent: false,
      headers: { ...gateway.clientHeaders(), 'content-type': 'application/json', 'anthropic-version': '2023-06-01' } }, res => {
      let text = '';
      res.on('data', chunk => { text += chunk; }); res.on('error', reject);
      res.on('end', () => resolve({ status: res.statusCode, text }));
    }); req.on('error', reject); req.end(JSON.stringify(body));
  });
  assert.equal(response.status, 400);
  assert.match(response.text, /UNSUPPORTED_REQUEST request=REQUEST_FIELDS/);
  assert.ok(!response.text.includes('SYNTHETIC_PRIVATE'));
  const status = await readRequestStatus({ ANTHROPIC_BASE_URL: `http://127.0.0.1:${gateway.port}`,
    ANTHROPIC_AUTH_TOKEN: gateway.clientHeaders().Authorization.slice(7) });
  assert.equal(status.recentRequests.at(-1).requestFailure, 'REQUEST_FIELDS');
  assert.equal(status.recentRequests.at(-1).failureStage, 'prepare');
  assert.equal(status.recentRequests.at(-1).attempts.length, 0);
  assert.equal(sends, 0);
  assert.ok(!JSON.stringify(status).includes('SYNTHETIC_PRIVATE'));
} finally { await gateway.close(); }
console.log(JSON.stringify({ passed: cases.length + 2 }));
