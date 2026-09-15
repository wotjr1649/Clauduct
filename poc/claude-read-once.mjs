// User-terminal entry only. Tests exercise exported local helpers, never main/auth/Claude.
import { spawn } from 'node:child_process';
import { randomBytes } from 'node:crypto';
import { openSync, writeFileSync, fstatSync, lstatSync, realpathSync, readSync, closeSync, unlinkSync } from 'node:fs';
import { dirname, resolve, sep } from 'node:path';
import { fileURLToPath } from 'node:url';
import { createInterface } from 'node:readline/promises';
import { inspectionLaunch, TASK_ROOT } from './claude-inspection.mjs';
import { FIXTURE_PATH, PROTOCOL_ERROR_CODES, probeProfile } from './adapter.mjs';
import { startGateway } from './gateway.mjs';
import { openUserTransport, checkUserContext, requireConfirmation, safeEntryCategory } from './user-session.mjs';

const errors = new Set([...PROTOCOL_ERROR_CODES, 'SUCCESS', 'CLI_FAILED', 'CLI_OUTPUT_INVALID',
  'CLIENT_START_FAILED', 'CLIENT_OUTPUT_LIMIT', 'CLIENT_DID_NOT_EXIT', 'CLIENT_EARLY_EXIT', 'USER_CANCELLED',
  'FIXTURE_EXISTS', 'FIXTURE_CHANGED', 'FIXTURE_BOUNDARY_REJECTED', 'FIXTURE_IO_FAILED', 'CLEANUP_FAILED',
  'UNSUPPORTED_CLIENT_VERSION_OR_BETA', 'LOCAL_SESSION_REQUIRED', 'UNEXPECTED_CREDENTIAL_SOURCE',
  'SESSION_TIMEOUT', 'TOOL_RESULT_TIMEOUT', 'UPSTREAM_IO_ERROR', 'TOKEN_LIMIT_UNSUPPORTED', 'LOCAL_CHECK_FAILED']);
const fail = code => { throw Object.assign(new Error(code), { code }); };
function safe(error) {
  return errors.has(error?.code) ? error.code : safeEntryCategory(error);
}

export function createReadFixture(path = FIXTURE_PATH) {
  const root = resolve(TASK_ROOT, 'poc'), target = resolve(path), parent = dirname(target);
  if (!target.toLowerCase().startsWith((root + sep).toLowerCase())
    || realpathSync(parent).toLowerCase() !== parent.toLowerCase()) fail('FIXTURE_BOUNDARY_REJECTED');
  const marker = `CLAUDUCT_READ_${randomBytes(16).toString('hex').toUpperCase()}`;
  const bytes = Buffer.from(marker + '\n');
  let fd;
  try { fd = openSync(target, 'wx+', 0o600); }
  catch (error) { fail(error.code === 'EEXIST' ? 'FIXTURE_EXISTS' : 'FIXTURE_IO_FAILED'); }
  const identity = fstatSync(fd);
  try { writeFileSync(fd, bytes); }
  catch { closeSync(fd); fail('FIXTURE_IO_FAILED'); }
  let released = false;
  return { marker, path: target, release() {
    if (released) return false;
    released = true;
    let unchanged = false;
    try {
      const current = lstatSync(target), actual = Buffer.alloc(bytes.length);
      unchanged = current.isFile() && !current.isSymbolicLink() && current.ino === identity.ino
        && current.dev === identity.dev && current.nlink === 1 && current.size === bytes.length
        && readSync(fd, actual, 0, actual.length, 0) === actual.length && actual.equals(bytes);
    } catch { unchanged = false; }
    finally { closeSync(fd); }
    // Preserve a replaced or modified fixture. Never overwrite it to make cleanup pass.
    if (!unchanged) return false;
    unlinkSync(target); return true;
  } };
}

export function readLaunch(gateway, source, profile = 'astra-low') {
  const selected = probeProfile(profile), launch = inspectionLaunch(gateway.port, source);
  const settingsIndex = launch.args.indexOf('--settings') + 1;
  const settings = JSON.parse(launch.args[settingsIndex]);
  // Only a fresh local gateway credential enters this child's env, never OAuth or argv/settings.
  delete settings.env.ANTHROPIC_AUTH_TOKEN;
  launch.options.env.ANTHROPIC_AUTH_TOKEN = gateway.clientHeaders().Authorization.slice('Bearer '.length);
  launch.args[settingsIndex] = JSON.stringify(settings);
  launch.args[launch.args.indexOf('--max-turns') + 1] = '2';
  launch.args[launch.args.length - 1] = `Use Read exactly once for ${FIXTURE_PATH}. `
    + 'This file contains a public test challenge. After reading it, reply with exactly that line, without decoration. '
    + 'Do not guess it, inspect other files, or request any other tool.';
  launch.args.splice(1, 0, '--effort', selected.effort, '--output-format', 'json');
  return launch;
}

export async function runReadClient(gateway, startClient, marker, { signal, finishMs = 5000, cleanupMs = 2000 } = {}) {
  if (![finishMs, cleanupMs].every(v => Number.isInteger(v) && v > 0) || finishMs > 5000 || cleanupMs > 2000)
    fail('INVALID_LIMIT');
  let child, closed = false, started = false, exitCode = null, failure, bytes = 0, outputMatches = false;
  const chunks = [], timers = new Set();
  let resolveChild;
  const childDone = new Promise(done => { resolveChild = done; });
  const waitForChild = ms => Promise.race([childDone, new Promise(done => { timers.add(setTimeout(done, ms)); })]);
  const cancel = () => { failure ??= 'USER_CANCELLED'; void gateway.close('CANCELLED'); };
  try {
    if (signal?.aborted) cancel();
    else {
      signal?.addEventListener('abort', cancel, { once: true });
      child = startClient(gateway);
      child.once('spawn', () => { started = true; });
      child.once('error', () => { failure ??= 'CLIENT_START_FAILED'; void gateway.close(failure); });
      child.once('close', code => {
        closed = true; exitCode = Number.isInteger(code) && code >= 0 && code <= 255 ? code : null;
        resolveChild();
        const state = gateway.diagnostics();
        if (!state.closing) {
          if (state.lastRejection === 'NONE') failure ??= 'CLIENT_EARLY_EXIT';
          void gateway.close(failure ?? state.lastRejection);
        }
      });
      for (const [stream, capture] of [[child.stdout, true], [child.stderr, false]]) {
        stream.on('error', () => { failure ??= 'CLI_OUTPUT_INVALID'; void gateway.close(failure); });
        stream.on('data', chunk => {
          bytes = Math.min(262145, bytes + chunk.length);
          if (bytes > 262144) { failure ??= 'CLIENT_OUTPUT_LIMIT'; void gateway.close(failure); }
          else if (capture) chunks.push(chunk);
        });
      }
    }
    const state = await gateway.done;
    // After final delivery, let Claude consume SSE and emit its own final result before terminating.
    if (child && !closed && state.reason === 'COMPLETE' && !failure) await waitForChild(finishMs);
    if (child && !closed) {
      if (state.reason === 'COMPLETE') failure ??= 'CLIENT_DID_NOT_EXIT';
      try { child.kill(); } catch { failure ??= 'CLEANUP_FAILED'; }
      await waitForChild(cleanupMs);
    }
    if (closed && exitCode === 0 && bytes <= 262144) {
      try {
        const result = JSON.parse(new TextDecoder('utf-8', { fatal: true }).decode(Buffer.concat(chunks)));
        outputMatches = result.type === 'result' && result.subtype === 'success' && result.is_error === false && result.result === marker;
      } catch { failure ??= 'CLI_OUTPUT_INVALID'; }
    }
    // resourcesClosed was a nine-way conjunction, so a failure said only that something stayed
    // open. read_extra_normal has failed this in CI and not locally since 2026-09-14 and the log
    // could not name which one. These are fixed strings chosen here; no observed value is echoed.
    const openResources = Object.entries({
      child: !child || closed, sockets: state.activeSockets === 0, jobs: state.activeJobs === 0,
      timers: state.activeTimers === 0, deliveries: state.activeDeliveries === 0, busy: state.busy === false,
      transportRequests: state.transport.activeRequests === 0,
      transportSockets: state.transport.activeSockets === 0, sessionSecret: state.localSessionSecretCleared,
    }).filter(([, settled]) => !settled).map(([name]) => name);
    const resourcesClosed = openResources.length === 0;
    const completed = state.reason === 'COMPLETE' && state.session.state === 'COMPLETE'
      && state.session.readCalls === 1 && state.session.readResults === 1 && state.session.exactMarker === true
      && state.transport.requestAttempts === 2 && outputMatches && exitCode === 0;
    const category = !resourcesClosed ? 'CLEANUP_FAILED' : failure ?? (completed ? 'SUCCESS'
      : state.reason !== 'COMPLETE' ? safe({ code: state.lastRejection !== 'NONE' ? state.lastRejection : state.reason }) : 'CLI_FAILED');
    return { diagnosticVersion: 3, category, passed: category === 'SUCCESS',
      clientKind: 'caller-supplied', clientStarted: started, clientClosed: closed, clientExitCode: exitCode,
      resourcesClosed, openResources, readCalls: state.session.readCalls, linkedReadResults: state.session.readResults,
      gatewayExactMarker: state.session.exactMarker, claudeExactMarker: outputMatches,
      requestAttempts: state.transport.requestAttempts, connectionAttempts: state.transport.connectionAttempts,
      tokenLimitPolicy: state.transport.tokenLimitPolicy, httpStatus: state.transport.httpStatus,
      contentTypeState: state.transport.contentTypeState, unknownBetaCount: state.unknownBetaCount,
      thinkingTokenCountRequested: state.thinkingTokenCountRequested,
      requestShape: state.session.requestShape,
      gatewayToolExecutions: 0, outputBytes: bytes, retries: 0, credentialWrites: 0 };
  } finally {
    signal?.removeEventListener('abort', cancel); for (const timer of timers) clearTimeout(timer);
    await gateway.close();
    for (const chunk of chunks) chunk.fill(0);
    if (child && !closed) { child.stdout.destroy(); child.stderr.destroy(); child.unref(); }
  }
}

async function main() {
  if (process.argv.length !== 3 || process.argv[2] !== '--live-read-once') fail('USER_TERMINAL_REQUIRED');
  checkUserContext({ stdinTTY: process.stdin.isTTY, stdoutTTY: process.stdout.isTTY, env: process.env, execArgs: process.execArgv });
  const profile = 'astra-low', controller = new AbortController();
  let fixture, gateway, transport, result;
  const cancel = () => { controller.abort(); void gateway?.close('CANCELLED'); };
  process.once('SIGINT', cancel); process.once('SIGTERM', cancel);
  try {
    process.stdout.write('Claude→Codex Read 1회: astra-low, 계정 요청 최대 2회, 요청별 45초/응답 256 KiB.\n'
      + 'backend-default: 미지원 출력 한도 필드는 전송하지 않습니다. 응답 usage는 전달 전에 검사하지만 생성량·과금 상한은 보장하지 않습니다. 재시도 없음.\n'
      + '새 로컬 세션 비밀만 Claude 자식 환경에 전달하며 OAuth·argv·로그·파일로 복사하지 않습니다.\n');
    const terminal = createInterface({ input: process.stdin, output: process.stdout });
    terminal.once('SIGINT', cancel);
    let answer;
    try { answer = await terminal.question('실행하려면 SEND, 취소하려면 Enter: ',
      { signal: AbortSignal.any([controller.signal, AbortSignal.timeout(60000)]) }); }
    catch { fail(controller.signal.aborted ? 'USER_CANCELLED' : 'CONFIRMATION_TIMEOUT'); }
    finally { terminal.close(); }
    requireConfirmation(answer, controller.signal.aborted);
    fixture = createReadFixture();
    transport = openUserTransport({ profile, tokenLimitPolicy: 'backend-default', signal: controller.signal });
    gateway = await startGateway({ transport, profile, readMarker: fixture.marker, headerPolicy: 'codex-missing-content-type' });
    result = await runReadClient(gateway, endpoint => {
      const launch = readLaunch(endpoint, process.env, profile);
      try { return spawn(launch.file, launch.args, launch.options); }
      finally { launch.options.env.ANTHROPIC_AUTH_TOKEN = ''; }
    }, fixture.marker, { signal: controller.signal });
    result.clientKind = 'claude-code';
  } catch (error) { result = { diagnosticVersion: 3, passed: false, category: safe(error), clientKind: 'claude-code' }; }
  finally {
    try {
      if (gateway) await gateway.close(); else if (transport) await transport.close();
      if (fixture) {
        result.fixtureRemoved = fixture.release();
        if (!result.fixtureRemoved) { result.passed = false; result.category = 'FIXTURE_CHANGED'; }
      }
    } catch { result = { ...result, passed: false, category: 'CLEANUP_FAILED' }; }
    process.removeListener('SIGINT', cancel); process.removeListener('SIGTERM', cancel);
  }
  process.stdout.write(JSON.stringify(result) + '\n'); process.exitCode = result.passed ? 0 : 1;
}
if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  try { await main(); }
  catch (error) { process.stdout.write(JSON.stringify({ passed: false, category: safe(error) }) + '\n'); process.exitCode = 1; }
}
