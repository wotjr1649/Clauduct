import assert from 'node:assert/strict';
import { writeFileSync, existsSync, mkdirSync } from 'node:fs';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { fork } from 'node:child_process';
import { createDevelopmentFixture, sourceHash } from '../verification/development-fixture.mjs';
import { createExecutionReservation, settleExecutionReservation } from '../verification/execution-reservation.mjs';

const project = fileURLToPath(new URL('../', import.meta.url));
const probe = join(project, 'verification', 'fixtures', 'reservation-entry-probe.mjs');
const env = Object.fromEntries(['SystemRoot', 'WINDIR'].filter(key => process.env[key]).map(key => [key, process.env[key]]));
const roots = [];
let checks = 0;
for (const mode of ['wrong-basis', 'previous-balance', 'under-reserved', 'already-settled']) {
  const fixture = createDevelopmentFixture({ waitMode: 'check', taskId: 'retry-delay-window' }); roots.push(fixture.root);
  const budget = { model: 'sol', effort: 'low', taskId: 'retry-delay-window', requestLimit: 6, phaseMs: 30000,
    maxObservedInputTokens: 131072, maxObservedOutputTokens: 32768 };
  const bytes = JSON.stringify(budget) + '\n';
  writeFileSync(join(fixture.root, 'budget.json'), bytes, { flag: 'wx' });
  writeFileSync(join(fixture.root, 'transport-development.jsonl'), '', { flag: 'wx' });
  const usage = join(fixture.root, 'usage-development'); mkdirSync(usage);
  const allowance = { attempts: 6, inputTokens: 131072, outputTokens: 32768, elapsedMs: 60000 };
  createExecutionReservation(usage, { basisHash: mode === 'wrong-basis' ? 'a'.repeat(64) : sourceHash(bytes),
    previous: { attempts: mode === 'previous-balance' ? 1 : 0, inputTokens: 0, outputTokens: 0, elapsedMs: 0 }, limits: allowance,
    allowance: { ...allowance, attempts: ['previous-balance', 'under-reserved'].includes(mode) ? 5 : 6 } });
  if (mode === 'already-settled') settleExecutionReservation(usage, { observed: allowance, evidenceHash: 'b'.repeat(64) });
  const child = fork(probe, [fixture.root], { cwd: fixture.work, env, execArgv: [], windowsHide: true,
    stdio: ['ignore', 'pipe', 'pipe', 'ipc'] });
  let out = '', errorBytes = 0, stopAttempted = false;
  child.stdout.on('data', bytes => { out += bytes.toString(); });
  child.stderr.on('data', bytes => { errorBytes += bytes.length; });
  const timer = setTimeout(() => { if (!stopAttempted) { stopAttempted = true; child.kill(); } }, 3000);
  const ended = await new Promise((done, reject) => { child.once('error', reject); child.once('close', (code, signal) => done({ code, signal })); });
  clearTimeout(timer);
  assert.equal(stopAttempted, false); assert.equal(ended.code, 0); assert.equal(errorBytes, 0); checks += 3;
  assert.ok(out.length <= 1024); checks++;
  assert.deepEqual(JSON.parse(out), { guardCode: 'INVALID_DEVELOPMENT_RESERVATION', transportStarts: 0 }); checks++;
  assert.equal(existsSync(join(usage, 'tool-usage.jsonl')), false); checks++;
}
console.log(JSON.stringify({ suite: 'native-reservation-admission', checks, fixtureRoots: roots,
  actualPublicEntryProcesses: 4, transportStarts: 0, actualModelRequests: 0, actualCredentialReads: 0 }));
