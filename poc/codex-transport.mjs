import { request as httpsRequest } from 'node:https';
import { request as httpRequest } from 'node:http';
import { ENDPOINT, LIMITS, CHAT_REQUESTS, PROTOCOL_ERROR_CODES, probeProfile } from './adapter.mjs';
import { buildHeaders, selectCredential, checkRuntime } from '../verification/manual-http-probe.mjs';

import { REFERENCE_CLIENT_VERSION, clientVersionPolicy } from '../src/client-version.mjs';
class TransportError extends Error { constructor(code) { super(code); this.code = code; } }
function requireThat(ok, code) { if (!ok) throw new TransportError(code); }
async function destroySocket(socket, tracked) {
  // Track the close event itself: socket.closed can become true before that event is delivered.
  if (!tracked.has(socket)) return;
  await new Promise(resolve => { socket.once('close', resolve); socket.destroy(); });
}

// The caller supplies an already owned, in-memory credential. No files, login, refresh or writes.
export function createCodexTransport({ credential, clientVersion, profile = 'astra-xhigh', tokenLimitPolicy = 'reject', requestBudget = LIMITS.requests }) {
  clientVersionPolicy(clientVersion);
  const selected = probeProfile(profile);
  checkRuntime(process.env, process.execArgv);
  requireThat(typeof credential?.accessToken === 'string' && typeof credential?.account === 'string', 'INVALID_CREDENTIAL');
  const owned = { accessToken: credential.accessToken, account: credential.account };
  return createSender(httpsRequest, ENDPOINT, owned, false, LIMITS.timeoutMs, selected, tokenLimitPolicy, requestBudget, clientVersion);
}

// Synthetic Codex protocol endpoint for loopback integration checks; accepts no real credential.
export function createLoopbackCodexTransport(port, { timeoutMs = LIMITS.timeoutMs, profile = 'astra-xhigh', tokenLimitPolicy = 'reject', requestBudget = LIMITS.requests, clientVersion = REFERENCE_CLIENT_VERSION } = {}) {
  requireThat(Number.isInteger(port) && port > 0 && port <= 65535, 'INVALID_LOOPBACK_PORT');
  requireThat(Number.isInteger(timeoutMs) && timeoutMs > 0 && timeoutMs <= LIMITS.timeoutMs, 'INVALID_TIMEOUT');
  return createSender(httpRequest, `http://127.0.0.1:${port}/backend-api/codex/responses`,
    { accessToken: 'synthetic', account: 'synthetic' }, true, timeoutMs, probeProfile(profile), tokenLimitPolicy, requestBudget, clientVersion);
}

function createSender(nativeRequest, destination, credential, synthetic, timeoutMs, profile, tokenLimitPolicy, requestBudget, clientVersion) {
  const compatibility = clientVersionPolicy(clientVersion);
  requireThat(Number.isInteger(requestBudget) && requestBudget > 0 && requestBudget <= CHAT_REQUESTS, 'INVALID_REQUEST_BUDGET');
  requireThat(['reject', 'preserve', 'backend-default'].includes(tokenLimitPolicy), 'INVALID_TOKEN_LIMIT_POLICY');
  let attempts = 0, connectionAttempts = 0, bytes = 0, active, closed = false, lastCategory = 'NONE';
  let httpStatus = null, contentTypeState = 'not-received', httpComplete = null;
  const sockets = new Set();
  function diagnostics() {
    return { synthetic, ...compatibility, tokenLimitPolicy, requestBudget, requestAttempts: attempts, connectionAttempts, responseBytes: bytes, httpStatus, contentTypeState, httpComplete,
      activeRequests: active ? 1 : 0, activeSockets: sockets.size, closed,
      category: lastCategory, retries: 0, credentialWrites: 0, redirectsFollowed: 0 };
  }
  async function close() {
    closed = true; credential = undefined;
    active?.controller.abort();
    await Promise.all([...sockets].map(socket => destroySocket(socket, sockets)));
    if (active) await active.finished;
    return diagnostics();
  }
  async function send(raw, sink, signal) {
    requireThat(!closed, 'TRANSPORT_CLOSED');
    requireThat(!active, 'TRANSPORT_BUSY');
    requireThat(attempts < requestBudget, 'REQUEST_BUDGET');
    requireThat(typeof raw === 'string' && Buffer.byteLength(raw) <= LIMITS.requestBytes, 'INVALID_UPSTREAM_REQUEST');
    requireThat(signal instanceof AbortSignal && !signal.aborted, 'CANCELLED');
    let body;
    try { body = JSON.parse(raw); } catch { throw new TransportError('INVALID_UPSTREAM_REQUEST'); }
    requireThat(body?.model === profile.model && body?.reasoning?.effort === profile.effort
      && body?.stream === true && body?.store === false, 'INVALID_UPSTREAM_REQUEST');
    // backend-default is explicit: generation uses backend limits; the adapter still checks final usage.
    requireThat(!Object.hasOwn(body, 'max_tokens') && (tokenLimitPolicy === 'reject'
      ? !Object.hasOwn(body, 'max_output_tokens')
      : Number.isSafeInteger(body.max_output_tokens) && body.max_output_tokens > 0 && body.max_output_tokens <= 64000),
    'TOKEN_LIMIT_UNSUPPORTED');
    if (tokenLimitPolicy === 'backend-default') {
      delete body.max_output_tokens;
      raw = JSON.stringify(body);
    }
    requireThat(typeof sink?.begin === 'function' && typeof sink?.push === 'function', 'INVALID_SINK');
    if (!synthetic) {
      try {
        // Reuse the existing expiry/account sanity check before every request, without refreshing.
        selectCredential(JSON.stringify({ auth_mode: 'chatgpt', tokens: {
          access_token: credential.accessToken, account_id: credential.account } }));
      } catch { throw new TransportError('CREDENTIAL_UNAVAILABLE_OR_EXPIRED'); }
    }
    const headers = buildHeaders(credential, clientVersion, raw);
    const controller = new AbortController();
    const abort = () => controller.abort();
    signal.addEventListener('abort', abort, { once: true });
    let timedOut = false, req, response, resolveFinished;
    const finished = new Promise(resolve => { resolveFinished = resolve; });
    active = { controller, finished };
    const timer = setTimeout(() => { timedOut = true; controller.abort(); }, timeoutMs);
    bytes = 0; httpStatus = null; contentTypeState = 'not-received'; httpComplete = null;
    try {
      response = await new Promise((resolve, reject) => {
        attempts++;
        req = nativeRequest(destination, { method: 'POST', agent: false, signal: controller.signal,
          rejectUnauthorized: true, maxHeaderSize: 8192, headers }, resolve);
        req.once('error', reject);
        req.once('upgrade', (_response, socket) => {
          socket.destroy(); reject(new TransportError('UPGRADE_REJECTED'));
        });
        req.once('socket', socket => {
          connectionAttempts++;
          sockets.add(socket);
          socket.once('close', () => sockets.delete(socket));
        });
        req.end(raw);
      });
      const names = response.rawHeaders.filter((_, index) => index % 2 === 0).map(name => name.toLowerCase());
      httpStatus = response.statusCode ?? null;
      const typeCount = names.filter(name => name === 'content-type').length;
      const mediaType = String(response.headers['content-type'] ?? '').trim();
      contentTypeState = typeCount > 1 ? 'multiple' : typeCount === 0 ? 'missing' : !mediaType ? 'empty'
        : mediaType.split(';', 1)[0].trim().toLowerCase() === 'text/event-stream' ? 'event-stream' : 'other';
      requireThat(names.filter(name => name === 'content-type').length <= 1, 'DUPLICATE_UPSTREAM_HEADER');
      requireThat(!response.headers['content-encoding'] || response.headers['content-encoding'] === 'identity',
        'UNSUPPORTED_UPSTREAM_ENCODING');
      if (tokenLimitPolicy === 'preserve' && response.statusCode === 400) {
        let rawError = '';
        for await (const chunk of response) {
          bytes += chunk.length; requireThat(bytes <= 8192, 'HTTP_ERROR'); rawError += chunk.toString('utf8');
        }
        let unsupported = false;
        try {
          const error = JSON.parse(rawError), detail = error.error?.message ?? error.detail;
          unsupported = (error.error?.param === 'max_output_tokens' && error.error?.code === 'unsupported_parameter')
            || (typeof detail === 'string' && /^Unsupported parameter: ['"]?max_output_tokens['"]?\.?$/i.test(detail));
        } catch { /* Unknown error bodies keep the ordinary HTTP failure classification. */ }
        rawError = '';
        throw new TransportError(unsupported ? 'UPSTREAM_TOKEN_LIMIT_REJECTED' : 'HTTP_ERROR');
      }
      // The loopback factory represents this protocol identity only; diagnostics marks it synthetic.
      // In the production factory ENDPOINT is also the actual, fixed HTTPS destination.
      sink.begin({ endpoint: ENDPOINT, httpStatus: response.statusCode ?? 0,
        contentTypePresent: names.includes('content-type'), contentType: String(response.headers['content-type'] ?? '') });
      for await (const chunk of response) {
        bytes += chunk.length;
        requireThat(bytes <= LIMITS.responseBytes, 'RESPONSE_TOO_LARGE');
        sink.push(chunk);
      }
      requireThat(response.complete, 'UPSTREAM_TRUNCATED');
      lastCategory = 'SUCCESS';
    } catch (error) {
      lastCategory = timedOut ? 'TIMEOUT' : controller.signal.aborted ? 'CANCELLED'
        : error instanceof TransportError || PROTOCOL_ERROR_CODES.includes(error?.code) ? error.code : 'UPSTREAM_IO_ERROR';
      closed = true; credential = undefined;
      throw new TransportError(lastCategory);
    } finally {
      clearTimeout(timer); signal.removeEventListener('abort', abort);
      httpComplete = response ? response.complete === true : null;
      response?.destroy(); req?.destroy();
      await Promise.all([...sockets].map(socket => destroySocket(socket, sockets)));
      active = undefined; resolveFinished();
    }
  }
  return Object.freeze({ send, close, diagnostics });
}
