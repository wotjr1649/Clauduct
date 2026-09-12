import assert from 'node:assert/strict';
import { spawn, spawnSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';
import { dirname } from 'node:path';
import { launchOptions } from './clauduct.mjs';
import { checkUserContext } from '../poc/user-session.mjs';

const self = fileURLToPath(import.meta.url), root = dirname(dirname(self));
const launcher = fileURLToPath(new URL('./clauduct.mjs', import.meta.url));
if (process.argv[2] === '--synthetic-client') {
  const failed = process.argv[3] === 'failure';
  process.stdout.write(JSON.stringify({ type: 'result', is_error: failed, result: 'SYNTHETIC_RESULT' }) + '\n');
  process.exitCode = failed ? 7 : 0;
} else if (process.argv[2] === '--synthetic-launcher') {
  const { main } = await import('./clauduct.mjs');
  await main({ args: ['-p', '--output-format', 'json', 'SYNTHETIC_PROMPT'],
    openTransport: () => ({ send: async () => { throw new Error('NO_MODEL_REQUEST_EXPECTED'); },
      close: async () => {}, diagnostics: () => ({ activeSockets: 0, activeRequests: 0, requestAttempts: 0 }) }),
    startClient: (_file, _args, options) => spawn(process.execPath,
      [self, '--synthetic-client', process.argv[3]], { ...options, stdio: ['ignore', 'inherit', 'inherit'] }) });
} else {
  let checks = 0;
  for (const flag of ['-p', '--print']) {
    const options = launchOptions([flag, '--output-format', 'json', 'SYNTHETIC_PROMPT']);
    assert.equal(options.print, true);
    assert.deepEqual(options.forward, [flag, '--output-format', 'json', 'SYNTHETIC_PROMPT']); checks++;
    assert.throws(() => launchOptions([`${flag}=true`]), /INVALID_ARGUMENTS/); checks++;
  }
  for (const args of [[], ['--', '-p'], ['--name', '-p']]) {
    assert.notEqual(launchOptions(args).print, true); checks++;
  }
  const context = { stdinTTY: false, stdoutTTY: false, env: {}, execArgs: [] };
  const locked = launchOptions(['-p', '--model', 'luna', '--effort', 'max', '--verify-model-route']);
  assert.equal(locked.verifyModelRoute, true);
  assert.equal(locked.forward.includes('--verify-model-route'), false); checks++;
  assert.throws(() => launchOptions(['--verify-model-route=true']), /INVALID_ARGUMENTS/); checks++;
  assert.throws(() => launchOptions(['--verify-model-route', '--verify-model-route']), /INVALID_ARGUMENTS/); checks++;
  assert.throws(() => checkUserContext(context), /USER_TERMINAL_REQUIRED/); checks++;
  checkUserContext({ ...context, nonInteractive: true }); checks++;
  assert.throws(() => checkUserContext({ ...context, nonInteractive: 'true' }), /INVALID_ARGUMENTS/); checks++;
  for (const [change, code] of [
    [{ env: { NODE_DEBUG: 'http' } }, 'DEBUG_RUNTIME_UNSUPPORTED'],
    [{ env: { NODE_OPTIONS: '--inspect' } }, 'DEBUG_RUNTIME_UNSUPPORTED'],
    [{ execArgs: ['--inspect'] }, 'DEBUG_RUNTIME_UNSUPPORTED'],
    [{ env: { NODE_USE_ENV_PROXY: '1' } }, 'TRANSPORT_RUNTIME_UNSUPPORTED'],
    [{ env: { NODE_TLS_REJECT_UNAUTHORIZED: '0' } }, 'TRANSPORT_RUNTIME_UNSUPPORTED'],
    [{ env: { CODEX_HOME: 'D:/synthetic-other' } }, 'UNEXPECTED_CODEX_HOME']
  ]) {
    assert.throws(() => checkUserContext({ ...context, nonInteractive: true, ...change }),
      error => error.code === code || error.message === code); checks++;
  }
  const childOptions = { cwd: root, env: { ...process.env }, encoding: 'utf8', timeout: 10000,
    maxBuffer: 256 * 1024, windowsHide: true, shell: false };
  const dry = spawnSync(process.execPath, [launcher, '--dry-run', '-p', '--output-format', 'json'], childOptions);
  assert.equal(dry.error, undefined); assert.equal(dry.status, 0);
  assert.equal(JSON.parse(dry.stdout).mode, 'print'); checks++;
  for (const outcome of ['success', 'failure']) {
    const result = spawnSync(process.execPath, [self, '--synthetic-launcher', outcome], childOptions);
    assert.equal(result.error, undefined);
    assert.equal(result.status, outcome === 'success' ? 0 : 1, result.stderr);
    assert.deepEqual(JSON.parse(result.stdout), { type: 'result', is_error: outcome === 'failure', result: 'SYNTHETIC_RESULT' });
    assert.match(result.stderr, /CLAUDUCT_REQUEST_STATUS /);
    const statusLine = result.stderr.split(/\r?\n/).find(line => line.startsWith('CLAUDUCT_REQUEST_STATUS '));
    const status = JSON.parse(statusLine.slice('CLAUDUCT_REQUEST_STATUS '.length));
    assert.equal(status.requestOutcome, 'no-requests');
    assert.ok(Object.values(status.cleanup).every(value => value === true)); checks++;
  }
  console.log(JSON.stringify({ suite: 'headless', checks, externalRequests: 0, actualCredentialReads: 0,
    actualClaudeExecutions: 0 }));
}
