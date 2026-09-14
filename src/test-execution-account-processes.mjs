import assert from 'node:assert/strict';
import { mkdirSync, existsSync, readdirSync } from 'node:fs';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { spawn } from 'node:child_process';
import { createExecutionAccount, readExecutionAccount, closeExecutionAccount } from '../verification/execution-account.mjs';
import { temporaryDir } from '../verification/temporary-dir.mjs';

const project = fileURLToPath(new URL('../', import.meta.url)), root = temporaryDir(project, 'execution-account-processes-');
const worker = join(project, 'verification', 'fixtures', 'account-worker.mjs');
const modules = ['execution-account.mjs', 'execution-reservation.mjs'].map(name => join(project, 'verification', name));
const env = Object.fromEntries(['SystemRoot', 'WINDIR'].filter(key => process.env[key]).map(key => [key, process.env[key]]));
let checks = 0, children = 0, stopped = 0;
const equal = (a, b) => { assert.deepEqual(a, b); checks++; };
function fixture(name) {
  const path = join(root, name); mkdirSync(path);
  createExecutionAccount(path, { basisHash: 'a'.repeat(64),
    previous: { attempts: 10, inputTokens: 100, outputTokens: 20, elapsedMs: 1000 },
    limits: { attempts: 30, inputTokens: 5000, outputTokens: 2000, elapsedMs: 50000 } });
  return path;
}
function start(path, mode) {
  const child = spawn(process.execPath, ['--permission', `--allow-fs-read=${worker}`, ...modules.map(file => `--allow-fs-read=${file}`),
    `--allow-fs-read=${path}`, `--allow-fs-write=${path}`, worker, path, mode],
  { cwd: path, env, windowsHide: true, stdio: ['ignore', 'pipe', 'pipe'] });
  children++;
  let ended = false, stopAttempted = false, text = '', errorBytes = 0, timer;
  const closed = new Promise(done => child.once('close', (code, signal) => { ended = true; stopped++; done({ code, signal }); }));
  const ready = new Promise((done, reject) => {
    timer = setTimeout(() => reject(new Error('ACCOUNT_WORKER_READY_TIMEOUT')), 3000);
    child.once('error', reject);
    child.stderr.on('data', bytes => { errorBytes += bytes.length; });
    child.stdout.on('data', bytes => {
      text += bytes.toString();
      if (text.length > 1024) { clearTimeout(timer); reject(new Error('ACCOUNT_WORKER_OUTPUT_LIMIT')); }
      else if (text.endsWith('\n')) {
        clearTimeout(timer);
        try { done(JSON.parse(text)); } catch { reject(new Error('ACCOUNT_WORKER_INVALID_OUTPUT')); }
      }
    });
  });
  async function stop() {
    clearTimeout(timer);
    if (!ended && !stopAttempted) { stopAttempted = true; child.kill(); }
    let stopTimer;
    try {
      const result = await Promise.race([closed, new Promise((_, reject) => { stopTimer = setTimeout(() => reject(new Error('ACCOUNT_WORKER_STOP_UNVERIFIED')), 3000); })]);
      equal(errorBytes, 0); return result;
    } finally { clearTimeout(stopTimer); }
  }
  return { ready, closed, stop };
}
for (let index = 0; index < 2; index++) {
  const path = fixture(`race-${index}`), runners = [start(path, 'claim'), start(path, 'claim')];
  try {
    const states = await Promise.all(runners.map(runner => runner.ready));
    equal(states.filter(value => value.state === 'PUBLIC_ACCOUNT_READY').length, 1);
    equal(states.filter(value => ['EXECUTION_ACCOUNT_BUSY', 'EXECUTION_ACCOUNT_PENDING', 'EXECUTION_ACCOUNT_ENTRY_INCOMPLETE'].includes(value.state)).length, 1);
    equal(readdirSync(join(path, 'executions')), ['000001']);
    equal(readExecutionAccount(path).charged.attempts, 16);
  } finally { await Promise.all(runners.map(runner => runner.stop())); }
  const state = readExecutionAccount(path);
  const closed = closeExecutionAccount(state.pending, { observed: { attempts: null, inputTokens: null, outputTokens: null, elapsedMs: null }, evidenceHash: 'c'.repeat(64) });
  equal(closed.charged, state.charged); equal(closed.pending, null);
}
for (const mode of ['claim', 'settled', 'torn']) {
  const path = fixture(mode), runner = start(path, mode);
  try {
    equal((await runner.ready).state, 'PUBLIC_ACCOUNT_READY');
    const before = readExecutionAccount(path);
    equal(before.charged, { attempts: 16, inputTokens: 1100, outputTokens: 220, elapsedMs: 21000 });
    assert.throws(() => closeExecutionAccount(before.pending, { observed: { attempts: 5, inputTokens: 150, outputTokens: 30, elapsedMs: 2000 }, evidenceHash: 'c'.repeat(64) }),
      { message: 'EXECUTION_ACCOUNT_OWNER_ACTIVE' }); checks++;
    equal(existsSync(join(before.pending, 'closure.json')), false);
  } finally { equal((await runner.stop()).signal, 'SIGTERM'); }
  const state = readExecutionAccount(path), observed = { attempts: 5, inputTokens: 150, outputTokens: 30, elapsedMs: 2000 };
  if (mode === 'torn') {
    assert.throws(() => closeExecutionAccount(state.pending, { observed, evidenceHash: 'c'.repeat(64) }),
      { message: 'EXECUTION_ACCOUNT_SETTLEMENT_CONFLICT' }); checks++;
    equal(readExecutionAccount(path).charged, state.charged);
    equal(existsSync(join(state.pending, 'closure.json')), false);
  } else {
    const closed = closeExecutionAccount(state.pending, { observed, evidenceHash: 'c'.repeat(64) });
    equal(closed.charged, { attempts: 15, inputTokens: 250, outputTokens: 50, elapsedMs: 3000 });
    equal(closed.pending, null);
  }
}
equal(children, stopped);
console.log(JSON.stringify({ suite: 'execution-account-processes', checks, evidenceRoot: root, actualPublicChildren: children,
  stoppedChildren: stopped, concurrentOwnerTrials: 2, actualModelRequests: 0, actualCredentialReads: 0, powerLossDurability: 'NOT_RUN' }));
