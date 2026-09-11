// User-operated connectivity probe, not an agent credential-access workaround.
// Live mode requires a separate interactive terminal and explicit SEND confirmation.
import { openSync, readSync, closeSync } from 'node:fs';
import { resolve, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { spawnSync } from 'node:child_process';
import { request } from 'node:https';
import { createInterface } from 'node:readline/promises';
import { liteSearchEnvelope } from '../src/native-protocol.mjs';
import { clientVersionPolicy } from '../src/client-version.mjs';
import { release, arch } from 'node:os';

export const endpoint = 'https://chatgpt.com/backend-api/codex/responses';
export const model = 'gpt-6-astra';
export const effort = 'xhigh';
// Search mode runs on the cheapest model: it is measuring whether the envelope is
// accepted and whether the backend searches, not how well the model answers.
export const liteModel = 'gpt-5.6-luna';
export const liteEffort = 'medium';
const expectedRoot = 'C:\\Users\\JS\\.codex';
const codexExe = 'C:\\Users\\JS\\AppData\\Local\\Programs\\OpenAI\\Codex\\bin\\codex.exe';
// The version this probe sends is the installed one, read at run time, exactly as the gateway
// does. A version the project has not validated end to end is reported as unverified rather
// than aborted: aborting hides the drift, reporting it puts the drift in the result the user
// reads before deciding anything. A version that cannot be read or parsed still stops the run.
export function readClientVersion(result) {
  if (result.error || result.status !== 0 || typeof result.stdout !== 'string') stop('CLI_VERSION_UNREADABLE');
  const match = result.stdout.trim().match(/^codex-cli (\S+)$/);
  if (!match) stop('CLI_VERSION_UNREADABLE');
  try { return clientVersionPolicy(match[1]); } catch { stop('CLI_VERSION_INVALID'); }
}
const limit = 256 * 1024;
const knownErrors = new Set(['USER_TERMINAL_REQUIRED', 'USER_CANCELLED', 'UNEXPECTED_CODEX_HOME',
  'CLI_VERSION_UNREADABLE', 'CLI_VERSION_INVALID', 'FILE_CACHE_UNAVAILABLE', 'FILE_TOO_LARGE', 'CONFIG_UNSUPPORTED',
  'CREDENTIAL_STORE_UNSUPPORTED', 'INVALID_AUTH_CACHE', 'TOKEN_EXPIRED', 'TIMEOUT',
  'RESPONSE_TOO_LARGE', 'NETWORK_OR_TLS_ERROR', 'DEBUG_RUNTIME_UNSUPPORTED', 'TRANSPORT_RUNTIME_UNSUPPORTED']);

function stop(code) { throw new Error(code); }
export function readSmall(path) {
  const fd = openSync(path, 'r');
  const buffer = Buffer.alloc(65537);
  let length = 0;
  try {
    while (length < buffer.length) {
      const count = readSync(fd, buffer, length, buffer.length - length, null);
      if (!count) break;
      length += count;
    }
    if (length > 65536) stop('FILE_TOO_LARGE');
    return new TextDecoder('utf-8', { fatal: true }).decode(buffer.subarray(0, length));
  } finally { buffer.fill(0); closeSync(fd); }
}

export function checkStore(config) {
  // This one-off probe supports simple top-level file-cache configuration only.
  // Do not silently select a stale file when keyring/auto is explicitly configured.
  const top = config.split(/^\s*\[/m)[0];
  const assignments = top.split(/\r?\n/).filter(line => /^\s*cli_auth_credentials_store\s*=/.test(line));
  if (!assignments.length) return;
  if (assignments.length !== 1) stop('CONFIG_UNSUPPORTED');
  const match = assignments[0].match(/^\s*cli_auth_credentials_store\s*=\s*["'](file|keyring|auto)["']\s*(?:#.*)?$/);
  if (!match) stop('CONFIG_UNSUPPORTED');
  if (match[1] !== 'file') stop('CREDENTIAL_STORE_UNSUPPORTED');
}

export function selectCredential(raw, now = Date.now()) {
  let doc;
  try { doc = JSON.parse(raw); } catch { stop('INVALID_AUTH_CACHE'); }
  if (!doc || (doc.auth_mode !== undefined && doc.auth_mode !== 'chatgpt')) stop('INVALID_AUTH_CACHE');
  const accessToken = doc.tokens?.access_token;
  const account = doc.tokens?.account_id;
  if (typeof accessToken !== 'string' || accessToken.length > 16000
    || !/^[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+$/.test(accessToken)
    || typeof account !== 'string' || !/^[A-Za-z0-9_-]{1,128}$/.test(account)) stop('INVALID_AUTH_CACHE');
  let claims;
  try { claims = JSON.parse(Buffer.from(accessToken.split('.')[1], 'base64url').toString('utf8')); }
  catch { stop('INVALID_AUTH_CACHE'); }
  // Expiry is a local sanity check, not signature verification; the server authenticates it.
  if (!Number.isFinite(claims?.exp)) stop('INVALID_AUTH_CACHE');
  if (claims.exp * 1000 <= now + 60000) stop('TOKEN_EXPIRED');
  return { accessToken, account };
}

export function buildBody() {
  return { model, instructions: 'This is a text-only connectivity test. Reply with exactly OK.',
    input: [{ role: 'user', content: [{ type: 'input_text', text: 'Reply with exactly OK.' }] }],
    reasoning: { effort }, tools: [], tool_choice: 'none', stream: true, store: false };
}

// The gateway's search envelope, built here the same way so one SEND says whether the backend
// accepts it and whether it runs its own search. Carries no client tools and no local text.
export function buildLiteBody(envelope) {
  return { model: liteModel,
    // No additional_tools item: the gateway omits it when there are no client tools to carry,
    // and a search side query declares only the search tool the backend supplies itself.
    // A question the model can answer from memory cannot tell "no search tool" apart from "did
    // not need one". This one cannot be answered without live access, and it names the reply to
    // give when there is no tool, so a refusal is a signal instead of prose to interpret.
    input: [{ role: 'user', content: [{ type: 'input_text', text: LITE_PROMPT }] }],
    tool_choice: 'auto', parallel_tool_calls: false,
    reasoning: { effort: liteEffort, context: 'all_turns' }, store: false, stream: true,
    include: ['reasoning.encrypted_content'], prompt_cache_key: envelope.cacheKey,
    text: { verbosity: 'medium' }, client_metadata: envelope.metadata };
}

export const LITE_PROMPT = 'Use your web search tool to look up the current top story on '
  + 'Hacker News (news.ycombinator.com) and reply with only its title. Do not answer from memory. '
  + 'If you have no web search tool available, reply with exactly NO_SEARCH and nothing else.';
// The one reply this probe names itself, so recognising it reports a boolean rather than text.
const NO_SEARCH = 'NO_SEARCH';

// Field names this probe itself sends. Reporting which of our own names an upstream rejection
// mentions narrows the envelope without echoing the upstream message.
// A parameter path such as input[0].tools is a name this probe sent back to it, so brackets
// and indices belong in the shape. No spaces, no punctuation that could carry a sentence.
const SAFE_LABEL = /^[A-Za-z][A-Za-z0-9_.\[\]-]{0,63}$/;
function errorLabels(text) {
  let doc;
  try { doc = JSON.parse(text); } catch { return {}; }
  const error = doc?.error;
  if (!error || typeof error !== 'object' || Array.isArray(error)) return {};
  const labels = {};
  for (const key of ['type', 'code', 'param']) {
    if (typeof error[key] === 'string' && SAFE_LABEL.test(error[key])) labels[key] = error[key];
  }
  return Object.keys(labels).length ? { errorLabels: labels } : {};
}

const LITE_FIELDS = Object.freeze(['instructions', 'tools', 'additional_tools', 'client_metadata',
  'prompt_cache_key', 'reasoning', 'context', 'text', 'verbosity', 'include', 'tool_choice',
  'parallel_tool_calls', 'store', 'stream', 'model']);

export function summarizeResponse(status, contentType, bytes, rawHeaders, lite = false) {
  // Only fixed labels and counts leave this boundary; never echo headers or body snippets.
  const mediaType = contentType.split(';', 1)[0].trim().toLowerCase();
  const result = { httpStatus: status, passed: false, category: 'UNEXPECTED_RESPONSE',
    responseBytes: bytes.length,
    mediaType: !mediaType ? 'missing' : ['text/event-stream', 'application/json',
      'text/html', 'text/plain', 'application/octet-stream'].includes(mediaType) ? mediaType : 'other',
    bodyFormat: 'unknown' };
  // rawHeaders alternates names and values. Inspect names only; values stay private.
  const rawHeadersAvailable = Array.isArray(rawHeaders) && rawHeaders.length % 2 === 0
    && rawHeaders.every((name, index) => index % 2 !== 0 || typeof name === 'string');
  result.headerDiagnostics = {
    normalizedContentTypeNonEmpty: contentType.trim().length > 0,
    rawHeadersAvailable,
    rawContentTypePresent: rawHeadersAvailable
      ? rawHeaders.some((name, index) => index % 2 === 0 && name.toLowerCase() === 'content-type') : null
  };
  let text;
  try { text = new TextDecoder('utf-8', { fatal: true }).decode(bytes); }
  catch { return { ...result, category: 'INVALID_UTF8' }; }
  if (!text.trim()) result.bodyFormat = 'empty';
  else if (/^\s*(?:<!doctype\s+html\b|<html\b)/i.test(text)) result.bodyFormat = 'html-like';
  else if (/^(?:data:|event:)/m.test(text)) result.bodyFormat = 'sse-like';
  else {
    try {
      const doc = JSON.parse(text);
      result.bodyFormat = 'json';
      result.jsonKind = doc && typeof doc === 'object' && !Array.isArray(doc)
        ? doc.error != null ? 'error' : doc.object === 'response' ? 'response'
          : ['response.completed', 'response.failed', 'response.incomplete'].includes(doc.type)
            ? 'response-event' : 'other-object'
        : 'non-object';
    } catch { result.bodyFormat = 'other-text'; }
  }
  if (status !== 200) {
    result.category = status >= 300 && status < 400 ? 'REDIRECT_REFUSED'
      : status === 401 ? 'AUTH_REJECTED' : status === 403 ? 'ACCESS_DENIED'
      : status === 429 ? 'RATE_LIMITED' : 'HTTP_ERROR';
    // Never echo the upstream message: it may include request headers or account information.
    if (status === 400 && /requires a newer version of Codex/i.test(text)) result.category = 'CLIENT_VERSION_REJECTED';
    else if (status === 400 && /unsupported parameter|unknown parameter|not supported/i.test(text)) result.category = 'REQUEST_NOT_SUPPORTED';
    // Only names this probe itself sent, never upstream text: enough to say which part of the
    // envelope the backend objected to without reproducing its message.
    result.rejectedFields = LITE_FIELDS.filter(name => text.includes(name));
    // The upstream error's own vocabulary and the name of a parameter this probe sent, both
    // bounded to a fixed shape. The message text itself is never reproduced.
    Object.assign(result, errorLabels(text));
    return result;
  }
  if (mediaType !== 'text/event-stream') {
    // Diagnostic parsing does not make a missing Content-Type a successful response.
    if (result.mediaType === 'missing' && result.bodyFormat === 'sse-like') {
      return { ...result, category: 'MISSING_CONTENT_TYPE', sseDiagnostics: inspectSse(text, lite) };
    }
    return result;
  }
  return { ...result, ...inspectSse(text, lite) };
}

function textPosition(event) {
  return Number.isSafeInteger(event.output_index) && event.output_index >= 0
    && Number.isSafeInteger(event.content_index) && event.content_index >= 0
    ? `${event.output_index}:${event.content_index}` : null;
}

function sameTextParts(left, right) {
  return left.size === right.size && [...left].every(([key, value]) => right.get(key) === value);
}

function joinTextParts(parts) {
  return [...parts].sort(([a], [b]) => {
    const [ai, ac] = a.split(':').map(Number), [bi, bc] = b.split(':').map(Number);
    return ai - bi || ac - bc;
  }).map(([, value]) => value).join('');
}

function inspectSse(text, lite = false) {
  const result = { passed: false, category: 'UNEXPECTED_RESPONSE' };
  let completed, eventCount = 0, failed = false, unexpectedTool = false;
  // In search mode a backend search is the thing being measured, so it is counted rather
  // than treated as an unexpected tool. The count is a number; no query or result is read.
  const searchItems = new Set();
  const deltas = new Map();
  const textDone = new Map();
  const itemDoneIndices = new Set();
  const messageSnapshotSizes = new Map();
  const contentDoneKeys = new Set();
  const textSnapshots = [];
  const nonMessageIndices = new Set();
  let deltaEventCount = 0, deltaShapeValid = true;
  let textDoneEventCount = 0, doneShapeValid = true, streamOrderValid = true, streamRefused = false;
  let completionSeen = false;
  try {
    for (const frame of text.replaceAll('\r\n', '\n').split('\n\n')) {
      const data = frame.split('\n').filter(line => line.startsWith('data:'))
        .map(line => line.slice(5).replace(/^ /, '')).join('\n');
      if (!data) continue;
      if (data === '[DONE]') {
        if (!completionSeen) return { ...result, category: 'PREMATURE_DONE', eventCount };
        continue;
      }
      if (++eventCount > 512) return { ...result, category: 'EVENT_LIMIT' };
      const event = JSON.parse(data);
      if (!event || typeof event !== 'object' || Array.isArray(event) || typeof event.type !== 'string') {
        return { ...result, category: 'INVALID_SSE_EVENT', eventCount };
      }
      if (completionSeen) {
        return { ...result, category: event.type === 'response.completed' ? 'DUPLICATE_COMPLETION' : 'EVENT_AFTER_COMPLETION', eventCount };
      }
      const key = textPosition(event);
      if (event.type === 'response.output_text.delta') {
        deltaEventCount++;
        if (typeof event.delta !== 'string' || key === null) {
          deltaShapeValid = false;
        } else {
          if (textDone.has(key) || itemDoneIndices.has(event.output_index)) streamOrderValid = false;
          deltas.set(key, (deltas.get(key) ?? '') + event.delta);
        }
      }
      if (event.type === 'response.output_text.done') {
        textDoneEventCount++;
        if (typeof event.text !== 'string' || key === null) doneShapeValid = false;
        else {
          if (textDone.has(key) || itemDoneIndices.has(event.output_index)) streamOrderValid = false;
          textDone.set(key, event.text);
        }
      }
      if (event.type === 'response.content_part.done' && event.part?.type === 'output_text') {
        if (key === null || typeof event.part.text !== 'string') doneShapeValid = false;
        else {
          if (!textDone.has(key) || contentDoneKeys.has(key)) streamOrderValid = false;
          contentDoneKeys.add(key);
          textSnapshots.push([key, event.part.text]);
        }
      }
      if (event.type.startsWith('response.refusal.') || event.part?.type === 'refusal') streamRefused = true;
      if (lite && event.type.startsWith('response.web_search_call.') && typeof event.item_id === 'string') {
        searchItems.add(event.item_id);
      } else if (/^response\..*(?:_call|_arguments)(?:\.|$)/.test(event.type)) unexpectedTool = true;
      if (['response.failed', 'response.incomplete', 'response.cancelled', 'response.canceled', 'error'].includes(event.type)) failed = true;
      if (lite && event.item?.type === 'web_search_call') {
        if (typeof event.item.id === 'string') searchItems.add(event.item.id);
      } else if (event.item?.type && !['message', 'reasoning'].includes(event.item.type)) unexpectedTool = true;
      if (event.item?.type === 'reasoning' && Number.isSafeInteger(event.output_index)) nonMessageIndices.add(event.output_index);
      if (event.item?.type === 'message' && Array.isArray(event.item.content)
        && event.item.content.some(part => part?.type === 'refusal')) streamRefused = true;
      if (event.type === 'response.output_item.done') {
        if (!Number.isSafeInteger(event.output_index) || event.output_index < 0 || !event.item?.type) doneShapeValid = false;
        else {
          if (itemDoneIndices.has(event.output_index)) streamOrderValid = false;
          itemDoneIndices.add(event.output_index);
          if (event.item.type === 'message') {
            if (!Array.isArray(event.item.content) || event.item.status !== 'completed'
              || event.item.role !== 'assistant') doneShapeValid = false;
            else {
              messageSnapshotSizes.set(event.output_index, event.item.content.length);
              event.item.content.forEach((part, contentIndex) => {
                if (!part || part.type !== 'output_text' || typeof part.text !== 'string') doneShapeValid = false;
                else textSnapshots.push([`${event.output_index}:${contentIndex}`, part.text]);
              });
            }
          }
        }
      }
      if (event.type === 'response.completed') {
        completionSeen = true;
        completed = event.response;
      }
    }
  } catch { return { ...result, category: 'INVALID_SSE_JSON' }; }
  if (failed) return { ...result, category: 'UPSTREAM_FAILED', eventCount };
  if (!completed || completed.status !== 'completed' || !Array.isArray(completed.output)) {
    return { ...result, category: 'NO_COMPLETED_RESPONSE', eventCount };
  }
  if (completed.error != null || completed.incomplete_details != null) return { ...result, category: 'UPSTREAM_FAILED', eventCount };
  if (completed.output.some(item => !item || typeof item !== 'object' || Array.isArray(item))) {
    return { ...result, category: 'INVALID_RESPONSE_SHAPE', eventCount };
  }
  for (const item of completed.output) {
    if (lite && item.type === 'web_search_call' && typeof item.id === 'string') searchItems.add(item.id);
  }
  unexpectedTool ||= completed.output.some(item => !['message', 'reasoning'].includes(item.type)
    && !(lite && item.type === 'web_search_call'));
  if (lite) result.webSearchCalls = searchItems.size;
  const messages = completed.output.filter(item => item.type === 'message');
  if (messages.some(item => !Array.isArray(item.content)
    || (item.role !== undefined && item.role !== 'assistant')
    || (item.status !== undefined && item.status !== 'completed'))) {
    return { ...result, category: 'INVALID_RESPONSE_SHAPE', eventCount };
  }
  const output = messages
    .flatMap(item => Array.isArray(item.content) ? item.content : []);
  if (output.some(part => !part || typeof part !== 'object' || Array.isArray(part)
    || !['output_text', 'refusal'].includes(part.type)
    || (part.type === 'output_text' && typeof part.text !== 'string'))) {
    return { ...result, category: 'INVALID_RESPONSE_SHAPE', eventCount };
  }
  const snapshotReply = output.filter(part => part.type === 'output_text').map(part => part.text).join('');
  const completedParts = new Map();
  completed.output.forEach((item, outputIndex) => {
    if (item.type !== 'message') nonMessageIndices.add(outputIndex);
    if (item.type !== 'message' || !Array.isArray(item.content)) return;
    item.content.forEach((part, contentIndex) => {
      if (part.type === 'output_text') completedParts.set(`${outputIndex}:${contentIndex}`, part.text);
    });
  });
  const streamReply = deltaShapeValid ? joinTextParts(deltas) : '';
  const streamMatchesCompleted = deltaEventCount > 0 && deltaShapeValid ? sameTextParts(deltas, completedParts) : null;
  const streamMatchesTextDone = deltaEventCount > 0 && deltaShapeValid && textDoneEventCount > 0 && doneShapeValid
    ? sameTextParts(deltas, textDone) : null;
  const comparisonParts = deltas.size ? deltas : completedParts;
  const snapshotsMatch = textSnapshots.every(([key, value]) => comparisonParts.get(key) === value)
    && [...messageSnapshotSizes].every(([index, size]) =>
      [...comparisonParts.keys()].filter(key => key.startsWith(`${index}:`)).length === size);
  const indicesCompatible = [...deltas.keys(), ...textDone.keys()].every(key => !nonMessageIndices.has(Number(key.split(':')[0])));
  const textDoneConsistent = textDoneEventCount === 0 || (doneShapeValid
    && sameTextParts(textDone, deltas.size ? deltas : completedParts));
  const streamConsistent = deltaShapeValid && doneShapeValid && streamOrderValid && snapshotsMatch && indicesCompatible
    && textDoneConsistent && (deltaEventCount === 0 || messages.length === 0 || streamMatchesCompleted === true);
  // A textless terminal snapshot may omit the message, not contradict a present one.
  const reconstructed = messages.length === 0 && streamMatchesTextDone === true && streamConsistent;
  const reply = reconstructed ? streamReply : snapshotReply;
  const replySource = reconstructed ? 'stream' : completedParts.size ? 'completed' : 'none';
  const textDiagnostics = {
    outputTextPartCount: completedParts.size,
    replyEmpty: snapshotReply.length === 0,
    replyChars: Array.from(snapshotReply).length,
    trimmedOK: snapshotReply.trim() === 'OK',
    deltaEventCount, deltaShapeValid,
    streamChars: deltaShapeValid ? [...deltas.values()].reduce((sum, part) => sum + Array.from(part).length, 0) : null,
    streamMatchesCompleted,
    streamExactOK: deltaShapeValid && deltaEventCount > 0 ? streamReply === 'OK' : null,
    textDoneEventCount, doneShapeValid, streamOrderValid, streamMatchesTextDone,
    snapshotsMatch, indicesCompatible, reconstructed
  };
  const modelMatches = completed.model === (lite ? liteModel : model);
  // Connectivity mode asks for the exact word OK. Search mode asks a real question, so the
  // measured outcome is whether the envelope was accepted and the backend ran a search.
  const exactOK = lite ? null : reply === 'OK';
  const refused = streamRefused || output.some(part => part.type === 'refusal');
  const effortEchoMatches = completed.reasoning?.effort === (lite ? liteEffort : effort);
  const passed = modelMatches && effortEchoMatches && !unexpectedTool && !refused && streamConsistent
    && (lite ? searchItems.size > 0 : exactOK);
  // The probe named this reply, so reporting it is reporting our own token, not upstream text.
  if (lite) result.noSearchDeclared = reply.trim() === NO_SEARCH;
  const counts = {};
  for (const name of ['input_tokens', 'output_tokens', 'total_tokens']) {
    const value = completed.usage?.[name];
    if (Number.isSafeInteger(value) && value >= 0) counts[name] = value;
  }
  return { ...result, passed, category: passed ? 'SUCCESS' : 'RESULT_MISMATCH',
    eventCount, modelMatches, exactOK, unexpectedTool, refused, replySource,
    effortEchoMatches,
    usage: counts, textDiagnostics };
}

export function selectTransport(args, stdinTTY, stdoutTTY) {
  if (args.length !== 1 || !['--live', '--live-fetch'].includes(args[0]) || !stdinTTY || !stdoutTTY) {
    stop('USER_TERMINAL_REQUIRED');
  }
  return args[0] === '--live-fetch' ? 'node-fetch' : 'node-https';
}

export function checkRuntime(env, execArgs) {
  if (env.NODE_DEBUG || env.NODE_OPTIONS || execArgs.length) stop('DEBUG_RUNTIME_UNSUPPORTED');
  if (env.NODE_USE_ENV_PROXY || env.NODE_TLS_REJECT_UNAUTHORIZED !== undefined) stop('TRANSPORT_RUNTIME_UNSUPPORTED');
}

// Captured from the reference client: web_search = "live" and web_search = "disabled" produce
// byte-identical requests apart from per-run identifiers, so the built-in search is not declared
// in the request at all. The lite envelope also sends no instructions, which means the backend
// supplies them — and which ones it supplies follows the client identity in these headers. So a
// lite request identifies itself the way the reference client does and omits the headers the
// reference client does not send.
const liteAgent = version => `codex_exec/${version} (${release()}; ${arch() === 'x64' ? 'x86_64' : arch()}) `
  + `xterm-256color (codex_exec; ${version})`;
export function buildHeaders(credential, version, body, lite = false) {
  const common = { Authorization: `Bearer ${credential.accessToken}`, 'chatgpt-account-id': credential.account,
    'Content-Type': 'application/json', Accept: 'text/event-stream', 'Accept-Encoding': 'identity',
    'Content-Length': Buffer.byteLength(body) };
  return lite
    ? { ...common, originator: 'codex_exec', 'User-Agent': liteAgent(version) }
    : { ...common, Version: version, 'User-Agent': `codex-cli/${version} (Windows; x64)`,
      originator: 'codex_cli_rs', 'Openai-Beta': 'responses=experimental' };
}

export function buildFetchOptions(body, headers, signal) {
  // A non-replayable body prevents Fetch's automatic HTTP 421 resend.
  const bytes = new TextEncoder().encode(body);
  return { method: 'POST', headers, signal, redirect: 'manual', credentials: 'omit', duplex: 'half',
    body: new ReadableStream({ start(controller) { controller.enqueue(bytes); controller.close(); } }) };
}

export async function inspectFetchResponse(response, lite = false) {
  const chunks = [];
  let length = 0;
  if (response.body) {
    for await (const chunk of response.body) {
      if ((length += chunk.length) > limit) stop('RESPONSE_TOO_LARGE');
      chunks.push(chunk);
    }
  }
  return { ...summarizeResponse(response.status, response.headers.get('content-type') ?? '', Buffer.concat(chunks), null, lite),
    transportDiagnostics: { contentTypePresent: response.headers.has('content-type') } };
}

async function sendFetchOnce(credential, version, envelope) {
  const body = JSON.stringify(envelope ? buildLiteBody(envelope) : buildBody());
  const controller = new AbortController();
  const timeout = setTimeout(() => controller.abort(), 45000);
  try {
    const headers = { ...buildHeaders(credential, version, body, envelope !== undefined), ...envelope?.headers };
    const response = await fetch(endpoint, buildFetchOptions(body, headers, controller.signal));
    return await inspectFetchResponse(response, envelope !== undefined);
  } catch (error) {
    stop(controller.signal.aborted ? 'TIMEOUT' : error.message === 'RESPONSE_TOO_LARGE' ? error.message : 'NETWORK_OR_TLS_ERROR');
  } finally { clearTimeout(timeout); }
}

async function sendOnce(credential, version, envelope) {
  const body = JSON.stringify(envelope ? buildLiteBody(envelope) : buildBody());
  return new Promise((resolveResult, reject) => {
    const controller = new AbortController();
    const timeout = setTimeout(() => controller.abort(), 45000);
    const req = request(endpoint, { method: 'POST', agent: false, signal: controller.signal,
      rejectUnauthorized: true,
      headers: { ...buildHeaders(credential, version, body, envelope !== undefined), ...envelope?.headers } }, res => {
      const chunks = [];
      let length = 0;
      res.on('data', chunk => {
        if ((length += chunk.length) > limit) { req.destroy(new Error('RESPONSE_TOO_LARGE')); return; }
        chunks.push(chunk);
      });
      res.on('error', () => { clearTimeout(timeout); reject(new Error(controller.signal.aborted ? 'TIMEOUT' : 'NETWORK_OR_TLS_ERROR')); });
      res.on('end', () => {
        clearTimeout(timeout);
        try {
          resolveResult(summarizeResponse(res.statusCode ?? 0, String(res.headers['content-type'] ?? ''),
            Buffer.concat(chunks), res.rawHeaders, envelope !== undefined));
        } catch { reject(new Error('LOCAL_CHECK_FAILED')); }
      });
    });
    req.on('error', error => {
      clearTimeout(timeout);
      reject(new Error(controller.signal.aborted ? 'TIMEOUT' : error.message === 'RESPONSE_TOO_LARGE' ? error.message : 'NETWORK_OR_TLS_ERROR'));
    });
    // Native https does not follow redirects or retry and no response can select a destination.
    req.end(body);
  });
}

async function main() {
  let attempted = false;
  let transport, lite = false, compatibility;
  try {
    const args = process.argv.slice(2);
    lite = args.includes('--lite');
    transport = selectTransport(args.filter(arg => arg !== '--lite'), process.stdin.isTTY, process.stdout.isTTY);
    checkRuntime(process.env, process.execArgv);
    // Read before the confirmation so a version the project has not validated is on screen
    // while the user decides, rather than reported after the request has already gone out.
    compatibility = readClientVersion(spawnSync(codexExe, ['--version'],
      { windowsHide: true, encoding: 'utf8', timeout: 5000, maxBuffer: 4096 }));
    console.log(`USER-OPERATED TEST: ${lite ? `${liteModel}/${liteEffort}; gateway search envelope` : `${model}/${effort}`}; ${transport}; one request to ${endpoint}`);
    console.log(`Client version sent: ${compatibility.clientVersion} (${compatibility.clientVersionStatus}; project baseline ${compatibility.referenceClientVersion}).`);
    console.log('Reads the existing file cache in memory only. No refresh, writes, tool execution, redirects, or retries.');
    console.log(`This consumes account usage.${lite ? ' The backend runs its own search for this request.' : ''} The backend compatibility path is not a public API support guarantee.`);
    const terminal = createInterface({ input: process.stdin, output: process.stdout });
    let answer;
    try { answer = await terminal.question('Type SEND to run once, or press Enter to cancel: '); }
    finally { terminal.close(); }
    if (answer !== 'SEND') stop('USER_CANCELLED');
    if (resolve(process.env.CODEX_HOME || expectedRoot).toLowerCase() !== resolve(expectedRoot).toLowerCase()) stop('UNEXPECTED_CODEX_HOME');
    let credential;
    try {
      checkStore(readSmall(join(expectedRoot, 'config.toml')));
      credential = selectCredential(readSmall(join(expectedRoot, 'auth.json')));
    } catch (error) {
      if (knownErrors.has(error.message)) throw error;
      stop('FILE_CACHE_UNAVAILABLE');
    }
    attempted = true;
    const envelope = lite ? liteSearchEnvelope() : undefined;
    const result = transport === 'node-fetch' ? await sendFetchOnce(credential, compatibility.clientVersion, envelope)
      : await sendOnce(credential, compatibility.clientVersion, envelope);
    credential = null;
    console.log(JSON.stringify({ ...result, mode: lite ? 'search-envelope' : 'connectivity',
      requestedModel: lite ? liteModel : model, requestedEffort: lite ? liteEffort : effort,
      ...compatibility, transport, requestAttempts: 1, credentialWrites: 0, retries: 0 }, null, 2));
    if (!result.passed) process.exitCode = 1;
  } catch (error) {
    console.log(JSON.stringify({ passed: false, category: knownErrors.has(error.message) ? error.message : 'LOCAL_CHECK_FAILED',
      ...compatibility, transport, requestAttempts: attempted ? 1 : 0, credentialWrites: 0, retries: 0 }, null, 2));
    process.exitCode = 1;
  }
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) await main();
