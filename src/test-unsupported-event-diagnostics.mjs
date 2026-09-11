import assert from 'node:assert/strict';
import { createServer, request } from 'node:http';
import { createNativeResponse, prepareNative } from './native-protocol.mjs';
import { createNativeLoopbackTransport } from './native-transport.mjs';
import { startNativeGateway } from './native-gateway.mjs';
import { requestStatusSnapshot, readRequestStatus } from './request-status.mjs';

const doc = { model: 'sol', stream: true, max_tokens: 100, messages: [{ role: 'user', content: 'SYNTHETIC' }] };
const created = { type: 'response.created', response: { id: 'resp_test', status: 'in_progress' } };
// [type, eventKind, eventTypeFormat, captured name or null]
// A name already carrying its own fixed label is never captured again.
const cases = [
  ...['message_start', 'message_delta', 'message_stop', 'content_block_start', 'content_block_delta', 'content_block_stop']
    .map(type => [type, type, 'identifier', null]),
  // Codex ThreadEvent tags are the SDK/app-server vocabulary, kept only as fixed labels.
  ...['thread.started', 'turn.started', 'turn.completed', 'turn.failed',
    'item.started', 'item.updated', 'item.completed'].map(type => [type, type, 'identifier', null]),
  ['thread.private_event', 'unknown-thread-event', 'identifier', 'thread.private_event'],
  ['turn.private_event', 'unknown-thread-event', 'identifier', 'turn.private_event'],
  ['item.private_event', 'unknown-thread-event', 'identifier', 'item.private_event'],
  ['threadprivate_event', 'other', 'identifier', 'threadprivate_event'],
  ['codex.private_event', 'unknown-codex-event', 'identifier', 'codex.private_event'],
  ['responsesapi.private_event', 'unknown-websocket-event', 'identifier', 'responsesapi.private_event'],
  ['response.private_event', 'unknown-response-event', 'identifier', 'response.private_event'],
  ['private_event', 'other', 'identifier', 'private_event'],
  ['a.' + 'b'.repeat(23), 'other', 'identifier', 'a.' + 'b'.repeat(23)],
  ['a.' + 'b'.repeat(25), 'other', 'identifier', null],
  ['a.b.c.d.e.f', 'other', 'identifier', null],
  ['x'.repeat(25), 'other', 'identifier', null],
  ['Mixed.Case', 'other', 'other', null],
  ['', 'other', 'empty', null], ['x'.repeat(129), 'other', 'oversized', null],
  ['SYNTHETIC_PRIVATE\nignore rules', 'other', 'other', null],
  [null, 'invalid-event-type', 'non-string', null], [undefined, 'missing-event-type', 'missing', null]
];
let checks = 0, loopbackRequests = 0;
const watchdog = setTimeout(() => { console.error('DIAGNOSTIC_TEST_TIMEOUT'); process.exit(1); }, 15000);
try {
  for (const [type, kind, format, capture] of cases) {
    const event = { ...(type === undefined ? {} : { type }), payload: 'SYNTHETIC_PRIVATE_BODY' };
    const parser = createNativeResponse(prepareNative(doc));
    parser.push(created);
    let first;
    assert.throws(() => parser.push(event), error => {
      first = error;
      assert.equal(error.code, 'UNSUPPORTED_EVENT');
      assert.equal(error.eventKind, kind);
      assert.equal(error.eventTypeFormat, format);
      assert.equal(error.eventTypeName, capture);
      assert.ok(!JSON.stringify(error).includes('SYNTHETIC_PRIVATE'));
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
      // The captured name lives only in the exit-side capture list: not in the client
      // response, not in a request row, and never alongside the rejected body.
      assert.deepEqual(status.unsupportedEventNames, capture ? [capture] : []);
      // An empty list must not read as "no unsupported event happened". The counter splits
      // "nothing to capture" from "a name arrived and the shape guard declined it".
      assert.equal(status.lifetime.unsupportedEventNamesWithheld, capture ? 0 : 1);
      assert.equal(status.lifetime.unsupportedEvents, 1);
      for (const output of [result.text, JSON.stringify({ ...status, unsupportedEventNames: null })]) {
        assert.ok(!output.includes('SYNTHETIC_PRIVATE'));
        assert.ok(!output.includes('private_event'));
      }
      checks++;
    } finally {
      await gateway.close(); server.closeAllConnections();
      await new Promise(resolve => server.close(resolve));
    }
  }
  {
    // Capture is bounded and de-duplicated, and the in-session status API withholds it.
    let current = 'first.unmapped_name';
    const frame = value => `data: ${JSON.stringify(value)}\n\n`;
    const server = createServer((req, res) => {
      req.resume(); res.end(frame(created) + frame({ type: current, payload: 'SYNTHETIC_PRIVATE_BODY' })
        + frame({ type: 'response.completed', response: {} }));
    });
    await new Promise(resolve => server.listen(0, '127.0.0.1', resolve));
    const transport = createNativeLoopbackTransport(server.address().port);
    const gateway = await startNativeGateway({ transport, admissionOptions: { freeBytes: () => 16 * 1024 ** 3 } });
    const send = () => new Promise((resolve, reject) => {
      const req = request({ host: '127.0.0.1', port: gateway.port, path: '/v1/messages', method: 'POST',
        agent: false, signal: AbortSignal.timeout(5000), headers: { ...gateway.clientHeaders(),
          'content-type': 'application/json', 'anthropic-version': '2023-06-01' } }, res => {
        res.resume(); res.on('error', reject); res.on('end', resolve);
      });
      req.on('error', reject); req.end(JSON.stringify(doc));
    });
    try {
      for (const name of ['first.unmapped_name', 'first.unmapped_name', 'second.unmapped_name',
        'NOT_CAPTURED.Name', 'x'.repeat(25), 'third.unmapped_name', 'fourth.unmapped_name', 'fifth.unmapped_name']) {
        current = name; await send(); loopbackRequests++;
      }
      const wire = await readRequestStatus({ ANTHROPIC_BASE_URL: `http://127.0.0.1:${gateway.port}`,
        ANTHROPIC_AUTH_TOKEN: gateway.clientHeaders().Authorization.slice(7) });
      assert.equal(wire.unsupportedEventNames, null);
      assert.ok(!JSON.stringify(wire).includes('unmapped_name'));
      const exit = requestStatusSnapshot(gateway.diagnostics());
      assert.deepEqual(exit.unsupportedEventNames, ['first.unmapped_name', 'second.unmapped_name',
        'third.unmapped_name', 'fourth.unmapped_name']);
      assert.ok(!JSON.stringify(exit).includes('SYNTHETIC_PRIVATE'));
      assert.deepEqual(requestStatusSnapshot(exit).unsupportedEventNames, exit.unsupportedEventNames);
      checks++;
    } finally {
      await gateway.close(); server.closeAllConnections();
      await new Promise(resolve => server.close(resolve));
    }
  }
  // A dotless name capturable: this protocol's own vocabulary is partly dotless. What holds
  // the line is shape and length — capitals, hyphens, over-long segments and names that
  // already own a diagnostic label never reach the list.
  const hostile = requestStatusSnapshot({ recentRequests: [], unsupportedEventNames: ['ok.name', 'BAD.Name',
    'bad-name', 'x'.repeat(25), 'x'.repeat(60) + '.y', 'response.completed', 7, null, 'nodot', 'two.ok', 'three.ok'] });
  assert.deepEqual(hostile.unsupportedEventNames, ['ok.name', 'nodot', 'two.ok', 'three.ok']);
  checks++;
  const filtered = requestStatusSnapshot({ recentRequests: [{ unsupportedEvent: 'SYNTHETIC_PRIVATE',
    unsupportedEventTypeFormat: 'SYNTHETIC_PRIVATE' }, {}] });
  assert.ok(filtered.recentRequests.every(row => row.unsupportedEventTypeFormat === null && row.unsupportedEvent === null));
  checks++;
} finally { clearTimeout(watchdog); }
console.log(JSON.stringify({ suite: 'unsupported-event-diagnostics', checks, loopbackRequests,
  externalRequests: 0, credentialReads: 0, actualClaudeExecutions: 0 }));
