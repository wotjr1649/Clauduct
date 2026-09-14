import assert from 'node:assert/strict';
import { mkdtempSync, mkdirSync, readFileSync } from 'node:fs';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { spawn } from 'node:child_process';
import { readExecutionReservation } from '../verification/execution-reservation.mjs';

const project = fileURLToPath(new URL('../', import.meta.url));
const root = mkdtempSync(join(project, '.tmp', 'execution-reservation-crash-'));
const worker = join(project, 'verification', 'fixtures', 'reservation-worker.mjs');
const module = join(project, 'verification', 'execution-reservation.mjs');
const env = Object.fromEntries(['SystemRoot', 'WINDIR'].filter(key => process.env[key]).map(key => [key, process.env[key]]));
let checks = 0;
for (const mode of ['reserved', 'torn-settlement', 'settled']) {
  const directory = join(root, mode); mkdirSync(directory);
  const child = spawn(process.execPath, ['--permission', `--allow-fs-read=${worker}`, `--allow-fs-read=${module}`,
    `--allow-fs-read=${directory}`, `--allow-fs-write=${directory}`, worker, directory, mode],
  { cwd: directory, env, windowsHide: true, stdio: ['ignore', 'pipe', 'pipe'] });
  let finished = false, stopAttempted = false, text = '', errorBytes = 0, timer;
  const closed = new Promise(done => child.once('close', (code, signal) => { finished = true; done({ code, signal }); }));
  child.stderr.on('data', data => { errorBytes += data.length; });
  const stop = () => { if (!finished && !stopAttempted) { stopAttempted = true; child.kill(); } };
  try {
    await new Promise((done, reject) => {
      timer = setTimeout(() => reject(new Error('RESERVATION_WORKER_READY_TIMEOUT')), 3000);
      child.once('error', reject);
      child.stdout.on('data', data => {
        text += data.toString();
        if (text.length > 1024) reject(new Error('RESERVATION_WORKER_OUTPUT_LIMIT'));
        else if (text === 'PUBLIC_RESERVATION_READY\n') done();
      });
    });
    clearTimeout(timer);
    const before = readFileSync(join(directory, 'execution-reservation.json'));
    stop();
    const ended = await Promise.race([closed, new Promise((_, reject) => {
      timer = setTimeout(() => reject(new Error('RESERVATION_WORKER_STOP_UNVERIFIED')), 3000);
    })]);
    clearTimeout(timer);
    assert.equal(ended.signal, 'SIGTERM'); assert.equal(errorBytes, 0); checks += 2;
    const state = readExecutionReservation(directory);
    assert.deepEqual(readFileSync(join(directory, 'execution-reservation.json')), before); checks++;
    assert.equal(state.settlementState, mode === 'reserved' ? 'missing' : mode === 'torn-settlement' ? 'invalid' : 'recorded'); checks++;
    assert.deepEqual(state.charged, mode === 'settled'
      ? { attempts: 15, inputTokens: 250, outputTokens: 50, elapsedMs: 3000 }
      : { attempts: 16, inputTokens: 1100, outputTokens: 220, elapsedMs: 21000 }); checks++;
    assert.equal(state.authorizesExecution, false); checks++;
  } finally {
    clearTimeout(timer); stop();
    if (!finished) {
      await Promise.race([closed, new Promise((_, reject) => { timer = setTimeout(() => reject(new Error('RESERVATION_WORKER_STOP_UNVERIFIED')), 3000); })]);
      clearTimeout(timer);
    }
  }
}
console.log(JSON.stringify({ suite: 'execution-reservation-crash', checks, evidenceRoot: root,
  actualPublicChildren: 3, stoppedChildren: 3, actualModelRequests: 0, actualCredentialReads: 0, powerLossDurability: 'NOT_RUN' }));
