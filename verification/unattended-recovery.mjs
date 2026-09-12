import { openSync, closeSync, readFileSync, writeFileSync, fsyncSync, statSync, lstatSync, mkdirSync, unlinkSync, existsSync } from 'node:fs';
import { join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { randomUUID, createHash } from 'node:crypto';
import { fork } from 'node:child_process';

// A bounded verification manager. Only the fixture's atomic receipt is an effect
// oracle; the journal cannot make arbitrary shell or MCP effects exactly-once.
const workerFile = fileURLToPath(new URL('./fixtures/recovery-worker.mjs', import.meta.url));
const ORACLE = '{"version":1,"operationCount":1,"report":"effect-reconciled"}\n';
const MAX_JOURNAL_BYTES = 64 * 1024;
const events = new Set(['INTENT', 'WORKER', 'INTERRUPTED', 'EFFECT_CONFIRMED', 'COMPLETION_CONFIRMED', 'VERIFIED', 'FIX_NEEDED', 'UNKNOWN_EFFECT']);
const faults = new Set(['none', 'before-effect', 'after-effect', 'before-record', 'after-record', 'fake-success']);
const known = new Set(['OWNER_ACTIVE', 'OWNER_UNCERTAIN', 'WORKER_ALIVE', 'JOURNAL_CORRUPT', 'JOURNAL_TRUNCATED',
  'ORACLE_CHANGED', 'MANIFEST_INVALID', 'STATE_IO_ERROR', 'STATE_PATH_REJECTED', 'WORKER_FAILED', 'WORKER_TIMEOUT', 'INVALID_ARGUMENTS']);
const fail = code => { const error = new Error(code); error.code = code; throw error; };
const need = (ok, code) => { if (!ok) fail(code); };
const digest = bytes => createHash('sha256').update(bytes).digest('hex');

function plainFile(path, max = MAX_JOURNAL_BYTES) {
  const stat = lstatSync(path);
  need(stat.isFile() && !stat.isSymbolicLink() && stat.size <= max, 'STATE_PATH_REJECTED');
  return readFileSync(path, 'utf8');
}
function put(path, value, flag = 'wx') {
  const fd = openSync(path, flag);
  try { writeFileSync(fd, value); fsyncSync(fd); } finally { closeSync(fd); }
}
function readJson(path, max) {
  try { return JSON.parse(plainFile(path, max)); }
  catch (error) { if (known.has(error.code)) throw error; fail('STATE_IO_ERROR'); }
}
function alive(pid) {
  need(Number.isInteger(pid) && pid > 0, 'OWNER_UNCERTAIN');
  try { process.kill(pid, 0); return true; }
  catch (error) { if (error.code === 'ESRCH') return false; fail('OWNER_UNCERTAIN'); }
}
function validateRoot(root) {
  let path = resolve(root);
  for (;;) {
    const stat = lstatSync(path);
    need(stat.isDirectory() && !stat.isSymbolicLink(), 'STATE_PATH_REJECTED');
    const parent = resolve(path, '..');
    if (parent === path) break;
    path = parent;
  }
  return resolve(root);
}

export function initializeRecovery(root, { taskId = 'public-effect', mode = 'queryable' } = {}) {
  need(/^[a-z0-9-]{1,64}$/.test(taskId) && ['queryable', 'opaque'].includes(mode), 'INVALID_ARGUMENTS');
  root = validateRoot(root);
  need(!existsSync(join(root, 'manifest.json')), 'MANIFEST_INVALID');
  mkdirSync(join(root, 'work'));
  put(join(root, 'oracle.json'), ORACLE);
  const manifest = { version: 1, taskId, operationId: randomUUID(), mode, oracleHash: digest(ORACLE),
    workerTimeoutMs: 5000, maxWorkers: 8 };
  put(join(root, 'manifest.json'), JSON.stringify(manifest) + '\n');
  put(join(root, 'journal.jsonl'), '');
  return manifest;
}

export function readRecoveryJournal(root) {
  const text = plainFile(join(root, 'journal.jsonl'));
  need(text === '' || text.endsWith('\n'), 'JOURNAL_TRUNCATED');
  const records = text === '' ? [] : text.trimEnd().split('\n').map(line => {
    try { return JSON.parse(line); } catch { fail('JOURNAL_CORRUPT'); }
  });
  for (const [index, row] of records.entries()) {
    need(row && row.seq === index + 1 && events.has(row.event) && Number.isSafeInteger(row.at)
      && Object.keys(row).every(key => ['seq', 'event', 'at', 'pid', 'phase', 'exitCode'].includes(key)), 'JOURNAL_CORRUPT');
    if (row.event === 'WORKER') need(Number.isInteger(row.pid) && row.pid > 0
      && ['effect', 'finish'].includes(row.phase), 'JOURNAL_CORRUPT');
  }
  return records;
}
function append(root, records, event, fields = {}) {
  const row = { seq: records.length + 1, event, at: Date.now(), ...fields };
  const text = JSON.stringify(row) + '\n';
  need(statSync(join(root, 'journal.jsonl')).size + Buffer.byteLength(text) <= MAX_JOURNAL_BYTES, 'STATE_IO_ERROR');
  put(join(root, 'journal.jsonl'), text, 'a');
  records.push(row);
}
export function recordRecoveryWorker(root, records, pid, phase) {
  need(Number.isInteger(pid) && pid > 0 && ['effect', 'finish'].includes(phase), 'INVALID_ARGUMENTS');
  append(root, records, 'WORKER', { pid, phase });
}

function acquire(root) {
  // Reclamation and initial acquisition use the same exclusive gate, preventing
  // two reclaimers from unlinking a newly acquired owner's file (the ABA race).
  // A crash inside this short critical section remains explicitly uncertain.
  const gate = join(root, 'acquiring.lock'), ownerPath = join(root, 'owner.json');
  let gateFd;
  try { gateFd = openSync(gate, 'wx'); }
  catch (error) { if (error.code === 'EEXIST') fail('OWNER_UNCERTAIN'); throw error; }
  const owner = { pid: process.pid, nonce: randomUUID() };
  try {
    if (existsSync(ownerPath)) {
      const previous = readJson(ownerPath, 1024);
      need(!alive(previous.pid), 'OWNER_ACTIVE');
      unlinkSync(ownerPath);
    }
    put(ownerPath, JSON.stringify(owner));
  } finally { closeSync(gateFd); unlinkSync(gate); }
  return () => {
    const current = readJson(ownerPath, 1024);
    need(current.pid === owner.pid && current.nonce === owner.nonce, 'OWNER_UNCERTAIN');
    unlinkSync(ownerPath);
  };
}

function effectReceipt(root, manifest) {
  const path = join(root, 'work', 'operation.json');
  if (!existsSync(path)) return false;
  const receipt = readJson(path, 1024);
  need(JSON.stringify(receipt) === JSON.stringify({ operationId: manifest.operationId, count: 1 }), 'STATE_IO_ERROR');
  return true;
}
export function recoveryOracle(root) {
  root = validateRoot(root);
  const manifest = readJson(join(root, 'manifest.json'), 4096);
  need(plainFile(join(root, 'oracle.json'), 4096) === ORACLE && manifest.oracleHash === digest(ORACLE), 'ORACLE_CHANGED');
  const receipt = effectReceipt(root, manifest);
  const path = join(root, 'work', 'report.json');
  const report = existsSync(path) ? readJson(path, 1024) : null;
  return receipt && JSON.stringify(report) === JSON.stringify({ operationId: manifest.operationId,
    operationCount: 1, result: 'effect-reconciled' });
}

async function worker(root, manifest, records, phase, fault) {
  need(records.filter(row => row.event === 'WORKER').length < manifest.maxWorkers, 'WORKER_FAILED');
  const work = join(root, 'work'), env = {};
  for (const name of ['SystemRoot', 'WINDIR', 'TEMP', 'TMP']) if (process.env[name]) env[name] = process.env[name];
  const child = fork(workerFile, [work, manifest.operationId, manifest.mode], { env, cwd: work,
    execArgv: ['--permission', `--allow-fs-read=${workerFile}`, `--allow-fs-read=${work}`, `--allow-fs-write=${work}`],
    stdio: ['ignore', 'ignore', 'pipe', 'ipc'], windowsHide: true, shell: false });
  let stderrBytes = 0, reply = null, timedOut = false, timer;
  child.stderr.on('data', chunk => { stderrBytes += chunk.length; if (stderrBytes > 8192) child.kill(); });
  const stopped = new Promise((resolveStop, reject) => {
    child.once('error', () => reject(Object.assign(new Error('WORKER_FAILED'), { code: 'WORKER_FAILED' })));
    child.once('close', (code, signal) => resolveStop({ code, signal }));
    child.on('message', message => { if (message?.type === 'result' && typeof message.completed === 'boolean') reply = message; });
  });
  try {
    // A worker has no effect until this durable PID record and explicit IPC go.
    recordRecoveryWorker(root, records, child.pid, phase);
    timer = setTimeout(() => { timedOut = true; child.kill(); }, manifest.workerTimeoutMs);
    child.send({ phase, fault });
    const exit = await stopped;
    need(!alive(child.pid), 'WORKER_ALIVE');
    if (timedOut) fail('WORKER_TIMEOUT');
    return { completed: exit.code === 0 && !exit.signal && reply?.completed === true && stderrBytes === 0, exitCode: exit.code };
  } finally {
    clearTimeout(timer);
    if (child.exitCode === null && child.signalCode === null) { child.kill(); await stopped; }
  }
}

export async function runRecovery(root, { fault = 'none', holdMs = 0, execute = worker } = {}) {
  need(faults.has(fault) && Number.isInteger(holdMs) && holdMs >= 0 && holdMs <= 3000
    && typeof execute === 'function', 'INVALID_ARGUMENTS');
  root = validateRoot(root);
  const release = acquire(root);
  try {
    const manifest = readJson(join(root, 'manifest.json'), 4096);
    need(manifest.version === 1 && /^[a-z0-9-]{1,64}$/.test(manifest.taskId)
      && /^[0-9a-f-]{36}$/.test(manifest.operationId) && ['queryable', 'opaque'].includes(manifest.mode)
      && manifest.workerTimeoutMs === 5000 && manifest.maxWorkers === 8, 'MANIFEST_INVALID');
    need(plainFile(join(root, 'oracle.json'), 4096) === ORACLE && manifest.oracleHash === digest(ORACLE), 'ORACLE_CHANGED');
    const records = readRecoveryJournal(root);
    for (const row of records.filter(row => row.event === 'WORKER')) need(!alive(row.pid), 'WORKER_ALIVE');
    if (holdMs) await new Promise(done => setTimeout(done, holdMs));
    const confirmed = records.some(row => row.event === 'EFFECT_CONFIRMED');
    if (manifest.mode === 'opaque' && records.some(row => row.event === 'INTENT') && !confirmed) {
      append(root, records, 'UNKNOWN_EFFECT');
      return { state: 'UNKNOWN_EFFECT', scenarioPassed: true, taskCompleted: false };
    }
    if (!confirmed && !(manifest.mode === 'queryable' && effectReceipt(root, manifest))) {
      append(root, records, 'INTENT');
      const result = await execute(root, manifest, records, 'effect', fault);
      if (!result.completed) {
        append(root, records, 'INTERRUPTED', { exitCode: result.exitCode });
        return { state: 'RECOVERING', scenarioPassed: false, taskCompleted: false };
      }
    }
    if (fault === 'before-record') process.exit(71);
    if (!confirmed) append(root, records, 'EFFECT_CONFIRMED');
    if (fault === 'after-record') process.exit(71);
    if (!records.some(row => row.event === 'COMPLETION_CONFIRMED') || !recoveryOracle(root)) {
      const result = await execute(root, manifest, records, 'finish', fault);
      if (!result.completed) {
        append(root, records, 'INTERRUPTED', { exitCode: result.exitCode });
        return { state: 'RECOVERING', scenarioPassed: false, taskCompleted: false };
      }
      if (recoveryOracle(root)) append(root, records, 'COMPLETION_CONFIRMED');
    }
    const taskCompleted = recoveryOracle(root);
    append(root, records, taskCompleted ? 'VERIFIED' : 'FIX_NEEDED');
    return { state: taskCompleted ? 'VERIFIED' : 'FIX_NEEDED', scenarioPassed: taskCompleted, taskCompleted };
  } finally { release(); }
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  try {
    const [root, fault = 'none', hold = '0'] = process.argv.slice(2);
    need(typeof root === 'string' && process.argv.length <= 5, 'INVALID_ARGUMENTS');
    const result = await runRecovery(root, { fault, holdMs: Number(hold) });
    process.stdout.write(JSON.stringify(result) + '\n');
    process.exitCode = result.taskCompleted ? 0 : 2;
  } catch (error) {
    process.stdout.write(JSON.stringify({ state: known.has(error.code) ? error.code : 'STATE_IO_ERROR',
      scenarioPassed: false, taskCompleted: false }) + '\n');
    process.exitCode = 1;
  }
}
