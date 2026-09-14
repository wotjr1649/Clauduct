import assert from 'node:assert/strict';
import { mkdtempSync, mkdirSync, readFileSync, appendFileSync, writeFileSync, statSync, rmSync } from 'node:fs';
import { spawn } from 'node:child_process';
import { createServer } from 'node:http';
import { join, resolve, sep } from 'node:path';
import { fileURLToPath } from 'node:url';
import { createVerificationLedger, readVerificationLedger } from '../verification/verification-ledger.mjs';
import { requestStatusSnapshot } from './request-status.mjs';
import { createNativeLoopbackTransport } from './native-transport.mjs';
import { guardFixtureTransport } from '../verification/fixture-tool-policy.mjs';

const codeRoot = fileURLToPath(new URL('../', import.meta.url));
const temporaryRoot = join(codeRoot, '.tmp');
const root = mkdtempSync(join(temporaryRoot, 'verification-ledger-'));
let checks = 0, workerStopped = true;
const directory = name => { const path = join(root, name); mkdirSync(path); return path; };
const invalid = action => assert.throws(action, error => error.code === 'VERIFICATION_LEDGER_INVALID');
try {
  const first = directory('normal'), ledger = createVerificationLedger(first);
  ledger.record('attempt', { requestAttempts: 1 });
  ledger.record('usage', { inputTokens: 40, outputTokens: 2, completions: 1 });
  ledger.record('final'); ledger.close();
  assert.deepEqual(readVerificationLedger(ledger.path), { requestAttempts: 1, inputTokens: 40, outputTokens: 2,
    completions: 1, imageFormatMask: 0, unobservedCompletions: 0, version: 2, recordCount: 4, finalRecorded: true, truncatedTail: false }); checks++;
  assert.throws(() => createVerificationLedger(first), error => error.code === 'EEXIST'); checks++;
  const exact = readFileSync(ledger.path, 'utf8');
  const legacy = exact.trim().split('\n').map(line => {
    const row = JSON.parse(line); row.version = 1; delete row.unobservedCompletions; return JSON.stringify(row);
  }).join('\n') + '\n';
  writeFileSync(ledger.path, legacy);
  const legacyRead = readVerificationLedger(ledger.path);
  assert.equal(legacyRead.version, 1); assert.equal(legacyRead.completions, 1);
  assert.equal(legacyRead.unobservedCompletions, 0); checks++;
  writeFileSync(ledger.path, exact.replace('"version":2', '"version":1'));
  invalid(() => readVerificationLedger(ledger.path)); checks++;
  writeFileSync(ledger.path, exact);
  const unknown = createVerificationLedger(directory('unobserved'));
  unknown.record('attempt', { requestAttempts: 1 });
  invalid(() => unknown.record('final', { unobservedCompletions: 1 })); checks++;
  invalid(() => unknown.record('usage-unobserved', { unobservedCompletions: 1, inputTokens: 1 })); checks++;
  invalid(() => unknown.record('usage-unobserved', { unobservedCompletions: 2 })); checks++;
  unknown.record('usage-unobserved', { unobservedCompletions: 1 });
  invalid(() => unknown.record('usage', { completions: 1 })); checks++;
  unknown.close();
  const unknownRead = readVerificationLedger(unknown.path);
  assert.equal(unknownRead.unobservedCompletions, 1); assert.equal(unknownRead.completions, 0);
  assert.equal(unknownRead.finalRecorded, false); checks++;
  const unknownText = readFileSync(unknown.path, 'utf8');
  writeFileSync(unknown.path, unknownText.replace('"kind":"usage-unobserved"', '"kind":"final"'));
  invalid(() => readVerificationLedger(unknown.path)); checks++;
  for (const failureCategory of ['ATTEMPT_OBSERVER_FAILED', 'VERIFICATION_LEDGER_INVALID', 'VERIFICATION_LEDGER_IO']) {
    assert.equal(requestStatusSnapshot({ recentRequests: [{ failureCategory }] }).recentRequests[0].failureCategory, failureCategory); checks++;
  }
  for (const change of [text => text.replace('"sequence":2', '"sequence":3'),
    text => text.replace('"inputTokens":0', '"inputTokens":null'),
    text => text.replace('"inputTokens":0', '"inputTokens":1'),
    text => text.replace('"kind":"attempt"', '"kind":"SYNTHETIC_PRIVATE"'),
    text => text + '{"unfinished":true}']) {
    writeFileSync(ledger.path, change(exact)); invalid(() => readVerificationLedger(ledger.path)); checks++;
  }
  writeFileSync(ledger.path, exact);
  const incomplete = createVerificationLedger(directory('incomplete'));
  incomplete.record('attempt', { requestAttempts: 1 }); incomplete.close();
  appendFileSync(incomplete.path, '{"requestAttempts":999');
  const partial = readVerificationLedger(incomplete.path);
  assert.equal(partial.requestAttempts, 1); assert.equal(partial.finalRecorded, false); assert.equal(partial.truncatedTail, true); checks++;
  const bounded = createVerificationLedger(directory('bounded'));
  let attempted = 0;
  try { for (let index = 1; index <= 4096; index++) { bounded.record('attempt', { requestAttempts: index }); attempted++; } }
  catch (error) { assert.equal(error.code, 'VERIFICATION_LEDGER_INVALID'); }
  bounded.close();
  assert.ok(attempted > 0 && attempted < 4096); assert.ok(statSync(bounded.path).size <= 262144);
  assert.equal(readVerificationLedger(bounded.path).requestAttempts, attempted); checks++;

  const pipeline = createVerificationLedger(directory('pipeline'));
  let guarded, received = 0, delivered = 0;
  const server = createServer((_req, res) => {
    received++;
    assert.equal(readVerificationLedger(pipeline.path).requestAttempts, received);
    const event = { type: 'response.completed', response: { output: [],
      ...(received === 1 ? { usage: { input_tokens: 41, output_tokens: 3 } } : {}) } };
    res.writeHead(200, { 'Content-Type': 'text/event-stream' });
    res.end(`event: response.completed\ndata: ${JSON.stringify(event)}\n\n`);
  });
  await new Promise(done => server.listen(0, '127.0.0.1', done));
  const transport = createNativeLoopbackTransport(server.address().port, {
    onAttempt: value => pipeline.record('attempt', { ...guarded.fixtureUsage(), ...value }) });
  guarded = guardFixtureTransport(transport, { version: 1, kind: 'none', workingRoot: root },
    { onUsage: value => pipeline.record('usage', value), onUsageUnobserved: value => pipeline.record('usage-unobserved', value) });
  try {
    await guarded.send({}, AbortSignal.timeout(3000), { onEvent: () => {
      const record = readVerificationLedger(pipeline.path);
      assert.equal(record.completions, 1); assert.equal(record.inputTokens, 41); assert.equal(record.outputTokens, 3);
      delivered++;
    } });
    await assert.rejects(guarded.send({}, AbortSignal.timeout(3000), { onEvent: () => { delivered++; } }), error => error.code === 'INVALID_USAGE');
    assert.equal(readVerificationLedger(pipeline.path).unobservedCompletions, 1); checks++;
    await assert.rejects(guarded.send({}, AbortSignal.timeout(3000)), error => error.code === 'INVALID_USAGE');
    pipeline.record('final', guarded.fixtureUsage());
    assert.equal(readVerificationLedger(pipeline.path).finalRecorded, true);
    assert.equal(received, 2); assert.equal(delivered, 1); checks++;
  } finally {
    pipeline.close(); await guarded.close(); server.closeAllConnections(); await new Promise(done => server.close(done));
  }

  const crashRoot = directory('crash');
  const worker = join(codeRoot, 'verification/fixtures/ledger-worker.mjs');
  const env = Object.fromEntries(['SystemRoot', 'WINDIR', 'TEMP', 'TMP'].filter(key => process.env[key]).map(key => [key, process.env[key]]));
  const readPaths = [join(codeRoot, 'src'), join(codeRoot, 'poc'), worker, join(codeRoot, 'verification/verification-ledger.mjs'), crashRoot];
  const child = spawn(process.execPath, ['--permission', ...readPaths.map(path => `--allow-fs-read=${path}`), `--allow-fs-write=${crashRoot}`, worker, crashRoot],
    { env, windowsHide: true, stdio: ['ignore', 'pipe', 'pipe'] });
  workerStopped = false;
  let stopped = false, stopRequested = false, ready = '', errorBytes = 0, timer;
  const closed = new Promise(done => child.once('close', () => { stopped = true; workerStopped = true; done(); }));
  child.stderr.on('data', chunk => { errorBytes += chunk.length; });
  const stop = () => { if (!stopped && !stopRequested) { stopRequested = true; child.kill(); } };
  try {
    await new Promise((done, reject) => {
      timer = setTimeout(() => reject(new Error('LEDGER_WORKER_READY_TIMEOUT')), 3000);
      child.once('error', reject);
      child.stdout.on('data', chunk => {
        ready += chunk.toString();
        if (ready.length > 1024) reject(new Error('LEDGER_WORKER_OUTPUT_LIMIT'));
        else if (ready === 'LEDGER_READY\n') done();
      });
    });
    clearTimeout(timer); stop();
    await Promise.race([closed, new Promise((_, reject) => { timer = setTimeout(() => reject(new Error('LEDGER_WORKER_STOP_TIMEOUT')), 3000); })]);
    clearTimeout(timer);
    assert.equal(errorBytes, 0); assert.equal(stopped, true);
    const recovered = readVerificationLedger(join(crashRoot, 'tool-usage.jsonl'));
    assert.equal(recovered.requestAttempts, 2); assert.equal(recovered.completions, 1);
    assert.equal(recovered.inputTokens, 40); assert.equal(recovered.outputTokens, 2);
    assert.equal(recovered.finalRecorded, false); assert.equal(recovered.truncatedTail, false); checks++;
  } finally {
    clearTimeout(timer); stop();
    if (!stopped) await Promise.race([closed, new Promise((_, reject) => { timer = setTimeout(() => reject(new Error('LEDGER_WORKER_STOP_UNCONFIRMED')), 3000); })]);
    clearTimeout(timer);
  }
  console.log(JSON.stringify({ suite: 'verification-ledger', checks, crashedWorkerStopped: stopped,
    externalRequests: 0, actualCredentialReads: 0, powerLossDurability: 'NOT_RUN' }));
} finally {
  if (!workerStopped) throw new Error('LEDGER_WORKER_STOP_UNCONFIRMED');
  assert.ok(resolve(root).startsWith(resolve(temporaryRoot) + sep));
  assert.ok(root.startsWith(join(temporaryRoot, 'verification-ledger-')));
  rmSync(root, { recursive: true });
}
