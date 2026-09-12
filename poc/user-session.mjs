// User-operated entry point. The agent must not execute its authenticated path.
import { resolve, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { spawnSync } from 'node:child_process';
import { createInterface } from 'node:readline/promises';
import { request } from 'node:http';
import { isDeepStrictEqual } from 'node:util';
import { readSmall, checkStore, selectCredential, checkRuntime } from '../verification/manual-http-probe.mjs';
import { ALIAS, FIXTURE_PATH, readTool, PROTOCOL_ERROR_CODES, protocolDiagnostics, probeProfile } from './adapter.mjs';
import { createCodexTransport } from './codex-transport.mjs';
import { clientVersionPolicy, isClientVersion } from '../src/client-version.mjs';
import { startGateway } from './gateway.mjs';

const expectedRoot = 'C:\\Users\\JS\\.codex';
const codexExe = 'C:\\Users\\JS\\AppData\\Local\\Programs\\OpenAI\\Codex\\bin\\codex.exe';
const configPath = join(expectedRoot, 'config.toml');
const credentialPath = join(expectedRoot, 'auth.json');
export const RESULT_MARKER = 'CLAUDUCT_MEMORY_GATEWAY_7';
const categories = new Set([...PROTOCOL_ERROR_CODES, 'SUCCESS', 'USER_TERMINAL_REQUIRED', 'INVALID_ARGUMENTS', 'USER_CANCELLED',
  'CONFIRMATION_TIMEOUT', 'UNEXPECTED_CODEX_HOME', 'DEBUG_RUNTIME_UNSUPPORTED', 'TRANSPORT_RUNTIME_UNSUPPORTED',
  'CLI_VERSION_CHANGED', 'CLI_VERSION_UNAVAILABLE', 'CLI_VERSION_INVALID', 'FILE_CACHE_UNAVAILABLE', 'FILE_TOO_LARGE', 'CONFIG_UNSUPPORTED',
  'CREDENTIAL_STORE_UNSUPPORTED', 'INVALID_AUTH_CACHE', 'TOKEN_EXPIRED', 'CREDENTIAL_UNAVAILABLE_OR_EXPIRED',
  'CREDENTIAL_ACCOUNT_CHANGED', 'CODEX_RELOGIN_REQUIRED',
  'NO_TOOL_CALL', 'INVALID_DOWNSTREAM_RESPONSE', 'FINAL_MARKER_MISMATCH', 'CLIENT_IO_ERROR', 'CLIENT_CANCELLED',
  'UPSTREAM_IO_ERROR', 'UPSTREAM_TRUNCATED', 'UPGRADE_REJECTED',
  'DUPLICATE_UPSTREAM_HEADER', 'UNSUPPORTED_UPSTREAM_ENCODING', 'PROTOCOL_REJECTED',
  'DELIVERY_TIMEOUT', 'REQUEST_TIMEOUT', 'SESSION_TIMEOUT', 'TOOL_RESULT_TIMEOUT',
  'LISTEN_FAILED', 'SERVER_ERROR', 'GATEWAY_FAILED', 'CLEANUP_FAILED', 'LOCAL_CHECK_FAILED', 'TOKEN_LIMIT_UNSUPPORTED']);
class EntryError extends Error { constructor(code) { super(code); this.code = code; } }
function requireThat(ok, code) { if (!ok) throw new EntryError(code); }
export function safeEntryCategory(error) {
  const code = error?.code ?? error?.message;
  return categories.has(code) ? code : 'LOCAL_CHECK_FAILED';
}

export function entryOptions(args) {
  const flags = args.slice(1);
  requireThat(args[0] === '--live-tool-roundtrip' && flags.length <= 2 && new Set(flags).size === flags.length
    && flags.every(flag => ['--allow-missing-content-type', '--profile=astra-low', '--profile=luna-low'].includes(flag))
    && flags.filter(flag => flag.startsWith('--profile=')).length <= 1, 'INVALID_ARGUMENTS');
  return { headerPolicy: flags.includes('--allow-missing-content-type') ? 'codex-missing-content-type' : 'strict',
    profile: flags.includes('--profile=luna-low') ? 'luna-low' : 'astra-low' };
}
export function entryPolicy(args) { return entryOptions(args).headerPolicy; }
export function checkUserContext({ stdinTTY, stdoutTTY, env, execArgs, nonInteractive = false }) {
  requireThat(typeof nonInteractive === 'boolean', 'INVALID_ARGUMENTS');
  requireThat(nonInteractive || (stdinTTY === true && stdoutTTY === true), 'USER_TERMINAL_REQUIRED');
  checkRuntime(env, execArgs);
  requireThat(resolve(env.CODEX_HOME || expectedRoot).toLowerCase() === resolve(expectedRoot).toLowerCase(),
    'UNEXPECTED_CODEX_HOME');
}
export function checkClientVersion(result) {
  requireThat(result && !result.error && !result.signal && result.status === 0, 'CLI_VERSION_UNAVAILABLE');
  requireThat(typeof result.stdout === 'string' && result.stdout.length <= 128, 'CLI_VERSION_INVALID');
  const match = /^codex-cli ([^\r\n]+)$/.exec(result.stdout.trim());
  requireThat(match && isClientVersion(match[1]), 'CLI_VERSION_INVALID');
  return match[1];
}
export function requireConfirmation(answer, aborted = false) {
  requireThat(answer === 'SEND' && !aborted, 'USER_CANCELLED');
}
export function credentialFromCache(config, cache) {
  checkStore(config);
  return selectCredential(cache);
}

function knownEntryError(error, fallback) {
  const code = error?.code ?? error?.message;
  return new EntryError(categories.has(code) ? code : fallback);
}

// The native path rereads the user's existing cache for every supplier call.
// It never refreshes, writes, or retries a partially read file.
export function createNativeCredentialSupplier({
  readConfig = () => readSmall(configPath),
  readCredential = () => readSmall(credentialPath),
  runtimeCheck = () => checkRuntime(process.env, process.execArgv),
  homeCheck = () => requireThat(resolve(process.env.CODEX_HOME || expectedRoot).toLowerCase() === resolve(expectedRoot).toLowerCase(),
    'UNEXPECTED_CODEX_HOME'),
  storeCheck = checkStore,
  credentialSelector = selectCredential,
  now = () => Date.now(),
  expectedAccount
} = {}) {
  let account = expectedAccount;
  return async (options = {}) => {
    requireThat(options !== null && typeof options === 'object' && !Array.isArray(options), 'INVALID_ARGUMENTS');
    const { force = false } = options;
    requireThat(typeof force === 'boolean', 'INVALID_ARGUMENTS');
    runtimeCheck();
    homeCheck();
    let config;
    try { config = await readConfig(); }
    catch (error) { throw knownEntryError(error, 'CODEX_RELOGIN_REQUIRED'); }
    requireThat(typeof config === 'string', 'CODEX_RELOGIN_REQUIRED');
    try { storeCheck(config); }
    catch (error) { throw knownEntryError(error, 'CONFIG_UNSUPPORTED'); }
    let raw;
    try { raw = await readCredential(); }
    catch (error) { throw knownEntryError(error, 'CODEX_RELOGIN_REQUIRED'); }
    requireThat(typeof raw === 'string', 'CODEX_RELOGIN_REQUIRED');
    let credential;
    try { credential = await credentialSelector(raw, now()); }
    catch (error) { throw knownEntryError(error, 'CREDENTIAL_UNAVAILABLE_OR_EXPIRED'); }
    requireThat(credential && typeof credential.accessToken === 'string' && typeof credential.account === 'string',
      'INVALID_AUTH_CACHE');
    if (account === undefined) account = credential.account;
    else requireThat(credential.account === account, 'CREDENTIAL_ACCOUNT_CHANGED');
    return { accessToken: credential.accessToken, account: credential.account };
  };
}
export function initialMessages() {
  return { model: ALIAS, stream: true,
    system: 'This is a synthetic tool protocol test. Request Read once for the declared fixture. '
      + 'The client supplies synthetic tool output without reading a file. After receiving it, reply with exactly its content.',
    messages: [{ role: 'user', content: `Request the Read tool for ${FIXTURE_PATH}. Wait for its result before answering.` }],
    tools: [readTool()], tool_choice: { type: 'auto' } };
}

// This client shares the owner's memory. No local secret is printed or sent via env/argv/files.
async function postMessages(gateway, body, signal) {
  const bytes = Buffer.from(JSON.stringify(body));
  let req, response, socketClosed;
  try {
    response = await new Promise((resolveResponse, reject) => {
      req = request({ hostname: '127.0.0.1', port: gateway.port, path: '/v1/messages?beta=true',
        method: 'POST', agent: false, signal, maxHeaderSize: 8192,
        headers: { ...gateway.clientHeaders(), 'Content-Length': bytes.length } }, resolveResponse);
      req.once('socket', socket => { socketClosed = new Promise(resolveClose => socket.once('close', resolveClose)); });
      req.once('error', reject); req.end(bytes);
    });
    let text = '', count = 0;
    const decoder = new TextDecoder('utf-8', { fatal: true });
    for await (const chunk of response) {
      count += chunk.length; requireThat(count <= 512 * 1024, 'INVALID_DOWNSTREAM_RESPONSE');
      text += decoder.decode(chunk, { stream: true });
    }
    text += decoder.decode();
    requireThat(response.complete, 'INVALID_DOWNSTREAM_RESPONSE');
    if (response.statusCode !== 200) {
      let error;
      try { error = JSON.parse(text).error; } catch { /* Classified below; never emit the body. */ }
      throw new EntryError(safeEntryCategory(error));
    }
    requireThat(response.headers['content-type'] === 'text/event-stream; charset=utf-8', 'INVALID_DOWNSTREAM_RESPONSE');
    const frames = text.trim().split('\n\n').map(frame => {
      const match = /^event: ([a-z_]+)\ndata: (.+)$/.exec(frame);
      requireThat(match, 'INVALID_DOWNSTREAM_RESPONSE');
      const event = JSON.parse(match[2]);
      requireThat(event.type === match[1], 'INVALID_DOWNSTREAM_RESPONSE'); return event;
    });
    requireThat(isDeepStrictEqual(frames.map(f => f.type), ['message_start', 'content_block_start',
      'content_block_delta', 'content_block_stop', 'message_delta', 'message_stop'])
      && frames[0].message?.model === ALIAS, 'INVALID_DOWNSTREAM_RESPONSE');
    return { block: frames[1].content_block, delta: frames[2].delta, stop: frames[4].delta?.stop_reason };
  } catch (error) {
    throw new EntryError(signal.aborted ? 'CLIENT_CANCELLED' : error instanceof EntryError ? error.code : 'CLIENT_IO_ERROR');
  } finally {
    bytes.fill(0);
    response?.destroy(); req?.destroy();
    if (socketClosed) await socketClosed;
  }
}

export async function runGatewayRoundtrip(gateway, signal) {
  const firstRequest = initialMessages();
  const first = await postMessages(gateway, firstRequest, signal);
  requireThat(first.block?.type === 'tool_use' && first.stop === 'tool_use', 'NO_TOOL_CALL');
  requireThat(first.block.name === 'Read' && typeof first.block.id === 'string' && /^[A-Za-z0-9_-]{1,128}$/.test(first.block.id)
    && first.delta?.type === 'input_json_delta', 'INVALID_DOWNSTREAM_RESPONSE');
  let input;
  try { input = JSON.parse(first.delta.partial_json); } catch { throw new EntryError('INVALID_DOWNSTREAM_RESPONSE'); }
  requireThat(isDeepStrictEqual(input, { file_path: FIXTURE_PATH }), 'INVALID_DOWNSTREAM_RESPONSE');
  const toolUse = { ...first.block, input };
  const next = { ...firstRequest, messages: [...firstRequest.messages,
    { role: 'assistant', content: [toolUse] },
    { role: 'user', content: [{ type: 'tool_result', tool_use_id: toolUse.id,
      content: RESULT_MARKER, is_error: false }] }] };
  const second = await postMessages(gateway, next, signal);
  requireThat(second.block?.type === 'text' && second.delta?.type === 'text_delta'
    && second.stop === 'end_turn', 'INVALID_DOWNSTREAM_RESPONSE');
  requireThat(second.delta.text === RESULT_MARKER, 'FINAL_MARKER_MISMATCH');
  return { callIdMatches: true, exactMarker: true, syntheticToolResult: true, toolExecutions: 0 };
}

export function entrySummary(category, diagnostics, outcome, profile = 'astra-xhigh') {
  const selected = probeProfile(profile);
  const integer = value => Number.isSafeInteger(value) && value >= 0 ? value : 0;
  const clean = diagnostics?.closing === true && diagnostics.activeSockets === 0 && diagnostics.activeJobs === 0
    && diagnostics.activeTimers === 0 && diagnostics.activeDeliveries === 0 && diagnostics.busy === false && diagnostics.session?.timerActive === false
    && diagnostics.session?.buffered === false && diagnostics.transport?.activeSockets === 0
    && diagnostics.transport?.activeRequests === 0 && diagnostics.localSessionSecretCleared === true;
  const attempts = integer(diagnostics?.transport?.requestAttempts);
  const successful = category === 'SUCCESS' && diagnostics?.reason === 'COMPLETE' && clean && attempts === 2
    && outcome?.callIdMatches === true && outcome?.exactMarker === true;
  const upstream = diagnostics?.transport;
  const status = upstream?.httpStatus;
  const contentTypes = ['not-received', 'missing', 'empty', 'event-stream', 'multiple', 'other'];
  return { diagnosticVersion: 5, passed: successful, category: safeEntryCategory({ code: category === 'SUCCESS' && !successful ? 'CLEANUP_FAILED' : category }),
    transport: diagnostics?.transport?.synthetic === true ? 'loopback-test-gateway' : 'node-https-gateway',
    upstreamMode: diagnostics?.transport ? diagnostics.transport.synthetic ? 'synthetic' : 'live' : 'not-started',
    requestedModel: selected.model, requestedEffort: selected.effort,
    ...(isClientVersion(upstream?.clientVersion) ? clientVersionPolicy(upstream.clientVersion)
      : { clientVersion: null, referenceClientVersion: null, clientVersionStatus: 'not-observed' }),
    requestAttempts: attempts, connectionAttempts: integer(diagnostics?.transport?.connectionAttempts),
    compatibilityApplied: integer(diagnostics?.counts?.compatibilityApplied), resourcesClosed: clean,
    reconstructedToolCalls: diagnostics?.counts?.reconstructedToolCalls === 1 ? 1 : 0,
    callIdMatches: outcome?.callIdMatches === true, exactMarker: outcome?.exactMarker === true,
    responseDiagnostics: { httpStatus: Number.isInteger(status) && status >= 100 && status <= 599 ? status : null,
      contentTypeState: contentTypes.includes(upstream?.contentTypeState) ? upstream.contentTypeState : 'not-received',
      responseBytes: integer(upstream?.responseBytes),
      httpComplete: typeof upstream?.httpComplete === 'boolean' ? upstream.httpComplete : null,
      ...protocolDiagnostics(diagnostics?.session) },
    toolResultMode: 'synthetic', toolExecutions: 0, credentialWrites: 0, retries: 0 };
}

// User-terminal boundary shared by the two user-operated entry points. Never called by tests.
export function openUserTransport({ profile = 'astra-low', tokenLimitPolicy = 'reject', signal, requestBudget, transportFactory, nonInteractive = false } = {}) {
  checkUserContext({ stdinTTY: process.stdin.isTTY, stdoutTTY: process.stdout.isTTY,
    env: process.env, execArgs: process.execArgv, nonInteractive });
  requireThat(!signal?.aborted, 'USER_CANCELLED');
  const clientVersion = checkClientVersion(spawnSync(codexExe, ['--version'], { windowsHide: true, encoding: 'utf8', timeout: 5000, maxBuffer: 4096 }));
  const compatibility = clientVersionPolicy(clientVersion);
  if (compatibility.clientVersionStatus === 'unverified') {
    process.stderr.write(`Clauduct: CLI_VERSION_UNVERIFIED detected=${clientVersion} reference=${compatibility.referenceClientVersion}; protocol checks remain active.\n`);
  }
  if (transportFactory) {
    const credentialSupplier = createNativeCredentialSupplier();
    requireThat(!signal?.aborted, 'USER_CANCELLED');
    return transportFactory({ credentialSupplier, clientVersion, profile, tokenLimitPolicy, requestBudget });
  }
  let credential;
  try {
    const config = readSmall(join(expectedRoot, 'config.toml'));
    checkStore(config);
    credential = credentialFromCache(config, readSmall(join(expectedRoot, 'auth.json')));
  } catch (error) { throw new EntryError(safeEntryCategory(error) === 'LOCAL_CHECK_FAILED' ? 'FILE_CACHE_UNAVAILABLE' : safeEntryCategory(error)); }
  requireThat(!signal?.aborted, 'USER_CANCELLED');
  try { return createCodexTransport({ credential, clientVersion, profile, tokenLimitPolicy, requestBudget }); }
  finally { credential = undefined; }
}

async function main() {
  let transport, gateway, outcome, diagnostics, category = 'LOCAL_CHECK_FAILED', profile = 'astra-low';
  const controller = new AbortController();
  const cancel = () => { controller.abort(); void gateway?.close('CANCELLED'); };
  try {
    if (process.argv.length === 3 && process.argv[2] === '--help') {
      process.stdout.write('사용자 터미널: node poc/user-session.mjs --live-tool-roundtrip --profile=astra-low 또는 --profile=luna-low [--allow-missing-content-type]\n'
        + '기본 astra-low. 각 모델은 별도 실행·별도 SEND이며 실행당 upstream 최대 2회, 실제 도구 실행 0회.\n');
      return;
    }
    const options = entryOptions(process.argv.slice(2));
    profile = options.profile;
    const { headerPolicy } = options, selected = probeProfile(profile);
    checkUserContext({ stdinTTY: process.stdin.isTTY, stdoutTTY: process.stdout.isTTY,
      env: process.env, execArgs: process.execArgv });
    process.once('SIGINT', cancel); process.once('SIGTERM', cancel);
    process.stdout.write(`사용자 운영 게이트웨이 검사: ${selected.model}/${selected.effort}, 최대 2회 계정 요청, 헤더 정책 ${headerPolicy}.\n`
      + 'Read 호출 요청과 합성 도구 결과의 프로토콜 왕복입니다. 파일·Claude 도구 실행, refresh·인증 쓰기·재시도는 없습니다.\n'
      + '출력 토큰 수 상한은 보내지 않습니다. 요청별 45초·응답 256 KiB 제한을 적용하며 서버의 즉시 추론/과금 중단은 보증하지 않습니다.\n');
    const terminal = createInterface({ input: process.stdin, output: process.stdout });
    terminal.once('SIGINT', cancel);
    let answer;
    try {
      answer = await terminal.question('실행하려면 SEND, 취소하려면 Enter: ',
        { signal: AbortSignal.any([controller.signal, AbortSignal.timeout(60000)]) });
    } catch { throw new EntryError(controller.signal.aborted ? 'USER_CANCELLED' : 'CONFIRMATION_TIMEOUT'); }
    finally { terminal.close(); }
    requireConfirmation(answer, controller.signal.aborted);
    transport = openUserTransport({ profile, signal: controller.signal });
    gateway = await startGateway({ transport, headerPolicy, profile });
    outcome = await runGatewayRoundtrip(gateway, controller.signal);
    diagnostics = await gateway.done; category = 'SUCCESS';
  } catch (error) { category = safeEntryCategory(error); }
  finally {
    try {
      if (gateway) diagnostics = await gateway.close();
      else if (transport) await transport.close();
    } catch { category = 'CLEANUP_FAILED'; }
    process.removeListener('SIGINT', cancel); process.removeListener('SIGTERM', cancel);
  }
  const result = entrySummary(category, diagnostics, outcome, profile);
  process.stdout.write(JSON.stringify(result) + '\n'); process.exitCode = result.passed ? 0 : 1;
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  try { await main(); }
  catch { process.stdout.write('{"passed":false,"category":"LOCAL_CHECK_FAILED"}\n'); process.exitCode = 1; }
}
