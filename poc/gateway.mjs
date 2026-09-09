import { createServer } from 'node:http';
import { randomBytes, timingSafeEqual } from 'node:crypto';
import { OfflineSession, LIMITS, CHAT_REQUESTS, PROTOCOL_ERROR_CODES } from './adapter.mjs';

export const GATEWAY_LIMITS = Object.freeze({ lifetimeMs: 180000, requestMs: 5000,
  upstreamMs: 45000, toolResultMs: 45000, deliveryMs: 5000, requests: 8, connections: 16 });
export const CHAT_GATEWAY_LIMITS = Object.freeze({ ...GATEWAY_LIMITS,
  lifetimeMs: 1800000, requests: CHAT_REQUESTS, connections: 128 });
const knownErrors = new Set([...PROTOCOL_ERROR_CODES, 'UPSTREAM_IO_ERROR', 'UPSTREAM_TRUNCATED', 'UPGRADE_REJECTED',
  'DUPLICATE_UPSTREAM_HEADER', 'UNSUPPORTED_UPSTREAM_ENCODING', 'CREDENTIAL_UNAVAILABLE_OR_EXPIRED',
  'TRANSPORT_CLOSED', 'TRANSPORT_BUSY', 'DELIVERY_TIMEOUT', 'REQUEST_TIMEOUT', 'TOKEN_LIMIT_UNSUPPORTED']);
class GatewayError extends Error { constructor(code) { super(code); this.code = code; } }
function requireThat(ok, code) { if (!ok) throw new GatewayError(code); }
const safeCode = error => knownErrors.has(error?.code) ? error.code : 'PROTOCOL_REJECTED';

// The caller owns the socket and closes it on failure. Only validated SSE enters this writer.
export function deliverSse(response, text, signal, timeoutMs = GATEWAY_LIMITS.deliveryMs) {
  requireThat(Number.isInteger(timeoutMs) && timeoutMs > 0 && timeoutMs <= GATEWAY_LIMITS.deliveryMs, 'INVALID_LIMIT');
  const bytes = Buffer.from(text);
  return new Promise((resolveDelivery, rejectDelivery) => {
    let offset = 0, pauses = 0, settled = false;
    const finish = () => settle();
    const failure = () => settle('DISCONNECTED');
    const abort = () => settle('CANCELLED');
    const timer = setTimeout(() => settle('DELIVERY_TIMEOUT'), timeoutMs);
    function settle(code) {
      if (settled) return; settled = true;
      clearTimeout(timer); signal.removeEventListener('abort', abort);
      response.removeListener('finish', finish); response.removeListener('close', failure);
      response.removeListener('error', failure); response.removeListener('drain', pump);
      if (code) rejectDelivery(new GatewayError(code));
      else resolveDelivery({ backpressurePauses: pauses });
    }
    function pump() {
      if (settled) return;
      try {
        while (offset < bytes.length) {
          const end = Math.min(offset + 16384, bytes.length);
          const accepted = response.write(bytes.subarray(offset, end)); offset = end;
          if (!accepted) { pauses++; response.once('drain', pump); return; }
        }
        response.end();
      } catch { settle('DISCONNECTED'); }
    }
    response.once('finish', finish); response.once('close', failure); response.once('error', failure);
    signal.addEventListener('abort', abort, { once: true });
    if (signal.aborted) abort(); else pump();
  });
}

// One finite session. The transport is owned by this gateway and closed with it.
// clientHeaders() is an in-memory owner API; never print, persist or pass it in shell arguments.
export const READ_BRIDGED_BETAS = Object.freeze(['claude-code-20250219', 'interleaved-thinking-2025-05-14',
  'context-management-2025-06-27', 'effort-2025-11-24', 'redact-thinking-2026-02-12',
  'prompt-caching-scope-2026-01-05', 'mid-conversation-system-2026-04-07', 'thinking-token-count-2026-05-13']);
export async function startGateway({ transport, headerPolicy = 'strict', limits = {}, profile = 'astra-xhigh', readMarker, chat = false }) {
  requireThat(typeof chat === 'boolean' && (!chat || readMarker === undefined), 'INVALID_POLICY');
  requireThat(typeof transport?.send === 'function' && typeof transport?.close === 'function'
    && typeof transport?.diagnostics === 'function', 'TRANSPORT_REQUIRED');
  requireThat(Object.keys(limits).every(key => Object.hasOwn(GATEWAY_LIMITS, key)), 'INVALID_LIMIT');
  const defaults = chat ? CHAT_GATEWAY_LIMITS : GATEWAY_LIMITS;
  const cap = { ...defaults, ...limits };
  for (const [name, maximum] of Object.entries(defaults)) {
    requireThat(Number.isInteger(cap[name]) && cap[name] > 0 && cap[name] <= maximum, 'INVALID_LIMIT');
  }
  const session = new OfflineSession({ headerPolicy, timeoutMs: cap.upstreamMs, profile,
    ...(chat ? { inputPolicy: 'claude-code-chat' } : {}),
    ...(readMarker === undefined ? {} : { inputPolicy: 'claude-code-read-once', readMarker }) });
  const secret = Buffer.from(`Bearer ${randomBytes(32).toString('base64url')}`);
  const sockets = new Set(), jobs = new Set(), timers = new Set();
  const counts = { receivedRequests: 0, connections: 0, rejected: 0, messages: 0, hello: 0,
    countTokens: 0, malformedHttp: 0, responses: 0, compatibilityApplied: 0, reconstructedToolCalls: 0,
    clientDisconnects: 0, transportErrors: 0, internalErrors: 0,
    backpressurePauses: 0 };
  let activeDeliveries = 0, lastRejection = 'NONE', unknownBetaCount = 0;
  let thinkingTokenCountRequested = false;
  let port, closing = false, busy = false, current, boundSession, toolTimer, lifetimeTimer, reason = 'NONE', resolveDone;
  const done = new Promise(resolve => { resolveDone = resolve; });
  const server = createServer({ maxHeaderSize: 8192, headersTimeout: cap.requestMs,
    requestTimeout: cap.requestMs, connectionsCheckingInterval: Math.min(100, cap.requestMs) }, (req, res) => {
    const job = handle(req, res).catch(() => {
      counts.internalErrors++; res.destroy(); void close('GATEWAY_FAILED');
    });
    jobs.add(job); void job.finally(() => jobs.delete(job));
  });
  server.maxConnections = 4; server.maxRequestsPerSocket = 1;
  // Request and delivery deadlines below are absolute; an active upstream has its own deadline.
  server.setTimeout(cap.requestMs, socket => { if (socket !== current?.socket) socket.destroy(); });

  function diagnostics() {
    return { closing, reason, lastRejection, unknownBetaCount, thinkingTokenCountRequested, counts: { ...counts }, activeSockets: sockets.size, activeJobs: jobs.size,
      activeTimers: timers.size, activeDeliveries, busy, session: session.diagnostics, transport: transport.diagnostics(),
      toolExecutions: 0, persistedBodies: 0, localSessionSecretCleared: closing };
  }
  function close(why = 'CLOSED_BY_CALLER') {
    if (closing) return done;
    closing = true; reason = why; secret.fill(0); boundSession = undefined;
    clearTimeout(lifetimeTimer); clearTimeout(toolTimer);
    for (const timer of timers) clearTimeout(timer);
    timers.clear(); session.cancel(); current?.controller.abort();
    const socketClosures = [...sockets].map(socket => new Promise(resolve => socket.once('close', resolve)));
    const serverClosed = new Promise(resolve => server.close(resolve));
    for (const socket of sockets) socket.destroy();
    void (async () => {
      await transport.close();
      await Promise.all([...socketClosures, serverClosed, ...jobs]);
      resolveDone(diagnostics());
    })().catch(() => { reason = 'CLEANUP_FAILED'; resolveDone(diagnostics()); });
    return done;
  }
  function reply(res, status, category) {
    if (res.destroyed || res.writableEnded) return;
    res.writeHead(status, { 'Content-Type': 'application/json', 'Cache-Control': 'no-store',
      Connection: 'close', 'X-Content-Type-Options': 'nosniff' });
    res.end(JSON.stringify({ type: 'error', error: { type: status === 401 ? 'authentication_error'
      : status >= 500 ? 'api_error' : 'invalid_request_error', message: category } }));
  }
  function reject(res, status, category) { lastRejection = category; counts.rejected++; reply(res, status, category); }
  function authorized(value) {
    if (typeof value !== 'string' || Buffer.byteLength(value) !== secret.length) return false;
    return timingSafeEqual(Buffer.from(value), secret);
  }
  async function handle(req, res) {
    counts.receivedRequests++;
    req.on('error', () => { counts.transportErrors++; }); res.on('error', () => { counts.transportErrors++; });
    let admitted = false, completed = false, upstreamStarted = false, abortCause = 'CANCELLED';
    let requestTimer, upstreamTimer, controller;
    const cancel = () => {
      if (admitted && !completed) {
        if (!controller.signal.aborted) { counts.clientDisconnects++; abortCause = 'DISCONNECTED'; session.disconnect(); }
        controller.abort(); void close(abortCause);
      }
    };
    res.once('close', cancel);
    try {
      if (closing) { reject(res, 503, 'GATEWAY_CLOSED'); return; }
      if (counts.receivedRequests > cap.requests) {
        reject(res, 429, 'REQUEST_BUDGET'); res.once('finish', () => { void close('REQUEST_BUDGET'); }); return;
      }
      const names = req.rawHeaders.filter((_, i) => i % 2 === 0).map(name => name.toLowerCase());
      if (new Set(names).size !== names.length) { reject(res, 400, 'DUPLICATE_HEADER'); return; }
      if (req.socket.remoteAddress !== '127.0.0.1' || req.headers.host !== `127.0.0.1:${port}`
        || req.headers.origin !== undefined || req.headers['sec-fetch-site'] !== undefined
        || req.headers.forwarded !== undefined || names.some(name => name.startsWith('x-forwarded-'))) {
        reject(res, 403, 'LOCAL_BOUNDARY_REJECTED'); return;
      }
      if (req.headers.cookie !== undefined || req.headers['x-api-key'] !== undefined || req.headers['proxy-authorization'] !== undefined) {
        reject(res, 401, 'UNEXPECTED_CREDENTIAL_SOURCE'); return;
      }
      if (!req.url.startsWith('/') || req.url.startsWith('//')) { reject(res, 400, 'INVALID_TARGET'); return; }
      const path = req.url.split('?', 1)[0];
      if (req.method === 'HEAD' && path === '/api/hello') {
        if (req.headers.authorization !== undefined && !authorized(req.headers.authorization)) {
          reject(res, 401, 'LOCAL_SESSION_REQUIRED'); return;
        }
        counts.hello++; res.writeHead(204, { Connection: 'close', 'Cache-Control': 'no-store' }); res.end(); return;
      }
      if (!authorized(req.headers.authorization)) { reject(res, 401, 'LOCAL_SESSION_REQUIRED'); return; }
      if (req.method !== 'POST' || !['/v1/messages', '/v1/messages/count_tokens'].includes(path)) {
        reject(res, 404, 'UNSUPPORTED_ROUTE'); return;
      }
      if (req.headers['x-claude-code-agent-id'] !== undefined || req.headers['x-claude-code-parent-agent-id'] !== undefined) {
        reject(res, 400, 'SUBAGENT_REJECTED'); return;
      }
      if (path === '/v1/messages/count_tokens') {
        counts.countTokens++; reject(res, 404, 'COUNT_TOKENS_UNAVAILABLE'); return;
      }
      if (busy) { reject(res, 409, 'REQUEST_IN_PROGRESS'); return; }
      const id = req.headers['x-claude-code-session-id'] ?? '';
      if (!/^[A-Za-z0-9_-]{0,128}$/.test(id) || (boundSession !== undefined && boundSession !== id)) {
        reject(res, 409, 'SESSION_MISMATCH'); return;
      }
      const betas = req.headers['anthropic-beta'] === undefined ? [] : req.headers['anthropic-beta'].split(',').map(value => value.trim());
      // Optional thinking progress hint only; never invent estimated_tokens or change authoritative usage.
      thinkingTokenCountRequested = betas.includes('thinking-token-count-2026-05-13');
      unknownBetaCount = betas.filter(value => !READ_BRIDGED_BETAS.includes(value)).length;
      if (req.headers['anthropic-version'] !== '2023-06-01'
        || (readMarker === undefined && !chat ? req.headers['anthropic-beta'] !== undefined : unknownBetaCount > 0 || new Set(betas).size !== betas.length)) {
        reject(res, 400, 'UNSUPPORTED_CLIENT_VERSION_OR_BETA'); return;
      }
      if (String(req.headers['content-type'] ?? '').split(';', 1)[0].trim().toLowerCase() !== 'application/json'
        || (req.headers['content-encoding'] !== undefined && req.headers['content-encoding'] !== 'identity')) {
        reject(res, 415, 'UNSUPPORTED_BODY_ENCODING'); return;
      }
      if (Number(req.headers['content-length']) > LIMITS.requestBytes) { reject(res, 413, 'INPUT_TOO_LARGE'); return; }
      controller = new AbortController(); admitted = busy = true;
      const expireRequest = () => { abortCause = 'REQUEST_TIMEOUT'; controller.abort(); req.destroy(); };
      current = { controller, socket: req.socket, expireRequest };
      requestTimer = setTimeout(expireRequest, cap.requestMs);
      timers.add(requestTimer);
      let raw = '', bytes = 0;
      const decoder = new TextDecoder('utf-8', { fatal: true });
      try {
        for await (const chunk of req) {
          bytes += chunk.length; requireThat(bytes <= LIMITS.requestBytes, 'INPUT_TOO_LARGE');
          raw += decoder.decode(chunk, { stream: true });
        }
        raw += decoder.decode();
      } catch (error) {
        throw new GatewayError(controller.signal.aborted ? 'REQUEST_TIMEOUT'
          : error instanceof GatewayError ? error.code : 'INVALID_UTF8');
      }
      clearTimeout(requestTimer); timers.delete(requestTimer);
      requireThat(!closing && !controller.signal.aborted, 'CANCELLED');
      const prepared = session.prepare(raw); raw = '';
      boundSession = id; clearTimeout(toolTimer);
      counts.messages++;
      upstreamStarted = true;
      upstreamTimer = setTimeout(() => { abortCause = 'TIMEOUT'; controller.abort(); }, cap.upstreamMs);
      timers.add(upstreamTimer);
      await transport.send(prepared, session, controller.signal);
      clearTimeout(upstreamTimer); timers.delete(upstreamTimer);
      requireThat(!closing && !controller.signal.aborted, 'CANCELLED');
      const output = session.finish();
      if (output.diagnostics.compatibilityApplied) counts.compatibilityApplied++;
      if (output.diagnostics.reconstructed && output.message.stop_reason === 'tool_use') counts.reconstructedToolCalls++;
      res.once('finish', () => { completed = true; });
      res.writeHead(200, { 'Content-Type': 'text/event-stream; charset=utf-8',
        'Cache-Control': 'no-store', Connection: 'close', 'X-Content-Type-Options': 'nosniff' });
      // ponytail: <=256 KiB upstream is validated before publishing any executable tool block.
      activeDeliveries++;
      try { counts.backpressurePauses += (await deliverSse(res, output.sse, controller.signal, cap.deliveryMs)).backpressurePauses; }
      finally { activeDeliveries--; }
      completed = true; counts.responses++;
      if (session.diagnostics.state === 'WAIT_TOOL_RESULT') {
        toolTimer = setTimeout(() => { void close('TOOL_RESULT_TIMEOUT'); }, Math.min(cap.toolResultMs, cap.upstreamMs));
      } else if (!chat) void close('COMPLETE');
    } catch (error) {
      completed = true;
      const code = controller?.signal.aborted && abortCause !== 'CANCELLED' ? abortCause
        : session.diagnostics.category !== 'NONE' ? safeCode({ code: session.diagnostics.category }) : safeCode(error);
      if (!res.headersSent && !res.destroyed && req.socket?.destroyed === false) {
        reply(res, upstreamStarted ? 502 : 400, code);
        res.once('finish', () => { void close(code); });
      } else void close(code);
      session.cancel(); controller?.abort();
    } finally {
      for (const timer of [requestTimer, upstreamTimer]) { clearTimeout(timer); timers.delete(timer); }
      res.removeListener('close', cancel);
      if (admitted) { busy = false; current = undefined; }
    }
  }
  server.on('connection', socket => {
    counts.connections++; sockets.add(socket);
    socket.on('error', () => { counts.transportErrors++; }); socket.once('close', () => sockets.delete(socket));
    if (closing || counts.connections > cap.connections) { socket.destroy(); void close('CONNECTION_BUDGET'); }
  });
  server.on('clientError', (error, socket) => {
    counts.malformedHttp++;
    if (error.code === 'ERR_HTTP_REQUEST_TIMEOUT' && socket === current?.socket) current.expireRequest();
    socket.destroy();
  });
  function rejectExpectation(_req, res) {
    counts.receivedRequests++; reject(res, 417, 'EXPECT_REJECTED');
    res.once('finish', () => { void close('EXPECT_REJECTED'); });
  }
  server.on('checkContinue', rejectExpectation);
  server.on('checkExpectation', rejectExpectation);
  for (const event of ['connect', 'upgrade']) server.on(event, (_req, socket) => {
    counts.receivedRequests++; counts.rejected++; socket.destroy(); void close('UPGRADE_REJECTED');
  });
  server.on('dropRequest', () => { counts.receivedRequests++; counts.rejected++; void close('PIPELINE_REJECTED'); });
  server.on('drop', () => { void close('CONCURRENCY_LIMIT'); });
  await new Promise((resolve, rejectListen) => {
    server.once('error', () => rejectListen(new GatewayError('LISTEN_FAILED')));
    server.listen(0, '127.0.0.1', resolve);
  });
  port = server.address().port;
  server.on('error', () => { void close('SERVER_ERROR'); });
  lifetimeTimer = setTimeout(() => { void close('SESSION_TIMEOUT'); }, cap.lifetimeMs);
  return Object.freeze({ port, done, close, diagnostics, clientHeaders() {
    requireThat(!closing, 'GATEWAY_CLOSED');
    return { Authorization: secret.toString(), 'anthropic-version': '2023-06-01', 'Content-Type': 'application/json' };
  } });
}
