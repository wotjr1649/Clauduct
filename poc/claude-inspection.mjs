// User-operated Claude request observation. No Codex transport or credential loader.
import { spawn } from 'node:child_process';
import { resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { startInspector, FIXTURE_MARKER } from './request-inspector.mjs';
import { ALIAS } from './adapter.mjs';
import { CLAUDE_EXE } from '../src/runtime-paths.mjs';

export { CLAUDE_EXE };
export const TASK_ROOT = resolve(fileURLToPath(new URL('../', import.meta.url)));
const outputLimit = 256 * 1024;
const inheritedNames = ['SystemRoot', 'WINDIR', 'ComSpec', 'PATH', 'PATHEXT', 'TEMP', 'TMP',
  'USERPROFILE', 'HOMEDRIVE', 'HOMEPATH', 'APPDATA', 'LOCALAPPDATA', 'ProgramData',
  'ProgramFiles', 'ProgramFiles(x86)', 'OS', 'PROCESSOR_ARCHITECTURE'];

export function inspectionLaunch(port, source) {
  if (!Number.isInteger(port) || port < 1 || port > 65535) throw new Error('INVALID_PORT');
  const env = {};
  // Read only named OS/runtime fields. Never copy the parent's OAuth or arbitrary env.
  for (const name of inheritedNames) {
    const keys = Object.keys(source).filter(key => key.toLowerCase() === name.toLowerCase());
    if (keys.length > 1) throw new Error('AMBIGUOUS_ENVIRONMENT');
    if (keys.length && typeof source[keys[0]] === 'string') env[name] = source[keys[0]];
  }
  const connection = { ANTHROPIC_BASE_URL: `http://127.0.0.1:${port}`,
    ANTHROPIC_AUTH_TOKEN: FIXTURE_MARKER, ANTHROPIC_API_KEY: '', CLAUDE_CODE_OAUTH_TOKEN: '',
    ANTHROPIC_CUSTOM_HEADERS: '',
    // The inspector rejects proxy/CONNECT traffic; only loopback bypasses the proxy.
    HTTP_PROXY: `http://127.0.0.1:${port}`, HTTPS_PROXY: `http://127.0.0.1:${port}`,
    ALL_PROXY: `http://127.0.0.1:${port}`, NO_PROXY: '127.0.0.1',
    CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC: '1' };
  Object.assign(env, connection);
  return { file: CLAUDE_EXE,
    args: ['-p', '--model', ALIAS, '--tools', 'Read', '--disallowedTools', 'mcp__*',
      '--max-turns', '1', '--no-session-persistence', '--settings', JSON.stringify({ env: connection }),
      'This is a local request-shape inspection. Ask to read a text fixture; do not execute any tool.'],
    options: { cwd: TASK_ROOT, env, shell: false, windowsHide: true, stdio: ['ignore', 'pipe', 'pipe'] } };
}

// The caller starts exactly one client; tests use a real, restricted synthetic subprocess.
export async function observeClient(startClient, { signal, inspectorOptions = {}, cleanupMs = 2000 } = {}) {
  if (!Number.isInteger(cleanupMs) || cleanupMs < 1 || cleanupMs > 2000) throw new Error('INVALID_LIMIT');
  const inspector = await startInspector(inspectorOptions);
  let child, closed = false, started = false, exitCode = null, bytes = 0, failure, finishChild;
  const childDone = new Promise(resolveDone => { finishChild = resolveDone; });
  const stop = () => { void inspector.close(); };
  const cancel = () => { failure ??= 'USER_CANCELLED'; stop(); };
  let cleanupTimer;
  try {
    if (signal?.aborted) cancel();
    else {
      signal?.addEventListener('abort', cancel, { once: true });
      child = startClient(inspector.port);
      child.once('spawn', () => { started = true; });
      child.once('error', () => { failure ??= 'CLIENT_START_OR_IO_FAILED'; stop(); });
      child.once('close', code => {
        closed = true; exitCode = Number.isInteger(code) && code >= 0 && code <= 255 ? code : null;
        finishChild(); stop();
      });
      for (const stream of [child.stdout, child.stderr]) {
        stream.on('error', () => { failure ??= 'CLIENT_OUTPUT_FAILED'; stop(); });
        stream.on('data', chunk => {
          bytes = Math.min(outputLimit + 1, bytes + chunk.length);
          if (bytes > outputLimit) { failure ??= 'CLIENT_OUTPUT_LIMIT'; stop(); }
        });
      }
    }
    const observation = await inspector.done;
    if (child && !closed) {
      try { if (!child.kill()) failure ??= 'CLIENT_STOP_FAILED'; }
      catch { failure ??= 'CLIENT_STOP_FAILED'; }
      await Promise.race([childDone, new Promise(resolveTimeout => {
        cleanupTimer = setTimeout(resolveTimeout, cleanupMs);
      })]);
    }
    const resourcesClosed = observation.activeSockets === 0 && observation.activeBodies === 0
      && observation.activeRequestTimers === 0 && (!child || closed);
    const captured = observation.counters.captured > 0;
    const category = !resourcesClosed ? 'CLEANUP_FAILED' : failure
      ?? (captured ? 'REQUEST_SHAPE_CAPTURED' : 'NO_MESSAGE_CAPTURED');
    return { diagnosticVersion: 2, category, requestCaptured: captured, resourcesClosed,
      clientStarted: started, clientClosed: closed, clientExitCode: exitCode,
      discardedOutputBytes: bytes, actualReadVerified: false, observation };
  } finally {
    clearTimeout(cleanupTimer); signal?.removeEventListener('abort', cancel);
    await inspector.close();
    if (child && !closed) {
      child.stdout.destroy(); child.stderr.destroy(); child.unref();
    }
  }
}

async function main() {
  if (process.argv.length !== 3 || process.argv[2] !== '--inspect'
    || !process.stdin.isTTY || !process.stdout.isTTY) throw new Error('USER_TERMINAL_REQUIRED');
  if (process.platform !== 'win32' || process.execArgv.length || process.env.NODE_OPTIONS || process.env.NODE_DEBUG)
    throw new Error('UNSUPPORTED_RUNTIME');
  const controller = new AbortController();
  const cancel = () => controller.abort();
  process.once('SIGINT', cancel); process.once('SIGTERM', cancel);
  try {
    process.stdout.write('Claude 요청 관찰: 최대 60초, 공개 marker, Codex 전송 없음. Claude 출력 원문은 표시하지 않습니다.\n');
    const result = await observeClient(port => {
      const launch = inspectionLaunch(port, process.env);
      return spawn(launch.file, launch.args, launch.options);
    }, { signal: controller.signal });
    process.stdout.write(JSON.stringify(result) + '\n');
    process.exitCode = result.category === 'REQUEST_SHAPE_CAPTURED' ? 0 : 1;
  } finally {
    process.removeListener('SIGINT', cancel); process.removeListener('SIGTERM', cancel);
  }
}
if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  try { await main(); }
  catch (error) {
    const category = ['USER_TERMINAL_REQUIRED', 'UNSUPPORTED_RUNTIME'].includes(error.message)
      ? error.message : 'INSPECTION_START_FAILED';
    process.stdout.write(JSON.stringify({ category, requestCaptured: false }) + '\n'); process.exitCode = 1;
  }
}
