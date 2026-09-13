import assert from 'node:assert/strict';
import { test } from 'node:test';
import { createServer } from 'node:http';
import { createNativeLoopbackTransport } from './native-transport.mjs';

const cases = [
  ['literal', '{"type":"keepalive"}', true],
  ['whitespace', ' { "type" : "keepalive" } ', true],
  ['escaped-key', '{"\\u0074ype":"keepalive"}', true],
  ['escaped-value', '{"type":"\\u006beepalive"}', true],
  ['duplicate-same', '{"type":"keepalive","type":"keepalive"}', false],
  ['duplicate-conflicting', '{"type":"response.failed","type":"keepalive"}', false],
  ['duplicate-escaped', '{"type":"response.failed","\\u0074ype":"keepalive"}', false],
  ['large-duplicate-value', '{"type":"' + 'x'.repeat(65536) + '","type":"keepalive"}', false]
];
for (const [name, raw, accepted] of cases) test(`keepalive transport ${name}`, async () => {
  let requests = 0;
  const server = createServer((_req, res) => {
    requests++;
    res.writeHead(200, { 'Content-Type': 'text/event-stream' });
    res.end('data: {"type":"response.created","response":{"id":"public_response"}}\n\n'
      + 'data: ' + raw + '\n\n'
      + 'data: {"type":"response.completed","response":{"id":"public_response"}}\n\n'
      + 'data: [DONE]\n\n');
  });
  await new Promise(done => server.listen(0, '127.0.0.1', done));
  const transport = createNativeLoopbackTransport(server.address().port);
  let heartbeats = 0, completions = 0;
  try {
    const pending = transport.send({}, AbortSignal.timeout(3000), { onEvent: event => {
      if (event.type === 'keepalive') heartbeats++;
      if (event.type === 'response.completed') completions++;
    } });
    if (accepted) await pending;
    else await assert.rejects(pending, error => error.code === 'INVALID_SSE');
    assert.equal(requests, 1);
    assert.equal(heartbeats, accepted ? 1 : 0);
    assert.equal(completions, accepted ? 1 : 0);
  } finally {
    await transport.close(); server.closeAllConnections(); await new Promise(done => server.close(done));
  }
});
