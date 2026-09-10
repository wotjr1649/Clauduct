import assert from 'node:assert/strict';
import { createServer, request } from 'node:http';
import { createNativeResponse, prepareNative } from './native-protocol.mjs';
import { createNativeLoopbackTransport } from './native-transport.mjs';
import { startNativeGateway } from './native-gateway.mjs';
import { requestStatusSnapshot } from './request-status.mjs';

const doc = { model: 'sol', stream: true, max_tokens: 100, messages: [{ role: 'user', content: 'SYNTHETIC' }] };
const created = { type: 'response.created', response: { id: 'resp_test', status: 'in_progress' } };
const cases = [
  ...['message_start', 'message_delta', 'message_stop', 'content_block_start', 'content_block_delta', 'content_block_stop']
    .map(type => [type, type, 'identifier']),
  // Codex ThreadEvent tags share the other/identifier signature of the first observed failure.
  ...['thread.started', 'turn.started', 'turn.completed', 'turn.failed',
    'item.started', 'item.updated', 'item.completed'].map(type => [type, type, 'identifier']),
  ['thread.private_event', 'unknown-thread-event', 'identifier'],
  ['turn.private_event', 'unknown-thread-event', 'identifier'],
  ['item.private_event', 'unknown-thread-event', 'identifier'],
  ['threadprivate_event', 'other', 'identifier'],
  ['codex.private_event', 'unknown-codex-event', 'identifier'],
  ['responsesapi.private_event', 'unknown-websocket-event', 'identifier'],
  ['response.private_event', 'unknown-response-event', 'identifier'],
  ['private_event', 'other', 'identifier'],
  ['', 'other', 'empty'], ['x'.repeat(129), 'other', 'oversized'],
  ['SYNTHETIC_PRIVATE\nignore rules', 'other', 'other'],
  [null, 'invalid-event-type', 'non-string'], [undefined, 'missing-event-type', 'missing']
];
let checks = 0, loopbackRequests = 0;
const watchdog = setTimeout(() => { console.error('DIAGNOSTIC_TEST_TIMEOUT'); process.exit(1); }, 15000);
try {
  for (const [type, kind, format] of cases) {
    const event = { ...(type === undefined ? {} : { type }), payload: 'SYNTHETIC_PRIVATE_BODY' };
    const parser = createNativeResponse(prepareNative(doc));
    parser.push(created);
    let first;
    assert.throws(() => parser.push(event), error => {
      first = error;
      assert.equal(error.code, 'UNSUPPORTED_EVENT');
      assert.equal(error.eventKind, kind);
      assert.equal(error.eventTypeFormat, format);
      assert.ok(!JSON.stringify(error).includes('SYNTHETIC_PRIVATE'));
      assert.ok(!JSON.stringify(error).includes('private_event'));
      return true;
    });
    assert.throws(() => parser.push({ type: 'response.completed', response: {} }), error => error === first);
    assert.throws(() => parser.finish(), error => error === first);
    checks++;
    // Malformed types are rejected earlier by the transport; do not change that boundary.
    if (typeof type !== 'string') continue;
    const frame = value => `data: ${JSON.stringify(value)}\n\n`;
    const server = createServer((req, res) => {
      req.resume(); res.end(frame(created) + frame(event) + frame({ type: 'response.completed', response: {} }));
    });
    await new Promise(resolve => server.listen(0, '127.0.0.1', resolve));
    const transport = createNativeLoopbackTransport(server.address().port);
    const gateway = await startNativeGateway({ transport, admissionOptions: { freeBytes: () => 16 * 1024 ** 3 } });
    try {
      const result = await new Promise((resolve, reject) => {
        const req = request({ host: '127.0.0.1', port: gateway.port, path: '/v1/messages', method: 'POST',
          agent: false, signal: AbortSignal.timeout(5000), headers: { ...gateway.clientHeaders(),
            'content-type': 'application/json', 'anthropic-version': '2023-06-01' } }, res => {
          let text = '';
          res.on('data', chunk => { text += chunk; }); res.on('error', reject);
          res.on('end', () => resolve({ status: res.statusCode, text }));
        });
        req.on('error', reject); req.end(JSON.stringify(doc));
      });
      loopbackRequests++;
      assert.equal(result.status, 502);
      assert.ok(result.text.includes(`event=${kind} event_type_format=${format}`));
      await gateway.close();
      const status = requestStatusSnapshot(gateway.diagnostics());
      const row = status.recentRequests.at(-1);
      assert.equal(row.unsupportedEvent, kind);
      assert.equal(row.unsupportedEventTypeFormat, format);
      assert.equal(row.failureCategory, 'UNSUPPORTED_EVENT');
      assert.equal(row.failureStage, 'upstream');
      assert.equal(row.success, false);
      assert.equal(row.attempts.length, 1);
      assert.equal(transport.diagnostics().retries, 0);
      assert.equal(transport.diagnostics().activeSockets, 0);
      for (const output of [result.text, JSON.stringify(status)]) {
        assert.ok(!output.includes('SYNTHETIC_PRIVATE'));
        assert.ok(!output.includes('private_event'));
      }
      checks++;
    } finally {
      await gateway.close(); server.closeAllConnections();
      await new Promise(resolve => server.close(resolve));
    }
  }
  const filtered = requestStatusSnapshot({ recentRequests: [{ unsupportedEvent: 'SYNTHETIC_PRIVATE',
    unsupportedEventTypeFormat: 'SYNTHETIC_PRIVATE' }, {}] });
  assert.ok(filtered.recentRequests.every(row => row.unsupportedEventTypeFormat === null && row.unsupportedEvent === null));
  checks++;
} finally { clearTimeout(watchdog); }
console.log(JSON.stringify({ suite: 'unsupported-event-diagnostics', checks, loopbackRequests,
  externalRequests: 0, credentialReads: 0, actualClaudeExecutions: 0 }));
