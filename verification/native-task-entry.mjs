import { createServer } from 'node:http';
import { readFileSync, writeFileSync, appendFileSync, lstatSync } from 'node:fs';
import { join, resolve } from 'node:path';
import { main } from '../src/clauduct.mjs';
import { createNativeLoopbackTransport } from '../src/native-transport.mjs';
import { createBackgroundTaskBindings } from './background-task-id.mjs';

const [root] = process.argv.slice(2);
if (!root || process.argv.length !== 3 || resolve(root, 'work') !== process.cwd()) throw new Error('TASK_ENTRY_ARGUMENTS');
const read = (path, maximum = 4096) => {
  const stat = lstatSync(path);
  if (!stat.isFile() || stat.isSymbolicLink() || stat.size > maximum) throw new Error('TASK_EVIDENCE_INVALID');
  return readFileSync(path, 'utf8');
};
const manifest = JSON.parse(read(join(root, 'manifest.json')));
const { model, effort, cancelled, sessionId } = manifest;
if (!['luna', 'sol'].includes(model) || effort !== (model === 'luna' ? 'max' : 'low')
  || !['alpha', 'beta'].includes(cancelled) || !/^[0-9a-f-]{36}$/.test(sessionId)) throw new Error('TASK_ENTRY_ARGUMENTS');
const survivor = cancelled === 'alpha' ? 'beta' : 'alpha';
const commands = Object.fromEntries(['alpha', 'beta'].map(name => [name,
  `node --permission --allow-fs-read=worker.mjs --allow-fs-write=${name}-events.jsonl worker.mjs ${name} ${cancelled}`]));
const marker = 'PUBLIC_TASK_ISOLATION_COMPLETE', bindings = createBackgroundTaskBindings();
const callFor = name => name === 'alpha' ? 'call_public_1' : 'call_public_2';
const ids = {}, evidence = { requests: 0, routeMatched: true, issued: [], stopIssuedAt: null,
  targetStoppedAt: null, survivorAliveAfterStop: false, workers: {}, failure: null,
  actualModelRequests: 0, credentialReads: 0 };
let server, transport, traceCount = 0, serial = 0;
function trace(point) {
  if (++traceCount > 64) throw new Error('TASK_TRACE_LIMIT');
  appendFileSync(join(root, 'progress.jsonl'), JSON.stringify({ at: Date.now(), point, requests: evidence.requests }) + '\n', { flush: true });
}
function worker(name) {
  const rows = read(join(root, 'work', `${name}-events.jsonl`)).trim().split('\n').map(JSON.parse);
  if (rows.length < 1 || rows.length > 2 || rows[0].event !== 'started' || !Number.isSafeInteger(rows[0].pid)
    || rows[0].pid <= 0 || !Number.isSafeInteger(rows[0].at) || rows[0].durationMs !== (name === cancelled ? 15000 : 5000)
    || rows.length === 2 && (rows[1].event !== 'finished' || !Number.isSafeInteger(rows[1].at))) throw new Error('TASK_EVIDENCE_INVALID');
  return { ...rows[0], finished: rows.length === 2, finishedAt: rows[1]?.at ?? null };
}
function alive(pid) {
  try { process.kill(pid, 0); return true; }
  catch (error) { if (error.code === 'ESRCH') return false; throw new Error('TASK_PROCESS_UNVERIFIED'); }
}
async function readyWorkers() {
  const started = Date.now(), deadline = started + 1500;
  for (;;) {
    try {
      const workers = { alpha: worker('alpha'), beta: worker('beta') };
      evidence.workerReadinessMs = Date.now() - started; trace('workers-ready'); return workers;
    } catch (error) {
      // Native returns a background task ID before the worker opens its first
      // log. Only missing/incomplete startup bytes may settle within this bound.
      if (error.code !== 'ENOENT' && !(error instanceof SyntaxError)) throw error;
      if (Date.now() >= deadline) throw new Error('TASK_WORKER_NOT_READY');
      await new Promise(done => setTimeout(done, 25));
    }
  }
}
function response(body, name, input = {}) {
  const responseId = `resp_public_${++serial}`, itemId = `item_public_${serial}`;
  const item = name ? { type: 'function_call', id: itemId, call_id: `call_public_${serial}`, name,
    arguments: JSON.stringify(input), status: 'completed' }
    : { type: 'message', id: itemId, role: 'assistant', status: 'completed',
      content: [{ type: 'output_text', text: marker, annotations: [] }] };
  const events = [{ type: 'response.created', response: { id: responseId, status: 'in_progress' } },
    { type: 'response.output_item.added', output_index: 0,
      item: name ? { ...item, arguments: '', status: 'in_progress' } : { ...item, content: [], status: 'in_progress' } },
    ...(name ? [
      { type: 'response.function_call_arguments.delta', output_index: 0, item_id: itemId, delta: item.arguments },
      { type: 'response.function_call_arguments.done', output_index: 0, item_id: itemId, arguments: item.arguments }
    ] : [
      { type: 'response.output_text.delta', output_index: 0, item_id: itemId, content_index: 0, delta: marker },
      { type: 'response.output_text.done', output_index: 0, item_id: itemId, content_index: 0, text: marker }
    ]), { type: 'response.output_item.done', output_index: 0, item },
    { type: 'response.completed', response: { id: responseId, status: 'completed', model: `gpt-5.6-${model}`,
      reasoning: { effort }, output: [item], usage: { input_tokens: 1, output_tokens: 1, total_tokens: 2 } } }];
  if (name) evidence.issued.push({ name, serial });
  return events.map(event => `data: ${JSON.stringify(event)}\n\n`).join('') + 'data: [DONE]\n\n';
}
async function select(body) {
  if (!Array.isArray(body.input) || body.input.length > 4096 || !Array.isArray(body.tools) || body.tools.length > 128) throw new Error('TASK_REQUEST_INVALID');
  const available = ['Bash', 'TaskStop', 'TaskOutput'].every(name => body.tools.some(tool => tool.name === name));
  const stopTool = body.tools.find(tool => tool.name === 'TaskStop');
  if (!available || stopTool.parameters?.properties?.task_id?.type !== 'string') throw new Error('TASK_TOOLS_UNAVAILABLE');
  const step = evidence.requests;
  if (step > 1) ids.alpha = bindings.observe(body, callFor('alpha'));
  if (step > 2) ids.beta = bindings.observe(body, callFor('beta'));
  if (step <= 2) {
    const name = step === 1 ? 'alpha' : 'beta';
    return response(body, 'Bash', { command: commands[name], run_in_background: true, description: `Public ${name} worker` });
  }
  if (step === 3) {
    const workers = await readyWorkers(), target = workers[cancelled], other = workers[survivor], now = Date.now();
    if (target.finished || other.finished || !alive(target.pid) || !alive(other.pid)
      || now >= target.at + 13000 || now >= other.at + 4000) throw new Error('TASK_STIMULUS_EXPIRED');
    evidence.workers = { [cancelled]: target, [survivor]: other }; evidence.stopIssuedAt = now;
    return response(body, 'TaskStop', { task_id: ids[cancelled] });
  }
  if (step === 4) {
    if (body.input.filter(item => item.type === 'function_call_output' && item.call_id === 'call_public_3').length !== 1) throw new Error('TASK_STOP_RESULT_MISSING');
    const deadline = Date.now() + 1000;
    while (alive(evidence.workers[cancelled].pid) && Date.now() < deadline) await new Promise(done => setTimeout(done, 25));
    if (alive(evidence.workers[cancelled].pid) || worker(cancelled).finished) throw new Error('TASK_STOP_UNVERIFIED');
    evidence.targetStoppedAt = Date.now();
    evidence.survivorAliveAfterStop = alive(evidence.workers[survivor].pid) && !worker(survivor).finished;
    if (!evidence.survivorAliveAfterStop) throw new Error('TASK_SIBLING_INTERRUPTED');
    return response(body, 'TaskOutput', { task_id: ids[survivor], block: true, timeout: 7000 });
  }
  if (step !== 5 || body.input.filter(item => item.type === 'function_call_output' && item.call_id === 'call_public_4').length !== 1
    || !worker(survivor).finished || worker(cancelled).finished) throw new Error('TASK_COMPLETION_UNVERIFIED');
  return response(body, null);
}
const known = new Set(['TASK_EVIDENCE_INVALID', 'TASK_PROCESS_UNVERIFIED', 'TASK_REQUEST_INVALID', 'TASK_TOOLS_UNAVAILABLE',
  'TASK_STIMULUS_EXPIRED', 'TASK_STOP_RESULT_MISSING', 'TASK_STOP_UNVERIFIED', 'TASK_SIBLING_INTERRUPTED',
  'TASK_COMPLETION_UNVERIFIED', 'TASK_ROUTE_MISMATCH', 'TASK_TRACE_LIMIT', 'TASK_WORKER_NOT_READY', 'VERIFICATION_TOOL_INPUT_REJECTED']);
try {
  trace('entry');
  server = createServer((req, res) => {
    void (async () => {
      trace('http');
      if (++evidence.requests > 8 || req.method !== 'POST' || req.url !== '/backend-api/codex/responses') throw new Error('TASK_REQUEST_INVALID');
      let bytes = 0, text = '';
      for await (const chunk of req) { bytes += chunk.length; if (bytes > 2097152) throw new Error('TASK_REQUEST_INVALID'); text += chunk.toString(); }
      const body = JSON.parse(text);
      if (body.model !== `gpt-5.6-${model}` || body.reasoning?.effort !== effort) { evidence.routeMatched = false; throw new Error('TASK_ROUTE_MISMATCH'); }
      const wire = await select(body);
      res.writeHead(200, { 'Content-Type': 'text/event-stream' }); res.end(wire); trace('response');
    })().catch(error => { evidence.failure ??= known.has(error.code ?? error.message) ? error.code ?? error.message : 'TASK_HANDLER_FAILED'; res.writeHead(400).end(); });
  });
  await new Promise((done, reject) => { server.once('error', reject); server.listen(0, '127.0.0.1', done); });
  await main({ args: ['-p', '--model', model, '--effort', effort, '--verify-model-route', '--verify-request-limit', '8',
    '--session-id', sessionId, '--output-format', 'stream-json', '--verbose', '--tools', 'Bash,TaskStop,TaskOutput',
    '--allowedTools', `Bash(${commands.alpha}),Bash(${commands.beta}),TaskStop,TaskOutput`, '--max-turns', '8', '--',
    `Start the two fixed public workers. Cancel ${cancelled} using its returned task ID. Wait for ${survivor} with TaskOutput. Reply ${marker} only after these steps.`],
    openTransport: () => { transport = createNativeLoopbackTransport(server.address().port, { requestBudget: 8 }); return transport; } });
} catch { evidence.failure ??= 'TASK_ENTRY_FAILED'; process.exitCode = 1; }
finally {
  if (transport) await transport.close();
  if (server) { server.closeAllConnections(); await new Promise(done => server.close(done)); }
  // Failed scenarios retain this owner while their finite workers drain. The
  // outer watchdog can then still identify the entire live descendant tree.
  const drainUntil = Date.now() + 16000;
  for (;;) {
    let pending = false;
    for (const name of ['alpha', 'beta']) {
      try { pending ||= alive(worker(name).pid); } catch { evidence.failure ??= 'TASK_FINAL_EVIDENCE_MISSING'; }
    }
    if (!pending || Date.now() >= drainUntil) break;
    await new Promise(done => setTimeout(done, 25));
  }
  for (const name of ['alpha', 'beta']) {
    try { evidence.workers[name] = { ...worker(name), alive: alive(worker(name).pid) }; }
    catch { evidence.failure ??= 'TASK_FINAL_EVIDENCE_MISSING'; }
  }
  writeFileSync(join(root, 'task-evidence.json'), JSON.stringify(evidence) + '\n', { flag: 'wx' });
}
