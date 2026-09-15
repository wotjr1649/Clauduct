// Synthetic HTTP on 127.0.0.1 only. Never invokes --live or reads the credential cache.
import assert from 'node:assert/strict';
import { createServer } from 'node:http';
import { spawn } from 'node:child_process';
import { createInterface } from 'node:readline';
import { once } from 'node:events';
import { fileURLToPath } from 'node:url';
import { existsSync, readdirSync } from 'node:fs';
import { join } from 'node:path';
import { buildBody, buildHeaders, model, effort, checkRuntime } from './manual-http-probe.mjs';

checkRuntime(process.env, process.execArgv);
const script = fileURLToPath(new URL('./manual-http-probe.ps1', import.meta.url));
const parser = fileURLToPath(new URL('./summarize-dotnet-response.mjs', import.meta.url));
const PACKAGE = /^Microsoft[.]PowerShell_7[.][0-9.]+_x64__8wekyb3d8bbwe$/;
// MSI keeps one stable path; MSIX renames its folder on every update, so the version is
// discovered rather than pinned here. Absolute candidates only: PATH is never searched.
function findPowerShell(programFiles = process.env.ProgramFiles) {
  if (typeof programFiles !== 'string' || !programFiles) throw new Error('POWERSHELL_7_NOT_FOUND');
  const msi = join(programFiles, 'PowerShell', '7', 'pwsh.exe');
  if (existsSync(msi)) return msi;
  const store = join(programFiles, 'WindowsApps');
  let packages = [];
  try { packages = readdirSync(store); } catch { packages = []; }
  const msix = packages.filter(name => PACKAGE.test(name)).sort().reverse()
    .map(name => join(store, name, 'pwsh.exe')).find(existsSync);
  if (!msix) throw new Error('POWERSHELL_7_NOT_FOUND');
  return msix;
}
const pwsh = findPowerShell();
const canary = 'SYNTHETIC_PRIVATE_CANARY';
const limit = 256 * 1024;
const event = value => `data: ${JSON.stringify(value)}\r\n\r\n`;
const body = Buffer.from([
  { type: 'response.output_text.delta', output_index: 1, content_index: 0, delta: 'OK' },
  { type: 'response.output_text.done', output_index: 1, content_index: 0, text: 'OK' },
  { type: 'response.completed', response: { model, status: 'completed', reasoning: { effort },
    output: [{ type: 'reasoning' }], usage: { input_tokens: 28, output_tokens: 5, total_tokens: 33 } } }
].map(event).join(''));
const requestBody = JSON.stringify(buildBody());
const headers = buildHeaders({ accessToken: 'synthetic', account: 'synthetic' }, '0.153.4', requestBody);
delete headers.Authorization; delete headers['chatgpt-account-id'];

// Hang guards, not timing checks: what the probe deadlines do is asserted from elapsedMs
// further down. These are sized for a cold CI runner rather than a warm developer box -- run
// 34907283086 killed the pwsh --self-test at the old 12s bound, a spawn that takes 0.75s here,
// because every spawn compiles DotnetHttpProbe.cs through Add-Type before it does anything.
// A run that genuinely hangs still ends, just later; neither value decides whether a case passes.
const SPAWN_GUARD_MS = 60000;
// One worker serves every case, and the last one deliberately spends up to 52s establishing the
// probe 45s deadline, so this has to cover that plus the other 34 cases and a cold start.
// It outlasts one case stalling to the full 120s normal budget (120 + 52 + the rest), not two.
// A second stall in the same run kills the worker instead, and the failure names that, not a case.
const WORKER_GUARD_MS = 240000;

function child(executable, args) {
  const process = spawn(executable, args, { windowsHide: true, stdio: ['pipe', 'pipe', 'pipe'] });
  let stderrBytes = 0;
  process.stderr.on('data', chunk => { if ((stderrBytes += chunk.length) > 4096) process.kill(); });
  const closed = once(process, 'close').then(([code]) => ({ code, stderrBytes }));
  // Reject a broken pipe without exposing arbitrary runtime error text.
  process.stdin.on('error', () => {});
  return { process, closed };
}
async function runOnce(executable, args, input = '') {
  const { process, closed } = child(executable, args);
  let stdout = '';
  process.stdout.on('data', chunk => { stdout += chunk; if (stdout.length > 16384) process.kill(); });
  const timer = setTimeout(() => process.kill(), SPAWN_GUARD_MS);
  process.stdin.end(input);
  try { return { ...(await closed), stdout }; }
  finally { clearTimeout(timer); }
}

let parserTests = 0;
for (const [input, expected] of [
  [JSON.stringify({ status: 200, contentType: 'text/event-stream', contentTypePresent: true, body: body.toString('base64') }), 'SUCCESS'],
  [JSON.stringify({ status: 200, contentType: '', contentTypePresent: false, body: body.toString('base64') }), 'MISSING_CONTENT_TYPE'],
  [JSON.stringify({ status: 200, contentType: '', contentTypePresent: true, body: body.toString('base64') }), 'MISSING_CONTENT_TYPE'],
  [JSON.stringify({ status: 401, contentType: canary, contentTypePresent: true, body: Buffer.from(canary).toString('base64'), [canary]: canary }), 'AUTH_REJECTED'],
  ['{', 'LOCAL_CHECK_FAILED'],
  [JSON.stringify({ status: 200, contentType: '', contentTypePresent: false, body: '!' }), 'LOCAL_CHECK_FAILED'],
  [JSON.stringify({ status: 200, contentType: '', contentTypePresent: false, body: Buffer.alloc(limit + 1).toString('base64') }), 'LOCAL_CHECK_FAILED'],
  [JSON.stringify({ status: 200, contentType: 'text/event-stream', contentTypePresent: false, body: '' }), 'LOCAL_CHECK_FAILED']
]) {
  const result = await runOnce(process.execPath, [parser], input);
  assert.equal(result.stderrBytes, 0);
  assert(!result.stdout.includes(canary));
  assert.equal(JSON.parse(result.stdout).category, expected);
  assert.equal(result.code, expected === 'LOCAL_CHECK_FAILED' ? 1 : 0);
  parserTests++;
}
const offline = await runOnce(pwsh, ['-NoLogo', '-NoProfile', '-File', script, '--self-test']);
assert.equal(offline.code, 0); assert.equal(offline.stderrBytes, 0);
const offlineResult = JSON.parse(offline.stdout);
assert.equal(offlineResult.offlineTests, offlineResult.passed);

const cases = [
  { name: 'sse-fixed', type: 'text/event-stream', passed: true },
  { name: 'sse-chunked', type: 'text/event-stream', chunked: true, passed: true },
  { name: 'mixed-case', type: 'Text/Event-Stream; charset=utf-8', passed: true },
  { name: 'missing-fixed', category: 'MISSING_CONTENT_TYPE' },
  { name: 'missing-chunked', chunked: true, category: 'MISSING_CONTENT_TYPE' },
  { name: 'empty', type: '', category: 'MISSING_CONTENT_TYPE' },
  { name: 'whitespace', type: '   ', category: 'MISSING_CONTENT_TYPE' },
  { name: 'wrong-type', type: 'application/json', category: 'UNEXPECTED_RESPONSE' },
  { name: 'wrong-type-suffix', type: 'text/event-stream-json', category: 'UNEXPECTED_RESPONSE' },
  { name: 'private-header', type: canary, category: 'UNEXPECTED_RESPONSE' },
  { name: 'private-error', status: 401, type: 'application/json', payload: Buffer.from(canary), category: 'AUTH_REJECTED' },
  { name: 'negotiate-challenge', status: 401, category: 'AUTH_REJECTED' },
  { name: 'proxy-challenge', status: 407, category: 'HTTP_ERROR' },
  { name: 'forbidden', status: 403, category: 'ACCESS_DENIED' },
  { name: 'server-error', status: 503, category: 'HTTP_ERROR' },
  ...[301, 302, 303, 307, 308].map(status => ({ name: `redirect-${status}`, status, category: 'REDIRECT_REFUSED' })),
  { name: 'no-421-retry', status: 421, category: 'HTTP_ERROR' },
  { name: 'no-429-retry', status: 429, category: 'RATE_LIMITED' },
  { name: 'limit-exact', type: 'text/event-stream', payload: Buffer.concat([body, Buffer.alloc(limit - body.length, 32)]), passed: true },
  { name: 'oversize-fixed', type: 'text/event-stream', payload: Buffer.alloc(limit + 1, 65), category: 'RESPONSE_TOO_LARGE', transportFailure: true },
  { name: 'oversize-chunked', type: 'text/event-stream', payload: Buffer.alloc(limit + 1, 65), chunked: true, category: 'RESPONSE_TOO_LARGE', transportFailure: true },
  { name: 'truncated-fixed', type: 'text/event-stream', category: 'RESPONSE_TRUNCATED', transportFailure: true },
  { name: 'truncated-chunked', type: 'text/event-stream', category: 'RESPONSE_TRUNCATED', transportFailure: true },
  { name: 'drop-after-request', category: 'RESPONSE_TRUNCATED', transportFailure: true },
  { name: 'invalid-utf8', type: 'text/event-stream', payload: Buffer.from([0xff]), category: 'INVALID_UTF8' },
  { name: 'invalid-json', type: 'text/event-stream', payload: Buffer.from('data: {\n\n'), category: 'INVALID_SSE_JSON' },
  { name: 'incomplete', type: 'text/event-stream', payload: Buffer.from(event({ type: 'response.incomplete' })), category: 'UPSTREAM_FAILED' },
  { name: 'cancel-after-headers', type: 'text/event-stream', mode: 'cancel-after-headers', hang: true, category: 'CANCELLED', transportFailure: true },
  { name: 'timeout-before-headers', mode: 'short-timeout', hang: true, category: 'TIMEOUT', transportFailure: true },
  { name: 'timeout-body', type: 'text/event-stream', mode: 'short-timeout', hang: true, category: 'TIMEOUT', transportFailure: true },
  { name: 'deadline-45s', type: 'text/event-stream', mode: 'deadline-45s', hang: true, category: 'TIMEOUT', transportFailure: true }
];
let activeCase, received = 0, invalidRequests = 0, connections = 0;
const sockets = new Set();
const server = createServer((req, res) => {
  received++;
  const testCase = activeCase;
  const chunks = [];
  let length = 0;
  req.on('error', () => { invalidRequests++; });
  req.on('data', chunk => { if ((length += chunk.length) > 4096) req.destroy(); else chunks.push(chunk); });
  req.on('end', () => {
    if (!testCase || req.method !== 'POST' || req.url !== '/probe' || req.httpVersion !== '1.1'
      || ['authorization', 'chatgpt-account-id', 'proxy-authorization', 'cookie', 'expect', 'traceparent'].some(name => req.headers[name] !== undefined)
      || req.headers.connection !== 'close' || Buffer.concat(chunks).toString('utf8') !== requestBody
      || Object.entries(headers).some(([key, value]) => req.headers[key.toLowerCase()] !== String(value))) {
      invalidRequests++; res.writeHead(400); res.end(); return;
    }
    if (testCase.name === 'drop-after-request') { req.socket.destroy(); return; }
    if (testCase.name === 'timeout-before-headers') return;
    res.statusCode = testCase.status ?? 200;
    if (testCase.type !== undefined) res.setHeader('cOnTeNt-TyPe', testCase.type);
    res.setHeader('Set-Cookie', `${canary}=1`);
    if (res.statusCode >= 300 && res.statusCode < 400) res.setHeader('Location', '/must-not-follow');
    if (testCase.status === 401) res.setHeader('WWW-Authenticate', 'Negotiate');
    if (testCase.status === 407) res.setHeader('Proxy-Authenticate', 'Negotiate');
    if ([421, 429, 503].includes(testCase.status)) res.setHeader('Retry-After', '0');
    const payload = testCase.payload ?? body;
    if (testCase.name === 'truncated-chunked') {
      // Valid headers and a truncated HTTP chunk; uses the same local accepted socket.
      req.socket.end('HTTP/1.1 200 OK\r\nContent-Type: text/event-stream\r\nTransfer-Encoding: chunked\r\nConnection: close\r\n\r\n10\r\nabc');
      return;
    }
    if (!testCase.chunked && !testCase.hang) res.setHeader('Content-Length', payload.length + (testCase.name === 'truncated-fixed' ? 10 : 0));
    res.setHeader('Connection', 'close');
    res.write(payload.subarray(0, 17));
    if (!testCase.hang) res.end(payload.subarray(17));
  });
});
server.on('connection', socket => {
  connections++; sockets.add(socket); socket.on('close', () => sockets.delete(socket));
});
server.maxConnections = 2;
// deadline-45s deliberately holds a connection for up to 52s, so these sat 8s above the case
// they had to outlast. They exist to stop a wedged socket lasting forever, nothing finer.
server.headersTimeout = 180000; server.requestTimeout = 180000;
server.setTimeout(180000, socket => socket.destroy());
await new Promise((resolve, reject) => { server.once('error', reject); server.listen(0, '127.0.0.1', resolve); });
const worker = child(pwsh, ['-NoLogo', '-NoProfile', '-File', script, '--loopback', String(server.address().port)]);
const lines = createInterface({ input: worker.process.stdout })[Symbol.asyncIterator]();
const deadline = setTimeout(() => { worker.process.kill(); server.closeAllConnections(); }, WORKER_GUARD_MS);
const observed = [];
try {
  for (const testCase of cases) {
    activeCase = testCase;
    const before = received, beforeConnections = connections;
    const started = performance.now();
    worker.process.stdin.write(`${testCase.mode ?? 'normal'}\n`);
    const line = await lines.next();
    assert.equal(line.done, false, testCase.name);
    assert(!line.value.includes(canary), testCase.name);
    const result = JSON.parse(line.value);
    const elapsedMs = Math.round(performance.now() - started);
    // The name alone does not say why a case failed. passed is asserted before category, so a
    // failure printed the case and stopped, and reading it took a trip through the probe to
    // learn whether the run timed out, was cancelled, or lost its parser. Both are in the
    // message now, and the elapsed time with them: a spawn that ran long says so on its own.
    const caseLabel = `${testCase.name} (category=${result.category}, ${elapsedMs}ms)`;
    assert.equal(result.passed, testCase.passed ?? false, caseLabel);
    assert.equal(result.category, testCase.category ?? 'SUCCESS', caseLabel);
    assert.equal(received, before + 1, caseLabel);
    assert.equal(connections, beforeConnections + 1, caseLabel);
    assert.equal(result.requestAttempts, 1, caseLabel);
    assert.equal(result.connectionAttempts, 1, caseLabel);
    assert.equal(result.reconnectBlocked, false, caseLabel);
    assert.equal(result.retries, 0); assert.equal(result.credentialWrites, 0);
    if (!testCase.transportFailure) {
      assert.equal(result.transportDiagnostics.contentTypePresent, testCase.type !== undefined, caseLabel);
      assert.equal(result.headerDiagnostics.normalizedContentTypeNonEmpty, Boolean(testCase.type?.trim()), caseLabel);
      assert.equal(result.headerDiagnostics.rawHeadersAvailable, false);
      assert.equal(result.headerDiagnostics.rawContentTypePresent, null);
      assert.equal(result.responseBytes, (testCase.payload ?? body).length, caseLabel);
    }
    if (testCase.passed || testCase.category === 'MISSING_CONTENT_TYPE') {
      assert.equal((result.sseDiagnostics ?? result).passed, true);
      assert.equal((result.sseDiagnostics ?? result).replySource, 'stream');
      assert.equal((result.sseDiagnostics ?? result).exactOK, true);
    }
    if (testCase.mode === 'deadline-45s') assert(elapsedMs >= 44000 && elapsedMs < 52000);
    if (testCase.mode === 'short-timeout') assert(elapsedMs >= 100 && elapsedMs < 3000);
    // Observe disposal before starting another case, with a bounded wait. Room for a stalled
    // runner costs nothing here: the loop ends the moment the socket closes, and one that never
    // closes still fails the assertion below however long the wait was.
    for (let i = 0; sockets.size && i < 1000; i++) await new Promise(resolve => setTimeout(resolve, 10));
    assert.equal(sockets.size, 0, testCase.name);
    observed.push({ name: testCase.name, category: result.category, ...(testCase.mode ? { elapsedMs } : {}) });
    if (testCase.mode === 'deadline-45s') console.log(JSON.stringify({ deadline45sVerified: true, elapsedMs }));
  }
  worker.process.stdin.end('STOP\n');
  const exit = await worker.closed;
  assert.equal(exit.code, 0); assert.equal(exit.stderrBytes, 0);
  assert.equal(invalidRequests, 0); assert.equal(received, cases.length); assert.equal(connections, cases.length);
  console.log(JSON.stringify({ offlineTests: offlineResult.passed, parserTests, loopbackTests: observed.length,
    localRequests: received, tcpConnections: connections, externalRequests: 0, credentialReads: 0,
    openSockets: sockets.size, cases: observed }));
} finally {
  if (worker.process.exitCode === null) worker.process.kill();
  await worker.closed;
  server.closeAllConnections();
  await new Promise(resolve => server.close(resolve));
  clearTimeout(deadline);
}
