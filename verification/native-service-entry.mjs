import { createServer } from 'node:http';
import { readFileSync, writeFileSync, lstatSync, existsSync } from 'node:fs';
import { join, resolve } from 'node:path';
import { main } from '../src/clauduct.mjs';
import { createNativeLoopbackTransport } from '../src/native-transport.mjs';
import { SERVICE_FAULTS, publicServiceEvents, serviceFailureWire, frame } from './fixtures/service-faults.mjs';

// Actual native and HTTP; no openUserTransport, credentials or external endpoint.
const [root, phase, kind, model] = process.argv.slice(2);
const effort = { luna: 'max', sol: 'low' }[model];
const failureLabels = new Set(['SERVICE_EVIDENCE_BOUNDARY', 'SERVICE_DUPLICATE_TOOL', 'SERVICE_TOOLS_UNAVAILABLE',
  'SERVICE_EFFECT_BOUNDARY', 'SERVICE_EFFECT_LOST', 'SERVICE_STATUS_MISSING', 'SERVICE_REPORT_MISSING',
  'SERVICE_REQUEST_BOUNDARY', 'SERVICE_INPUT_LIMIT', 'SERVICE_ROUTE_MISMATCH', 'SERVICE_INPUT_SHAPE']);
if (!root || resolve(root, 'work') !== process.cwd() || !['effect', 'finish'].includes(phase)
  || !SERVICE_FAULTS.includes(kind) || !effort || !process.send) throw new Error('SERVICE_ENTRY_ARGUMENTS');
const read = (path, limit = 4096) => {
  const stat = lstatSync(path);
  if (!stat.isFile() || stat.isSymbolicLink() || stat.size > limit) throw new Error('SERVICE_EVIDENCE_BOUNDARY');
  return readFileSync(path, 'utf8');
};
const manifest = JSON.parse(read(join(root, 'manifest.json')));
if (!/^[0-9a-f-]{36}$/.test(manifest.operationId)) throw new Error('SERVICE_ENTRY_ARGUMENTS');
const evidence = { phase, kind, model, effort, requests: 0, successes: 0, serviceFailures: 0,
  partialFailures: 0, faultAfterReceipt: false, routeMatched: true, issued: [], failure: null,
  deferredAtMs: null, actualModelRequests: 0, credentialReads: 0, serverClosed: false, socketsRemaining: null };
const issued = new Map(), timers = new Set(), sockets = new Set(), socketClosures = new Set();
let transport, server, active = 0, serial = phase === 'effect' ? 0 : 16;
const receiptPath = join(root, 'work', 'operation.json'), reportPath = join(root, 'work', 'report.json');
const receipt = () => existsSync(receiptPath) && read(receiptPath) === JSON.stringify({ operationId: manifest.operationId, count: 1 });
const report = () => existsSync(reportPath) && read(reportPath) === JSON.stringify({ operationId: manifest.operationId,
  operationCount: 1, result: 'effect-reconciled' });
function hasResult(body, name) {
  const id = issued.get(name);
  return id !== undefined && body.input.filter(item => item?.type === 'function_call_output' && item.call_id === id).length === 1;
}
function answer(body, name, input = {}) {
  if (name && issued.has(name)) throw new Error('SERVICE_DUPLICATE_TOOL');
  const events = publicServiceEvents(body.model, body.reasoning.effort, ++serial, name, input);
  if (name) { issued.set(name, `call_public_${serial}`); evidence.issued.push(name); }
  evidence.successes++;
  return events.map(frame).join('') + 'data: [DONE]\n\n';
}
function select(body) {
  const required = phase === 'effect' ? ['mcp__fixture__apply_effect']
    : ['mcp__fixture__effect_status', 'mcp__fixture__complete_report'];
  if (required.some(name => !body.tools.some(tool => tool?.name === name))) {
    if (issued.has('ToolSearch') || !body.tools.some(tool => tool?.name === 'ToolSearch')) throw new Error('SERVICE_TOOLS_UNAVAILABLE');
    return answer(body, 'ToolSearch', { query: 'fixture' });
  }
  if (phase === 'effect') {
    if (!receipt()) return answer(body, 'mcp__fixture__apply_effect');
    if (!hasResult(body, 'mcp__fixture__apply_effect') || report()) throw new Error('SERVICE_EFFECT_BOUNDARY');
    evidence.faultAfterReceipt = true;
    return null;
  }
  if (!receipt()) throw new Error('SERVICE_EFFECT_LOST');
  if (!issued.has('mcp__fixture__effect_status')) return answer(body, 'mcp__fixture__effect_status');
  if (!hasResult(body, 'mcp__fixture__effect_status')) throw new Error('SERVICE_STATUS_MISSING');
  if (!report()) return answer(body, 'mcp__fixture__complete_report');
  if (!hasResult(body, 'mcp__fixture__complete_report')) throw new Error('SERVICE_REPORT_MISSING');
  return answer(body, null);
}
async function handle(req, res) {
  if (++active !== 1 || ++evidence.requests > 16 || req.method !== 'POST'
    || req.url !== '/backend-api/codex/responses') throw new Error('SERVICE_REQUEST_BOUNDARY');
  let bytes = 0; const chunks = [];
  for await (const chunk of req) {
    bytes += chunk.length;
    if (bytes > 2 * 1024 * 1024) throw new Error('SERVICE_INPUT_LIMIT');
    chunks.push(chunk);
  }
  const body = JSON.parse(Buffer.concat(chunks).toString('utf8'));
  if (body.model !== `gpt-5.6-${model}` || body.reasoning?.effort !== effort) {
    evidence.routeMatched = false; throw new Error('SERVICE_ROUTE_MISMATCH');
  }
  if (!Array.isArray(body.input) || body.input.length > 4096 || !Array.isArray(body.tools)
    || body.tools.length > 256) throw new Error('SERVICE_INPUT_SHAPE');
  if (kind === 'flapping-503' && evidence.requests <= 2) {
    evidence.serviceFailures++; res.writeHead(503, { 'Retry-After': '0' }); res.end(); return;
  }
  const wire = select(body);
  if (wire !== null) { res.writeHead(200, { 'Content-Type': 'text/event-stream' }); res.end(wire); return; }
  if (kind === 'flapping-503') {
    evidence.serviceFailures++; res.writeHead(503, { 'Retry-After': '0' }); res.end(); return;
  }
  if (kind === 'deferred-503') {
    evidence.serviceFailures++; evidence.deferredAtMs = Date.now();
    res.writeHead(503, { 'Retry-After': '60' }); res.end(); return;
  }
  evidence.partialFailures++;
  const failure = serviceFailureWire(kind);
  res.writeHead(200, { 'Content-Type': 'text/event-stream' }); res.write(failure.prefix);
  await new Promise(done => {
    const timer = setTimeout(() => { timers.delete(timer); done(); }, 40); timers.add(timer);
  });
  if (failure.destroy) res.destroy(); else res.end(failure.tail);
}
const startTimer = setTimeout(() => process.exit(1), 5000);
await new Promise(done => process.once('message', message => { if (message?.start === true) done(); else process.exit(1); }));
clearTimeout(startTimer);
try {
  server = createServer((req, res) => {
    void handle(req, res).catch(error => {
      evidence.failure ??= failureLabels.has(error.message) ? error.message : 'SERVICE_HANDLER_FAILED'; res.destroy();
    }).finally(() => { active--; });
  });
  server.on('connection', socket => {
    sockets.add(socket);
    const closed = new Promise(done => socket.once('close', () => {
      sockets.delete(socket); socketClosures.delete(closed); done();
    }));
    socketClosures.add(closed);
  });
  await new Promise((done, reject) => { server.once('error', reject); server.listen(0, '127.0.0.1', done); });
  const names = phase === 'effect' ? 'ToolSearch,mcp__fixture__apply_effect,mcp__fixture__complete_report'
    : 'ToolSearch,mcp__fixture__effect_status,mcp__fixture__complete_report';
  await main({ args: ['--model', model, '--effort', effort, '--verify-model-route', '--verify-request-limit', '16',
    '-p', '--output-format', 'json', '--tools', 'ToolSearch', '--allowedTools', names, '--max-turns', '8',
    ...(phase === 'effect' ? ['--session-id', manifest.operationId]
      : ['--resume', manifest.operationId, '--disallowedTools', 'mcp__fixture__apply_effect']), '--',
    phase === 'effect' ? 'PUBLIC_SERVICE_TASK: apply the one public fixture effect, complete its report, then return PUBLIC_SERVICE_RECOVERY_COMPLETE.'
      : 'Resume the same PUBLIC_SERVICE_TASK. Reconcile with effect_status, then complete_report. Do not apply the effect again. Return PUBLIC_SERVICE_RECOVERY_COMPLETE only after the report succeeds.'],
    openTransport: () => {
      transport = createNativeLoopbackTransport(server.address().port, { requestBudget: 16 });
      return transport;
    } });
} catch { evidence.failure ??= 'SERVICE_ENTRY_FAILED'; process.exitCode = 1; }
finally {
  for (const timer of timers) clearTimeout(timer);
  if (transport) await transport.close();
  if (server) {
    const closed = new Promise(done => server.close(done));
    server.closeAllConnections();
    await closed; evidence.serverClosed = true;
    let deadline;
    const socketsClosed = await Promise.race([Promise.all([...socketClosures]).then(() => true),
      new Promise(done => { deadline = setTimeout(() => done(false), 1000); })]);
    clearTimeout(deadline);
    if (!socketsClosed) { evidence.failure ??= 'SERVICE_SOCKET_CLOSE_UNVERIFIED'; process.exitCode = 1; }
  }
  evidence.socketsRemaining = sockets.size;
  evidence.transport = transport?.diagnostics() ?? null;
  writeFileSync(join(root, `service-${phase}.json`), JSON.stringify(evidence) + '\n', { flag: 'wx' });
  if (process.connected) process.disconnect();
}
