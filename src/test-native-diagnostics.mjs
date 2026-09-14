import assert from 'node:assert/strict';
import { createNativeResponse, prepareNative } from './native-protocol.mjs';
import { requestStatusSnapshot } from './request-status.mjs';
import { createNativeOutputCapture, nativeOutputDiagnostics } from '../verification/native-output.mjs';
import { request } from 'node:http';
import { startNativeGateway } from './native-gateway.mjs';

let checks = 0;
const prepared = prepareNative({ model: 'luna', stream: true, max_tokens: 100,
  messages: [{ role: 'user', content: 'PUBLIC_DIAGNOSTIC' }] });
for (const [event, shape] of [
  [{ type: 'keepalive' }, 'type-only'],
  [{ type: 'keepalive', sequence_number: 3 }, 'type-sequence'],
  [{ type: 'keepalive', sequence_number: 'SYNTHETIC_PRIVATE' }, 'type-sequence'],
  [{ type: 'keepalive', payload: 'SYNTHETIC_PRIVATE' }, 'other'],
  [{ type: 'keepalive', response: { output: 'SYNTHETIC_PRIVATE' } }, 'other']
]) {
  const parser = createNativeResponse(prepared);
  parser.push({ type: 'response.created', response: { id: 'public_response' } });
  if (shape === 'type-only') {
    assert.deepEqual(parser.push(event), []);
    assert.throws(() => parser.finish(), error => error.code === 'INCOMPLETE_RESPONSE');
  } else {
    assert.throws(() => parser.push(event), error => error.code === 'UNSUPPORTED_EVENT'
      && error.eventKind === 'keepalive' && error.keepaliveShape === shape && !JSON.stringify(error).includes('SYNTHETIC_PRIVATE'));
    assert.throws(() => parser.finish(), error => error.code === 'UNSUPPORTED_EVENT');
  }
  checks++;
}
const projected = requestStatusSnapshot({ recentRequests: [{ unsupportedEvent: 'keepalive', keepaliveShape: 'type-only' },
  { unsupportedEvent: 'SYNTHETIC_PRIVATE', keepaliveShape: 'SYNTHETIC_PRIVATE' }] });
assert.equal(projected.recentRequests[0].keepaliveShape, 'type-only');
assert.equal(projected.recentRequests[1].keepaliveShape, null);
assert.ok(!JSON.stringify(projected).includes('SYNTHETIC_PRIVATE')); checks++;

const status = { requestOutcome: 'has-failures', lifetime: { started: 3, succeeded: 2, failed: 1 },
  failureHistory: { records: [{ failureCategory: 'UNSUPPORTED_EVENT', unsupportedEvent: 'keepalive', keepaliveShape: 'type-only',
    body: 'SYNTHETIC_PRIVATE', headers: 'SYNTHETIC_PRIVATE' }] } };
const capture = createNativeOutputCapture();
capture.push('stderr', Buffer.from('CLAUDUCT_REQUEST_STATUS ' + JSON.stringify(status) + '\n'));
capture.push('stdout', Buffer.from(JSON.stringify({ type: 'result', is_error: true,
  session_id: '00000000-0000-4000-8000-000000000001', result: 'SYNTHETIC_PRIVATE' }) + '\n'));
capture.end('stdout'); capture.end('stderr');
const diagnostics = nativeOutputDiagnostics(capture.snapshot());
assert.deepEqual(diagnostics, { requestOutcome: 'has-failures', started: 3, succeeded: 2, failed: 1,
  keepaliveEvents: null, failures: [{ category: 'UNSUPPORTED_EVENT', eventKind: 'keepalive', keepaliveShape: 'type-only' }], failure: 'UNSUPPORTED_EVENT' });
assert.ok(!JSON.stringify(diagnostics).includes('SYNTHETIC_PRIVATE')); checks++;
for (const snapshot of [null, {}, { status: 'SYNTHETIC_PRIVATE' }, { status: { requestOutcome: 'SYNTHETIC_PRIVATE',
  lifetime: { started: -1, succeeded: 1.5, failed: 'SYNTHETIC_PRIVATE' }, failureHistory: { records: [null, {
    failureCategory: 'SYNTHETIC_PRIVATE', unsupportedEvent: 'SYNTHETIC_PRIVATE', keepaliveShape: 'SYNTHETIC_PRIVATE' }] } } }]) {
  assert.ok(!JSON.stringify(nativeOutputDiagnostics(snapshot)).includes('SYNTHETIC_PRIVATE')); checks++;
}
assert.equal(nativeOutputDiagnostics({ nativeResult: { is_error: true } }).failure, 'NATIVE_RESULT_ERROR'); checks++;
assert.equal(nativeOutputDiagnostics({ status: { requestOutcome: 'has-failures' } }).failure, 'NATIVE_REQUEST_FAILED'); checks++;
assert.equal(nativeOutputDiagnostics({ status: { requestOutcome: 'all-succeeded' }, nativeResult: { is_error: false } }).failure, null); checks++;
assert.equal(nativeOutputDiagnostics({ status: { failureHistory: { records: Array(100).fill({ failureCategory: 'INVALID_USAGE' }) } } }).failures.length, 16); checks++;
let loopbackRequests = 0;
for (const [event, shape] of [[{ type: 'keepalive' }, 'type-only'], [{ type: 'keepalive', payload: 'SYNTHETIC_PRIVATE' }, 'other']]) {
  const gateway = await startNativeGateway({ admissionOptions: { freeBytes: () => 16 * 1024 ** 3 }, transport: {
    send: async () => [{ type: 'response.created', response: { id: 'public_response' } }, event],
    close: async () => {}, diagnostics: () => ({}) } });
  try {
    const reply = await new Promise((done, reject) => {
      const body = JSON.stringify({ model: 'luna', stream: true, max_tokens: 100,
        messages: [{ role: 'user', content: 'PUBLIC_DIAGNOSTIC' }] });
      const req = request({ host: '127.0.0.1', port: gateway.port, path: '/v1/messages', method: 'POST',
        headers: { ...gateway.clientHeaders(), 'Content-Type': 'application/json', 'Content-Length': Buffer.byteLength(body), 'anthropic-version': '2023-06-01' },
        signal: AbortSignal.timeout(3000) }, res => {
        let bytes = 0;
        res.on('data', chunk => { if ((bytes += chunk.length) > 65536) req.destroy(new Error('PUBLIC_RESPONSE_LIMIT')); });
        res.once('error', reject); res.once('end', () => done(res.statusCode));
      });
      req.once('error', reject); req.end(body); loopbackRequests++;
    });
    assert.equal(reply, 502);
    const projected = requestStatusSnapshot(gateway.diagnostics());
    const evidence = nativeOutputDiagnostics({ status: projected, nativeResult: { is_error: true } });
    const accepted = shape === 'type-only';
    assert.equal(evidence.failure, accepted ? 'INCOMPLETE_RESPONSE' : 'UNSUPPORTED_EVENT');
    assert.equal(evidence.failures[0].eventKind, accepted ? null : 'keepalive');
    assert.equal(evidence.failures[0].keepaliveShape, accepted ? null : shape);
    assert.equal(evidence.keepaliveEvents, accepted ? 1 : 0);
    assert.ok(!JSON.stringify(evidence).includes('SYNTHETIC_PRIVATE')); checks++;
  } finally { await gateway.close(); }
}
console.log(JSON.stringify({ suite: 'native-diagnostics', checks, loopbackRequests,
  emptyKeepaliveAccepted: true, completionStillRequired: true, externalRequests: 0, rawPayloadRecorded: false }));
