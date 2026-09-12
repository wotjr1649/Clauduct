import assert from 'node:assert/strict';
import { createServer } from 'node:http';
import { mkdtempSync, writeFileSync, rmSync } from 'node:fs';
import { dirname, join, resolve, sep } from 'node:path';
import { fileURLToPath } from 'node:url';
import { readSmall } from '../verification/manual-http-probe.mjs';
import { MODEL, FIXTURE_PATH, probeProfile } from './adapter.mjs';
import { createLoopbackCodexTransport } from './codex-transport.mjs';
import { startGateway } from './gateway.mjs';
import { installHttpClose } from '../src/http-close.mjs';
import { entryPolicy, entryOptions, checkUserContext, checkClientVersion, requireConfirmation, credentialFromCache,
  safeEntryCategory, runGatewayRoundtrip, entrySummary, RESULT_MARKER } from './user-session.mjs';

let passed = 0, failed = 0, sessions = 0, received = 0;
const watchdog = setTimeout(() => { process.stderr.write('USER_SESSION_TEST_TIMEOUT\n'); process.exit(1); }, 20000);
async function test(name, action) {
  try { await action(); passed++; }
  catch { failed++; process.stderr.write(JSON.stringify({ failure: name }) + '\n'); }
}
const rejects = (action, code) => assert.throws(action, error => safeEntryCategory(error) === code);
const context = { stdinTTY: true, stdoutTTY: true, env: {}, execArgs: [] };
const version = { status: 0, stdout: 'codex-cli 0.153.4\n' };
function cache(seconds = 3600) {
  const payload = Buffer.from(JSON.stringify({ exp: Math.floor(Date.now() / 1000) + seconds })).toString('base64url');
  return JSON.stringify({ auth_mode: 'chatgpt', tokens: { access_token: `synthetic.${payload}.synthetic`, account_id: 'synthetic' } });
}
await test('strict_default_and_explicit_compatibility', () => {
  assert.equal(entryPolicy(['--live-tool-roundtrip']), 'strict');
  assert.equal(entryPolicy(['--live-tool-roundtrip', '--allow-missing-content-type']), 'codex-missing-content-type');
});
await test('live_default_is_astra_low', () => assert.equal(entryOptions(['--live-tool-roundtrip']).profile, 'astra-low'));
for (const profile of ['astra-low', 'luna-low']) await test(`live_profile_${profile}`, () => {
  const options = entryOptions(['--live-tool-roundtrip', `--profile=${profile}`, '--allow-missing-content-type']);
  assert.deepEqual(options, { profile, headerPolicy: 'codex-missing-content-type' });
  assert.deepEqual(entryOptions(['--live-tool-roundtrip', '--allow-missing-content-type', `--profile=${profile}`]), options);
  const summary = entrySummary('USER_CANCELLED', {}, undefined, profile);
  assert.equal(summary.requestedModel, probeProfile(profile).model); assert.equal(summary.requestedEffort, 'low');
  assert.equal(summary.requestAttempts, 0); assert.equal(summary.passed, false);
  rejects(() => requireConfirmation(''), 'USER_CANCELLED'); requireConfirmation('SEND');
});
for (const flags of [['--profile=astra-xhigh'], ['--profile=other'], ['--profile=astra-low', '--profile=luna-low'],
  ['--profile=luna-low', '--profile=luna-low'], ['--profile=astra-low', '--effort=xhigh']]) {
  await test('live_profile_invalid_or_multiple', () => rejects(() => entryOptions(['--live-tool-roundtrip', ...flags]), 'INVALID_ARGUMENTS'));
}
for (const [name, args] of [['empty', []], ['unknown', ['--live']], ['extra', ['--live-tool-roundtrip', '--token']],
  ['duplicate', ['--live-tool-roundtrip', '--allow-missing-content-type', '--allow-missing-content-type']]]) {
  await test(`arguments_${name}`, () => rejects(() => entryPolicy(args), 'INVALID_ARGUMENTS'));
}
await test('interactive_context', () => checkUserContext(context));
for (const [name, change, code] of [
  ['stdin_pipe', { stdinTTY: false }, 'USER_TERMINAL_REQUIRED'],
  ['stdout_pipe', { stdoutTTY: false }, 'USER_TERMINAL_REQUIRED'],
  ['debug', { env: { NODE_DEBUG: 'http' } }, 'DEBUG_RUNTIME_UNSUPPORTED'],
  ['node_options', { env: { NODE_OPTIONS: '--inspect' } }, 'DEBUG_RUNTIME_UNSUPPORTED'],
  ['exec_args', { execArgs: ['--inspect'] }, 'DEBUG_RUNTIME_UNSUPPORTED'],
  ['proxy', { env: { NODE_USE_ENV_PROXY: '1' } }, 'TRANSPORT_RUNTIME_UNSUPPORTED'],
  ['tls', { env: { NODE_TLS_REJECT_UNAUTHORIZED: '0' } }, 'TRANSPORT_RUNTIME_UNSUPPORTED'],
  ['home', { env: { CODEX_HOME: 'D:\\synthetic-other' } }, 'UNEXPECTED_CODEX_HOME']
]) await test(`context_${name}`, () => rejects(() => checkUserContext({ ...context, ...change }), code));
await test('confirmation_exact', () => requireConfirmation('SEND'));
for (const answer of ['', 'send', ' SEND', 'SEND ']) await test(`confirmation_rejected_${answer.length}`, () => rejects(() => requireConfirmation(answer), 'USER_CANCELLED'));
await test('confirmation_aborted', () => rejects(() => requireConfirmation('SEND', true), 'USER_CANCELLED'));
await test('version_reference_valid', () => assert.equal(checkClientVersion(version), '0.153.4'));
await test('version_update_detected', () => assert.equal(checkClientVersion({ status: 0, stdout: 'codex-cli 0.153.5' }), '0.153.5'));
for (const result of [{ status: 1, stdout: version.stdout }, { ...version, error: new Error('SYNTHETIC_PRIVATE_VALUE') }]) {
  await test('version_query_reject', () => rejects(() => checkClientVersion(result), 'CLI_VERSION_UNAVAILABLE'));
}
await test('version_output_reject', () => rejects(() => checkClientVersion({ ...version, stdout: version.stdout + 'unexpected' }), 'CLI_VERSION_INVALID'));
await test('cache_memory_selection', () => assert.equal(credentialFromCache('cli_auth_credentials_store = "file"', cache()).account, 'synthetic'));
for (const store of ['keyring', 'auto']) await test(`cache_${store}_rejected`, () => rejects(
  () => credentialFromCache(`cli_auth_credentials_store = "${store}"`, cache()), 'CREDENTIAL_STORE_UNSUPPORTED'));
await test('expired_cache', () => rejects(() => credentialFromCache('', cache(0)), 'TOKEN_EXPIRED'));
await test('near_expiry_cache', () => rejects(() => credentialFromCache('', cache(30)), 'TOKEN_EXPIRED'));
await test('api_key_mode_rejected', () => rejects(() => credentialFromCache('', JSON.stringify({ auth_mode: 'apikey' })), 'INVALID_AUTH_CACHE'));
await test('invalid_cache', () => rejects(() => credentialFromCache('', '{'), 'INVALID_AUTH_CACHE'));
await test('reader_exact_bound_and_cleanup', () => {
  const root = dirname(fileURLToPath(import.meta.url)), directory = mkdtempSync(join(root, 'entry-fixture-'));
  try {
    const path = join(directory, 'synthetic-cache.txt');
    writeFileSync(path, 'x'.repeat(65536)); assert.equal(readSmall(path).length, 65536);
    writeFileSync(path, 'x'.repeat(65537)); rejects(() => readSmall(path), 'FILE_TOO_LARGE');
    writeFileSync(path, Buffer.from([0xff])); assert.throws(() => readSmall(path));
    // Reopening and replacing the synthetic file checks that failure paths release the handle.
    writeFileSync(path, cache()); assert.equal(credentialFromCache('', readSmall(path)).account, 'synthetic');
  } finally {
    assert.ok(resolve(directory).startsWith(resolve(root) + sep));
    assert.ok(directory.startsWith(join(root, 'entry-fixture-')));
    rmSync(directory, { recursive: true });
  }
});

function wire(tool, number, text, fault, profile) {
  const selected = probeProfile(profile);
  const value = tool ? JSON.stringify({ file_path: FIXTURE_PATH }) : text;
  const item = tool ? { id: `fc_${number}`, type: 'function_call', call_id: `call_${number}`, name: 'Read', arguments: value, status: 'completed' }
    : { id: `msg_${number}`, type: 'message', role: 'assistant', status: 'completed', content: [{ type: 'output_text', text: value, annotations: [] }] };
  const prefix = tool ? 'response.function_call_arguments' : 'response.output_text';
  const pos = { item_id: item.id, output_index: 0, ...(tool ? {} : { content_index: 0 }) };
  const events = [
    { type: 'response.created', response: { id: `resp_${number}`, status: 'in_progress' } },
    { type: 'response.output_item.added', output_index: 0, item: { ...item, status: 'in_progress', ...(tool ? { arguments: '' } : { content: [] }) } },
    { type: `${prefix}.delta`, ...pos, delta: value },
    { type: `${prefix}.done`, ...pos, ...(tool ? { arguments: value } : { text: value }) },
    { type: 'response.output_item.done', output_index: 0, item },
    { type: 'response.completed', response: { id: `resp_${number}`, status: 'completed', model: selected.model, reasoning: { effort: selected.effort },
      output: [item], usage: { input_tokens: 28, output_tokens: 5, total_tokens: 33 } } }
  ];
  if (fault === 'profile_wrong_model') events.at(-1).response.model = selected.model === MODEL ? 'gpt-5.6-luna' : MODEL;
  if (fault === 'profile_wrong_effort') events.at(-1).response.reasoning.effort = 'xhigh';
  if (fault === 'sequence' || fault === 'missing_sequence') events[0].sequence_number = 7;
  if (fault === 'bad_reasoning_item') events[1].item = { id: 'rs_fixture', type: 'reasoning', status: 'in_progress', summary: [] };
  if (fault === 'refusal') events[2] = { type: 'response.refusal.delta', delta: 'SYNTHETIC_PRIVATE_VALUE' };
  if (fault === 'provider_failed') events[0] = { type: 'response.failed', response: { error: { message: 'SYNTHETIC_PRIVATE_VALUE' } } };
  if (fault === 'unknown_event') events[2].type = 'SYNTHETIC_PRIVATE_VALUE';
  if (fault === 'cached_usage') events.at(-1).response.usage.input_tokens_details = { cached_tokens: 1 };
  if (fault.startsWith('empty_output')) {
    events.at(-1).response.output = [];
    if (tool) {
      events[3].name = 'Read';
      if (fault === 'empty_output_bad_snapshot') events[4].item.call_id = 'other';
      if (fault === 'empty_output_missing_item_done') events.splice(4, 1);
      if (fault === 'empty_output_cached_usage') events.at(-1).response.usage.input_tokens_details = { cached_tokens: 1 };
      // 26 synthetic events exercise fragmented arguments; this is not a saved backend response.
      const delta = events[2];
      events.splice(2, 1, ...Array.from({ length: 21 }, (_, index) => ({ ...delta,
        delta: value.slice(Math.floor(index * value.length / 21), Math.floor((index + 1) * value.length / 21)) })));
    }
    events.forEach((event, index) => { event.sequence_number = index; event.response_id = `resp_${number}`; });
  }
  if (fault.startsWith('reasoning_')) {
    events.at(-1).response.output = [];
    if (!tool || fault === 'reasoning_all' || fault === 'reasoning_opaque_all') {
      const reasoning = { id: `rs_${number}`, type: 'reasoning',
        summary: [{ type: 'summary_text', text: 'SYNTHETIC_PRIVATE_REASONING' }], encrypted_content: 'SYNTHETIC_PRIVATE_ENCRYPTED' };
      const pos = { item_id: reasoning.id, output_index: 0, summary_index: 0 };
      const prefix = [
        { type: 'response.in_progress', response: { id: `resp_${number}`, status: 'in_progress' } },
        { type: 'response.output_item.added', output_index: 0, item: { id: reasoning.id, type: 'reasoning', summary: [] } },
        { type: 'response.reasoning_summary_part.added', ...pos, part: { type: 'summary_text', text: '' } },
        { type: 'response.reasoning_summary_text.delta', ...pos, delta: reasoning.summary[0].text },
        { type: 'response.reasoning_summary_text.done', ...pos, text: reasoning.summary[0].text },
        { type: 'response.reasoning_summary_part.done', ...pos, part: { ...reasoning.summary[0] } },
        { type: 'response.output_item.done', output_index: 0, item: reasoning }
      ];
      if (fault.startsWith('reasoning_snapshot_')) {
        prefix.splice(2, 4); // Adjacent added/done, as located by the user's sanitized result.
        if (fault === 'reasoning_snapshot_invalid_id') reasoning.id = 'SYNTHETIC INVALID ID';
        if (fault === 'reasoning_snapshot_changed_id') reasoning.id = 'SYNTHETIC_OTHER_ITEM';
        if (fault === 'reasoning_snapshot_encrypted') prefix[1].item.encrypted_content = 'SYNTHETIC_INITIAL_ENCRYPTED';
      }
      if (fault.startsWith('reasoning_opaque_')) {
        prefix[1].item.encrypted_content = 'SYNTHETIC_INITIAL_ENCRYPTED';
        if (fault === 'reasoning_opaque_wrong_id') reasoning.id = 'SYNTHETIC_OTHER_ITEM';
        if (fault === 'reasoning_opaque_bad_summary') reasoning.summary[0].text = 'SYNTHETIC_OTHER_SUMMARY';
      }
      for (const event of events) {
        if (event.output_index !== undefined) event.output_index++;
        if (event.item) delete event.item.status;
      }
      if (fault.startsWith('reasoning_snapshot_message_')) {
        prefix[1].item.encrypted_content = 'SYNTHETIC_INITIAL_ENCRYPTED';
        const added = events[1].item;
        added.status = 'in_progress';
        if (fault !== 'reasoning_snapshot_message_other') added.phase = 'final_answer';
        if (['reasoning_snapshot_message_other', 'reasoning_snapshot_message_mixed'].includes(fault)) added.SYNTHETIC_PRIVATE_KEY = 'SYNTHETIC_PRIVATE_VALUE';
        if (fault === 'reasoning_snapshot_message_phase') item.phase = 'final_answer';
        if (fault === 'reasoning_snapshot_message_commentary') added.phase = 'commentary';
        const delta = events[2];
        events.splice(2, 1, ...Array.from({ length: 10 }, (_, index) => ({ ...delta,
          delta: value.slice(Math.floor(index * value.length / 10), Math.floor((index + 1) * value.length / 10)) })));
      }
      if (fault === 'reasoning_all' || fault === 'reasoning_opaque_all') {
        events.at(-1).response.output = [structuredClone(reasoning), item];
        if (fault === 'reasoning_opaque_all') events.at(-1).response.output[0].encrypted_content = 'SYNTHETIC_FINAL_ENCRYPTED';
      }
      if (fault === 'reasoning_opaque_incomplete') events.at(-1).response.status = 'incomplete';
      if (fault === 'reasoning_bad_final') reasoning.summary[0].text = 'CONFLICT';
      if (fault === 'reasoning_only_final') events.splice(1, 4);
      if (fault === 'reasoning_wrong_index_final') events[1].output_index = 0;
      if (fault === 'reasoning_cache_final') events.at(-1).response.usage.input_tokens_details = { cached_tokens: 1 };
      events.splice(1, 0, ...prefix);
    }
    events.forEach((event, index) => { event.sequence_number = index; event.response_id = `resp_${number}`; });
    if (!tool && fault === 'reasoning_snapshot_response_id') events[3].response_id = 'SYNTHETIC_OTHER_RESPONSE';
  }
  return events.map(e => `event: ${e.type}\ndata: ${JSON.stringify(e)}\n\n`).join('');
}
async function session(mode, expected, policy = 'strict', profile = 'astra-xhigh') {
  const sockets = new Set(), requests = [];
  let gateway, transport, invalidRequests = 0;
  const server = createServer((req, res) => {
    req.on('error', () => {}); res.on('error', () => {});
    if (req.headers.authorization !== 'Bearer synthetic' || req.headers['chatgpt-account-id'] !== 'synthetic'
      || req.url !== '/backend-api/codex/responses' || req.method !== 'POST') invalidRequests++;
    let raw = '';
    req.on('data', chunk => { raw += chunk; if (Buffer.byteLength(raw) > 65536) req.destroy(); });
    req.on('end', () => {
      try { requests.push(JSON.parse(raw)); } catch { invalidRequests++; res.destroy(); return; }
      if (Object.hasOwn(requests.at(-1), 'max_output_tokens') || !requests.at(-1).instructions) invalidRequests++;
      if (requests.at(-1).model !== probeProfile(profile).model || requests.at(-1).reasoning?.effort !== probeProfile(profile).effort) invalidRequests++;
      if (mode !== 'missing' && mode !== 'missing_sequence' && !String(mode).startsWith('empty_output') && !String(mode).startsWith('reasoning_')) {
        res.setHeader('Content-Type', mode === 'empty' ? '' : 'text/event-stream');
      }
      if (Number.isInteger(mode)) { res.writeHead(mode); res.end('SYNTHETIC_PRIVATE_VALUE'); return; }
      const tool = requests.length === 1 && mode !== 'no_tool';
      res.end(wire(tool, requests.length, mode === 'wrong_marker' ? 'WRONG' : RESULT_MARKER, mode, profile));
    });
  });
  server.on('connection', socket => { installHttpClose(socket); sockets.add(socket); socket.on('error', () => {}); socket.once('close', () => sockets.delete(socket)); });
  await new Promise(resolveListen => server.listen(0, '127.0.0.1', resolveListen));
  let outcome, category = 'SUCCESS';
  try {
    transport = createLoopbackCodexTransport(server.address().port, { timeoutMs: 1000, profile });
    gateway = await startGateway({ transport, headerPolicy: policy, profile, limits: { lifetimeMs: 3000, upstreamMs: 1000 } });
    try { outcome = await runGatewayRoundtrip(gateway, AbortSignal.timeout(2500)); }
    catch (error) { category = safeEntryCategory(error); }
    const diagnostics = await gateway.close();
    const result = entrySummary(category, diagnostics, outcome, profile);
    assert.equal(result.requestedModel, probeProfile(profile).model); assert.equal(result.requestedEffort, probeProfile(profile).effort);
    assert.equal(result.category, expected); assert.equal(result.passed, expected === 'SUCCESS');
    assert.equal(result.upstreamMode, 'synthetic'); assert.equal(result.transport, 'loopback-test-gateway');
    assert.equal(result.resourcesClosed, true); assert.equal(result.toolExecutions, 0);
    assert.equal(diagnostics.activeDeliveries, 0); assert.equal(diagnostics.counts.internalErrors, 0);
    assert.equal(result.credentialWrites, 0); assert.equal(result.retries, 0);
    assert.equal(JSON.stringify(result).includes('SYNTHETIC_PRIVATE_VALUE'), false);
    assert.equal(JSON.stringify(result).includes('Bearer '), false);
    assert.equal(JSON.stringify(result).includes('SYNTHETIC_PRIVATE_REASONING'), false);
    assert.equal(JSON.stringify(result).includes('SYNTHETIC_PRIVATE_ENCRYPTED'), false);
    assert.equal(result.diagnosticVersion, 5);
    const detail = result.responseDiagnostics;
    assert.equal(detail.httpStatus, Number.isInteger(mode) ? mode : 200);
    if (['sequence', 'missing_sequence', 'bad_reasoning_item', 'refusal', 'provider_failed', 'unknown_event'].includes(mode)) {
      assert.equal(detail.category, expected); assert.equal(detail.httpComplete, true); assert.ok(detail.responseBytes > 0);
      assert.equal(detail.headersAccepted, true);
    }
    if (mode === 'sequence' || mode === 'missing_sequence') {
      assert.equal(detail.phase, 'sse-framing'); assert.equal(detail.eventKind, 'created');
      assert.equal(detail.eventNumber, 1); assert.equal(detail.parsedEvents, 1);
      assert.equal(detail.sequenceNumber, 7); assert.equal(detail.outputIndex, null);
    }
    if (mode === 'missing_sequence') {
      assert.equal(result.compatibilityApplied, 0); assert.equal(detail.headerCompatibilityUsed, true);
      assert.equal(detail.contentTypeState, 'missing');
    }
    if (mode === 'bad_reasoning_item') {
      assert.equal(detail.phase, 'response-validation'); assert.equal(detail.eventKind, 'tool-delta');
      assert.equal(detail.eventNumber, 3); assert.equal(detail.outputIndex, 0);
    }
    if (mode === 'refusal') assert.equal(detail.eventKind, 'refusal');
    if (mode === 'provider_failed') assert.equal(detail.eventKind, 'failed');
    if (mode === 'unknown_event') assert.equal(detail.eventKind, 'other');
    if (Number.isInteger(mode)) { assert.equal(detail.headersAccepted, false); assert.equal(detail.phase, 'headers'); }
    const reasoningMode = String(mode).startsWith('reasoning_');
    assert.equal(result.reconstructedToolCalls,
      (String(mode).startsWith('empty_output') && expected === 'SUCCESS') || (reasoningMode && !['reasoning_all', 'reasoning_opaque_all'].includes(mode)) ? 1 : 0);
    if (reasoningMode) {
      assert.equal(detail.headerCompatibilityUsed, true);
      assert.equal(result.compatibilityApplied, expected === 'SUCCESS' ? 2 : 1);
      assert.equal(diagnostics.counts.responses, expected === 'SUCCESS' ? 2 : 1);
      if (expected === 'SUCCESS') assert.equal(detail.reasoningItemCount, 1);
      if (mode.startsWith('reasoning_snapshot_message_')) {
        const success = expected === 'SUCCESS';
        assert.equal(detail.category, success ? 'NONE' : expected); assert.equal(detail.phase, success ? 'completion' : 'response-validation');
        assert.equal(detail.parsedEvents, 18); assert.equal(detail.eventNumber, success ? 18 : 5);
        assert.equal(detail.sequenceNumber, success ? 17 : 4); assert.equal(detail.outputIndex, success ? null : 1);
        assert.equal(detail.eventKind, success ? 'completed' : 'item-added'); assert.equal(detail.itemKind, success ? 'not-observed' : 'message');
        assert.equal(detail.itemStatus, success ? 'not-observed' : 'in_progress'); assert.equal(detail.httpComplete, true);
        assert.equal(detail.reasoningItemCount, 1); assert.equal(detail.reasoningEncryptedUpdates, 1);
        assert.equal(detail.messagePhase, mode === 'reasoning_snapshot_message_other' ? 'not-provided'
          : mode === 'reasoning_snapshot_message_commentary' ? 'commentary' : 'final_answer');
        assert.equal(detail.messageExtraFields, ['reasoning_snapshot_message_phase', 'reasoning_snapshot_message_commentary'].includes(mode) ? 'phase-only'
          : mode === 'reasoning_snapshot_message_other' ? 'other-only' : 'phase-and-other');
        assert.equal(result.callIdMatches, success); assert.equal(result.exactMarker, success);
        assert.equal(JSON.stringify(result).includes('SYNTHETIC_'), false);
      }
      const snapshotCases = { reasoning_snapshot_response_id: 'RESPONSE_ID',
        reasoning_snapshot_invalid_id: 'REASONING_ID_FORMAT', reasoning_snapshot_changed_id: 'REASONING_ITEM_ID' };
      if (Object.hasOwn(snapshotCases, mode)) {
        assert.equal(detail.snapshotCheck, snapshotCases[mode]);
        assert.equal(detail.category, 'SNAPSHOT_MISMATCH'); assert.equal(detail.phase, 'response-validation');
        assert.equal(detail.eventNumber, 4); assert.equal(detail.sequenceNumber, 3); assert.equal(detail.outputIndex, 0);
        assert.equal(detail.eventKind, 'item-done'); assert.equal(detail.itemKind, 'reasoning');
        assert.equal(detail.itemStatus, 'not-provided'); assert.equal(detail.reasoningItemCount, 0);
        assert.equal(detail.httpComplete, true); assert.equal(result.resourcesClosed, true);
        assert.equal(result.callIdMatches, false); assert.equal(result.exactMarker, false);
        assert.equal(JSON.stringify(result).includes('SYNTHETIC_'), false);
      }
      if (mode === 'reasoning_snapshot_encrypted' || mode === 'reasoning_opaque_all') {
        assert.equal(detail.reasoningEncryptedUpdates, mode === 'reasoning_opaque_all' ? 2 : 1);
        assert.equal(detail.snapshotCheck, 'not-observed');
        assert.equal(result.callIdMatches, true); assert.equal(result.exactMarker, true);
        assert.equal(JSON.stringify(result).includes('SYNTHETIC_'), false);
      }
      if (mode === 'reasoning_all' || mode === 'reasoning_opaque_all') {
        assert.deepEqual(requests[1].input.at(-3), { id: 'rs_1', type: 'reasoning',
          summary: [{ type: 'summary_text', text: 'SYNTHETIC_PRIVATE_REASONING' }],
          encrypted_content: mode === 'reasoning_opaque_all' ? 'SYNTHETIC_FINAL_ENCRYPTED' : 'SYNTHETIC_PRIVATE_ENCRYPTED' });
        assert.equal(requests[1].input.at(-2).type, 'function_call');
        assert.equal(JSON.stringify(requests[1]).includes('SYNTHETIC_INITIAL_ENCRYPTED'), false);
      }
      if (mode === 'reasoning_opaque_wrong_id') assert.equal(detail.snapshotCheck, 'REASONING_ITEM_ID');
      if (mode === 'reasoning_opaque_bad_summary') assert.equal(detail.snapshotCheck, 'REASONING_SUMMARY_STREAM');
      if (mode === 'reasoning_opaque_incomplete') {
        assert.equal(detail.reasoningEncryptedUpdates, 1);
        assert.equal(result.callIdMatches, false); assert.equal(result.exactMarker, false);
      }
    }
    if (String(mode).startsWith('empty_output')) {
      assert.equal(detail.contentTypeState, 'missing');
      assert.equal(detail.headerCompatibilityUsed, true);
      if (expected === 'SUCCESS') {
        assert.equal(result.compatibilityApplied, 2);
        assert.equal(result.callIdMatches, true); assert.equal(result.exactMarker, true);
        assert.equal(diagnostics.counts.responses, 2);
        assert.deepEqual(requests[1].input.at(-2), { id: 'fc_1', type: 'function_call', call_id: 'call_1',
          name: 'Read', arguments: JSON.stringify({ file_path: FIXTURE_PATH }), status: 'completed' });
      } else {
        assert.equal(diagnostics.counts.responses, 0); assert.equal(result.compatibilityApplied, 0);
        assert.equal(result.callIdMatches, false); assert.equal(result.exactMarker, false);
      }
    }
    const secondSent = reasoningMode || ['SUCCESS', 'FINAL_MARKER_MISMATCH'].includes(expected);
    assert.equal(requests.length, secondSent ? 2 : 1); assert.equal(result.requestAttempts, requests.length);
    if (secondSent) {
      assert.equal(requests[1].input.at(-1).call_id, 'call_1');
      assert.deepEqual(JSON.parse(requests[1].input.at(-1).output), { is_error: false, content: RESULT_MARKER });
      assert.equal(requests[1].tool_choice, 'none');
    }
    sessions++; received += requests.length;
  } finally {
    if (gateway) await gateway.close(); else if (transport) await transport.close();
    const closed = new Promise(resolveClose => server.close(resolveClose));
    const pending = [...sockets].map(socket => new Promise(resolveClose => socket.once('close', resolveClose)));
    for (const socket of sockets) socket.destroy();
    await Promise.all([closed, ...pending]); assert.equal(sockets.size, 0); assert.equal(invalidRequests, 0);
  }
}
for (const [mode, expected, policy] of [['normal', 'SUCCESS'], ['missing', 'SUCCESS', 'codex-missing-content-type'],
  ['missing', 'MISSING_CONTENT_TYPE'], ['empty', 'EMPTY_CONTENT_TYPE', 'codex-missing-content-type'],
  ['no_tool', 'NO_TOOL_CALL'], ['wrong_marker', 'FINAL_MARKER_MISMATCH'], [401, 'UNAUTHENTICATED'],
  [421, 'HTTP_ERROR'], [429, 'RATE_LIMITED'], ['sequence', 'SEQUENCE_MISMATCH'],
  ['missing_sequence', 'SEQUENCE_MISMATCH', 'codex-missing-content-type'],
  ['bad_reasoning_item', 'ITEM_INDEX_MISMATCH'], ['refusal', 'REFUSAL'], ['provider_failed', 'UPSTREAM_FAILURE'],
  ['unknown_event', 'UNSUPPORTED_EVENT'], ['cached_usage', 'SUCCESS'],
  ['empty_output', 'SUCCESS', 'codex-missing-content-type'],
  ['empty_output_bad_snapshot', 'SNAPSHOT_MISMATCH', 'codex-missing-content-type'],
  ['empty_output_missing_item_done', 'INCOMPLETE_RESPONSE', 'codex-missing-content-type'],
  ['empty_output_cached_usage', 'SUCCESS', 'codex-missing-content-type'],
  ['reasoning_final', 'SUCCESS', 'codex-missing-content-type'],
  ['reasoning_all', 'SUCCESS', 'codex-missing-content-type'],
  ['reasoning_bad_final', 'SNAPSHOT_MISMATCH', 'codex-missing-content-type'],
  ['reasoning_only_final', 'INCOMPLETE_RESPONSE', 'codex-missing-content-type'],
  ['reasoning_wrong_index_final', 'UNSUPPORTED_OUTPUT', 'codex-missing-content-type'],
  ['reasoning_cache_final', 'SUCCESS', 'codex-missing-content-type'],
  ['reasoning_snapshot_response_id', 'SNAPSHOT_MISMATCH', 'codex-missing-content-type'],
  ['reasoning_snapshot_invalid_id', 'SNAPSHOT_MISMATCH', 'codex-missing-content-type'],
  ['reasoning_snapshot_changed_id', 'SNAPSHOT_MISMATCH', 'codex-missing-content-type'],
  ['reasoning_snapshot_encrypted', 'SUCCESS', 'codex-missing-content-type'],
  ['reasoning_snapshot_message_phase', 'SUCCESS', 'codex-missing-content-type'],
  ['reasoning_snapshot_message_other', 'UNSUPPORTED_FIELDS', 'codex-missing-content-type'],
  ['reasoning_snapshot_message_mixed', 'UNSUPPORTED_FIELDS', 'codex-missing-content-type'],
  ['reasoning_opaque_all', 'SUCCESS', 'codex-missing-content-type'],
  ['reasoning_opaque_wrong_id', 'SNAPSHOT_MISMATCH', 'codex-missing-content-type'],
  ['reasoning_opaque_bad_summary', 'SNAPSHOT_MISMATCH', 'codex-missing-content-type'],
  ['reasoning_opaque_incomplete', 'INCOMPLETE_RESPONSE', 'codex-missing-content-type']
]) await test(`roundtrip_${mode}_${expected}`, () => session(mode, expected, policy));
for (const profile of ['astra-low', 'luna-low']) for (const [mode, expected] of [
  ['reasoning_snapshot_message_phase', 'SUCCESS'], ['reasoning_snapshot_message_commentary', 'UNSUPPORTED_MESSAGE_PHASE'],
  ['profile_wrong_model', 'MODEL_EFFORT_MISMATCH'], ['profile_wrong_effort', 'MODEL_EFFORT_MISMATCH']
]) await test(`roundtrip_${profile}_${mode}`, () => session(mode, expected, 'codex-missing-content-type', profile));
await test('summary_is_allowlisted', () => {
  const output = entrySummary('SYNTHETIC_PRIVATE_VALUE', { reason: 'SYNTHETIC_PRIVATE_VALUE', transport: { requestAttempts: 'SYNTHETIC_PRIVATE_VALUE' } });
  assert.equal(output.category, 'LOCAL_CHECK_FAILED'); assert.equal(output.requestAttempts, 0);
  assert.equal(JSON.stringify(output).includes('SYNTHETIC_PRIVATE_VALUE'), false);
});
await test('success_requires_observed_cleanup', () => assert.equal(entrySummary('SUCCESS', {}).category, 'CLEANUP_FAILED'));
await test('detailed_diagnostics_never_echo_unknown_values', () => {
  const secret = 'SYNTHETIC_PRIVATE_VALUE';
  const output = entrySummary(secret, {
    counts: { reconstructedToolCalls: secret },
    session: { category: secret, phase: secret, eventKind: secret, itemKind: secret, itemStatus: secret,
      sequenceNumber: secret, outputIndex: 9999999999, parsedEvents: secret, eventNumber: 999999, reasoningItemCount: secret,
      snapshotCheck: secret, reasoningEncryptedUpdates: secret, messagePhase: secret, messageExtraFields: secret },
    transport: { httpStatus: secret, contentTypeState: secret, responseBytes: secret, httpComplete: secret }
  });
  assert.equal(JSON.stringify(output).includes(secret), false);
  assert.equal(output.responseDiagnostics.category, 'UNKNOWN');
  assert.equal(output.responseDiagnostics.httpStatus, null); assert.equal(output.responseDiagnostics.eventNumber, 0);
  assert.equal(output.reconstructedToolCalls, 0);
  assert.equal(output.responseDiagnostics.reasoningItemCount, 0);
  assert.equal(output.responseDiagnostics.snapshotCheck, 'not-observed');
  assert.equal(output.responseDiagnostics.reasoningEncryptedUpdates, 0);
  assert.equal(output.responseDiagnostics.messagePhase, 'not-observed');
  assert.equal(output.responseDiagnostics.messageExtraFields, 'not-observed');
});
clearTimeout(watchdog);
process.stdout.write(JSON.stringify({ suite: 'user-session', passed, failed, closedSessions: sessions,
  syntheticUpstreamRequests: received, actualCredentialReads: 0, externalRequests: 0, toolExecutions: 0 }) + '\n');
process.exitCode = failed ? 1 : 0;
