const object = value => value && typeof value === 'object' && !Array.isArray(value);
const fields = (value, names) => object(value) && Object.keys(value).every(key => names.includes(key));
const home = value => typeof value === 'string' && value.length <= 4096 && /^[A-Za-z]:[\\/]/.test(value)
  && !/[\x00-\x1f]/.test(value) ? value.replaceAll('/', '\\').replace(/\\+$/, '').toLowerCase() : null;

// Pure protocol boundary: no process launch, socket, credential read/write or
// arbitrary RPC method. The caller must bind the existing owner before sending
// takeOutgoing() packets, and must separately prove a real credential renewal.
export function createAuthOwnerProtocol({ expectedCodexHome, refresh = false }) {
  const expectedHome = home(expectedCodexHome);
  if (!expectedHome || typeof refresh !== 'boolean') throw new Error('AUTH_OWNER_ARGUMENTS');
  const buffer = Buffer.alloc(16384), decoder = new TextDecoder('utf-8', { fatal: true });
  let used = 0, bytes = 0, records = 0, failure = null, ended = false, phase = 'initialize', awaiting = null;
  let chatgptAccountObserved = false, authModeObserved = false, refreshRequested = false, refreshReplyReceived = false;
  let outgoing = [{ method: 'initialize', id: 0, params: { clientInfo: {
    name: 'clauduct_auth_owner', title: 'Clauduct auth owner', version: '1.0.0'
  } } }];
  function reject(code) { failure ??= code; outgoing = []; buffer.fill(0); used = 0; }
  function message(value) {
    if (!object(value) || value.jsonrpc !== undefined && value.jsonrpc !== '2.0') { reject('AUTH_OWNER_MESSAGE_INVALID'); return; }
    if (Object.hasOwn(value, 'method')) {
      // Never acknowledge an approval, tool call, token-supply request or a
      // notification from another thread. No server text becomes a command.
      if (!fields(value, ['jsonrpc', 'method', 'params']) || value.method !== 'account/updated'
        || !fields(value.params, ['authMode', 'planType']) || value.params.authMode !== 'chatgpt') {
        reject('AUTH_OWNER_UNEXPECTED_METHOD'); return;
      }
      authModeObserved = true; return;
    }
    if (!fields(value, ['jsonrpc', 'id', 'result', 'error']) || Object.hasOwn(value, 'error')
      || !Object.hasOwn(value, 'result') || !Number.isInteger(value.id) || awaiting === null || value.id !== awaiting
      || value.id !== ({ initialize: 0, account: 1, refresh: 2 })[phase]) {
      reject(Object.hasOwn(value, 'error') ? 'AUTH_OWNER_RPC_ERROR' : 'AUTH_OWNER_RESPONSE_MISMATCH'); return;
    }
    awaiting = null;
    if (phase === 'initialize') {
      if (!fields(value.result, ['codexHome', 'platformFamily', 'platformOs', 'userAgent'])
        || home(value.result.codexHome) !== expectedHome || value.result.platformFamily !== 'windows'
        || value.result.platformOs !== 'windows' || typeof value.result.userAgent !== 'string' || value.result.userAgent.length > 4096) {
        reject('AUTH_OWNER_IDENTITY_MISMATCH'); return;
      }
      phase = 'account';
      outgoing.push({ method: 'initialized', params: {} }, { method: 'account/read', id: 1, params: { refreshToken: false } });
      return;
    }
    if (!fields(value.result, ['account', 'requiresOpenaiAuth']) || value.result.requiresOpenaiAuth !== true
      || !fields(value.result.account, ['type', 'email', 'planType']) || value.result.account.type !== 'chatgpt') {
      reject('AUTH_OWNER_NO_CHATGPT_ACCOUNT'); return;
    }
    // Email, plan and userAgent are intentionally discarded, never returned,
    // logged or copied to a request. This reply alone proves no token renewal.
    chatgptAccountObserved = true;
    if (phase === 'account' && refresh) {
      phase = 'refresh'; refreshRequested = true;
      outgoing.push({ method: 'account/read', id: 2, params: { refreshToken: true } });
    } else {
      if (phase === 'refresh') refreshReplyReceived = true;
      phase = 'complete';
    }
  }
  function push(chunk) {
    if (!Buffer.isBuffer(chunk)) throw new Error('AUTH_OWNER_ARGUMENTS');
    bytes = Math.min(Number.MAX_SAFE_INTEGER, bytes + chunk.length);
    if (ended) reject('AUTH_OWNER_AFTER_END');
    if (bytes > 65536) reject('AUTH_OWNER_OUTPUT_LIMIT');
    if (failure) return;
    let offset = 0;
    while (offset < chunk.length && !failure) {
      const newline = chunk.indexOf(10, offset), end = newline < 0 ? chunk.length : newline;
      const count = end - offset;
      if (used + count > buffer.length) { reject('AUTH_OWNER_LINE_LIMIT'); break; }
      chunk.copy(buffer, used, offset, end); used += count;
      if (newline >= 0) {
        if (++records > 64) { reject('AUTH_OWNER_RECORD_LIMIT'); break; }
        let value;
        try { value = JSON.parse(decoder.decode(buffer.subarray(0, used))); }
        catch { reject('AUTH_OWNER_INVALID_JSON_OR_UTF8'); break; }
        buffer.fill(0, 0, used); used = 0; message(value);
      }
      offset = newline < 0 ? chunk.length : newline + 1;
    }
  }
  function takeOutgoing() {
    if (failure || ended) return [];
    for (const value of outgoing) if (Object.hasOwn(value, 'id')) awaiting = value.id;
    const packets = outgoing.map(value => JSON.stringify(value) + '\n'); outgoing = []; return packets;
  }
  function end() {
    if (ended) reject('AUTH_OWNER_DUPLICATE_END');
    ended = true;
    if (used) reject('AUTH_OWNER_TRUNCATED_LINE');
    else if (phase !== 'complete') reject('AUTH_OWNER_INCOMPLETE');
    buffer.fill(0); used = 0; outgoing = [];
  }
  function snapshot() {
    return { exchangeComplete: !failure && ended && phase === 'complete', phase, failure, ended,
      bytes, records, pendingBytes: used, bufferCapacityBytes: buffer.length, outgoingCount: outgoing.length,
      chatgptAccountObserved, authModeObserved, refreshRequested, refreshReplyReceived,
      normalRefreshVerified: false, credentialValuesReturned: false };
  }
  return Object.freeze({ push, takeOutgoing, end, snapshot });
}
