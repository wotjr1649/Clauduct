import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { once } from 'node:events';
import { mkdtempSync, readFileSync, writeFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { createNativeOutputCapture, nativeOutputCompleted } from '../verification/native-output.mjs';

const self = fileURLToPath(import.meta.url), project = dirname(dirname(self));
const sessionId = '10000000-0000-4000-8000-000000000001', marker = 'PUBLIC_OUTPUT_COMPLETE';
const cleanup = Object.fromEntries(['childClosed', 'gatewaySocketsClosed', 'gatewayJobsClosed', 'gatewayTimersClosed',
  'gatewayDeliveriesClosed', 'gatewayIdle', 'gatewayCleanupCompleted', 'transportSocketsClosed', 'transportRequestsClosed'].map(key => [key, true]));
const final = result => ({ type: 'result', is_error: false, session_id: sessionId, result });
const status = { requestOutcome: 'all-succeeded', cleanup };
const statusLine = 'CLAUDUCT_REQUEST_STATUS ' + JSON.stringify(status) + '\n';
const encode = value => Buffer.from(JSON.stringify(value) + '\n');
const largeMarker = marker + 'A'.repeat(524288);

if (process.argv[2] === '--producer') {
  const [kind, root] = process.argv.slice(3);
  if (!['slow', 'flood', 'large', 'oversize', 'cut', 'truncated'].includes(kind)
    || !root?.startsWith(join(project, '.tmp', 'native-output-'))) throw new Error('PUBLIC_PRODUCER_ARGUMENTS');
  let backpressure = 0, writeErrors = 0, maxWriteMs = 0;
  process.stdout.on('error', () => { writeErrors++; });
  const write = data => new Promise((done, reject) => {
    const started = performance.now();
    if (!process.stdout.write(data, error => {
      maxWriteMs = Math.max(maxWriteMs, performance.now() - started);
      if (error) reject(new Error('PUBLIC_PIPE_CLOSED')); else done();
    })) backpressure++;
  });
  writeFileSync(join(root, 'effect.json'), JSON.stringify({ count: 1 }), { flag: 'wx' });
  const line = encode({ type: 'assistant', message: 'PUBLIC_TEXT_' + 'a'.repeat(kind === 'slow' ? 131072 : 8192) });
  try {
    if (kind === 'slow') process.send({ beforeSlowWrite: true });
    const count = kind === 'flood' ? 4096 : kind === 'slow' ? 128 : 4;
    for (let index = 0; index < count; index++) await write(line);
    writeFileSync(join(root, 'report.json'), JSON.stringify({ completed: true }), { flag: 'wx' });
    if (kind === 'cut') {
      process.send({ beforeResult: true });
      await once(process, 'message');
    }
    const output = encode(final(kind === 'large' ? largeMarker : kind === 'oversize' ? 'A'.repeat(2097152) : marker));
    await write(kind === 'truncated' ? output.subarray(0, output.length - 10) : output);
  } catch (error) {
    if (kind !== 'cut' || error.message !== 'PUBLIC_PIPE_CLOSED') process.exitCode = 1;
  } finally {
    process.stderr.write(statusLine);
    writeFileSync(join(root, 'producer.json'), JSON.stringify({ backpressure, writeErrors, maxWriteMs }), { flag: 'wx' });
    if (process.connected) process.disconnect();
  }
} else {
  let checks = 0, realChildren = 0, peakRssDelta = 0;
  const rows = [];
  function inspect(stdout, stderr = Buffer.from(statusLine), { exitCode = 0, oraclePassed = true, resultText = marker } = {}) {
    const capture = createNativeOutputCapture();
    for (let offset = 0; offset < stdout.length; offset += 7) capture.push('stdout', stdout.subarray(offset, offset + 7));
    capture.push('stderr', stderr); capture.end('stdout'); capture.end('stderr');
    const snapshot = capture.snapshot();
    return { snapshot, complete: nativeOutputCompleted(snapshot, { exitCode, sessionId, resultText, oraclePassed }) };
  }
  assert.equal(inspect(encode(final(marker))).complete, true); checks++;
  assert.equal(inspect(encode(final('한글🙂완료')), undefined, { resultText: '한글🙂완료' }).complete, true); checks++;
  for (const [stdout, stderr, options, failure] of [
    [Buffer.alloc(0), undefined, {}, null],
    [encode(final(marker)), Buffer.alloc(0), {}, null],
    [encode(final(marker)), undefined, { exitCode: 7 }, null],
    [encode(final(marker)), undefined, { oraclePassed: false }, null],
    [encode({ ...final(marker), is_error: true }), undefined, {}, null],
    [encode({ ...final(marker), session_id: '20000000-0000-4000-8000-000000000001' }), undefined, {}, null],
    [encode(final('different')), undefined, {}, null],
    [Buffer.concat([encode(final(marker)), encode(final(marker))]), undefined, {}, 'OUTPUT_DUPLICATE_RESULT'],
    [encode(final(marker)), Buffer.from(statusLine.repeat(2)), {}, 'OUTPUT_DUPLICATE_STATUS'],
    [Buffer.concat([encode(final(marker)), encode({ type: 'assistant' })]), undefined, {}, 'OUTPUT_AFTER_RESULT'],
    [Buffer.from('{\n'), undefined, {}, 'OUTPUT_INVALID_JSON'],
    [Buffer.from([0xc3, 0x28, 0x0a]), undefined, {}, 'OUTPUT_INVALID_UTF8'],
    [encode(final(marker)).subarray(0, -1), undefined, {}, 'OUTPUT_TRUNCATED_LINE'],
    [encode(final(marker)), Buffer.from(statusLine.slice(0, -1)), {}, 'OUTPUT_TRUNCATED_LINE'],
    [encode(final(marker)), Buffer.from('CLAUDUCT_REQUEST_STATUS {\n'), {}, 'OUTPUT_INVALID_JSON'],
    [encode({ ...final(marker), is_error: 0 }), undefined, {}, 'OUTPUT_INVALID_RESULT'],
    [encode(final(marker)), Buffer.from('CLAUDUCT_REQUEST_STATUS ' + JSON.stringify({ ...status, requestOutcome: 'has-failures' }) + '\n'), {}, null],
    [encode(final(marker)), Buffer.from('CLAUDUCT_REQUEST_STATUS ' + JSON.stringify({ ...status, cleanup: { ...cleanup, gatewayIdle: false } }) + '\n'), {}, null]
  ]) {
    const result = inspect(stdout, stderr, options);
    assert.equal(result.complete, false); assert.equal(result.snapshot.evidence.failure, failure); checks++;
  }
  for (const options of [{ maxBytes: 1023 }, { maxBytes: Infinity }, { maxLineBytes: 255 },
    { maxLineBytes: 2097153 }, { maxRecords: 0 }, { maxRecords: 1.5 }]) {
    assert.throws(() => createNativeOutputCapture(options), /INVALID_OUTPUT_CAPTURE/); checks++;
  }
  {
    const capture = createNativeOutputCapture({ maxBytes: 1024, maxLineBytes: 256, maxRecords: 2 });
    capture.push('stdout', Buffer.from('x'.repeat(1025)));
    assert.equal(capture.evidence().failure, 'OUTPUT_BYTE_LIMIT'); checks++;
  }
  {
    const capture = createNativeOutputCapture({ maxRecords: 1 });
    capture.push('stdout', Buffer.concat([encode({ type: 'assistant' }), encode(final(marker))]));
    assert.equal(capture.evidence().failure, 'OUTPUT_RECORD_LIMIT'); checks++;
  }
  for (const kind of ['slow', 'flood', 'large', 'oversize', 'cut', 'truncated']) {
    const root = mkdtempSync(join(project, '.tmp', 'native-output-'));
    const capture = createNativeOutputCapture({ maxBytes: 50331648, maxLineBytes: 1048576, maxRecords: 8192 });
    const env = Object.fromEntries(['SystemRoot', 'WINDIR', 'TEMP', 'TMP'].filter(name => process.env[name]).map(name => [name, process.env[name]]));
    const child = spawn(process.execPath, ['--permission', `--allow-fs-read=${project}`, `--allow-fs-write=${root}`,
      self, '--producer', kind, root], { cwd: project, env, windowsHide: true, stdio: ['ignore', 'pipe', 'pipe', 'ipc'] });
    realChildren++;
    const baselineRss = process.memoryUsage().rss;
    let peakRss = baselineRss, timedOut = false, pauses = 0, heldConsumer = false;
    const pending = new Set();
    const stopped = once(child, 'close');
    const watchdog = setTimeout(() => { timedOut = true; child.kill(); }, 10000);
    capture.watch(child.stdout, 'stdout'); capture.watch(child.stderr, 'stderr');
    if (kind === 'slow') child.stdout.pause();
    child.stdout.on('data', () => {
      peakRss = Math.max(peakRss, process.memoryUsage().rss);
      if (kind === 'slow') {
        child.stdout.pause(); pauses++;
        const timer = setTimeout(() => { pending.delete(timer); child.stdout.resume(); }, 5); pending.add(timer);
      }
    });
    child.on('message', message => {
      if (kind === 'slow') {
        assert.deepEqual(message, { beforeSlowWrite: true }); heldConsumer = true;
        const timer = setTimeout(() => { pending.delete(timer); child.stdout.resume(); }, 100); pending.add(timer);
        return;
      }
      assert.equal(kind, 'cut'); assert.deepEqual(message, { beforeResult: true });
      child.stdout.destroy(); child.send({ proceed: true });
    });
    const [exitCode] = await stopped;
    clearTimeout(watchdog); for (const timer of pending) clearTimeout(timer);
    const snapshot = capture.snapshot(), producer = JSON.parse(readFileSync(join(root, 'producer.json'), 'utf8'));
    const oraclePassed = readFileSync(join(root, 'effect.json'), 'utf8') === '{"count":1}'
      && readFileSync(join(root, 'report.json'), 'utf8') === '{"completed":true}';
    const complete = nativeOutputCompleted(snapshot, { exitCode, sessionId, resultText: kind === 'large' ? largeMarker : marker, oraclePassed });
    rows.push({ kind, root, exitCode, oraclePassed, complete, producer, pauses, heldConsumer, rssDelta: peakRss - baselineRss, ...snapshot.evidence });
    writeFileSync(join(root, 'result.json'), JSON.stringify(rows.at(-1)) + '\n', { flag: 'wx' });
    assert.equal(timedOut, false); assert.equal(exitCode, 0); assert.equal(oraclePassed, true);
    assert.equal(complete, ['slow', 'flood', 'large'].includes(kind));
    assert.ok(snapshot.evidence.peakBufferedBytes <= 2097152 && snapshot.evidence.retainedBytes <= 2097152);
    assert.ok(peakRss - baselineRss <= 134217728, 'OUTPUT_RSS_BOUND');
    // Windows stdout pipes write synchronously. A false write() return alone
    // cannot prove slow-consumer pressure there; measure the blocked write.
    if (kind === 'slow') { assert.ok(pauses > 1 && heldConsumer); assert.ok(producer.maxWriteMs >= 50); }
    if (kind === 'flood') assert.ok(snapshot.evidence.bytes > 33554432 && snapshot.evidence.retainedBytes < 4096);
    if (kind === 'oversize') assert.equal(snapshot.evidence.failure, 'OUTPUT_LINE_LIMIT');
    if (kind === 'cut') { assert.equal(snapshot.evidence.failure, 'OUTPUT_PIPE_CLOSED'); assert.equal(snapshot.evidence.resultCount, 0); }
    if (kind === 'truncated') assert.equal(snapshot.evidence.failure, 'OUTPUT_TRUNCATED_LINE');
    peakRssDelta = Math.max(peakRssDelta, peakRss - baselineRss);
    checks++;
  }
  console.log(JSON.stringify({ suite: 'native-output', checks, realChildren, peakRssDelta,
    externalRequests: 0, actualCredentialReads: 0, actualClaudeExecutions: 0, rows }));
}
