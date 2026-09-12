import assert from 'node:assert/strict';
import { createServer } from 'node:http';
import { mkdtempSync, mkdirSync, writeFileSync, readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { spawn } from 'node:child_process';
import { createHash } from 'node:crypto';

// Explicit wall-clock test: two fixed public jobs, no credentials or external requests.
assert.deepEqual(process.argv.slice(2), ['--live-clock']);
const project = dirname(dirname(fileURLToPath(import.meta.url)));
const worker = join(project, 'verification', 'fixtures', 'retry-recovery-worker.mjs');
const root = mkdtempSync(join(project, '.tmp', 'retry-after-clock-'));
const write = (name, value) => writeFileSync(join(root, name), JSON.stringify(value) + '\n', { flag: 'wx', flush: true });
const sourceFiles = ['src/native-transport.mjs', 'src/native-protocol.mjs', 'src/retry-after.mjs', 'src/retry-after-seconds.mjs',
  'src/client-version.mjs', 'src/models.mjs', 'poc/adapter.mjs', 'verification/manual-http-probe.mjs', 'verification/fixtures/retry-recovery-worker.mjs'];
const hashes = () => Object.fromEntries(sourceFiles.map(path => [path, createHash('sha256').update(readFileSync(join(project, path))).digest('hex')]));
const sourceHashes = hashes(), started = Date.now();
write('budget.json', { maxElapsedMs: 330000, maxWorkers: 6, maxUpstreamRequests: 4, workerTimeoutMs: 5000,
  concurrentWorkers: 1, delaysSeconds: [60, 300], sourceHashes });
const cases = [60, 300].map(seconds => ({ seconds, taskId: `public-retry-${seconds}`, arrivals: [], bodyMatches: true, finished: false }));
const env = Object.fromEntries(['SystemRoot', 'WINDIR'].filter(key => process.env[key]).map(key => [key, process.env[key]]));
let workers = 0, processRecords = [];
const server = createServer((req, res) => {
  let text = '', bytes = 0;
  req.setEncoding('utf8'); req.on('data', chunk => { bytes += Buffer.byteLength(chunk); if (bytes > 1024) req.destroy(); else text += chunk; });
  req.on('end', () => {
    let body; try { body = JSON.parse(text); } catch { res.writeHead(400); res.end(); return; }
    const item = cases.find(row => row.taskId === body.taskId);
    if (!item || item.arrivals.length >= 2) { res.writeHead(400); res.end(); return; }
    item.bodyMatches &&= JSON.stringify(body) === JSON.stringify({ taskId: item.taskId, instruction: 'Complete the original public report.' });
    item.arrivals.push(Date.now());
    if (item.arrivals.length === 1) { res.writeHead(item.seconds === 60 ? 429 : 503, { 'Retry-After': String(item.seconds) }); res.end(); }
    else {
      res.writeHead(200, { 'Content-Type': 'text/event-stream' });
      res.end('data: {"type":"response.created"}\n\ndata: {"type":"response.completed"}\n\ndata: [DONE]\n\n');
    }
  });
});
await new Promise(done => server.listen(0, '127.0.0.1', done));
// The loopback server stays responsive while each bounded child is running.
async function run(item, phase) {
  assert.deepEqual(hashes(), sourceHashes); assert.ok(++workers <= 6);
  const args = ['--permission', `--allow-fs-read=${join(project, 'src')}`, `--allow-fs-read=${join(project, 'poc')}`,
    `--allow-fs-read=${join(project, 'verification')}`, `--allow-fs-read=${item.root}`, `--allow-fs-write=${item.root}`, worker, item.root, phase];
  const child = spawn(process.execPath, args, { cwd: item.root, env, windowsHide: true, stdio: ['ignore', 'pipe', 'pipe'] });
  let output = '', stderr = '', bytes = 0, timedOut = false;
  const timer = setTimeout(() => { timedOut = true; child.kill(); }, 5000);
  child.stdout.on('data', data => { bytes += data.length; if (bytes > 8192) child.kill(); else output += data.toString(); });
  child.stderr.on('data', data => { bytes += data.length; if (bytes > 8192) child.kill(); else stderr += data.toString(); });
  const code = await new Promise((done, reject) => { child.once('error', reject); child.once('close', done); });
  clearTimeout(timer);
  assert.equal(timedOut, false); assert.equal(stderr, ''); assert.ok(bytes <= 8192);
  assert.throws(() => process.kill(child.pid, 0), error => error.code === 'ESRCH');
  const result = JSON.parse(output); processRecords.push({ pid: child.pid, phase, code, ...result });
  return result;
}
let failure = null;
try {
  for (const item of cases) {
    item.root = join(root, item.taskId); mkdirSync(item.root);
    writeFileSync(join(item.root, 'manifest.json'), JSON.stringify({ taskId: item.taskId, port: server.address().port }), { flag: 'wx' });
    const first = await run(item, 'initial'); assert.equal(first.state, 'WAITING'); assert.equal(first.attempts, 1);
    item.retryAtMs = JSON.parse(readFileSync(join(item.root, 'waiting.json'))).retryAtMs;
    const early = await run(item, 'resume'); assert.equal(early.state, 'WAITING'); assert.equal(early.attempts, 0); assert.equal(early.credentialReads, 0);
  }
  let lastNotice = 0;
  while (cases.some(item => !item.finished)) {
    assert.ok(Date.now() - started <= 330000);
    for (const item of cases.filter(row => !row.finished && Date.now() >= row.retryAtMs)) {
      const result = await run(item, 'resume'); assert.equal(result.state, 'VERIFIED'); assert.equal(result.attempts, 1);
      assert.deepEqual(JSON.parse(readFileSync(join(item.root, 'report.json'))), { taskId: item.taskId, completed: true });
      assert.equal(item.arrivals.length, 2); assert.ok(item.arrivals[1] - item.arrivals[0] >= item.seconds * 1000);
      assert.ok(item.bodyMatches); item.finished = true;
    }
    if (Date.now() - lastNotice >= 30000) {
      console.log(JSON.stringify({ event: 'RETRY_RECOVERY_PROGRESS', root, elapsedMs: Date.now() - started,
        states: cases.map(item => ({ seconds: item.seconds, completed: item.finished })) })); lastNotice = Date.now();
    }
    if (cases.some(item => !item.finished)) await new Promise(done => setTimeout(done, 250));
  }
  assert.deepEqual(hashes(), sourceHashes);
} catch { failure = 'RETRY_RECOVERY_CHECK_FAILED'; }
finally { server.closeAllConnections(); await new Promise(done => server.close(done)); }
const result = { suite: 'retry-after-wall-clock', passed: failure === null, failure, root, elapsedMs: Date.now() - started,
  workers: processRecords, cases: cases.map(item => ({ seconds: item.seconds, arrivals: item.arrivals,
    retryAtMs: item.retryAtMs, bodyMatches: item.bodyMatches, completed: item.finished })), externalRequests: 0, actualCredentialReads: 0 };
write('result.json', result); console.log(JSON.stringify(result)); process.exitCode = result.passed ? 0 : 1;
