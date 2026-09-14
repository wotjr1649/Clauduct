import assert from 'node:assert/strict';
import { createInterface } from 'node:readline';
import { appendFileSync } from 'node:fs';

function respond(message) {
  if (!Object.hasOwn(message, 'id')) return;
  if (typeof message.id !== 'number' && typeof message.id !== 'string') return;
  const reply = { jsonrpc: '2.0', id: message.id };
  if (message.method === 'initialize' && /^20\d\d-\d\d-\d\d$/.test(message.params?.protocolVersion)) {
    return { ...reply, result: { protocolVersion: message.params.protocolVersion, capabilities: { tools: {} },
      serverInfo: { name: 'clauduct-release-fixture', version: '1.0.0' } } };
  }
  if (message.method === 'ping') return { ...reply, result: {} };
  if (message.method === 'tools/list') return { ...reply, result: { tools: [{ name: 'add',
    description: 'Add two small public integers for a local release verification.',
    inputSchema: { type: 'object', properties: { a: { type: 'integer' }, b: { type: 'integer' } },
      required: ['a', 'b'], additionalProperties: false } }] } };
  const args = message.params?.arguments;
  if (message.method === 'tools/call' && message.params?.name === 'add' && args
    && Object.keys(args).sort().join(',') === 'a,b' && [args.a, args.b].every(n => Number.isInteger(n) && Math.abs(n) <= 1000)) {
    return { ...reply, result: { content: [{ type: 'text', text: String(args.a + args.b) }] } };
  }
  return { ...reply, error: { code: -32602, message: 'INVALID_FIXTURE_REQUEST' } };
}

if (process.argv[2] === '--self-test') {
  assert.equal(respond({ method: 'notifications/initialized' }), undefined);
  assert.equal(respond({ id: 1, method: 'tools/list' }).result.tools.length, 1);
  assert.equal(respond({ id: 2, method: 'tools/call', params: { name: 'add', arguments: { a: 2, b: 3 } } }).result.content[0].text, '5');
  assert.equal(respond({ id: 3, method: 'tools/call', params: { name: 'add', arguments: { a: '2', b: 3 } } }).error.code, -32602);
  console.log('Native MCP fixture: 4/4');
} else {
  const lines = createInterface({ input: process.stdin });
  for await (const line of lines) {
    if (line.length > 65536) { process.exitCode = 1; break; }
    let message;
    try { message = JSON.parse(line); } catch { process.exitCode = 1; break; }
    if (!message || typeof message !== 'object' || Array.isArray(message)) { process.exitCode = 1; break; }
    const reply = respond(message);
    if (reply?.result && message.method === 'tools/call' && process.argv[2]) {
      appendFileSync(process.argv[2], JSON.stringify({ a: message.params.arguments.a, b: message.params.arguments.b }) + '\n');
    }
    if (reply) process.stdout.write(JSON.stringify(reply) + '\n');
  }
}
