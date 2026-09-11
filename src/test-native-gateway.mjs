import assert from 'node:assert/strict';
import { createServer, request } from 'node:http';
import { createNativeLoopbackTransport } from './native-transport.mjs';
import { startNativeGateway } from './native-gateway.mjs';
import { NativeError, FAILURE_DIAGNOSTIC_CATEGORIES } from './native-protocol.mjs';
import { Writable } from 'node:stream';
import { writeFrames } from './native-delivery.mjs';
import { readRequestStatus, requestStatusSnapshot } from './request-status.mjs';
import { createAgentSelection } from './agent-selection.mjs';

const doc = { model: 'astra', stream: true, max_tokens: 10000, messages: [{ role: 'user', content: 'SYNTHETIC_PROMPT' }],
  tools: [{ name: 'Read', input_schema: { type: 'object', properties: {} } }] };
function frames(body, malformed = false) {
  const message = { id: 'msg_0', type: 'message', role: 'assistant', status: 'completed',
    content: [{ type: 'output_text', text: 'SYNTHETIC_TEXT', annotations: [] }] };
  const tool = { id: 'fc_1', type: 'function_call', call_id: 'call_1', name: 'Read', arguments: '{}', status: 'completed' };
  return [
    { type: 'response.created', response: { id: 'resp_1', status: 'in_progress' } },
    { type: 'response.output_item.added', output_index: 0, item: { ...message, content: [], status: 'in_progress' } },
    { type: 'response.output_text.delta', output_index: 0, item_id: 'msg_0', content_index: 0, delta: 'SYNTHETIC_TEXT' },
    { type: 'response.output_text.done', output_index: 0, item_id: 'msg_0', content_index: 0, text: 'SYNTHETIC_TEXT' },
    { type: 'response.output_item.done', output_index: 0, item: message },
    { type: 'response.output_item.added', output_index: 1, item: { ...tool, arguments: '', status: 'in_progress' } },
    { type: 'response.function_call_arguments.delta', output_index: 1, item_id: 'fc_1', delta: '{}' },
    { type: 'response.function_call_arguments.done', output_index: 1, item_id: 'fc_1', arguments: '{}' },
    { type: 'response.output_item.done', output_index: 1, item: tool },
    { type: 'response.completed', response: { id: 'resp_1', status: 'completed', model: body.model,
      reasoning: body.reasoning, output: [message, malformed ? { ...tool, arguments: '{"different":true}' } : tool],
      usage: { input_tokens: 30, output_tokens: 5, total_tokens: 35, input_tokens_details: { cached_tokens: 0 } } } }
  ];
}
const wire = events => events.map(event => `event: ${event.type}\ndata: ${JSON.stringify(event)}\n\n`).join('');
function call(gateway, { path = '/v1/messages', body = doc, onChunk = () => {}, signal = AbortSignal.timeout(5000), idleMs, headers = {},
  sendBody = (req, raw) => req.end(raw) } = {}) {
  const raw = JSON.stringify(body);
  return new Promise((resolve, reject) => {
    const req = request({ host: '127.0.0.1', port: gateway.port, method: 'POST', path, agent: false, signal,
      headers: { ...gateway.clientHeaders(), 'anthropic-version': '2023-06-01', 'content-type': 'application/json', 'content-length': Buffer.byteLength(raw), ...headers } }, res => {
      let text = '';
      res.on('data', chunk => { text += chunk; onChunk(text); }); res.on('error', reject);
      res.on('end', () => resolve({ status: res.statusCode, text }));
    });
    if (idleMs) req.setTimeout(idleMs, () => req.destroy(new Error('SYNTHETIC_CLIENT_IDLE')));
    req.on('error', reject); sendBody(req, raw);
  });
}
const ample = { freeBytes: () => 16 * 1024 ** 3 };
const emptyStages = { request: 0, selection: 0, prepare: 0, review: 0, upstream: 0, 'output-validation': 0, delivery: 0 };
let passed = 0;
const watchdog = setTimeout(() => { console.error('GATEWAY_TEST_TIMEOUT'); process.exit(1); }, 20000);
try {
  for (const phase of ['admission', 'body']) for (const stop of [true, false]) {
    const selection = createAgentSelection({ timeoutMs: 10,
      readMetadata: async () => ({ agentType: 'general-purpose', toolUseId: 'early' }) });
    selection.remember({ content: [{ type: 'tool_use', id: 'early', name: 'Agent',
      input: { subagent_type: 'general-purpose' } }] }, 'session');
    let free = phase === 'admission' ? 0 : 16 * 1024 ** 3, sends = 0, finishBody;
    const gateway = await startNativeGateway({ agentSelection: selection,
      admissionOptions: { freeBytes: () => free, pollMs: 5 }, transport: {
        send: async body => { sends++; return frames(body); }, close: async () => {}, diagnostics: () => ({}) } });
    const register = stopping => call(gateway, { path: '/clauduct/agents', body: {
      id: 'early', role: 'general-purpose', stop: stopping, sessionId: 'session' } });
    const headers = { 'x-claude-code-session-id': 'session', 'x-claude-code-agent-id': 'early' };
    const until = async predicate => {
      const deadline = Date.now() + 1000;
      while (!predicate()) {
        assert.ok(Date.now() < deadline, `${phase}: expected state transition`);
        await new Promise(resolve => setTimeout(resolve, 5));
      }
    };
    let pending;
    try {
      assert.equal((await register(false)).status, 200);
      pending = call(gateway, { headers, ...(phase === 'body' && { sendBody: (req, raw) => {
        req.write(raw.slice(0, 1)); finishBody = () => req.end(raw.slice(1));
      } }) }).then(result => result, error => ({ error }));
      await until(() => phase === 'admission' ? gateway.diagnostics().admission.queued === 1 : gateway.diagnostics().activeBodies === 1);
      assert.equal((await register(stop)).status, 200);
      await until(() => gateway.diagnostics().lifetime.failed === 1);
      const result = await pending;
      if (phase === 'body') assert.equal(result.error?.code, 'ECONNRESET');
      else { assert.equal(result.status, 400); assert.match(result.text, /CANCELLED/); }
      assert.equal(sends, 0);
      assert.equal(gateway.diagnostics().activeBodies, 0);
      assert.equal(gateway.diagnostics().admission.queued, 0);
      assert.equal(gateway.diagnostics().admission.active, 0);
      assert.equal(gateway.diagnostics().activeTimers, 0);
      assert.equal(gateway.diagnostics().recentRequests.at(-1).failureCategory, 'CANCELLED');
      assert.notEqual(gateway.diagnostics().recentRequests.at(-1).clientDisconnected, true);
      free = 16 * 1024 ** 3;
      if (stop) assert.equal((await register(false)).status, 200);
      assert.equal((await call(gateway, { headers })).status, 200);
      assert.equal(sends, 1); passed++;
    } finally {
      free = 16 * 1024 ** 3; finishBody?.();
      await pending;
      await gateway.close();
    }
  }
  for (const stop of [true, false]) for (const streaming of [false, true]) {
    const selection = createAgentSelection({ timeoutMs: 10,
      readMetadata: async binding => ({ agentType: 'general-purpose', toolUseId: binding.id }) });
    for (const id of ['target', 'sibling']) selection.remember({ content: [{ type: 'tool_use', id,
      name: 'Agent', input: { subagent_type: 'general-purpose' } }] }, 'session');
    let hold = 0, entered, targetsEntered;
    const started = new Promise(resolve => { entered = resolve; });
    const targetsStarted = new Promise(resolve => { targetsEntered = resolve; });
    const signals = [], releases = [], pending = [];
    const gateway = await startNativeGateway({ agentSelection: selection, admissionOptions: ample, transport: {
      send: async (body, signal, callbacks) => {
        let offset = 0;
        if (hold > 0) {
          hold--; signals.push(signal);
          if (streaming) {
            offset = 5;
            for (const event of frames(body).slice(0, offset)) await callbacks.onEvent(event);
          }
          await new Promise((resolve, reject) => {
            // Also exercise a legacy transport returning events after cancellation.
            const abort = () => streaming ? reject(new NativeError('CANCELLED')) : resolve();
            signal.addEventListener('abort', abort, { once: true });
            releases.push(() => { signal.removeEventListener('abort', abort); resolve(); });
            if (signals.length === 2) targetsEntered();
            if (signals.length === 3) entered();
          });
        }
        return frames(body).slice(offset);
      }, close: async () => {}, diagnostics: () => ({}) } });
    const register = (id, stopping = false) => call(gateway, { path: '/clauduct/agents',
      body: { id, role: 'general-purpose', stop: stopping, sessionId: 'session' } });
    const headers = id => ({ 'x-claude-code-session-id': 'session', 'x-claude-code-agent-id': id });
    try {
      for (const id of ['target', 'sibling']) assert.equal((await register(id)).status, 200);
      // Warm the cached selection before starting two active transport requests.
      assert.equal((await call(gateway, { headers: headers('target') })).status, 200);
      hold = 3;
      for (let i = 0; i < 2; i++) pending.push(call(gateway, { headers: headers('target') })
        .then(result => result, error => ({ error })));
      // Capture target signals first, without assuming cross-socket scheduling order.
      await targetsStarted;
      pending.push(call(gateway, { headers: headers('sibling') }).then(result => result, error => ({ error })));
      await started;
      assert.equal((await call(gateway, { path: '/clauduct/agents', body: {
        id: 'target', role: 'Plan', stop, sessionId: 'session' } })).status, 400);
      assert.ok(signals.every(signal => !signal.aborted));
      assert.equal((await register('target', stop)).status, 200);
      assert.ok(signals.slice(0, 2).every(signal => signal.aborted), 'stop/re-registration must abort active child transports');
      assert.equal(signals[2].aborted, false);
      for (const result of await Promise.all(pending.slice(0, 2))) {
        if (streaming) assert.equal(result.error?.code, 'ECONNRESET');
        else { assert.equal(result.status, 502); assert.match(result.text, /CANCELLED/); }
      }
      const cancelled = gateway.diagnostics().recentRequests.filter(row => row.failureCategory === 'CANCELLED');
      assert.equal(cancelled.length, 2);
      assert.ok(cancelled.every(row => row.success === false && row.failureCategory === 'CANCELLED'));
      releases[2]();
      assert.equal((await pending[2]).status, 200);
      assert.equal((await call(gateway)).status, 200);
      assert.equal(gateway.diagnostics().admission.active, 0);
      assert.equal(gateway.diagnostics().activeDeliveries, 0);
      passed++;
    } finally {
      for (const release of releases) release();
      await Promise.allSettled(pending);
      await gateway.close();
    }
  }
  for (const [code, tail] of [
    ['INVALID_SSE', 'data: SYNTHETIC_PRIVATE\n\n'],
    ['INVALID_UTF8', Buffer.from([0xff])],
    ['SEQUENCE_MISMATCH', wire([{ type: 'response.in_progress', sequence_number: 2 }])],
    ['TRUNCATED_STREAM', 'data: {'],
    ['INCOMPLETE_RESPONSE', ''],
    ['STREAM_ORDER', wire([{ type: 'response.output_item.done', output_index: 0, item: {} }])],
    ['SNAPSHOT_MISMATCH', wire([{ type: 'response.in_progress', response_id: 'SYNTHETIC_PRIVATE' }])],
    ['UNSUPPORTED_METADATA_EVENT', wire([{ type: 'codex.response.metadata', metadata: 'SYNTHETIC_PRIVATE' }])]
  ]) {
    const upstream = createServer((_req, res) => {
      res.writeHead(200, { 'Content-Type': 'text/event-stream' });
      res.write(wire([{ type: 'response.created', response: { id: 'resp_1', status: 'in_progress' } }]));
      res.end(tail);
    });
    await new Promise(resolve => upstream.listen(0, '127.0.0.1', resolve));
    const transport = createNativeLoopbackTransport(upstream.address().port);
    const gateway = await startNativeGateway({ transport, admissionOptions: ample });
    try {
      const result = await call(gateway);
      assert.equal(result.status, 502); assert.ok(result.text.includes(code));
      const status = await readRequestStatus({ ANTHROPIC_BASE_URL: `http://127.0.0.1:${gateway.port}`,
        ANTHROPIC_AUTH_TOKEN: gateway.clientHeaders().Authorization.slice(7) });
      const row = status.recentRequests.at(-1);
      assert.equal(row.failureCategory, code); assert.equal(row.failureStage, 'upstream');
      assert.equal(row.snapshotMismatchPhase, code === 'SNAPSHOT_MISMATCH' ? 'stream' : null);
      if (code === 'SNAPSHOT_MISMATCH') assert.equal(typeof row.snapshotMismatchMs, 'number');
      assert.equal(row.success, false); assert.equal(row.attempts.length, 1);
      assert.equal(row.attempts[0].terminalState, 'open');
      assert.equal(row.attempts[0].completed, false);
      assert.equal(row.firstDownstreamWriteMs, null); assert.deepEqual(row.retryScheduledMs, []);
      assert.equal(transport.diagnostics().activeSockets, 0);
      assert.equal(status.lifetime.failed, 1);
      assert.ok(!JSON.stringify(status).includes('SYNTHETIC_PRIVATE'));
      assert.ok(!result.text.includes('SYNTHETIC_PRIVATE')); passed++;
    } finally { await gateway.close(); upstream.closeAllConnections(); await new Promise(resolve => upstream.close(resolve)); }
  }
  {
    // Exercise every fixed category through both status boundaries, not upstream parsing.
    let code;
    const gateway = await startNativeGateway({ admissionOptions: ample, transport: {
      send: async () => { throw Object.assign(new NativeError(code), { requestFailure: 'REQUEST_FIELDS' }); }, close: async () => {}, diagnostics: () => ({})
    } });
    try {
      assert.equal(new Set(FAILURE_DIAGNOSTIC_CATEGORIES).size, FAILURE_DIAGNOSTIC_CATEGORIES.length);
      // A composed local code keeps only its fixed prefix; an unlisted one stays OTHER.
      const composed = new Map([['AGENT_SELECTION_UNVERIFIED_MODEL', 'AGENT_SELECTION_UNVERIFIED'],
        ['AGENT_SELECTION_UNVERIFIED_SYNTHETIC_PRIVATE', 'AGENT_SELECTION_UNVERIFIED'],
        ['UNSUPPORTED_BETA known=FILES_API unknown=1', 'UNSUPPORTED_BETA'],
        ['UNSUPPORTED_BETA_SYNTHETIC_PRIVATE', 'OTHER'], ['UNLISTED_SYNTHETIC_CODE', 'OTHER']]);
      for (code of [...FAILURE_DIAGNOSTIC_CATEGORIES, ...composed.keys()]) {
        await call(gateway);
        const status = await readRequestStatus({ ANTHROPIC_BASE_URL: `http://127.0.0.1:${gateway.port}`,
          ANTHROPIC_AUTH_TOKEN: gateway.clientHeaders().Authorization.slice(7) });
        assert.equal(status.recentRequests.at(-1).failureCategory, composed.get(code) ?? code);
        assert.equal(status.recentRequests.at(-1).requestFailure, null);
        assert.ok(!JSON.stringify(status).includes('SYNTHETIC_PRIVATE'));
      }
      passed++;
    } finally { await gateway.close(); }
  }
  {
    // An inference request rejected by a header compatibility check is a diagnosed failure,
    // not a silent 400. Other routes stay outside the request counters.
    let sends = 0;
    const gateway = await startNativeGateway({ admissionOptions: ample, transport: {
      send: async body => { sends++; return frames(body); }, close: async () => {}, diagnostics: () => ({}) } });
    const status = () => readRequestStatus({ ANTHROPIC_BASE_URL: `http://127.0.0.1:${gateway.port}`,
      ANTHROPIC_AUTH_TOKEN: gateway.clientHeaders().Authorization.slice(7) });
    try {
      assert.equal((await call(gateway)).status, 200);
      const recorded = [['UNSUPPORTED_VERSION', { 'anthropic-version': '2024-01-01' }],
        ['INVALID_BETA_HEADER', { 'anthropic-beta': 'per-turn-control-2026-07-01,per-turn-control-2026-07-01' }],
        ['UNSUPPORTED_ENCODING', { 'content-type': 'text/plain' }]];
      for (const [category, headers] of recorded) {
        const before = (await status()).lifetime;
        const result = await call(gateway, { headers });
        assert.equal(result.status, 400);
        assert.ok(result.text.includes(category));
        const after = await status();
        assert.equal(after.lifetime.started, before.started + 1);
        assert.equal(after.lifetime.failed, before.failed + 1);
        assert.equal(after.lifetime.rejectedBeforeStart, before.rejectedBeforeStart);
        assert.equal(after.requestOutcome, 'has-failures');
        const row = after.recentRequests.at(-1);
        assert.equal(row.failureCategory, category);
        assert.equal(row.failureStage, 'request');
        assert.deepEqual(row.judgedBetaLabels, []);
        assert.equal(row.success, false); assert.equal(row.model, null); assert.equal(row.attempts.length, 0);
        assert.equal(after.failureHistory.records.at(-1).failureCategory, category);
        assert.equal(after.failureHistory.omitted, 0);
      }
      // A beta this project judged incompatible is carried through and recorded by label.
      const before = (await status()).lifetime;
      assert.equal((await call(gateway, { headers: { 'anthropic-beta': 'files-api-2025-04-14' } })).status, 200);
      const carried = await status();
      assert.equal(carried.lifetime.failed, before.failed);
      assert.deepEqual(carried.judgedBetaLabels, ['FILES_API']);
      assert.deepEqual(carried.recentRequests.at(-1).judgedBetaLabels, ['FILES_API']);
      assert.equal(carried.recentRequests.at(-1).success, true);
      // A route the gateway does not serve never reached request diagnostics.
      const boundary = (await status()).lifetime;
      assert.equal((await call(gateway, { path: '/v1/messages/count_tokens' })).status, 400);
      assert.equal((await call(gateway, { headers: { 'x-claude-code-agent-id': 'not valid' } })).status, 400);
      const after = await status();
      assert.equal(after.lifetime.started, boundary.started);
      assert.equal(after.lifetime.failed, boundary.failed);
      assert.equal(after.lifetime.rejectedBeforeStart, 2);
      assert.equal(after.lifetime.firstRejectedCategory, 'UNSUPPORTED_ROUTE');
      // One label cannot describe a run that refused 97 requests; the tally is what can.
      assert.equal(Object.values(after.lifetime.rejectedCategories).reduce((sum, n) => sum + n, 0), 2);
      assert.equal(after.lifetime.rejectedCategories.UNSUPPORTED_ROUTE, 1);
      // The baseline request and the carried-beta request both reached upstream; the refused
      // headers and the other routes never did.
      assert.equal(sends, 2);
      passed++;
    } finally { await gateway.close(); }
  }
  {
    // A model name the gateway does not map costs only that block's routing evidence:
    // the completed turn is still delivered and the skip is counted and announced once.
    const agentDoc = { ...doc, tools: [{ name: 'Agent', input_schema: { type: 'object', properties: {} } }] };
    const message = { id: 'msg_0', type: 'message', role: 'assistant', status: 'completed',
      content: [{ type: 'output_text', text: 'SYNTHETIC_TEXT', annotations: [] }] };
    let notices = 0;
    for (const [selected, unmapped] of [['opus', 0], ['synthetic-unmapped-model', 1], ['synthetic-unmapped-model', 1]]) {
      const args = JSON.stringify({ subagent_type: 'general-purpose', model: selected });
      const tool = { id: 'fc_1', type: 'function_call', call_id: 'call_1', name: 'Agent', arguments: args, status: 'completed' };
      const gateway = await startNativeGateway({ admissionOptions: ample,
        agentSelection: createAgentSelection({ timeoutMs: 10, readMetadata: async () => ({}) }),
        onUnmappedAgentModel: () => { notices++; },
        transport: { send: async body => [
          { type: 'response.created', response: { id: 'resp_1', status: 'in_progress' } },
          { type: 'response.output_item.added', output_index: 0, item: { ...message, content: [], status: 'in_progress' } },
          { type: 'response.output_text.delta', output_index: 0, item_id: 'msg_0', content_index: 0, delta: 'SYNTHETIC_TEXT' },
          { type: 'response.output_text.done', output_index: 0, item_id: 'msg_0', content_index: 0, text: 'SYNTHETIC_TEXT' },
          { type: 'response.output_item.done', output_index: 0, item: message },
          { type: 'response.output_item.added', output_index: 1, item: { ...tool, arguments: '', status: 'in_progress' } },
          { type: 'response.function_call_arguments.delta', output_index: 1, item_id: 'fc_1', delta: args },
          { type: 'response.function_call_arguments.done', output_index: 1, item_id: 'fc_1', arguments: args },
          { type: 'response.output_item.done', output_index: 1, item: tool },
          { type: 'response.completed', response: { id: 'resp_1', status: 'completed', model: body.model,
            reasoning: body.reasoning, output: [message, tool],
            usage: { input_tokens: 30, output_tokens: 5, total_tokens: 35, input_tokens_details: { cached_tokens: 0 } } } }
        ], close: async () => {}, diagnostics: () => ({}) } });
      try {
        const result = await call(gateway, { body: agentDoc, headers: { 'x-claude-code-session-id': 'session1' } });
        const state = gateway.diagnostics(), row = state.recentRequests.at(-1);
        assert.equal(result.status, 200);
        assert.equal(row.success, true);
        assert.match(result.text, /event: message_stop/);
        assert.match(result.text, /"type":"tool_use"/);
        assert.equal(state.lifetime.unmappedAgentModels, unmapped);
        assert.equal(requestStatusSnapshot(state).lifetime.unmappedAgentModels, unmapped);
        assert.ok(!JSON.stringify(requestStatusSnapshot(state)).includes('synthetic-unmapped-model'));
        passed++;
      } finally { await gateway.close(); }
    }
    // Each gateway is its own session, so the notice fires once in each unmapped one.
    assert.equal(notices, 2);
  }
  {
    // The registration table is bounded. Eviction takes the least recently used entry and
    // never one with a live request, and the HTTP server's own rejections are counted apart.
    let release;
    const held = new Promise(resolve => { release = resolve; });
    const gateway = await startNativeGateway({ admissionOptions: ample, maxAgents: 4, transport: {
      send: async body => { await held; return frames(body); }, close: async () => {}, diagnostics: () => ({}) } });
    const register = id => call(gateway, { path: '/clauduct/agents',
      body: { id, role: 'general-purpose', stop: false, sessionId: 'session' } });
    let busy;
    try {
      assert.equal((await register('busy')).status, 200);
      busy = call(gateway, { headers: { 'x-claude-code-session-id': 'session', 'x-claude-code-agent-id': 'busy' } });
      const deadline = Date.now() + 1000;
      while (gateway.diagnostics().lifetime.started === 0) {
        assert.ok(Date.now() < deadline, 'expected the held request to start');
        await new Promise(resolve => setTimeout(resolve, 5));
      }
      for (const id of ['idle0', 'idle1', 'idle2']) assert.equal((await register(id)).status, 200);
      assert.equal(gateway.diagnostics().registeredAgents, 4);
      assert.equal(gateway.diagnostics().lifetime.agentRegistrationsEvicted, 0);
      assert.equal((await register('idle3')).status, 200);
      const state = gateway.diagnostics();
      assert.equal(state.registeredAgents, 4);
      assert.equal(state.maxAgents, 4);
      assert.equal(state.lifetime.agentRegistrationsEvicted, 1);
      assert.equal(requestStatusSnapshot(state).lifetime.agentRegistrationsEvicted, 1);
      // The busy registration survived, so its held request still resolves normally.
      release();
      assert.equal((await busy).status, 200);
      assert.equal(gateway.diagnostics().recentRequests.at(-1).success, true);
      // Expect: 100-continue is answered by the server itself, before any request handler.
      assert.equal(gateway.diagnostics().lifetime.transportRejections, 0);
      const expected = await call(gateway, { headers: { Expect: '100-continue' } });
      assert.equal(expected.status, 417);
      const after = requestStatusSnapshot(gateway.diagnostics());
      assert.equal(after.lifetime.transportRejections, 1);
      assert.equal(after.lifetime.rejectedBeforeStart, 0);
      passed++;
    } finally { release(); await busy?.catch(() => {}); await gateway.close(); }
  }
  {
    // The delivered block kinds are recorded, so a client that reads only the first block can
    // be diagnosed without guessing. frames() answers with text first, then a tool call.
    const gateway = await startNativeGateway({ admissionOptions: ample, transport: {
      send: async body => frames(body), close: async () => {}, diagnostics: () => ({}) } });
    try {
      assert.equal((await call(gateway)).status, 200);
      const row = requestStatusSnapshot(gateway.diagnostics()).recentRequests.at(-1);
      assert.deepEqual(row.contentBlocks, { text: 1, toolUse: 1, thinking: 0, serverToolUse: 0, searchResult: 0 });
      assert.equal(row.firstContentBlock, 'text');
      assert.equal(row.success, true);
      // A hostile snapshot cannot introduce a kind outside the fixed list.
      const hostile = requestStatusSnapshot({ recentRequests: [{ firstContentBlock: 'SYNTHETIC_PRIVATE',
        contentBlocks: { text: -1, toolUse: 'SYNTHETIC_PRIVATE' } }] }).recentRequests[0];
      assert.equal(hostile.firstContentBlock, null);
      assert.deepEqual(hostile.contentBlocks, { text: 0, toolUse: 0, thinking: 0, serverToolUse: 0, searchResult: 0 });
      passed++;
    } finally { await gateway.close(); }
  }
  {
    // A request that asks the backend to search and gets no search back is a silent no-op:
    // the request succeeds, the counters say the feature did nothing, and it is announced once.
    const searchDoc = { ...doc, tools: [...doc.tools, { type: 'web_search_20250305', name: 'web_search' }] };
    let notices = 0;
    const gateway = await startNativeGateway({ admissionOptions: ample, onWebSearchUnused: () => { notices++; },
      transport: { send: async body => frames(body), close: async () => {}, diagnostics: () => ({}) } });
    try {
      assert.equal((await call(gateway, { body: searchDoc })).status, 200);
      assert.equal((await call(gateway, { body: searchDoc })).status, 200);
      assert.equal((await call(gateway)).status, 200);
      const state = gateway.diagnostics(), status = requestStatusSnapshot(state);
      assert.equal(state.lifetime.webSearchRequests, 2);
      assert.equal(state.lifetime.webSearchCalls, 0);
      assert.equal(status.lifetime.webSearchRequests, 2);
      assert.equal(notices, 1);
      assert.deepEqual(status.recentRequests.map(row => [row.webSearchRequested, row.webSearchCalls]),
        [[true, 0], [true, 0], [false, 0]]);
      assert.ok(status.recentRequests.every(row => row.success));
      passed++;
    } finally { await gateway.close(); }
  }
  {
    // A search request the backend rejects is the one the totals most need to show, so the
    // request is counted where it is shaped rather than where it succeeds.
    const searchDoc = { ...doc, tools: [...doc.tools, { type: 'web_search_20250305', name: 'web_search' }] };
    const gateway = await startNativeGateway({ admissionOptions: ample, transport: {
      send: async () => { throw new NativeError('UPSTREAM_HTTP_ERROR'); },
      close: async () => {}, diagnostics: () => ({}) } });
    try {
      assert.equal((await call(gateway, { body: searchDoc })).status, 502);
      const state = gateway.diagnostics();
      assert.equal(state.lifetime.webSearchRequests, 1);
      assert.equal(state.lifetime.webSearchCalls, 0);
      const row = requestStatusSnapshot(state).recentRequests.at(-1);
      assert.equal(row.webSearchRequested, true);
      assert.equal(row.success, false);
      assert.equal(row.failureCategory, 'UPSTREAM_HTTP_ERROR');
      passed++;
    } finally { await gateway.close(); }
  }
  {
    // The client's search side query is answered by this gateway, not by the model: the model
    // transport is never reached, the reply carries the blocks the client reduces, and the
    // query never appears in a status response.
    const searchDoc = { model: 'astra', stream: true, max_tokens: 10000,
      messages: [{ role: 'user', content: 'Perform a web search for the query: SYNTHETIC_QUERY' }],
      tools: [{ type: 'web_search_20250305', name: 'web_search' }],
      tool_choice: { type: 'tool', name: 'web_search' } };
    let sends = 0, searches = 0, sent;
    const gateway = await startNativeGateway({ admissionOptions: ample, transport: {
      send: async body => { sends++; return frames(body); },
      search: async body => { searches++; sent = body; return { encrypted_output: 'opaque',
        output: 'SYNTHETIC_DIGEST', results: [{ type: 'text_result', url: 'https://example.com/a',
          title: 'A', snippet: 'SYNTHETIC_SNIPPET' }] }; },
      close: async () => {}, diagnostics: () => ({}) } });
    try {
      const result = await call(gateway, { body: searchDoc });
      assert.equal(result.status, 200);
      assert.equal(searches, 1);
      assert.equal(sends, 0);
      // Only the query travels: the client's prompt wrapper does not.
      assert.deepEqual(sent.commands, { search_query: [{ q: 'SYNTHETIC_QUERY' }] });
      assert.equal(JSON.stringify(sent).includes('Perform a web search'), false);
      for (const type of ['server_tool_use', 'web_search_tool_result', 'web_search_result']) {
        assert.ok(result.text.includes(type), type);
      }
      assert.ok(result.text.includes('SYNTHETIC_DIGEST'));
      const state = gateway.diagnostics(), status = requestStatusSnapshot(state);
      assert.equal(state.lifetime.webSearchRequests, 1);
      assert.equal(state.lifetime.webSearchCalls, 1);
      assert.equal(state.lifetime.webSearchLinks, 1);
      const row = status.recentRequests.at(-1);
      assert.equal(row.success, true);
      assert.equal(row.webSearchAnswered, true);
      assert.equal(row.webSearchLinks, 1);
      assert.equal(row.firstContentBlock, 'server_tool_use');
      // Every block the reply carries is counted, not just the model-path kinds.
      assert.deepEqual(row.contentBlocks, { text: 1, toolUse: 0, thinking: 0, serverToolUse: 1, searchResult: 1 });
      assert.equal(JSON.stringify(status).includes('SYNTHETIC_QUERY'), false);
      assert.equal(JSON.stringify(status).includes('example.com'), false);
      // The same tool in an ordinary conversation is still a model request.
      assert.equal((await call(gateway, { body: { ...searchDoc, tool_choice: { type: 'auto' },
        messages: [{ role: 'user', content: 'SYNTHETIC_PROMPT' }] } })).status, 200);
      assert.equal(sends, 1);
      assert.equal(searches, 1);
      passed++;
    } finally { await gateway.close(); }
  }
  {
    // A failing search names its own stage instead of arriving as an empty success.
    const searchDoc = { model: 'astra', stream: true, max_tokens: 10000,
      messages: [{ role: 'user', content: 'Perform a web search for the query: SYNTHETIC_QUERY' }],
      tools: [{ type: 'web_search_20250305', name: 'web_search' }],
      tool_choice: { type: 'tool', name: 'web_search' } };
    for (const [reply, category] of [[{ output: '', results: [] }, 'SEARCH_RESULTS_EMPTY'],
      [{ results: [] }, 'SEARCH_RESPONSE_SHAPE'], [new NativeError('SEARCH_HTTP_ERROR'), 'SEARCH_HTTP_ERROR']]) {
      const gateway = await startNativeGateway({ admissionOptions: ample, transport: {
        send: async body => frames(body),
        search: async () => { if (reply instanceof Error) throw reply; return reply; },
        close: async () => {}, diagnostics: () => ({}) } });
      try {
        const result = await call(gateway, { body: searchDoc });
        assert.equal(result.status, 502, category);
        const row = requestStatusSnapshot(gateway.diagnostics()).recentRequests.at(-1);
        assert.equal(row.success, false);
        assert.equal(row.failureCategory, category);
        assert.ok(FAILURE_DIAGNOSTIC_CATEGORIES.includes(category));
        passed++;
      } finally { await gateway.close(); }
    }
  }
  {
    // An idle registration past the window is released before the cap is ever reached.
    const gateway = await startNativeGateway({ admissionOptions: ample, agentIdleMs: 1, transport: {
      send: async body => frames(body), close: async () => {}, diagnostics: () => ({}) } });
    const register = id => call(gateway, { path: '/clauduct/agents',
      body: { id, role: 'general-purpose', stop: false, sessionId: 'session' } });
    try {
      assert.equal((await register('stale')).status, 200);
      await new Promise(resolve => setTimeout(resolve, 5));
      assert.equal((await register('fresh')).status, 200);
      const state = gateway.diagnostics();
      assert.equal(state.registeredAgents, 1);
      assert.equal(state.lifetime.agentRegistrationsExpired, 1);
      assert.equal(state.lifetime.agentRegistrationsEvicted, 0);
      assert.equal(requestStatusSnapshot(state).lifetime.agentRegistrationsExpired, 1);
      // Re-registering the same id is never treated as stale by its own sweep.
      assert.equal((await register('fresh')).status, 200);
      assert.equal(gateway.diagnostics().registeredAgents, 1);
      passed++;
    } finally { await gateway.close(); }
  }
  {
    const upstream = createServer((_req, res) => res.end(JSON.stringify({ recentRequests: [{
      failureCategory: 'SYNTHETIC_PRIVATE', requestFailure: 'SYNTHETIC_PRIVATE', upstreamFailureEvent: { toString: null, valueOf: null },
      upstreamErrorCode: 'server_error', upstreamErrorType: 'server_error', upstreamIncompleteReason: 'max_output_tokens', attempts: [{ terminalState: 'SYNTHETIC_PRIVATE',
        postCompletionFrame: 'SYNTHETIC_PRIVATE', postCompletionSequence: 'SYNTHETIC_PRIVATE' }]
    }, { failureCategory: 'UPSTREAM_ERROR_EVENT', upstreamErrorCode: 'SYNTHETIC_PRIVATE', upstreamErrorType: 'SYNTHETIC_PRIVATE',
      upstreamIncompleteReason: 'max_output_tokens' },
    { failureCategory: 'UPSTREAM_RESPONSE_INCOMPLETE', upstreamErrorCode: { toString: null },
      upstreamIncompleteReason: 'SYNTHETIC_PRIVATE' }], transport: { clientVersion: 'SYNTHETIC_PRIVATE' } })));
    await new Promise(resolve => upstream.listen(0, '127.0.0.1', resolve));
    try {
      const status = await readRequestStatus({ ANTHROPIC_BASE_URL: `http://127.0.0.1:${upstream.address().port}`,
        ANTHROPIC_AUTH_TOKEN: 'x'.repeat(43) });
      const row = status.recentRequests[0];
      assert.equal(status.clientVersion, null);
      assert.equal(status.clientVersionStatus, 'not-observed');
      assert.equal(row.failureCategory, null);
      assert.equal(row.requestFailure, null);
      assert.equal(row.upstreamFailureEvent, null);
      assert.equal(row.upstreamErrorCode, null);
      assert.equal(row.upstreamErrorType, null);
      assert.equal(row.upstreamIncompleteReason, null);
      for (const extra of status.recentRequests.slice(1)) {
        assert.equal(extra.upstreamErrorCode, null);
        assert.equal(extra.upstreamErrorType, null);
        assert.equal(extra.upstreamIncompleteReason, null);
      }
      for (const key of ['terminalState', 'postCompletionFrame', 'postCompletionSequence']) assert.equal(row.attempts[0][key], null);
      assert.ok(!JSON.stringify(status).includes('SYNTHETIC_PRIVATE')); passed++;
    } finally { upstream.closeAllConnections(); await new Promise(resolve => upstream.close(resolve)); }
  }
  for (const mode of ['silent-baseline', 'ping', 'ping-retry', 'ping-cancel']) {
    let attempts = 0; const timers = new Set();
    const upstream = createServer((req, res) => {
      let raw = ''; req.on('data', chunk => { raw += chunk; }); req.on('end', () => {
        attempts++; const events = frames(JSON.parse(raw));
        res.writeHead(200, { 'Content-Type': 'text/event-stream' }); res.write(wire(events.slice(0, 1)));
        const retry = mode === 'ping-retry' && attempts === 1;
        const timer = setTimeout(() => { timers.delete(timer); if (retry) res.destroy(); else res.end(wire(events.slice(1))); }, retry ? 70 : 150);
        timers.add(timer);
      });
    });
    await new Promise(resolve => upstream.listen(0, '127.0.0.1', resolve));
    const transport = createNativeLoopbackTransport(upstream.address().port, { retryBaseMs: 1 });
    const gateway = await startNativeGateway({ transport, admissionOptions: ample, heartbeatMs: mode === 'silent-baseline' ? 1000 : 10 });
    try {
      const cancel = new AbortController(); let sawPing = false;
      const work = call(gateway, { idleMs: 60, signal: cancel.signal, onChunk: text => {
        if (text.includes('event: ping')) { sawPing = true; if (mode === 'ping-cancel') cancel.abort(); }
      } });
      if (mode === 'silent-baseline') await assert.rejects(work, /SYNTHETIC_CLIENT_IDLE/);
      else if (mode === 'ping-cancel') await assert.rejects(work);
      else {
        const result = await work;
        assert.equal(result.status, 200); assert.ok(sawPing);
        assert.ok(result.text.indexOf('event: ping') < result.text.indexOf('event: message_start'));
        assert.match(result.text, /event: message_stop/);
        assert.equal(attempts, mode === 'ping-retry' ? 2 : 1);
        const row = gateway.diagnostics().recentRequests.at(-1);
        assert.ok(row.pingCount > 0); assert.ok(row.lastUpstreamEventMs >= row.firstEventMs);
        assert.ok(row.firstDownstreamWriteMs >= 100);
        assert.equal(row.retryScheduledMs.length, mode === 'ping-retry' ? 1 : 0);
        const status = await readRequestStatus({ ANTHROPIC_BASE_URL: `http://127.0.0.1:${gateway.port}`,
          ANTHROPIC_AUTH_TOKEN: gateway.clientHeaders().Authorization.slice(7) });
        assert.ok(status.recentRequests.at(-1).pingCount > 0);
        assert.equal(status.recentRequests.at(-1).failureCategory, null);
      }
      passed++;
    } finally {
      await gateway.close(); assert.equal(gateway.diagnostics().activeTimers, 0);
      for (const timer of timers) clearTimeout(timer);
      upstream.closeAllConnections(); await new Promise(resolve => upstream.close(resolve));
    }
  }
  // Retryable truncation is re-sent only while nothing has been delivered downstream.
  // After content reaches the client a second upstream turn could repeat a tool call.
  for (const [sent, expectedAttempts] of [[1, 2], [3, 1]]) {
    let attempts = 0; const timers = new Set();
    const upstream = createServer((req, res) => {
      let raw = ''; req.on('data', chunk => { raw += chunk; }); req.on('end', () => {
        attempts++; const events = frames(JSON.parse(raw));
        res.writeHead(200, { 'Content-Type': 'text/event-stream' });
        if (attempts > 1) { res.end(wire(events)); return; }
        res.write(wire(events.slice(0, sent)));
        const timer = setTimeout(() => { timers.delete(timer); res.destroy(); }, 10);
        timers.add(timer);
      });
    });
    await new Promise(resolve => upstream.listen(0, '127.0.0.1', resolve));
    const transport = createNativeLoopbackTransport(upstream.address().port, { retryBaseMs: 1 });
    const gateway = await startNativeGateway({ transport, admissionOptions: ample });
    try {
      const result = await call(gateway);
      assert.equal(attempts, expectedAttempts);
      assert.equal(transport.diagnostics().retries, expectedAttempts - 1);
      const row = gateway.diagnostics().recentRequests.at(-1);
      assert.equal(row.retryScheduledMs.length, expectedAttempts - 1);
      assert.equal(row.attempts.length, expectedAttempts);
      if (sent === 1) {
        assert.equal(result.status, 200);
        assert.equal(row.success, true); assert.equal(row.failureCategory, undefined);
        assert.equal((result.text.match(/event: message_start/g) ?? []).length, 1);
        assert.equal((result.text.match(/"type":"tool_use"/g) ?? []).length, 1);
      } else {
        // Headers are already sent, so the preserved failure arrives as a trailing error frame.
        assert.equal(result.status, 200);
        assert.equal(row.success, false);
        assert.ok(['TRUNCATED_STREAM', 'UPSTREAM_IO_ERROR'].includes(row.failureCategory));
        assert.ok(result.text.includes(row.failureCategory));
        assert.ok(!result.text.includes('message_stop'));
        assert.ok(!result.text.includes('"type":"tool_use"'));
        assert.equal(transport.diagnostics().activeSockets, 0);
      }
      passed++;
    } finally {
      await gateway.close();
      for (const timer of timers) clearTimeout(timer);
      upstream.closeAllConnections(); await new Promise(resolve => upstream.close(resolve));
    }
  }
  for (const mode of ['drain', 'timeout', 'cancel']) {
    const controller = new AbortController(); let bytes = 0;
    const sink = new Writable({ highWaterMark: 1, write(chunk, _encoding, callback) {
      bytes += chunk.length;
      if (mode === 'drain') setImmediate(callback);
    } });
    try {
      const work = writeFrames(sink, [{ type: 'content_block_delta', text: 'x'.repeat(40000) }], controller.signal, 20);
      if (mode === 'cancel') controller.abort();
      if (mode === 'drain') { await work; assert.ok(bytes > 40000); }
      else await assert.rejects(work, error => error.code === (mode === 'cancel' ? 'CANCELLED' : 'DELIVERY_TIMEOUT'));
      assert.equal(sink.listenerCount('drain'), 0); passed++;
    } finally { sink.destroy(); }
  }
  for (const mode of ['valid', 'malformed', 'disconnect', 'retry', 'post-completion']) {
    let attempts = 0, ended = false, sawEarlyText = false, sawEarlyTool = false;
    const timers = new Set();
    const upstream = createServer((req, res) => {
      let raw = ''; req.on('data', chunk => { raw += chunk; }); req.on('end', () => {
        attempts++;
        const events = frames(JSON.parse(raw), mode === 'malformed');
        if (mode === 'retry' && attempts === 1) {
          res.writeHead(200, { 'content-type': 'text/event-stream' }); res.write(wire(events.slice(0, 1)));
          const timer = setTimeout(() => { timers.delete(timer); res.destroy(); }, 20); timers.add(timer);
          return;
        }
        res.writeHead(200, { 'content-type': 'text/event-stream' }); res.write(wire(events.slice(0, 9)));
        const timer = setTimeout(() => {
          timers.delete(timer); ended = true;
          if (mode === 'disconnect') res.destroy();
          else res.end(wire(events.slice(9)) + (mode === 'post-completion'
            ? wire([{ type: 'codex.response.metadata', payload: 'SYNTHETIC_PRIVATE' }]) : ''));
        }, 100); timers.add(timer);
      });
    });
    await new Promise(resolve => upstream.listen(0, '127.0.0.1', resolve));
    const transport = createNativeLoopbackTransport(upstream.address().port);
    const gateway = await startNativeGateway({ transport, admissionOptions: ample });
    try {
      const result = await call(gateway, { onChunk: text => {
        if (!ended && text.includes('SYNTHETIC_TEXT')) sawEarlyText = true;
        if (!ended && text.includes('"type":"tool_use"')) sawEarlyTool = true;
      } });
      assert.equal(result.status, 200); assert.equal(sawEarlyText, true); assert.equal(sawEarlyTool, false);
      assert.equal(attempts, mode === 'retry' ? 2 : 1);
      const timing = gateway.diagnostics().recentRequests.at(-1);
      assert.equal(timing.model, 'gpt-6-astra');
      if (mode === 'malformed') {
        assert.equal(timing.failureCategory, 'SNAPSHOT_MISMATCH');
        assert.equal(timing.snapshotMismatchPhase, 'final');
        assert.ok(timing.snapshotMismatchMs >= timing.transportFinishedMs);
      }
      assert.equal(timing.retryScheduledMs.length, mode === 'retry' ? 1 : 0);
      assert.equal(timing.attempts.length, attempts);
      assert.equal(timing.attempts.at(-1).status, 200);
      assert.ok(timing.attempts.at(-1).headersMs >= timing.attempts.at(-1).startedMs);
      assert.ok(timing.attempts.at(-1).endedMs >= timing.attempts.at(-1).firstBodyMs);
      timing.attempts[0].status = -1;
      assert.equal(gateway.diagnostics().recentRequests.at(-1).attempts[0].status, 200);
      if (mode === 'valid') {
        const collected = await readRequestStatus({ ANTHROPIC_BASE_URL: `http://127.0.0.1:${gateway.port}`,
          ANTHROPIC_AUTH_TOKEN: gateway.clientHeaders().Authorization.slice(7) });
        assert.equal(collected.recentRequests.at(-1).attempts[0].status, 200);
        // The two capture lists are the only fields the in-session API withholds.
        const local = requestStatusSnapshot(gateway.diagnostics());
        assert.deepEqual([local.unsupportedEventNames, local.unknownBetaNames], [[], []]);
        assert.deepEqual([collected.unsupportedEventNames, collected.unknownBetaNames], [null, null]);
        assert.deepEqual({ ...local, unsupportedEventNames: null, unknownBetaNames: null }, collected);
        assert.ok(!JSON.stringify(collected).includes('SYNTHETIC'));
      }
      if (mode === 'post-completion') {
        assert.match(result.text, /EVENT_AFTER_COMPLETION/);
        const status = await readRequestStatus({ ANTHROPIC_BASE_URL: `http://127.0.0.1:${gateway.port}`,
          ANTHROPIC_AUTH_TOKEN: gateway.clientHeaders().Authorization.slice(7) });
        const row = status.recentRequests.at(-1), attempt = row.attempts[0];
        assert.equal(row.failureCategory, 'EVENT_AFTER_COMPLETION');
        assert.equal(row.failureStage, 'upstream');
        assert.equal(attempt.terminalState, 'completed');
        assert.equal(attempt.postCompletionFrame, 'codex.response.metadata');
        assert.equal(attempt.postCompletionSequence, 'unsequenced');
        assert.equal(attempt.completed, false);
        assert.equal(row.auxiliaryMetadataEvents, 0);
        assert.equal(row.success, false);
        assert.ok(!JSON.stringify(status).includes('SYNTHETIC_PRIVATE'));
      }
      assert.ok(timing.preparedMs >= timing.admittedMs);
      assert.ok(timing.firstDownstreamWriteMs >= timing.firstTextDeltaMs);
      assert.ok(timing.finishedMs >= timing.firstDownstreamWriteMs);
      assert.equal(timing.success, mode === 'valid' || mode === 'retry');
      assert.ok(!JSON.stringify(timing).includes('SYNTHETIC'));
      timing.retryScheduledMs.push(-1);
      assert.ok(!gateway.diagnostics().recentRequests.at(-1).retryScheduledMs.includes(-1));
      if (mode === 'valid' || mode === 'retry') {
        assert.ok(result.text.includes('"type":"tool_use"')); assert.ok(result.text.includes('event: message_stop'));
        const delta = result.text.split('\n').filter(line => line.startsWith('data: '))
          .map(line => JSON.parse(line.slice(6))).find(event => event.type === 'message_delta');
        assert.equal(delta.usage.input_tokens, 30);
        assert.equal(gateway.diagnostics().maxObservedInputTokens.main, 30);
        assert.equal(gateway.diagnostics().contextPolicyRuntimeVerified, false);
        assert.equal(result.text.split('event: message_start').length - 1, 1);
      } else {
        assert.ok(result.text.includes('event: error')); assert.ok(result.text.includes('explicit resume'));
        assert.ok(!result.text.includes('"type":"tool_use"')); assert.ok(!result.text.includes('event: message_stop'));
      }
      passed++;
    } finally {
      const state = await gateway.close(); assert.equal(state.cleanupFailed, false); assert.equal(state.activeJobs, 0);
      for (const timer of timers) clearTimeout(timer);
      upstream.closeAllConnections(); await new Promise(resolve => upstream.close(resolve));
    }
  }
  for (const code of ['UNAUTHENTICATED', 'CREDENTIAL_UNAVAILABLE_OR_EXPIRED', 'CREDENTIAL_ACCOUNT_MISMATCH', 'TRUNCATED_STREAM']) {
    const transport = { send: async () => { throw new NativeError(code); }, close: async () => {},
      diagnostics: () => ({ activeSockets: 0, activeRequests: 0 }) };
    const gateway = await startNativeGateway({ transport, admissionOptions: ample });
    try { const result = await call(gateway); assert.equal(result.status, code === 'TRUNCATED_STREAM' ? 502 : 503); passed++; }
    finally { await gateway.close(); }
  }
  {
    const gateway = await startNativeGateway({ admissionOptions: ample, transport: {
      send: async () => { throw new NativeError('UPSTREAM_IO_ERROR'); },
      close: async () => {}, diagnostics: () => ({ activeSockets: 0, activeRequests: 0 }) } });
    try {
      const ids = [];
      assert.equal(requestStatusSnapshot(gateway.diagnostics()).requestOutcome, 'no-requests');
      for (let i = 0; i < 24; i++) {
        assert.equal((await call(gateway)).status, 502);
        ids.push(gateway.diagnostics().recentRequests.at(-1).request);
      }
      const state = gateway.diagnostics();
      const status = requestStatusSnapshot(state);
      assert.equal(status.requestOutcome, 'has-failures');
      assert.equal(status.failureHistory.omitted, 8);
      const wireStatus = await readRequestStatus({ ANTHROPIC_BASE_URL: `http://127.0.0.1:${gateway.port}`,
        ANTHROPIC_AUTH_TOKEN: gateway.clientHeaders().Authorization.slice(7) });
      assert.deepEqual(wireStatus.failureHistory, status.failureHistory);
      assert.ok(Buffer.byteLength(JSON.stringify(wireStatus)) < 256 * 1024);
      assert.deepEqual(status.failureHistory.records.map(row => row.request), [...ids.slice(0, 8), ...ids.slice(-8)]);
      state.failureRequests[0].attempts.push({ status: 999 });
      state.failureRequests[0].failureCategory = 'SYNTHETIC_PRIVATE';
      assert.equal(gateway.diagnostics().failureRequests[0].attempts.length, 0);
      assert.equal(gateway.diagnostics().failureRequests[0].failureCategory, 'UPSTREAM_IO_ERROR');
      const projected = requestStatusSnapshot(status);
      assert.deepEqual(projected.failureHistory, status.failureHistory);
      assert.deepEqual(projected.lifetime, status.lifetime);
      // Re-projection keeps the version evidence instead of downgrading to not-observed.
      const observed = requestStatusSnapshot({ recentRequests: [], transport: { clientVersion: '0.153.4' } });
      assert.deepEqual([observed.clientVersion, observed.clientVersionStatus], ['0.153.4', 'reference']);
      assert.deepEqual(requestStatusSnapshot(observed), observed);
      assert.equal(requestStatusSnapshot({ recentRequests: [], clientVersion: 'SYNTHETIC_PRIVATE' }).clientVersionStatus, 'not-observed');
      const old = requestStatusSnapshot({ recentRequests: [] });
      assert.equal(old.failureHistory, null); assert.equal(old.requestOutcome, 'not-observed');
      assert.equal(old.clientExecutionPolicy.nonStreamingFallbackDisabled, null);
      assert.equal(requestStatusSnapshot({ recentRequests: [], lifetime: { started: 2, succeeded: 2, failed: 0 } }).requestOutcome, 'all-succeeded');
      const hostile = requestStatusSnapshot({ recentRequests: [], failureRequests: Array.from({ length: 25 }, () => ({
        failureCategory: 'SYNTHETIC_PRIVATE', unsupportedEventTypeFormat: 'SYNTHETIC_PRIVATE',
        payload: 'SYNTHETIC_PRIVATE', attempts: [{ terminalState: 'SYNTHETIC_PRIVATE' }] })) });
      assert.equal(hostile.failureHistory.records.length, 16);
      assert.ok(!JSON.stringify(hostile).includes('SYNTHETIC_PRIVATE'));
      passed++;
    } finally { await gateway.close(); }
  }
  {
    let releaseFirst, entered;
    const ready = new Promise(resolve => { entered = resolve; });
    const pendingFailure = new Promise(resolve => { releaseFirst = resolve; });
    let first = true;
    const gateway = await startNativeGateway({ admissionOptions: ample, transport: {
      send: async body => {
        if (!first) return frames(body);
        first = false; entered(); await pendingFailure;
        throw new NativeError('UNSUPPORTED_EVENT');
      }, close: async () => { releaseFirst(); }, diagnostics: () => ({}) } });
    const pending = call(gateway);
    try {
      await ready;
      const firstId = gateway.diagnostics().recentRequests[0].request;
      assert.equal(requestStatusSnapshot(gateway.diagnostics()).requestOutcome, 'in-progress');
      for (let i = 0; i < 18; i++) assert.equal((await call(gateway)).status, 200);
      assert.ok(!gateway.diagnostics().recentRequests.some(row => row.request === firstId));
      releaseFirst(); assert.equal((await pending).status, 502);
      await gateway.close();
      const status = requestStatusSnapshot(gateway.diagnostics());
      assert.equal(status.failureHistory.records[0].request, firstId);
      assert.equal(status.failureHistory.omitted, 0);
      assert.equal(status.requestOutcome, 'has-failures');
      passed++;
    } finally { releaseFirst(); await pending; await gateway.close(); }
  }
  {
    const oversized = createServer((req, res) => {
      req.resume(); res.end(JSON.stringify({ recentRequests: [], padding: 'x'.repeat(256 * 1024) }));
    });
    await new Promise(resolve => oversized.listen(0, '127.0.0.1', resolve));
    try {
      await assert.rejects(readRequestStatus({ ANTHROPIC_BASE_URL: `http://127.0.0.1:${oversized.address().port}`,
        ANTHROPIC_AUTH_TOKEN: 's'.repeat(43) }), /STATUS_UNAVAILABLE/);
      passed++;
    } finally { oversized.closeAllConnections(); await new Promise(resolve => oversized.close(resolve)); }
  }
  {
    let first = true;
    const gateway = await startNativeGateway({ admissionOptions: ample, transport: {
      send: async body => {
        if (!first) return frames(body);
        first = false;
        return [{ type: 'response.created', response: { id: 'resp_diagnostic', status: 'in_progress' } },
          { type: 'response.SYNTHETIC_PRIVATE', payload: 'SYNTHETIC_PRIVATE_BODY' }];
      },
      close: async () => {}, diagnostics: () => ({}) } });
    try {
      const result = await call(gateway);
      assert.equal(result.status, 502);
      assert.match(result.text, /UNSUPPORTED_EVENT event=unknown-response-event/);
      assert.ok(!result.text.includes('SYNTHETIC_PRIVATE'));
      const status = await readRequestStatus({ ANTHROPIC_BASE_URL: `http://127.0.0.1:${gateway.port}`,
        ANTHROPIC_AUTH_TOKEN: gateway.clientHeaders().Authorization.slice(7) });
      assert.equal(status.recentRequests.at(-1).unsupportedEvent, 'unknown-response-event');
      assert.equal(status.recentRequests.at(-1).failureStage, 'upstream');
      assert.equal(status.recentRequests.at(-1).success, false);
      assert.equal(status.lifetime.unsupportedEvents, 1);
      for (let i = 0; i < 17; i++) assert.equal((await call(gateway)).status, 200);
      const after = await readRequestStatus({ ANTHROPIC_BASE_URL: `http://127.0.0.1:${gateway.port}`,
        ANTHROPIC_AUTH_TOKEN: gateway.clientHeaders().Authorization.slice(7) });
      assert.ok(after.recentRequests.every(row => row.success));
      assert.equal(after.requestOutcome, 'has-failures');
      assert.equal(after.failureHistory.records.length, 1);
      assert.equal(after.failureHistory.records[0].unsupportedEvent, 'unknown-response-event');
      assert.equal(after.failureHistory.omitted, 0);
      assert.deepEqual(after.lifetime, { scope: 'gateway-lifetime', started: 18, succeeded: 17,
        failed: 1, auxiliaryMetadataEvents: 0, unsupportedEvents: 1, unsupportedEventNamesWithheld: 1, injectedStreamErrors: 0,
        rejectedBeforeStart: 0, firstRejectedCategory: null, rejectedCategories: {},
        unmappedAgentModels: 0, transportRejections: 0, agentRegistrationsEvicted: 0,
        agentRegistrationsExpired: 0, webSearchRequests: 0, webSearchCalls: 0, webSearchLinks: 0,
        failuresByStage: { ...emptyStages, upstream: 1 } });
      assert.ok(!JSON.stringify(status).includes('SYNTHETIC_PRIVATE')); passed++;
    } finally { await gateway.close(); }
  }
  for (const streaming of [false, true]) {
    const gateway = await startNativeGateway({ admissionOptions: ample, transport: {
      send: async (body, _signal, { onEvent }) => {
        const events = frames(body);
        events.splice(1, 0, { type: 'codex.response.metadata', metadata: { synthetic: 'SYNTHETIC_PRIVATE_METADATA' } });
        if (streaming) { for (const event of events) await onEvent(event); }
        else return events;
      }, close: async () => {}, diagnostics: () => ({}) } });
    try {
      const result = await call(gateway);
      assert.match(result.text, /event: message_stop/);
      assert.ok(!result.text.includes('SYNTHETIC_PRIVATE_METADATA'));
      const status = await readRequestStatus({ ANTHROPIC_BASE_URL: `http://127.0.0.1:${gateway.port}`,
        ANTHROPIC_AUTH_TOKEN: gateway.clientHeaders().Authorization.slice(7) });
      assert.equal(status.recentRequests.at(-1).auxiliaryMetadataEvents, 1);
      assert.equal(status.recentRequests.at(-1).success, true);
      assert.deepEqual(status.lifetime, { scope: 'gateway-lifetime', started: 1, succeeded: 1,
        failed: 0, auxiliaryMetadataEvents: 1, unsupportedEvents: 0, unsupportedEventNamesWithheld: 0, injectedStreamErrors: 0,
        rejectedBeforeStart: 0, firstRejectedCategory: null, rejectedCategories: {},
        unmappedAgentModels: 0, transportRejections: 0,
        agentRegistrationsEvicted: 0, agentRegistrationsExpired: 0, webSearchLinks: 0, webSearchRequests: 0,
        webSearchCalls: 0, failuresByStage: emptyStages });
      const snapshot = gateway.diagnostics(); snapshot.lifetime.started = -1;
      assert.equal(gateway.diagnostics().lifetime.started, 1);
      snapshot.lifetime.failuresByStage.upstream = -1;
      assert.equal(gateway.diagnostics().lifetime.failuresByStage.upstream, 0);
      assert.ok(!JSON.stringify(status).includes('SYNTHETIC_PRIVATE_METADATA')); passed++;
    } finally { await gateway.close(); }
  }
  for (const mode of ['reject', 'hang']) {
    const transport = { send: async () => [], close: () => mode === 'reject' ? Promise.reject(new Error('SYNTHETIC')) : new Promise(() => {}),
      diagnostics: () => ({ activeSockets: 0, activeRequests: 0 }) };
    const gateway = await startNativeGateway({ transport, cleanupMs: 30, admissionOptions: ample });
    const state = await gateway.close(); assert.equal(state.cleanupFailed, true); assert.equal(state.reason, 'CLEANUP_FAILED');
    assert.equal((await gateway.done).cleanupFailed, true); passed++;
  }
  {
    let free = 0, attempts = 0;
    const transport = { send: async body => { attempts++; return frames(body); }, close: async () => {},
      diagnostics: () => ({ activeSockets: 0, activeRequests: 0 }) };
    const gateway = await startNativeGateway({ transport, admissionOptions: { freeBytes: () => free, headroomBytes: 20, requestReserveBytes: 40, pollMs: 5 } });
    try {
      const pending = call(gateway);
      await new Promise(resolve => setTimeout(resolve, 30));
      assert.equal(attempts, 0); assert.equal(gateway.diagnostics().admission.queued, 1); assert.equal(gateway.diagnostics().activeBodies, 0);
      free = 100; assert.equal((await pending).status, 200); assert.equal(attempts, 1); passed++;
      const timing = gateway.diagnostics().recentRequests.at(-1);
      assert.ok(timing.admittedMs - timing.admissionStartedMs >= 20);
      for (let i = 0; i < 18; i++) assert.equal((await call(gateway)).status, 200);
      assert.equal(gateway.diagnostics().recentRequests.length, 16);
      const status = await readRequestStatus({ ANTHROPIC_BASE_URL: `http://127.0.0.1:${gateway.port}`,
        ANTHROPIC_AUTH_TOKEN: gateway.clientHeaders().Authorization.slice(7) });
      assert.equal(status.recentRequests.length, 16);
      assert.equal(status.recentRequests.at(-1).model, 'gpt-6-astra');
      assert.equal(status.lifetime.started, 19); assert.equal(status.lifetime.succeeded, 19);
      assert.equal(status.lifetime.failed, 0);
      assert.ok(!JSON.stringify(status).includes('Bearer'));
      await assert.rejects(readRequestStatus({ ANTHROPIC_BASE_URL: 'https://example.invalid', ANTHROPIC_AUTH_TOKEN: 'synthetic' }));
      const binding = (role, stop) => call(gateway, { path: '/clauduct/agents', body: { id: 'reused', role, stop } });
      assert.equal((await binding('Explore', false)).status, 200);
      assert.equal((await binding('Explore', true)).status, 200);
      assert.equal((await binding('Plan', false)).status, 200);
      assert.equal((await binding('Explore', true)).status, 400);
      assert.equal(gateway.diagnostics().registeredAgents, 1); passed++;
    } finally { await gateway.close(); }
  }
  const scopes = [], references = [];
  for (let run = 0; run < 2; run++) {
    const gateway = await startNativeGateway({ admissionOptions: ample, transport: {
      send: async body => frames(body), close: async () => {}, diagnostics: () => ({}) } });
    try {
      const headers = { 'x-claude-code-session-id': 'PRIVATE_SESSION' };
      const parent = { ...headers, 'x-claude-code-agent-id': 'PRIVATE_PARENT' };
      const child = { ...headers, 'x-claude-code-agent-id': 'PRIVATE_CHILD', 'x-claude-code-parent-agent-id': 'PRIVATE_PARENT' };
      for (const h of [headers, parent, child, child, { ...child, 'x-claude-code-session-id': 'OTHER_SESSION' }]) {
        assert.equal((await call(gateway, { headers: h })).status, 200);
      }
      assert.equal((await call(gateway, { headers: child, body: { ...doc, model: 'unknown' } })).status, 400);
      const status = await readRequestStatus({ ANTHROPIC_BASE_URL: `http://127.0.0.1:${gateway.port}`,
        ANTHROPIC_AUTH_TOKEN: gateway.clientHeaders().Authorization.slice(7) });
      const [main, p, c, repeat, other, failed] = status.recentRequests;
      assert.match(status.correlationScope, /^[a-f0-9]{32}$/);
      assert.match(main.sessionRef, /^[a-f0-9]{32}$/); assert.equal(main.agentRef, null);
      assert.equal(main.sessionRef, p.sessionRef); assert.equal(c.parentRef, p.agentRef);
      assert.notEqual(c.agentRef, p.agentRef); assert.equal(repeat.agentRef, c.agentRef);
      assert.notEqual(other.sessionRef, c.sessionRef); assert.notEqual(other.agentRef, c.agentRef);
      assert.equal(failed.agentRef, c.agentRef);
      assert.deepEqual(status.lifetime.failuresByStage, { ...emptyStages, prepare: 1 });
      assert.equal(Object.values(status.lifetime.failuresByStage).reduce((a, b) => a + b, 0), status.lifetime.failed);
      assert.ok(!JSON.stringify(gateway.diagnostics()).includes('PRIVATE_'));
      scopes.push(status.correlationScope); references.push(c.agentRef); passed++;
    } finally { await gateway.close(); }
  }
  assert.notEqual(scopes[0], scopes[1]); assert.notEqual(references[0], references[1]);
  console.log(JSON.stringify({ suite: 'native-gateway', passed, realClaude: 0, credentialReads: 0, externalRequests: 0 }));
} finally { clearTimeout(watchdog); }
