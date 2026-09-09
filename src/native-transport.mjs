import { request as httpsRequest, Agent as HttpsAgent } from 'node:https';
import { request as httpRequest, Agent as HttpAgent } from 'node:http';
import { buildHeaders, checkRuntime } from '../verification/manual-http-probe.mjs';
import { CLIENT_VERSION } from '../poc/codex-transport.mjs';
import { ENDPOINT } from '../poc/adapter.mjs';
import { NativeError, need, NATIVE_LIMITS } from './native-protocol.mjs';

export const NATIVE_TRANSPORT_LIMITS = Object.freeze({
  maxRetries: 5,
  maxFrameBytes: 8 * 1024 * 1024,
  maxEvents: 100_000,
  maxResponseBytes: NATIVE_LIMITS.responseBytes,
  timeoutMs: 600_000,
  retryBaseMs: 100,
  retryMaxMs: 2_000,
  retryAfterMaxMs: 5_000,
  maxSockets: Infinity,
  maxFreeSockets: 2,
  idleSocketMs: 600_000
});

const criticalHeaders = ['content-type', 'content-encoding', 'content-length', 'transfer-encoding'];

export function createNativeTransport({ credential, credentialSupplier, clientVersion }) {
  need(clientVersion === CLIENT_VERSION, 'CLI_VERSION_CHANGED');
  checkRuntime(process.env, process.execArgv);
  need(typeof credentialSupplier === 'function' || validCredential(credential), 'INVALID_CREDENTIAL');
  return sender(httpsRequest, HttpsAgent, ENDPOINT,
    { credential, credentialSupplier, synthetic: false });
}

export function createNativeLoopbackTransport(port, options = {}) {
  need(Number.isInteger(port) && port > 0 && port < 65536, 'INVALID_LOOPBACK_PORT');
  const credential = options.credential ?? { accessToken: 'synthetic', account: 'synthetic' };
  need(typeof options.credentialSupplier === 'function' || validCredential(credential), 'INVALID_CREDENTIAL');
  return sender(httpRequest, HttpAgent, `http://127.0.0.1:${port}/backend-api/codex/responses`, {
    credential, credentialSupplier: options.credentialSupplier, synthetic: true, options
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

function sender(request, Agent, destination, { credential, credentialSupplier, synthetic, options = {} }) {
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

  function diagnostics() {
    const idleSockets = Object.values(agent.freeSockets).reduce((count, list) => count + list.length, 0);
    return { activeRequests: active.size, activeSockets: sockets.size, idleSockets,
      requestAttempts: attempts, retries, connectionAttempts, responseBytes, totalResponseBytes, httpStatus: lastStatus,
      category: lastCategory, synthetic, closed };
  }

  function trackSocket(socket) {
    if (sockets.has(socket)) return;
    connectionAttempts++;
    sockets.add(socket);
    socket.once('close', () => sockets.delete(socket));
    socket.once('error', () => {});
    socketDone.set(socket, new Promise(resolve => {
      if (socket.destroyed) resolve();
      else socket.once('close', resolve);
    }));
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

  function retryAfter(headers) {
    const value = Array.isArray(headers['retry-after']) ? headers['retry-after'][0] : headers['retry-after'];
    if (typeof value !== 'string') return 0;
    const seconds = Number(value.trim());
    if (Number.isFinite(seconds) && seconds >= 0) return Math.min(settings.retryAfterMaxMs, seconds * 1000);
    const date = Date.parse(value);
    return Number.isFinite(date) ? Math.min(settings.retryAfterMaxMs, Math.max(0, date - Date.now())) : 0;
  }

  function retryDelay(attempt, failure) {
    const exponential = Math.min(settings.retryMaxMs, settings.retryBaseMs * 2 ** (attempt - 1));
    return Math.max(exponential, failure.retryAfterMs ?? 0);
  }

  function errorForStatus(response) {
    const status = response.statusCode;
    const error = new NativeError(status === 401 ? 'UNAUTHENTICATED'
      : status === 429 ? 'RATE_LIMITED' : 'UPSTREAM_HTTP_ERROR');
    error.statusCode = status;
    error.retryAfterMs = retryAfter(response.headers);
    error.retryable = status === 429 || (status >= 500 && status <= 599);
    return error;
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

  function parser({ onEvent, events }) {
    const decoder = new TextDecoder('utf-8', { fatal: true });
    let pending = '', eventCount = 0, completed = false, sentinel = false;
    let sequenceMode = false, nextSequence = 0;

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
      if (sentinel) throw new NativeError('EVENT_AFTER_COMPLETION');
      const raw = data.join('\n');
      if (raw === '[DONE]') {
        need(name === undefined && completed, 'INCOMPLETE_RESPONSE');
        sentinel = true; return;
      }
      need(!completed, 'EVENT_AFTER_COMPLETION');
      eventCount++;
      need(eventCount <= settings.maxEvents, 'TOO_MANY_EVENTS');
      let event;
      try { event = JSON.parse(raw); } catch { throw new NativeError('INVALID_SSE'); }
      need(event && typeof event === 'object' && !Array.isArray(event) && typeof event.type === 'string', 'INVALID_SSE');
      need(name === undefined || name === event.type, 'INVALID_SSE');
      if (event.sequence_number !== undefined) {
        need(Number.isSafeInteger(event.sequence_number) && event.sequence_number >= 0, 'SEQUENCE_MISMATCH');
        if (!sequenceMode) { need(event.sequence_number === 0, 'SEQUENCE_MISMATCH'); sequenceMode = true; }
        need(event.sequence_number === nextSequence, 'SEQUENCE_MISMATCH');
        nextSequence++;
      } else need(!sequenceMode, 'SEQUENCE_MISMATCH');
      if (event.type === 'response.completed') {
        need(!completed, 'DUPLICATE_COMPLETION'); completed = true;
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
    let req, response, socket, socketClosed, timedOut = false, reusable = false, streaming = false, bytes = 0;
    const collected = onEvent ? undefined : [];
    const state = parser({ onEvent, events: collected });
    const headers = buildHeaders(current, CLIENT_VERSION, raw);
    const elapsed = () => Math.round((performance.now() - job.started) * 100) / 100;
    const timing = { attempt: job.attemptTimings.length + 1, startedMs: elapsed(), requestFlushedMs: null,
      headersMs: null, firstBodyMs: null, endedMs: null, status: null, completed: false };
    job.attemptTimings.push(timing);
    try {
      attempts++; lastStatus = null; responseBytes = 0;
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
        });
        req.setTimeout(settings.timeoutMs, () => {
          timedOut = true; req.destroy(new NativeError('UPSTREAM_IDLE_TIMEOUT'));
        });
        req.end(raw);
      });
      lastStatus = response.statusCode ?? null;
      timing.status = lastStatus; timing.headersMs = elapsed();
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
      if (job.controller.signal.aborted) {
        if (timedOut) {
          const timeout = new NativeError('UPSTREAM_IDLE_TIMEOUT'); timeout.retryable = true; throw timeout;
        }
        throw new NativeError('CANCELLED');
      }
      if (error instanceof NativeError) throw error;
      if (streaming) {
        const truncated = new NativeError('TRUNCATED_STREAM'); truncated.retryable = true; throw truncated;
      }
      const io = new NativeError(timedOut ? 'UPSTREAM_IDLE_TIMEOUT' : 'UPSTREAM_IO_ERROR');
      io.retryable = timedOut || !String(error?.code ?? '').startsWith('HPE_'); throw io;
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
            current = await resolveCredential(true, account); refreshed = true; retryCount++; isRetry = true;
            await onRetry?.(retryCount);
            await wait(job, retryDelay(retryCount, failure));
            continue;
          }
          const transient = failure.retryable === true || (failure.code === 'UPSTREAM_IO_ERROR'
            && failure.retryable !== false) || failure.code === 'UPSTREAM_IDLE_TIMEOUT';
          if (!transient || retryCount >= settings.maxRetries || !await retryAllowed()) throw failure;
          retryCount++; isRetry = true;
          await onRetry?.(retryCount);
          await wait(job, retryDelay(retryCount, failure));
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

  async function close() {
    if (!closed) {
      closed = true; supplier = undefined; staticCredential = undefined;
      for (const job of active) job.controller.abort();
      agent.destroy();
    }
    await Promise.all([...active].map(job => job.finished));
    await Promise.all([...sockets].map(socket => new Promise(resolve => {
      if (socket.destroyed) { resolve(); return; }
      socket.once('close', resolve); socket.destroy();
    })));
    return diagnostics();
  }

  return Object.freeze({ send, close, diagnostics });
}
