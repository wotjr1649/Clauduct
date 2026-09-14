import { randomUUID } from 'node:crypto';
import { request as httpsRequest, Agent as HttpsAgent } from 'node:https';
import { request as httpRequest, Agent as HttpAgent } from 'node:http';
import { buildHeaders, buildSearchHeaders, checkRuntime } from '../verification/manual-http-probe.mjs';
import { REFERENCE_CLIENT_VERSION, clientVersionPolicy } from './client-version.mjs';
import { ENDPOINT } from '../poc/adapter.mjs';
import { NativeError, need, NATIVE_LIMITS, EVENT_DIAGNOSTIC_TYPES, FAILURE_DIAGNOSTIC_CATEGORIES, UPSTREAM_FAILURES, upstreamFailure, searchEnvelope } from './native-protocol.mjs';
import { parseRetryAfter } from './retry-after.mjs';
import { observeRateLimitHeaders } from './rate-limit-observation.mjs';

export const NATIVE_TRANSPORT_LIMITS = Object.freeze({
  maxRetries: 5,
  maxFrameBytes: 8 * 1024 * 1024,
  maxEvents: 100_000,
  maxResponseBytes: NATIVE_LIMITS.responseBytes,
  timeoutMs: 600_000,
  retryBaseMs: 100,
  retryMaxMs: 2_000,
  retryAfterMaxMs: 5_000, // Short in-process wait budget; longer server delays are preserved and deferred.
  maxSockets: Infinity,
  maxFreeSockets: 2,
  idleSocketMs: 600_000
});

const criticalHeaders = ['content-type', 'content-encoding', 'content-length', 'transfer-encoding'];
const certificateErrors = new Set(['DEPTH_ZERO_SELF_SIGNED_CERT', 'SELF_SIGNED_CERT_IN_CHAIN',
  'UNABLE_TO_VERIFY_LEAF_SIGNATURE', 'UNABLE_TO_GET_ISSUER_CERT', 'UNABLE_TO_GET_ISSUER_CERT_LOCALLY',
  'INVALID_CA', 'PATH_LENGTH_EXCEEDED']);
function connectionFailure(error, timedOut) {
  // Keep raw Node/OpenSSL messages and unrecognized codes out of diagnostics.
  // A verification or access denial must not enter the transient I/O retry loop.
  const code = typeof error?.code === 'string' ? error.code : '';
  const category = timedOut ? 'UPSTREAM_IDLE_TIMEOUT'
    : certificateErrors.has(code) || ['CERT_', 'ERR_TLS_', 'ERR_SSL_', 'ERR_OSSL_'].some(prefix => code.startsWith(prefix)) ? 'UPSTREAM_TLS_ERROR'
    : ['EACCES', 'EPERM'].includes(code) ? 'UPSTREAM_ACCESS_DENIED'
    : ['EAI_AGAIN', 'ENOTFOUND'].includes(code) ? 'UPSTREAM_DNS_ERROR' : 'UPSTREAM_IO_ERROR';
  const failure = new NativeError(category);
  failure.retryable = category === 'UPSTREAM_IDLE_TIMEOUT' || category === 'UPSTREAM_DNS_ERROR'
    || category === 'UPSTREAM_IO_ERROR' && !code.startsWith('HPE_');
  return failure;
}
function searchConnectionFailure(error) {
  if (error instanceof NativeError) return error;
  const failure = connectionFailure(error, false);
  return failure.code === 'UPSTREAM_IO_ERROR'
    ? Object.assign(new NativeError('SEARCH_HTTP_ERROR'), { retryable: failure.retryable }) : failure;
}

export function createNativeTransport({ credential, credentialSupplier, clientVersion, requestBudget, onAttempt, onResponseLimits }) {
  clientVersionPolicy(clientVersion);
  checkRuntime(process.env, process.execArgv);
  need(typeof credentialSupplier === 'function' || validCredential(credential), 'INVALID_CREDENTIAL');
  return sender(httpsRequest, HttpsAgent, ENDPOINT,
    { credential, credentialSupplier, clientVersion, synthetic: false, options: { requestBudget, onAttempt, onResponseLimits } });
}

export function createNativeLoopbackTransport(port, options = {}) {
  need(Number.isInteger(port) && port > 0 && port < 65536, 'INVALID_LOOPBACK_PORT');
  const credential = options.credential ?? { accessToken: 'synthetic', account: 'synthetic' };
  need(typeof options.credentialSupplier === 'function' || validCredential(credential), 'INVALID_CREDENTIAL');
  return sender(httpRequest, HttpAgent, `http://127.0.0.1:${port}/backend-api/codex/responses`, {
    credential, credentialSupplier: options.credentialSupplier, clientVersion: options.clientVersion ?? REFERENCE_CLIENT_VERSION, synthetic: true, options
  });
}

function validCredential(value) {
  return value && typeof value.accessToken === 'string' && value.accessToken.length > 0
    && value.accessToken.length <= 16000 && !/[\r\n]/.test(value.accessToken)
    && typeof value.account === 'string' && /^[A-Za-z0-9_-]{1,128}$/.test(value.account);
}

function credentialCopy(value, account) {
  if (!validCredential(value) || (account !== undefined && value.account !== account)) {
    throw new NativeError(account !== undefined && validCredential(value) ? 'CREDENTIAL_ACCOUNT_MISMATCH'
      : 'CREDENTIAL_UNAVAILABLE_OR_EXPIRED');
  }
  return { accessToken: value.accessToken, account: value.account };
}

function fixedCredentialError() {
  return new NativeError('CREDENTIAL_UNAVAILABLE_OR_EXPIRED');
}

function optionInteger(value, fallback, minimum, maximum) {
  return value === undefined ? fallback : Number.isSafeInteger(value) && value >= minimum && value <= maximum ? value : null;
}

function sender(request, Agent, destination, { credential, credentialSupplier, clientVersion, synthetic, options = {} }) {
  const requestBudget = options.requestBudget;
  need(requestBudget === undefined || (Number.isSafeInteger(requestBudget) && requestBudget >= 1 && requestBudget <= 4096), 'INVALID_LIMIT');
  need(options.onAttempt === undefined || typeof options.onAttempt === 'function', 'INVALID_OPTIONS');
  need(options.onResponseLimits === undefined || typeof options.onResponseLimits === 'function', 'INVALID_OPTIONS');
  const compatibility = clientVersionPolicy(clientVersion);
  const settings = {
    maxRetries: NATIVE_TRANSPORT_LIMITS.maxRetries,
    maxFrameBytes: optionInteger(options.maxFrameBytes, NATIVE_TRANSPORT_LIMITS.maxFrameBytes, 1, 8 * 1024 * 1024),
    maxEvents: optionInteger(options.maxEvents, NATIVE_TRANSPORT_LIMITS.maxEvents, 1, 100_000),
    maxResponseBytes: optionInteger(options.maxResponseBytes, NATIVE_TRANSPORT_LIMITS.maxResponseBytes, 1, NATIVE_LIMITS.responseBytes),
    timeoutMs: optionInteger(options.timeoutMs ?? options.requestTimeoutMs, NATIVE_TRANSPORT_LIMITS.timeoutMs, 1, 600_000),
    retryBaseMs: optionInteger(options.retryDelayMs ?? options.retryBaseMs, NATIVE_TRANSPORT_LIMITS.retryBaseMs, 0, 60_000),
    retryMaxMs: optionInteger(options.retryDelayMs ?? options.retryMaxMs, NATIVE_TRANSPORT_LIMITS.retryMaxMs, 0, 600_000),
    retryAfterMaxMs: optionInteger(options.retryAfterMaxMs, NATIVE_TRANSPORT_LIMITS.retryAfterMaxMs, 0, 600_000),
    maxSockets: options.maxSockets === undefined ? NATIVE_TRANSPORT_LIMITS.maxSockets
      : optionInteger(options.maxSockets, NATIVE_TRANSPORT_LIMITS.maxSockets, 1, 1024),
    maxFreeSockets: optionInteger(options.maxFreeSockets, NATIVE_TRANSPORT_LIMITS.maxFreeSockets, 0, 32),
    idleSocketMs: optionInteger(options.idleSocketMs, NATIVE_TRANSPORT_LIMITS.idleSocketMs, 1, 600_000)
  };
  for (const value of Object.values(settings)) need(value !== null, 'INVALID_LIMIT');
  need(settings.maxFreeSockets <= settings.maxSockets, 'INVALID_LIMIT');

  const agent = new Agent({ keepAlive: true, maxSockets: settings.maxSockets,
    maxFreeSockets: settings.maxFreeSockets, keepAliveMsecs: 1000, timeout: settings.idleSocketMs });
  const active = new Set(), sockets = new Set(), socketDone = new WeakMap();
  let staticCredential = validCredential(credential) ? credentialCopy(credential) : undefined;
  let supplier = typeof credentialSupplier === 'function' ? credentialSupplier : undefined;
  let closed = false, attempts = 0, retries = 0, connectionAttempts = 0;
  let lastCategory = 'NONE', lastStatus = null, responseBytes = 0, totalResponseBytes = 0;
  let retryNotBeforeMs = 0, retryAfterUnrepresentable = false;
  let responseObserverFailed = false;

  function beginAttempt() {
    attempts++;
    // A process-owned synchronous observer can persist this reservation before
    // any socket is opened. No credentials, destination, headers or body escape.
    try { options.onAttempt?.(Object.freeze({ requestAttempts: attempts })); }
    catch { throw new NativeError('ATTEMPT_OBSERVER_FAILED'); }
    return attempts;
  }

  function observeResponseLimits(response, requestAttempt, kind) {
    need(!responseObserverFailed, 'RESPONSE_OBSERVER_FAILED');
    if (!options.onResponseLimits) return;
    try {
      const result = options.onResponseLimits(Object.freeze({ requestAttempt, kind, httpStatus: response.statusCode,
        observation: observeRateLimitHeaders(response.rawHeaders) }));
      // The reservation/evidence callback must finish synchronously, before
      // response events are released. An async callback cannot attest that.
      if (result !== undefined) {
        Promise.resolve(result).catch(() => {});
        throw new NativeError('RESPONSE_OBSERVER_FAILED');
      }
    } catch {
      responseObserverFailed = true;
      throw new NativeError('RESPONSE_OBSERVER_FAILED');
    }
  }

  function checkAttemptState(signal) {
    need(!responseObserverFailed, 'RESPONSE_OBSERVER_FAILED');
    need(!signal.aborted, 'CANCELLED');
    need(requestBudget === undefined || attempts < requestBudget, 'REQUEST_BUDGET');
    checkRetryDeadline();
  }

  function diagnostics() {
    const idleSockets = Object.values(agent.freeSockets).reduce((count, list) => count + list.length, 0);
    return { ...compatibility, activeRequests: active.size, activeSockets: sockets.size, idleSockets,
      requestAttempts: attempts, retries, connectionAttempts, responseBytes, totalResponseBytes, httpStatus: lastStatus,
      category: lastCategory, retryNotBeforeMs, retryAfterUnrepresentable, synthetic, closed };
  }

  function trackSocket(socket) {
    if (socketDone.has(socket)) return;
    // ClientRequest emits its socket asynchronously. A socket may have closed
    // before this observer runs; its close event will never fire a second time.
    if (socket.closed) { socketDone.set(socket, Promise.resolve()); return; }
    connectionAttempts++;
    sockets.add(socket);
    socket.once('close', () => sockets.delete(socket));
    socket.once('error', () => {});
    socketDone.set(socket, new Promise(resolve => socket.once('close', resolve)));
  }

  async function resolveCredential(force, account) {
    if (!supplier) {
      if (!staticCredential) throw fixedCredentialError();
      if (account !== undefined && staticCredential.account !== account) throw new NativeError('CREDENTIAL_ACCOUNT_MISMATCH');
      return credentialCopy(staticCredential, account);
    }
    let value;
    try { value = await supplier({ force: force === true }); }
    catch (error) {
      const code = error?.code;
      if (['CREDENTIAL_ACCOUNT_MISMATCH', 'CREDENTIAL_ACCOUNT_CHANGED'].includes(code)) throw new NativeError(code);
      throw fixedCredentialError();
    }
    if (closed) throw new NativeError('TRANSPORT_CLOSED');
    try { return credentialCopy(value, account); }
    catch (error) { throw error instanceof NativeError ? error : fixedCredentialError(); }
  }

  function mark(error) {
    lastCategory = error?.code ?? 'UPSTREAM_IO_ERROR';
    lastStatus = Number.isInteger(error?.statusCode) ? error.statusCode : lastStatus;
    return error;
  }

  function deferred(failure) {
    return Object.assign(new NativeError('UPSTREAM_RETRY_DEFERRED'), { retryAtMs: retryNotBeforeMs,
      retryAfterMs: Math.max(0, retryNotBeforeMs - Date.now()), statusCode: failure?.statusCode ?? lastStatus });
  }

  function checkRetryDeadline() {
    need(!retryAfterUnrepresentable, 'UPSTREAM_RETRY_UNREPRESENTABLE');
    if (retryNotBeforeMs > Date.now()) throw deferred();
  }

  function preserveRetryAfter(error, response) {
    const parsed = parseRetryAfter(response.headers['retry-after']);
    if (error.retryable === true && parsed) {
      if (parsed.unrepresentable) {
        retryAfterUnrepresentable = true;
        return Object.assign(new NativeError('UPSTREAM_RETRY_UNREPRESENTABLE'), { retryable: false, statusCode: error.statusCode });
      }
      retryNotBeforeMs = Math.max(retryNotBeforeMs, parsed.retryAtMs);
      error.retryAtMs = retryNotBeforeMs;
      error.retryAfterMs = Math.max(0, retryNotBeforeMs - Date.now());
    }
    return error;
  }

  function retryDelay(attempt, failure) {
    const exponential = Math.min(settings.retryMaxMs, settings.retryBaseMs * 2 ** (attempt - 1));
    const minimum = Math.max(failure.retryAfterMs ?? 0, retryNotBeforeMs - Date.now());
    if (minimum > settings.retryAfterMaxMs) throw deferred(failure);
    const half = Math.ceil(exponential / 2);
    const jittered = half + Math.floor(Math.random() * (exponential - half + 1));
    return Math.max(jittered, minimum);
  }

  function errorForStatus(response) {
    const status = response.statusCode;
    const error = new NativeError(status === 401 ? 'UNAUTHENTICATED'
      : status === 429 ? 'RATE_LIMITED' : 'UPSTREAM_HTTP_ERROR');
    error.statusCode = status;
    error.retryable = status === 429 || (status >= 500 && status <= 599);
    return preserveRetryAfter(error, response);
  }

  function validateHeaders(response) {
    const names = response.rawHeaders.filter((_, index) => index % 2 === 0).map(name => name.toLowerCase());
    for (const name of criticalHeaders) need(names.filter(item => item === name).length <= 1, 'DUPLICATE_UPSTREAM_HEADER');
    const encoding = response.headers['content-encoding'];
    need(!encoding || encoding === 'identity', 'UNSUPPORTED_ENCODING');
    const contentType = response.headers['content-type'];
    need(contentType === undefined || (typeof contentType === 'string' && !/[\x00-\x1f\x7f,]/.test(contentType)
      && contentType.split(';', 1)[0].trim().toLowerCase() === 'text/event-stream'), 'UNSUPPORTED_CONTENT_TYPE');
  }

  function boundary(text) {
    const lf = text.indexOf('\n\n'), crlf = text.indexOf('\r\n\r\n');
    if (lf < 0) return crlf < 0 ? null : [crlf, 4];
    if (crlf < 0 || lf < crlf) return [lf, 2];
    return [crlf, 4];
  }

  function parser({ onEvent, events, timing }) {
    const decoder = new TextDecoder('utf-8', { fatal: true });
    let pending = '', eventCount = 0, completed = false, sentinel = false;
    let sequenceMode = false, nextSequence = 0;

    // Diagnostic parsing never accepts a rejected frame or retains upstream text.
    const rejectAfterCompletion = raw => {
      timing.postCompletionFrame = raw === '[DONE]' ? 'done' : 'invalid-json';
      if (raw !== '[DONE]') {
        // Bound diagnostic-only work; large trailers remain rejected and unclassified.
        if (Buffer.byteLength(raw) > 16384) timing.postCompletionFrame = 'oversized';
        else {
          let event;
          try { event = JSON.parse(raw); } catch { /* Keep fixed invalid-json classification. */ }
          if (event !== undefined) {
            timing.postCompletionFrame = EVENT_DIAGNOSTIC_TYPES.includes(event?.type) ? event.type : 'other';
            const sequence = event?.sequence_number;
            timing.postCompletionSequence = sequence === undefined ? (sequenceMode ? 'missing' : 'unsequenced')
              : !Number.isSafeInteger(sequence) || sequence < 0 ? 'invalid'
                : sequence === nextSequence ? 'expected' : 'unexpected';
          }
        }
      }
      throw new NativeError('EVENT_AFTER_COMPLETION');
    };

    const deliver = async value => {
      if (onEvent) await onEvent(value);
      else events.push(value);
    };

    const frame = async value => {
      need(Buffer.byteLength(value) <= settings.maxFrameBytes, 'FRAME_TOO_LARGE');
      const normalized = value.replaceAll('\r\n', '\n');
      need(!normalized.includes('\r'), 'INVALID_SSE');
      if (!normalized) return;
      let name, data = [];
      for (const line of normalized.split('\n')) {
        if (line.startsWith(':')) continue;
        if (line.startsWith('event:')) {
          need(name === undefined, 'INVALID_SSE'); name = line.slice(6).trim();
        } else if (line.startsWith('data:')) {
          data.push(line.startsWith('data: ') ? line.slice(6) : line.slice(5));
        } else throw new NativeError('INVALID_SSE');
      }
      if (!data.length) { need(name === undefined, 'INVALID_SSE'); return; }
      const raw = data.join('\n');
      if (sentinel) rejectAfterCompletion(raw);
      if (raw === '[DONE]') {
        need(name === undefined && completed, 'INCOMPLETE_RESPONSE');
        sentinel = true; timing.terminalState = 'done'; return;
      }
      if (completed) rejectAfterCompletion(raw);
      eventCount++;
      need(eventCount <= settings.maxEvents, 'TOO_MANY_EVENTS');
      let event;
      try { event = JSON.parse(raw); } catch { throw new NativeError('INVALID_SSE'); }
      need(event && typeof event === 'object' && !Array.isArray(event) && typeof event.type === 'string', 'INVALID_SSE');
      if (event.type === 'keepalive' && Object.keys(event).length === 1) {
        // JSON.parse collapses duplicate keys. An accepted empty heartbeat
        // must contain exactly one string pair in the original JSON as well.
        need(/^\s*\{\s*"(?:[^"\\]|\\.)*"\s*:\s*"(?:[^"\\]|\\.)*"\s*\}\s*$/s.test(raw), 'INVALID_SSE');
      }
      need(name === undefined || name === event.type, 'INVALID_SSE');
      if (event.sequence_number !== undefined) {
        need(Number.isSafeInteger(event.sequence_number) && event.sequence_number >= 0, 'SEQUENCE_MISMATCH');
        if (!sequenceMode) { need(event.sequence_number === 0, 'SEQUENCE_MISMATCH'); sequenceMode = true; }
        need(event.sequence_number === nextSequence, 'SEQUENCE_MISMATCH');
        nextSequence++;
      } else need(!sequenceMode, 'SEQUENCE_MISMATCH');
      if (Object.hasOwn(UPSTREAM_FAILURES, event.type)) {
        timing.terminalState = event.type;
        throw upstreamFailure(event);
      }
      if (event.type === 'response.completed') {
        need(!completed, 'DUPLICATE_COMPLETION'); completed = true; timing.terminalState = 'completed';
      }
      await deliver(event);
    };

    const pushText = async text => {
      if (!text) return;
      pending += text;
      for (;;) {
        const found = boundary(pending);
        if (!found) {
          need(Buffer.byteLength(pending) <= settings.maxFrameBytes, 'FRAME_TOO_LARGE');
          break;
        }
        const [index, length] = found, value = pending.slice(0, index);
        pending = pending.slice(index + length);
        await frame(value);
      }
    };

    return {
      async push(chunk) {
        try { await pushText(decoder.decode(chunk, { stream: true })); }
        catch (error) { throw error instanceof NativeError ? error : new NativeError('INVALID_UTF8'); }
      },
      async finish(complete) {
        try { await pushText(decoder.decode()); }
        catch (error) { throw error instanceof NativeError ? error : new NativeError('INVALID_UTF8'); }
        need(complete && pending.length === 0, 'TRUNCATED_STREAM');
        need(completed, 'INCOMPLETE_RESPONSE');
        return events;
      }
    };
  }

  async function requestOnce(job, raw, current, onEvent, isRetry) {
    checkAttemptState(job.controller.signal);
    let req, response, socket, socketClosed, requestAttempt, timedOut = false, reusable = false, streaming = false, bytes = 0;
    const collected = onEvent ? undefined : [];
    const headers = buildHeaders(current, clientVersion, raw);
    const elapsed = () => Math.round((performance.now() - job.started) * 100) / 100;
    const timing = { attempt: job.attemptTimings.length + 1, startedMs: elapsed(), requestFlushedMs: null,
      headersMs: null, firstBodyMs: null, endedMs: null, status: null, completed: false, failureCategory: null,
      terminalState: 'open', postCompletionFrame: null, postCompletionSequence: null };
    const state = parser({ onEvent, events: collected, timing });
    job.attemptTimings.push(timing);
    try {
      lastStatus = null; responseBytes = 0; requestAttempt = beginAttempt();
      if (isRetry) retries++;
      response = await new Promise((resolve, reject) => {
        req = request(destination, { method: 'POST', agent, signal: job.controller.signal,
          rejectUnauthorized: true, maxHeaderSize: 16384, headers }, resolve);
        req.once('error', reject);
        req.once('finish', () => { timing.requestFlushedMs = elapsed(); });
        req.once('upgrade', (_response, socket) => { socket.destroy(); reject(new NativeError('UPGRADE_REJECTED')); });
        req.once('socket', value => {
          socket = value;
          trackSocket(socket);
          socketClosed = socketDone.get(socket);
          // An already-emitted close cannot reject ClientRequest's response
          // promise. Fail this attempt directly; do not await another event.
          if (socket.closed) {
            const error = new NativeError('UPSTREAM_IO_ERROR');
            req.destroy(error); reject(error);
          }
        });
        req.setTimeout(settings.timeoutMs, () => {
          timedOut = true; req.destroy(new NativeError('UPSTREAM_IDLE_TIMEOUT'));
        });
        req.end(raw);
      });
      lastStatus = response.statusCode ?? null;
      timing.status = lastStatus; timing.headersMs = elapsed();
      observeResponseLimits(response, requestAttempt, 'responses');
      if (response.statusCode !== 200) {
        throw errorForStatus(response);
      }
      validateHeaders(response);
      streaming = true;
      for await (const chunk of response) {
        timing.firstBodyMs ??= elapsed();
        bytes += chunk.length; totalResponseBytes += chunk.length;
        need(bytes <= settings.maxResponseBytes, 'RESPONSE_TOO_LARGE');
        await state.push(chunk);
      }
      const result = await state.finish(response.complete === true);
      reusable = true;
      return result;
    } catch (error) {
      const reject = failure => {
        timing.failureCategory = FAILURE_DIAGNOSTIC_CATEGORIES.includes(failure.code) ? failure.code : 'OTHER';
        throw failure;
      };
      // A validated mismatch predates any cancellation arriving while the callback unwinds.
      if (error instanceof NativeError && error.code === 'SNAPSHOT_MISMATCH') reject(error);
      if (job.controller.signal.aborted) {
        if (timedOut) {
          const timeout = new NativeError('UPSTREAM_IDLE_TIMEOUT'); timeout.retryable = true; reject(timeout);
        }
        reject(new NativeError('CANCELLED'));
      }
      if (error instanceof NativeError) reject(error);
      if (streaming) {
        const truncated = new NativeError('TRUNCATED_STREAM'); truncated.retryable = true; reject(truncated);
      }
      reject(connectionFailure(error, timedOut));
    } finally {
      timing.endedMs = elapsed(); timing.completed = reusable;
      if (response?.statusCode === 200) responseBytes = bytes;
      if (!reusable) {
        response?.destroy(); req?.destroy(); socket?.destroy();
        if (socketClosed) await socketClosed;
      } else if (req && !req.writableFinished) req.destroy();
    }
  }

  async function wait(job, milliseconds) {
    if (!milliseconds) return;
    if (job.controller.signal.aborted) throw new NativeError('CANCELLED');
    await new Promise((resolve, reject) => {
      let timer;
      const abort = () => { clearTimeout(timer); job.timers.delete(timer); job.controller.signal.removeEventListener('abort', abort); reject(new NativeError('CANCELLED')); };
      timer = setTimeout(() => { job.timers.delete(timer); job.controller.signal.removeEventListener('abort', abort); resolve(); }, milliseconds);
      job.timers.add(timer); job.controller.signal.addEventListener('abort', abort, { once: true });
    });
  }

  async function allowRetry(canRetry) {
    try { return (await canRetry()) === true; } catch { return false; }
  }

  async function send(body, signal, options = {}) {
    need(!responseObserverFailed, 'RESPONSE_OBSERVER_FAILED');
    need(requestBudget === undefined || attempts < requestBudget, 'REQUEST_BUDGET');
    checkRetryDeadline();
    need(signal instanceof AbortSignal && !signal.aborted, 'CANCELLED');
    need(!closed, 'TRANSPORT_CLOSED');
    need(options && typeof options === 'object', 'INVALID_OPTIONS');
    const onEvent = options.onEvent;
    const canRetry = options.canRetry === undefined ? async () => true : options.canRetry;
    const onRetry = options.onRetry;
    need(onEvent === undefined || typeof onEvent === 'function', 'INVALID_OPTIONS');
    need(typeof canRetry === 'function' && (onRetry === undefined || typeof onRetry === 'function'), 'INVALID_OPTIONS');
    need(options.attemptTimings === undefined || (Array.isArray(options.attemptTimings)
      && options.attemptTimings.length === 0 && Object.isExtensible(options.attemptTimings)), 'INVALID_OPTIONS');
    let raw;
    try { raw = JSON.stringify(body); } catch { throw new NativeError('INVALID_REQUEST'); }
    need(typeof raw === 'string' && Buffer.byteLength(raw) <= NATIVE_LIMITS.requestBytes, 'INPUT_TOO_LARGE');

    const controller = new AbortController();
    const abort = () => controller.abort();
    signal.addEventListener('abort', abort, { once: true });
    let finish;
    const job = { controller, timers: new Set(), started: performance.now(), attemptTimings: options.attemptTimings ?? [],
      finished: new Promise(resolve => { finish = resolve; }) };
    active.add(job);
    const canRetryConfigured = options.canRetry !== undefined;
    let outputStarted = false, retryCount = 0, refreshed = false, current, isRetry = false;
    const deliver = onEvent ? async event => {
      outputStarted = true;
      try { await onEvent(event); }
      catch (error) { throw error instanceof NativeError ? error : new NativeError('DOWNSTREAM_CALLBACK_FAILED'); }
    } : undefined;
    const retryAllowed = async () => (canRetryConfigured || !outputStarted) && await allowRetry(canRetry);
    try {
      current = await resolveCredential(false);
      const account = current.account;
      for (;;) {
        need(!controller.signal.aborted, 'CANCELLED');
        try {
          const result = await requestOnce(job, raw, current, deliver, isRetry);
          isRetry = false;
          lastCategory = 'SUCCESS';
          return onEvent ? undefined : result;
        } catch (error) {
          const failure = mark(error);
          if (failure.code === 'UNAUTHENTICATED' && !refreshed && supplier && retryCount < settings.maxRetries) {
            if (!await retryAllowed()) throw failure;
            checkAttemptState(controller.signal);
            const delay = retryDelay(retryCount + 1, failure);
            current = await resolveCredential(true, account); refreshed = true; retryCount++; isRetry = true;
            await onRetry?.(retryCount);
            await wait(job, delay);
            continue;
          }
          const transient = failure.retryable === true || (failure.code === 'UPSTREAM_IO_ERROR'
            && failure.retryable !== false) || failure.code === 'UPSTREAM_IDLE_TIMEOUT';
          if (!transient || retryCount >= settings.maxRetries || !await retryAllowed()) throw failure;
          const delay = retryDelay(retryCount + 1, failure);
          retryCount++; isRetry = true;
          await onRetry?.(retryCount);
          await wait(job, delay);
        }
      }
    } catch (error) {
      lastCategory = error?.code ?? 'UPSTREAM_IO_ERROR';
      throw error instanceof NativeError ? error : new NativeError('UPSTREAM_IO_ERROR');
    } finally {
      signal.removeEventListener('abort', abort);
      raw = undefined; current = undefined;
      for (const timer of job.timers) clearTimeout(timer);
      job.timers.clear(); active.delete(job); finish();
    }
  }

  // The client's search side query is answered from the backend's standalone search endpoint:
  // one plain JSON POST, no stream, no model turn. Same host and same credential as a model
  // request, so nothing new is trusted and no second secret exists.
  const searchDestination = destination.replace(/\/responses$/, '/alpha/search');
  need(searchDestination !== destination, 'INVALID_ENDPOINT');
  const searchSession = randomUUID();

  async function search(body, signal) {
    need(!responseObserverFailed, 'RESPONSE_OBSERVER_FAILED');
    need(requestBudget === undefined || attempts < requestBudget, 'REQUEST_BUDGET');
    checkRetryDeadline();
    need(!closed, 'TRANSPORT_CLOSED');
    need(signal instanceof AbortSignal, 'INVALID_OPTIONS');
    // An already-aborted signal never fires its listener, so check it rather than sending.
    need(!signal.aborted, 'CANCELLED');
    const job = { controller: new AbortController(), timers: new Set(), started: performance.now(),
      attemptTimings: [], finished: Promise.resolve() };
    let finish;
    job.finished = new Promise(resolve => { finish = resolve; });
    const abort = () => job.controller.abort();
    signal.addEventListener('abort', abort, { once: true });
    active.add(job);
    let current;
    try {
      current = await resolveCredential(false);
      const account = current.account;
      try { return await searchOnce(job, body, current); }
      catch (error) {
        // A search is an idempotent read, so one retry cannot duplicate an effect. Exactly one:
        // a side query the client is waiting on is not the place to spend a retry budget.
        if (error?.retryable === true) {
          await wait(job, retryDelay(1, error));
          need(!job.controller.signal.aborted, 'CANCELLED');
          return await searchOnce(job, body, current, true);
        }
        if (error?.code !== 'UNAUTHENTICATED' || !supplier) throw error;
        checkAttemptState(job.controller.signal);
        current = await resolveCredential(true, account);
        return await searchOnce(job, body, current, true);
      }
    } catch (error) {
      lastCategory = error?.code ?? 'SEARCH_UNAVAILABLE';
      throw error instanceof NativeError ? error : new NativeError('SEARCH_UNAVAILABLE');
    } finally {
      signal.removeEventListener('abort', abort);
      current = undefined;
      for (const timer of job.timers) clearTimeout(timer);
      job.timers.clear(); active.delete(job); finish();
    }
  }

  function searchOnce(job, body, credential, isRetry = false) {
    checkAttemptState(job.controller.signal);
    const raw = JSON.stringify({ ...body, id: searchSession });
    const headers = buildSearchHeaders(credential, clientVersion, raw,
      searchEnvelope().headers['x-codex-turn-metadata']);
    const timeoutMs = Math.min(settings.timeoutMs, 45_000);
    lastStatus = null; responseBytes = 0; const requestAttempt = beginAttempt();
    if (isRetry) retries++;
    return new Promise((resolveResult, reject) => {
      let settled = false, req;
      const done = (action, value) => { if (!settled) { settled = true; clearTimeout(timer); action(value); } };
      const timer = setTimeout(() => { req?.destroy(); done(reject, new NativeError('UPSTREAM_IDLE_TIMEOUT')); }, timeoutMs);
      job.timers.add(timer);
      try { req = request(searchDestination, { method: 'POST', agent, headers,
        ...(request === httpsRequest && { rejectUnauthorized: true }) }, res => {
        const chunks = [];
        let length = 0;
        lastStatus = res.statusCode ?? 0;
        try { observeResponseLimits(res, requestAttempt, 'search'); }
        catch (error) { res.destroy(); req.destroy(); done(reject, error); return; }
        res.on('data', chunk => {
          if ((length += chunk.length) > settings.maxResponseBytes) {
            req.destroy(); done(reject, new NativeError('RESPONSE_TOO_LARGE'));
            return;
          }
          chunks.push(chunk);
        });
        res.on('error', () => done(reject, new NativeError('SEARCH_UNAVAILABLE')));
        res.on('end', () => {
          responseBytes = length; totalResponseBytes += length;
          const status = res.statusCode ?? 0;
          if (status === 401) { done(reject, new NativeError('UNAUTHENTICATED')); return; }
          // An alpha endpoint that is gone is a different problem from one that is briefly
          // unwell: the first ends the feature, the second is worth one more try.
          if (status === 404 || status === 410) { done(reject, new NativeError('SEARCH_UNAVAILABLE')); return; }
          if (status !== 200) {
            done(reject, preserveRetryAfter(Object.assign(new NativeError('SEARCH_HTTP_ERROR'),
              { statusCode: status, retryable: status === 429 || (status >= 500 && status <= 599) }), res));
            return;
          }
          let doc;
          try { doc = JSON.parse(Buffer.concat(chunks).toString('utf8')); }
          catch { done(reject, new NativeError('SEARCH_RESPONSE_SHAPE')); return; }
          lastCategory = 'SUCCESS';
          done(resolveResult, doc);
        });
      });
      req.on('socket', socket => {
        trackSocket(socket);
        if (socket.closed) {
          const error = Object.assign(new NativeError('SEARCH_HTTP_ERROR'), { retryable: true });
          req.destroy(error); done(reject, error);
        }
      }); } catch (error) { done(reject, searchConnectionFailure(error)); return; }
      req.on('error', error => done(reject, job.controller.signal.aborted ? new NativeError('CANCELLED')
        : searchConnectionFailure(error)));
      job.controller.signal.addEventListener('abort', () => { req.destroy(); done(reject, new NativeError('CANCELLED')); }, { once: true });
      req.end(raw);
    });
  }

  async function close() {
    if (!closed) {
      closed = true; supplier = undefined; staticCredential = undefined;
      for (const job of active) job.controller.abort();
      agent.destroy();
    }
    await Promise.all([...active].map(job => job.finished));
    await Promise.all([...sockets].map(socket => {
      socket.destroy();
      return socketDone.get(socket); // destroyed is not the close event that clears tracking.
    }));
    return diagnostics();
  }

  return Object.freeze({ send, search, close, diagnostics });
}
