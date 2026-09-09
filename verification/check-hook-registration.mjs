// Metadata only: no thread, model turn, trust write, or hook bypass.
import { spawn, spawnSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';
import assert from 'node:assert/strict';

const cwd = fileURLToPath(new URL('../', import.meta.url));
const executable = process.argv[2];
assert(executable && /^[A-Za-z]:[\\/].*codex\.exe$/i.test(executable), 'Absolute codex.exe path required');
const hookPath = fileURLToPath(new URL('tool-allowlist.mjs', import.meta.url));
const command = [process.execPath, hookPath].map(path => {
  assert(!/["\r\n]/.test(path), 'Unsafe command path');
  return `"${path.replaceAll('\\', '/')}"`;
}).join(' ');
const statusMessage = 'Clauduct verification allowlist';
const override = `hooks.PreToolUse=[{matcher=".*",hooks=[{type="command",command=${JSON.stringify(command)},timeout=5,statusMessage=${JSON.stringify(statusMessage)}}]}]`;

async function inspect(extraArgs) {
  const child = spawn(executable, ['app-server', ...extraArgs], { cwd, windowsHide: true, stdio: ['pipe', 'pipe', 'pipe'] });
  const pending = new Map();
  let sequence = 0, size = 0, stderrSize = 0, buffer = '', fatal;
  const closed = new Promise(resolve => child.once('close', resolve));
  function fail(message) {
    fatal ??= new Error(message);
    for (const entry of pending.values()) entry.reject(fatal);
    pending.clear();
  }
  child.on('error', () => fail('App-server startup failed (details withheld)'));
  child.on('close', () => fail('App-server closed'));
  child.stdin.on('error', () => fail('App-server input failed'));
  child.stderr.on('data', chunk => { if ((stderrSize += chunk.length) > 1024 * 1024) fail('stderr bound exceeded'); });
  child.stdout.setEncoding('utf8');
  child.stdout.on('data', chunk => {
    if ((size += Buffer.byteLength(chunk)) > 4 * 1024 * 1024) return fail('stdout bound exceeded');
    buffer += chunk;
    let end;
    while ((end = buffer.indexOf('\n')) >= 0) {
      const line = buffer.slice(0, end); buffer = buffer.slice(end + 1);
      if (!line.trim()) continue;
      let message;
      try { message = JSON.parse(line); } catch { return fail('Invalid RPC JSON'); }
      if (message.method && message.id !== undefined) return fail('Unexpected server request; no approvals granted');
      const entry = pending.get(message.id);
      if (!entry) continue;
      pending.delete(message.id);
      if (message.error) entry.reject(new Error(`RPC error ${Number(message.error.code)}`));
      else entry.resolve(message.result);
    }
  });
  function rpc(method, params) {
    if (fatal) return Promise.reject(fatal);
    const id = ++sequence;
    return new Promise((resolve, reject) => {
      pending.set(id, { resolve, reject });
      child.stdin.write(JSON.stringify({ id, method, params }) + '\n');
    });
  }
  const deadline = setTimeout(() => fail('Metadata check exceeded 20 seconds'), 20000);
  try {
    await rpc('initialize', { clientInfo: { name: 'clauduct_hook_check', version: '0.0.0' }, capabilities: { experimentalApi: true } });
    child.stdin.write(JSON.stringify({ method: 'initialized' }) + '\n');
    const result = await rpc('hooks/list', { cwds: [cwd] });
    assert(result.data?.length === 1, 'Unexpected hooks/list result');
    return result.data[0];
  } finally {
    clearTimeout(deadline);
    child.stdin.end();
    let grace;
    const exited = await Promise.race([closed.then(() => true), new Promise(resolve => { grace = setTimeout(() => resolve(false), 2000); })]);
    clearTimeout(grace);
    if (!exited && child.pid) {
      const killed = spawnSync('C:\\Windows\\System32\\taskkill.exe', ['/PID', String(child.pid), '/T', '/F'], { windowsHide: true, stdio: 'ignore', timeout: 5000 });
      assert.equal(killed.status, 0, 'Task process cleanup failed');
      throw new Error('Metadata process did not exit normally');
    }
  }
}

try {
  const baseline = await inspect([]);
  const augmented = await inspect(['-c', override]);
  // Compare sensitive hook metadata in memory; never print commands or hashes.
  const signature = hook => JSON.stringify([hook.source, hook.sourcePath, hook.eventName, hook.currentHash, hook.enabled, hook.trustStatus]);
  const remaining = augmented.hooks.map(signature);
  const existingHooksPreserved = baseline.hooks.every(hook => {
    const index = remaining.indexOf(signature(hook));
    if (index < 0) return false;
    remaining.splice(index, 1); return true;
  });
  const added = augmented.hooks.filter(hook => hook.command === command && hook.statusMessage === statusMessage);
  const ready = existingHooksPreserved && !baseline.errors.length && !augmented.errors.length
    && added.length === 1 && added[0].enabled && ['trusted', 'managed'].includes(added[0].trustStatus);
  console.log(JSON.stringify({ baselineHooks: baseline.hooks.length, augmentedHooks: augmented.hooks.length,
    existingHooksPreserved, addedHooks: added.length, addedHookSource: added[0]?.source ?? null,
    addedHookTrust: added[0]?.trustStatus ?? null, addedHookEnabled: added[0]?.enabled ?? null,
    discoveryErrors: baseline.errors.length + augmented.errors.length,
    discoveryWarnings: baseline.warnings.length + augmented.warnings.length,
    readyForIntegration: ready, modelRequestSent: false, globalSettingsWritten: false }, null, 2));
  if (!ready) process.exitCode = 2;
} catch {
  console.log(JSON.stringify({ failed: true, reason: 'Hook metadata check failed; details withheld', modelRequestSent: false }));
  process.exitCode = 1;
}
