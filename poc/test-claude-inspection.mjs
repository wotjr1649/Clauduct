import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { request } from 'node:http';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import { inspectionLaunch, observeClient, CLAUDE_EXE, TASK_ROOT } from './claude-inspection.mjs';
import { FIXTURE_MARKER } from './request-inspector.mjs';

const root = dirname(fileURLToPath(import.meta.url));
let checks = 0, passed = 0, failed = 0, clients = 0;
async function test(name, action) {
  checks++;
  try { await action(); passed++; }
  catch (error) { failed++; process.stderr.write(JSON.stringify({ failure: name, error: String(error?.message ?? error).slice(0, 200), at: (error?.stack ?? '').split('\n').map(line => line.trim()).find(line => line.includes(import.meta.url.split('/').pop())) ?? null }) + '\n'); }
}
function clean(result) {
  assert.equal(result.diagnosticVersion, 2);
  assert.equal(result.resourcesClosed, true);
  assert.equal(result.actualReadVerified, false);
  assert.equal(result.observation.upstreamRequests, 0);
  assert.equal(result.observation.toolExecutions, 0);
  assert.equal(result.observation.activeSockets, 0);
  assert.equal(result.observation.activeBodies, 0);
  assert.equal(result.observation.activeRequestTimers, 0);
  for (const value of ['SYNTHETIC_PRIVATE_VALUE', 'PRIVATE_HEADER'])
    assert.equal(JSON.stringify(result).includes(value), false);
}
function client(port, mode) {
  clients++;
  const { options } = inspectionLaunch(port, process.env);
  return spawn(process.execPath, ['--permission', `--allow-fs-read=${root}`,
    join(root, 'inspection-test-client.mjs'), String(port), mode], { ...options, cwd: root });
}
const limits = { lifetimeMs: 2500, observationMs: 100, requestMs: 1000 };
await test('fixed_launch_no_shell_or_permission_bypass', () => {
  const spec = inspectionLaunch(12345, { Path: 'native-path', USERPROFILE: 'native-home' });
  assert.equal(spec.file, CLAUDE_EXE); assert.equal(spec.options.cwd, TASK_ROOT);
  assert.equal(spec.options.shell, false); assert.equal(spec.options.windowsHide, true);
  assert.equal(spec.options.env.PATH, 'native-path');
  const settings = JSON.parse(spec.args[spec.args.indexOf('--settings') + 1]);
  assert.equal(settings.env.ANTHROPIC_AUTH_TOKEN, FIXTURE_MARKER);
  assert.equal(settings.env.ANTHROPIC_BASE_URL, 'http://127.0.0.1:12345');
  assert.equal(settings.env.HTTPS_PROXY, settings.env.ANTHROPIC_BASE_URL);
  assert.equal(settings.env.NO_PROXY, '127.0.0.1');
  assert.equal(spec.args.includes('--no-session-persistence'), true);
  for (const flag of ['--bare', '--safe-mode', '--settings-sources', '--dangerously-skip-permissions', '--allowedTools'])
    assert.equal(spec.args.includes(flag), false);
});
await test('parent_secrets_not_read_or_modified', () => {
  const source = { USERPROFILE: 'native-home' };
  for (const key of ['CLAUDE_CODE_OAUTH_TOKEN', 'ANTHROPIC_AUTH_TOKEN', 'OPENAI_API_KEY', 'NODE_OPTIONS'])
    Object.defineProperty(source, key, { enumerable: true, get() { throw new Error('SECRET_READ'); } });
  const spec = inspectionLaunch(1, Object.freeze(source));
  assert.equal(spec.options.env.CLAUDE_CODE_OAUTH_TOKEN, '');
  assert.equal(spec.options.env.OPENAI_API_KEY, undefined);
  assert.equal(spec.options.env.NODE_OPTIONS, undefined);
});
await test('invalid_port_rejected', () => {
  for (const port of [0, 65536, -1, '1234', NaN]) assert.throws(() => inspectionLaunch(port, {}));
});
await test('ambiguous_windows_env_rejected', () => {
  assert.throws(() => inspectionLaunch(1, { PATH: 'a', Path: 'b' }));
});
for (const [mode, category] of [['capture', 'REQUEST_SHAPE_CAPTURED'], ['repeat', 'REQUEST_SHAPE_CAPTURED'],
  ['reject', 'NO_MESSAGE_CAPTURED'], ['early', 'NO_MESSAGE_CAPTURED'], ['noise', 'CLIENT_OUTPUT_LIMIT']]) {
  await test(`subprocess_${mode}`, async () => {
    const result = await observeClient(port => client(port, mode), { inspectorOptions: limits });
    assert.equal(result.category, category); assert.equal(result.clientStarted, true);
    assert.equal(result.clientClosed, true); clean(result);
    if (mode === 'capture') {
      assert.equal(result.observation.counters.captured, 1);
      assert.equal(result.observation.records[0].shape.transportTokenLimit, 'explicit-unsupported');
    }
    if (mode === 'repeat') assert.equal(result.observation.counters.captured, 2);
    if (mode === 'early') assert.equal(result.clientExitCode, 7);
  });
}
await test('lifetime_stops_real_idle_child', async () => {
  const result = await observeClient(port => client(port, 'idle'), {
    inspectorOptions: { ...limits, lifetimeMs: 300 } });
  assert.equal(result.category, 'NO_MESSAGE_CAPTURED'); assert.equal(result.clientClosed, true);
  assert.equal(result.observation.closeReason, 'SESSION_TIMEOUT'); clean(result);
});
await test('cancellation_stops_real_child', async () => {
  const controller = new AbortController(); let timer;
  try {
    const result = await observeClient(port => {
      const process = client(port, 'idle'); process.once('spawn', () => {
        timer = setTimeout(() => controller.abort(), 20);
      }); return process;
    }, { signal: controller.signal, inspectorOptions: limits });
    assert.equal(result.category, 'USER_CANCELLED'); assert.equal(result.clientClosed, true); clean(result);
  } finally { clearTimeout(timer); }
});
await test('pre_cancel_never_spawns', async () => {
  const result = await observeClient(() => { throw new Error('MUST_NOT_SPAWN'); },
    { signal: AbortSignal.abort(), inspectorOptions: limits });
  assert.equal(result.category, 'USER_CANCELLED'); assert.equal(result.clientStarted, false); clean(result);
});
await test('spawn_failure_is_sanitized', async () => {
  const result = await observeClient(port => {
    const { options } = inspectionLaunch(port, process.env);
    return spawn(join(root, 'nonexistent-inspection-client.exe'), [], options);
  }, { inspectorOptions: limits });
  assert.equal(result.category, 'CLIENT_START_OR_IO_FAILED'); clean(result);
});
await test('synchronous_start_failure_closes_listener', async () => {
  let port;
  await assert.rejects(observeClient(value => { port = value; throw new Error('SYNTHETIC_PRIVATE_VALUE'); },
    { inspectorOptions: limits }));
  await new Promise((resolveDone, reject) => {
    const req = request({ hostname: '127.0.0.1', port, agent: false, signal: AbortSignal.timeout(1000) }, () => reject());
    req.once('error', error => error.code === 'ECONNREFUSED' ? resolveDone() : reject()); req.end();
  });
});
process.stdout.write(JSON.stringify({ suite: 'claude-inspection', checks, passed, failed,
  syntheticClients: clients, actualClaudeExecutions: 0, codexRequests: 0 }) + '\n');
process.exitCode = failed ? 1 : 0;
