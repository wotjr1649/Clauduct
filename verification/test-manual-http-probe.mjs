// Offline only: imports pure checks; never calls live mode, reads credentials, or opens a socket.
import assert from 'node:assert/strict';
import { buildBody, buildLiteBody, buildHeaders, checkStore, selectCredential, summarizeResponse, model, endpoint,
  liteModel, liteEffort, readClientVersion, LITE_PROMPT } from './manual-http-probe.mjs';
import { liteSearchEnvelope } from '../src/native-protocol.mjs';
import { REFERENCE_CLIENT_VERSION } from '../src/client-version.mjs';

let count = 0;
function check(name, action) { action(); count++; }
const encoded = data => Buffer.from(JSON.stringify(data)).toString('base64url');
const fixture = exp => JSON.stringify({ auth_mode: 'chatgpt', tokens: {
  access_token: `synthetic.${encoded({ exp })}.synthetic`, account_id: 'synthetic-account', refresh_token: 'not-used' } });
const complete = { type: 'response.completed', response: { model, status: 'completed', reasoning: { effort: 'xhigh' },
  output: [{ type: 'message', content: [{ type: 'output_text', text: 'OK' }] }], usage: { input_tokens: 1, output_tokens: 1, total_tokens: 2 } } };
const sse = event => Buffer.from(`event: ${event.type}\r\ndata: ${JSON.stringify(event)}\r\n\r\n`);
const summary = event => summarizeResponse(200, 'text/event-stream; charset=utf-8', sse(event));
check('fixed destination', () => assert.equal(endpoint, 'https://chatgpt.com/backend-api/codex/responses'));
check('fixed request', () => {
  const body = buildBody(); assert.equal(body.model, model); assert.equal(body.reasoning.effort, 'xhigh');
  assert.deepEqual(body.tools, []); assert.equal(body.tool_choice, 'none'); assert.equal(body.store, false);
});
check('file cache', () => checkStore('cli_auth_credentials_store = "file"\n'));
check('keyring refused', () => assert.throws(() => checkStore('cli_auth_credentials_store = "keyring"'), /CREDENTIAL_STORE_UNSUPPORTED/));
check('auto refused', () => assert.throws(() => checkStore("cli_auth_credentials_store = 'auto'"), /CREDENTIAL_STORE_UNSUPPORTED/));
check('malformed store refused', () => assert.throws(() => checkStore('cli_auth_credentials_store = false'), /CONFIG_UNSUPPORTED/));
check('valid synthetic cache', () => assert.equal(selectCredential(fixture(3600), 0).account, 'synthetic-account'));
check('expired cache', () => assert.throws(() => selectCredential(fixture(0), 0), /TOKEN_EXPIRED/));
check('invalid JSON', () => assert.throws(() => selectCredential('{'), /INVALID_AUTH_CACHE/));
check('missing credential', () => assert.throws(() => selectCredential('{}'), /INVALID_AUTH_CACHE/));
check('header injection', () => {
  const doc = JSON.parse(fixture(3600)); doc.tokens.account_id = 'bad\r\ninjected';
  assert.throws(() => selectCredential(JSON.stringify(doc), 0), /INVALID_AUTH_CACHE/);
});
check('successful SSE', () => assert.equal(summary(complete).passed, true));
check('no terminal event', () => assert.equal(summary({ type: 'response.created' }).passed, false));
check('wrong model', () => assert.equal(summary({ ...complete, response: { ...complete.response, model: 'different' } }).passed, false));
check('unexpected tool', () => assert.equal(summary({ ...complete, response: { ...complete.response, output: [{ type: 'function_call' }] } }).unexpectedTool, true));
check('malformed event', () => assert.equal(summarizeResponse(200, 'text/event-stream', Buffer.from('data: {\n\n')).category, 'INVALID_SSE_JSON'));
check('invalid UTF8', () => assert.equal(summarizeResponse(200, 'text/event-stream', Buffer.from([0xff])).category, 'INVALID_UTF8'));
check('redirect refused', () => assert.equal(summarizeResponse(302, 'text/plain', Buffer.alloc(0)).category, 'REDIRECT_REFUSED'));
check('auth error redacted', () => {
  const result = summarizeResponse(401, 'application/json', Buffer.from('SYNTHETIC_PRIVATE_CANARY'));
  assert.equal(result.category, 'AUTH_REJECTED'); assert(!JSON.stringify(result).includes('SYNTHETIC_PRIVATE_CANARY'));
});
check('old client error classified', () => assert.equal(summarizeResponse(400, 'application/json', Buffer.from('requires a newer version of Codex')).category, 'CLIENT_VERSION_REJECTED'));
check('duplicate terminal rejected', () => assert.equal(summarizeResponse(200, 'text/event-stream', Buffer.concat([sse(complete), sse(complete)])).category, 'DUPLICATE_COMPLETION'));
check('wrong reply', () => assert.equal(summary({ ...complete, response: { ...complete.response,
  output: [{ type: 'message', content: [{ type: 'output_text', text: 'different' }] }] } }).passed, false));
check('incomplete response', () => assert.equal(summary({ type: 'response.incomplete' }).category, 'UPSTREAM_FAILED'));
check('rate limit', () => assert.equal(summarizeResponse(429, 'text/plain', Buffer.alloc(0)).category, 'RATE_LIMITED'));
check('non-SSE response', () => assert.equal(summarizeResponse(200, 'application/json', Buffer.from('{}')).passed, false));
check('reported failure now has JSON diagnostics', () => {
  const result = summarizeResponse(200, 'application/json', Buffer.from('{}'));
  assert.deepEqual(result, { httpStatus: 200, passed: false, category: 'UNEXPECTED_RESPONSE',
    responseBytes: 2, mediaType: 'application/json', bodyFormat: 'json', jsonKind: 'other-object',
    headerDiagnostics: { normalizedContentTypeNonEmpty: true, rawHeadersAvailable: false, rawContentTypePresent: null } });
});
check('HTML diagnostic without disclosure', () => {
  const result = summarizeResponse(200, 'text/html; private=SYNTHETIC_PRIVATE_CANARY',
    Buffer.from('<!DOCTYPE html><html>SYNTHETIC_PRIVATE_CANARY</html>'));
  assert.equal(result.bodyFormat, 'html-like'); assert.equal(result.mediaType, 'text/html');
  assert.equal(result.passed, false); assert(!JSON.stringify(result).includes('SYNTHETIC_PRIVATE_CANARY'));
});
check('missing header with SSE body does not bypass validation', () => {
  const result = summarizeResponse(200, '', sse(complete));
  assert.equal(result.mediaType, 'missing'); assert.equal(result.bodyFormat, 'sse-like'); assert.equal(result.passed, false);
});
check('unknown header and text redacted', () => {
  const result = summarizeResponse(200, 'SYNTHETIC_PRIVATE_CANARY', Buffer.from('SYNTHETIC_PRIVATE_CANARY'));
  assert.equal(result.mediaType, 'other'); assert.equal(result.bodyFormat, 'other-text');
  assert(!JSON.stringify(result).includes('SYNTHETIC_PRIVATE_CANARY'));
});
check('JSON error redacted even with HTTP 200', () => {
  const result = summarizeResponse(200, 'application/json', Buffer.from(JSON.stringify({
    error: { message: 'SYNTHETIC_PRIVATE_CANARY' }, SYNTHETIC_PRIVATE_CANARY: 'private' })));
  assert.equal(result.jsonKind, 'error'); assert.equal(result.passed, false);
  assert(!JSON.stringify(result).includes('SYNTHETIC_PRIVATE_CANARY'));
});
check('JSON response classified but not accepted as SSE', () => {
  const result = summarizeResponse(200, 'application/json', Buffer.from(JSON.stringify({ object: 'response', ...complete.response })));
  assert.equal(result.jsonKind, 'response'); assert.equal(result.passed, false);
});
check('JSON event and primitive diagnostics', () => {
  assert.equal(summarizeResponse(200, 'application/json', Buffer.from(JSON.stringify(complete))).jsonKind, 'response-event');
  for (const text of ['null', '[]', 'true', '"SYNTHETIC_PRIVATE_CANARY"']) {
    const result = summarizeResponse(200, 'application/json', Buffer.from(text));
    assert.equal(result.jsonKind, 'non-object'); assert(!JSON.stringify(result).includes('SYNTHETIC_PRIVATE_CANARY'));
  }
});
check('empty response diagnostics', () => {
  const result = summarizeResponse(200, '', Buffer.alloc(0));
  assert.equal(result.responseBytes, 0); assert.equal(result.bodyFormat, 'empty'); assert.equal(result.passed, false);
});
check('missing Content-Type isolates successful payload from overall failure', () => {
  const result = summarizeResponse(200, '', sse(complete));
  assert.equal(result.category, 'MISSING_CONTENT_TYPE'); assert.equal(result.passed, false);
  assert.equal(result.sseDiagnostics.passed, true); assert.equal(result.sseDiagnostics.modelMatches, true);
  assert.equal(result.sseDiagnostics.exactOK, true); assert.equal(result.sseDiagnostics.eventCount, 1);
});
check('missing Content-Type retains every SSE failure check', () => {
  const cases = [
    [Buffer.from('data: {\n\n'), 'INVALID_SSE_JSON'],
    [sse({ type: 'response.created' }), 'NO_COMPLETED_RESPONSE'],
    [sse({ type: 'response.failed', error: { message: 'SYNTHETIC_PRIVATE_CANARY' } }), 'UPSTREAM_FAILED'],
    [sse({ type: 'response.incomplete' }), 'UPSTREAM_FAILED'],
    [Buffer.concat([sse(complete), sse(complete)]), 'DUPLICATE_COMPLETION'],
    [sse({ ...complete, response: { ...complete.response, model: 'SYNTHETIC_PRIVATE_CANARY' } }), 'RESULT_MISMATCH'],
    [sse({ ...complete, response: { ...complete.response, output: [{ type: 'function_call' }] } }), 'RESULT_MISMATCH'],
    [sse({ ...complete, response: { ...complete.response,
      output: [{ type: 'message', content: [{ type: 'output_text', text: 'SYNTHETIC_PRIVATE_CANARY' }] }] } }), 'RESULT_MISMATCH'],
    [Buffer.concat(Array.from({ length: 513 }, () => sse({ type: 'response.created' }))), 'EVENT_LIMIT']
  ];
  for (const [body, category] of cases) {
    const result = summarizeResponse(200, '', body);
    assert.equal(result.passed, false); assert.equal(result.category, 'MISSING_CONTENT_TYPE');
    assert.equal(result.sseDiagnostics.passed, false); assert.equal(result.sseDiagnostics.category, category);
    assert(!JSON.stringify(result).includes('SYNTHETIC_PRIVATE_CANARY'));
  }
});
check('explicit non-SSE header does not enter diagnostic fallback', () => {
  for (const contentType of ['application/json', 'text/html', 'text/plain']) {
    const result = summarizeResponse(200, contentType, sse(complete));
    assert.equal(result.passed, false); assert.equal(result.sseDiagnostics, undefined);
  }
});
check('SSE-looking media type suffix or conflicting list is not SSE', () => {
  for (const contentType of ['text/event-stream-json', 'text/event-stream+json', 'text/event-stream, application/json']) {
    const result = summarizeResponse(200, contentType, sse(complete));
    assert.equal(result.passed, false); assert.equal(result.category, 'UNEXPECTED_RESPONSE');
    assert.equal(result.sseDiagnostics, undefined);
  }
});
check('HTTP errors do not enter diagnostic fallback', () => {
  for (const status of [302, 400, 401, 403, 429, 500]) {
    const result = summarizeResponse(status, '', sse(complete));
    assert.equal(result.passed, false); assert.equal(result.sseDiagnostics, undefined);
  }
});
const withReply = text => ({ ...complete, response: { ...complete.response,
  output: [{ type: 'message', content: [{ type: 'output_text', text }] }] } });
const delta = (text, outputIndex = 0, contentIndex = 0) => ({ type: 'response.output_text.delta',
  output_index: outputIndex, content_index: contentIndex, delta: text });
const streamSummary = events => summarizeResponse(200, 'text/event-stream', Buffer.concat(events.map(sse)));
check('whitespace diagnostic never replaces exact OK check', () => {
  const result = summary(withReply(' \r\nOK\t'));
  assert.equal(result.passed, false); assert.equal(result.exactOK, false);
  assert.equal(result.textDiagnostics.trimmedOK, true); assert.equal(result.textDiagnostics.replyChars, 6);
});
check('empty text differs from missing output text part', () => {
  const empty = summary(withReply('')).textDiagnostics;
  const missing = summary({ ...complete, response: { ...complete.response, output: [] } }).textDiagnostics;
  assert.equal(empty.replyEmpty, true); assert.equal(empty.outputTextPartCount, 1);
  assert.equal(missing.replyEmpty, true); assert.equal(missing.outputTextPartCount, 0);
  assert.equal(empty.trimmedOK, false); assert.equal(empty.replyChars, 0);
});
check('Unicode diagnostic counts code points', () => {
  assert.equal(summary(withReply('한😀')).textDiagnostics.replyChars, 2);
});
check('delta chunks match completed text', () => {
  const result = streamSummary([delta('O'), delta('K'), complete]);
  assert.equal(result.passed, true);
  assert.deepEqual(result.textDiagnostics, { outputTextPartCount: 1, replyEmpty: false, replyChars: 2,
    trimmedOK: true, deltaEventCount: 2, deltaShapeValid: true, streamChars: 2, streamMatchesCompleted: true,
    streamExactOK: true, textDoneEventCount: 0, doneShapeValid: true, streamOrderValid: true,
    streamMatchesTextDone: null, snapshotsMatch: true, indicesCompatible: true, reconstructed: false });
});
check('delta mismatch now prevents accepting a contradictory final text', () => {
  const result = streamSummary([delta('SYNTHETIC_PRIVATE_CANARY'), complete]);
  assert.equal(result.textDiagnostics.streamMatchesCompleted, false); assert.equal(result.passed, false);
  assert(!JSON.stringify(result).includes('SYNTHETIC_PRIVATE_CANARY'));
});
check('absent deltas mean not checked, not mismatch', () => {
  const info = summary(complete).textDiagnostics;
  assert.equal(info.deltaEventCount, 0); assert.equal(info.streamMatchesCompleted, null);
});
check('interleaved delta parts use output and content indices', () => {
  const terminal = { ...complete, response: { ...complete.response, output: [
    { type: 'reasoning' },
    { type: 'message', content: [{ type: 'output_text', text: 'O' }, { type: 'output_text', text: 'K' }] }
  ] } };
  const result = streamSummary([delta('K', 1, 1), delta('O', 1, 0), terminal]);
  assert.equal(result.exactOK, true); assert.equal(result.textDiagnostics.outputTextPartCount, 2);
  assert.equal(result.textDiagnostics.streamMatchesCompleted, true);
});
check('right text at wrong index is not a matching stream', () => {
  assert.equal(streamSummary([delta('OK', 1), complete]).textDiagnostics.streamMatchesCompleted, false);
});
check('reasoning and done events are not counted as text deltas', () => {
  const result = streamSummary([
    { type: 'response.reasoning_summary_text.delta', delta: 'SYNTHETIC_PRIVATE_CANARY' },
    delta('OK'), { type: 'response.output_text.done', output_index: 0, content_index: 0, text: 'OK' }, complete
  ]);
  assert.equal(result.textDiagnostics.deltaEventCount, 1); assert.equal(result.textDiagnostics.streamMatchesCompleted, true);
  assert.equal(result.passed, true);
  assert(!JSON.stringify(result).includes('SYNTHETIC_PRIVATE_CANARY'));
});
check('malformed delta cannot produce a trusted text comparison', () => {
  for (const event of [delta(123), delta('OK', -1), delta('OK', 0, 0.5), delta('OK', Number.MAX_SAFE_INTEGER + 1),
    { type: 'response.output_text.delta', delta: 'SYNTHETIC_PRIVATE_CANARY' }]) {
    const result = streamSummary([event, complete]);
    assert.equal(result.textDiagnostics.deltaShapeValid, false);
    assert.equal(result.textDiagnostics.streamMatchesCompleted, null); assert.equal(result.textDiagnostics.streamChars, null);
    assert(!JSON.stringify(result).includes('SYNTHETIC_PRIVATE_CANARY'));
  }
});
check('malformed completed text gives a fixed failure instead of coercion or exception', () => {
  for (const event of [withReply(123), withReply({ secret: 'SYNTHETIC_PRIVATE_CANARY' }),
    { ...complete, response: { ...complete.response, output: [null] } },
    { ...complete, response: { ...complete.response, output: [{ type: 'message', content: [null] }] } }]) {
    const result = summary(event);
    assert.equal(result.passed, false); assert.equal(result.category, 'INVALID_RESPONSE_SHAPE');
    assert(!JSON.stringify(result).includes('SYNTHETIC_PRIVATE_CANARY'));
  }
});
check('raw header names are case insensitive and values never disclosed', () => {
  const result = summarizeResponse(200, '', sse(complete),
    ['cOnTeNt-TyPe', 'SYNTHETIC_PRIVATE_CANARY', 'Set-Cookie', 'SYNTHETIC_PRIVATE_CANARY']);
  assert.deepEqual(result.headerDiagnostics, { normalizedContentTypeNonEmpty: false,
    rawHeadersAvailable: true, rawContentTypePresent: true });
  assert.equal(result.passed, false); assert(!JSON.stringify(result).includes('SYNTHETIC_PRIVATE_CANARY'));
});
check('raw header values cannot masquerade as names', () => {
  const result = summarizeResponse(200, '', sse(complete), ['X-Test', 'Content-Type']);
  assert.equal(result.headerDiagnostics.rawContentTypePresent, false);
});
check('empty raw header list is confirmed absent, not unavailable', () => {
  const result = summarizeResponse(200, '', sse(complete), []);
  assert.deepEqual(result.headerDiagnostics, { normalizedContentTypeNonEmpty: false,
    rawHeadersAvailable: true, rawContentTypePresent: false });
});
check('malformed raw headers are not reported as confirmed absence', () => {
  for (const headers of [undefined, null, 'SYNTHETIC_PRIVATE_CANARY', ['Content-Type'], [null, 'value']]) {
    const result = summarizeResponse(200, '', sse(complete), headers);
    assert.equal(result.headerDiagnostics.rawHeadersAvailable, false);
    assert.equal(result.headerDiagnostics.rawContentTypePresent, null);
    assert(!JSON.stringify(result).includes('SYNTHETIC_PRIVATE_CANARY'));
  }
});
check('both header representations remain separately observable', () => {
  const present = summarizeResponse(200, 'text/event-stream', sse(complete), ['Content-Type', 'text/event-stream']);
  const absent = summarizeResponse(200, 'text/event-stream', sse(complete), []);
  assert.equal(present.headerDiagnostics.normalizedContentTypeNonEmpty, true);
  assert.equal(present.headerDiagnostics.rawContentTypePresent, true);
  assert.equal(absent.headerDiagnostics.normalizedContentTypeNonEmpty, true);
  assert.equal(absent.headerDiagnostics.rawContentTypePresent, false);
});
check('combined missing-header and whitespace case provides all diagnostics while failing', () => {
  const result = summarizeResponse(200, '', Buffer.concat([sse(delta('OK\n')), sse(withReply('OK\n'))]), []);
  assert.equal(result.passed, false); assert.equal(result.category, 'MISSING_CONTENT_TYPE');
  assert.equal(result.headerDiagnostics.rawContentTypePresent, false);
  assert.equal(result.sseDiagnostics.passed, false); assert.equal(result.sseDiagnostics.exactOK, false);
  assert.equal(result.sseDiagnostics.textDiagnostics.trimmedOK, true);
  assert.equal(result.sseDiagnostics.textDiagnostics.streamMatchesCompleted, true);
});
const textDone = (text, outputIndex = 1, contentIndex = 0) => ({ type: 'response.output_text.done',
  output_index: outputIndex, content_index: contentIndex, text });
const textless = { ...complete, response: { ...complete.response, output: [{ type: 'reasoning' }] } };
const restoredEvents = (text = 'OK') => [delta(text, 1), textDone(text), textless];
const itemDone = text => ({ type: 'response.output_item.done', output_index: 1,
  item: { type: 'message', role: 'assistant', status: 'completed', content: [{ type: 'output_text', text }] } });
check('reconstruct text omitted by reasoning-only terminal snapshot', () => {
  const result = streamSummary(restoredEvents());
  assert.equal(result.passed, true); assert.equal(result.replySource, 'stream'); assert.equal(result.exactOK, true);
  assert.equal(result.textDiagnostics.outputTextPartCount, 0); assert.equal(result.textDiagnostics.replyEmpty, true);
  assert.equal(result.textDiagnostics.streamMatchesCompleted, false); assert.equal(result.textDiagnostics.streamMatchesTextDone, true);
  assert.equal(result.textDiagnostics.reconstructed, true);
});
check('empty terminal output can also be reconstructed', () => {
  const terminal = { ...complete, response: { ...complete.response, output: [] } };
  const result = streamSummary([delta('OK'), textDone('OK', 0), terminal]);
  assert.equal(result.passed, true); assert.equal(result.replySource, 'stream');
});
check('realistic nine-event textless completion is reconstructed without executing anything', () => {
  const events = [{ type: 'response.created' }, { type: 'response.in_progress' },
    { type: 'response.output_item.added', output_index: 1, item: { type: 'message' } },
    { type: 'response.content_part.added', output_index: 1, content_index: 0, part: { type: 'output_text', text: '' } },
    delta('OK', 1), textDone('OK'),
    { type: 'response.content_part.done', output_index: 1, content_index: 0, part: { type: 'output_text', text: 'OK' } },
    itemDone('OK'), textless];
  const result = streamSummary(events);
  assert.equal(result.passed, true); assert.equal(result.eventCount, 9); assert.equal(result.replySource, 'stream');
});
check('successful reconstruction never makes a missing Content-Type pass', () => {
  const result = summarizeResponse(200, '', Buffer.concat(restoredEvents().map(sse)), []);
  assert.equal(result.passed, false); assert.equal(result.category, 'MISSING_CONTENT_TYPE');
  assert.equal(result.sseDiagnostics.passed, true); assert.equal(result.sseDiagnostics.replySource, 'stream');
  assert.equal(result.headerDiagnostics.rawContentTypePresent, false);
});
check('reconstruction requires both delta and matching text completion', () => {
  for (const events of [[delta('OK', 1), textless], [textDone('OK'), textless],
    [delta('OK', 1), textDone('NO'), textless], [delta('OK', 1), textDone('OK', 2), textless]]) {
    const result = streamSummary(events);
    assert.equal(result.passed, false); assert.equal(result.replySource, 'none');
  }
});
check('reconstructed response must still equal OK without trimming', () => {
  for (const text of ['NO', 'OK\n', '', 'SYNTHETIC_PRIVATE_CANARY']) {
    const result = streamSummary(restoredEvents(text));
    assert.equal(result.passed, false); assert.equal(result.exactOK, false);
    assert(!JSON.stringify(result).includes('SYNTHETIC_PRIVATE_CANARY'));
  }
});
check('reconstruction preserves exact model and effort checks', () => {
  for (const response of [{ ...textless.response, model: 'different' },
    { ...textless.response, reasoning: { effort: 'high' } }, { ...textless.response, reasoning: {} }]) {
    assert.equal(streamSummary([delta('OK', 1), textDone('OK'), { ...textless, response }]).passed, false);
  }
});
check('duplicate text completion and delta after done cannot be reconstructed', () => {
  for (const events of [[delta('OK', 1), textDone('OK'), textDone('OK'), textless],
    [textDone('OK'), delta('OK', 1), textless],
    [delta('O', 1), textDone('OK'), delta('K', 1), textless]]) {
    const result = streamSummary(events);
    assert.equal(result.passed, false); assert.equal(result.textDiagnostics.streamOrderValid, false);
  }
});
check('terminal events are unique and no later payload can alter the answer', () => {
  assert.equal(streamSummary([...restoredEvents(), textless]).category, 'DUPLICATE_COMPLETION');
  assert.equal(streamSummary([...restoredEvents(), delta('NO', 1)]).category, 'EVENT_AFTER_COMPLETION');
  const malformedTerminal = { type: 'response.completed' };
  assert.equal(streamSummary([malformedTerminal, textless]).category, 'DUPLICATE_COMPLETION');
});
check('DONE marker cannot stand in for a completion event', () => {
  const early = summarizeResponse(200, 'text/event-stream', Buffer.concat([Buffer.from('data: [DONE]\n\n'), sse(complete)]));
  assert.equal(early.passed, false); assert.equal(early.category, 'PREMATURE_DONE');
  const trailing = summarizeResponse(200, 'text/event-stream', Buffer.concat([...restoredEvents().map(sse), Buffer.from('data: [DONE]\n\n')]));
  assert.equal(trailing.passed, true);
});
check('existing terminal messages are never replaced by reconstructed text', () => {
  for (const terminal of [withReply('NO'), withReply(''),
    { ...complete, response: { ...complete.response, output: [{ type: 'message', content: [] }] } }]) {
    const result = streamSummary([delta('OK'), textDone('OK', 0), terminal]);
    assert.equal(result.passed, false); assert.equal(result.textDiagnostics.reconstructed, false);
  }
});
check('upstream errors, incomplete and cancellation prevent reconstruction', () => {
  for (const type of ['error', 'response.failed', 'response.incomplete', 'response.cancelled', 'response.canceled']) {
    const result = streamSummary([{ type }, ...restoredEvents()]);
    assert.equal(result.passed, false); assert.equal(result.category, 'UPSTREAM_FAILED');
  }
  for (const field of ['error', 'incomplete_details']) {
    const result = streamSummary([delta('OK', 1), textDone('OK'),
      { ...textless, response: { ...textless.response, [field]: { message: 'SYNTHETIC_PRIVATE_CANARY' } } }]);
    assert.equal(result.passed, false); assert.equal(result.category, 'UPSTREAM_FAILED');
    assert(!JSON.stringify(result).includes('SYNTHETIC_PRIVATE_CANARY'));
  }
});
check('tool calls and refusals cannot be hidden by a textless terminal', () => {
  const hazards = [{ type: 'response.function_call_arguments.delta', delta: 'SYNTHETIC_PRIVATE_CANARY' },
    { type: 'response.output_item.added', item: { type: 'function_call' } },
    { type: 'response.refusal.delta', delta: 'SYNTHETIC_PRIVATE_CANARY' },
    { type: 'response.content_part.added', part: { type: 'refusal' } },
    { type: 'response.output_item.added', item: { type: 'message', content: [{ type: 'refusal' }] } }];
  for (const event of hazards) {
    const result = streamSummary([event, ...restoredEvents()]);
    assert.equal(result.passed, false); assert(result.unexpectedTool || result.refused);
    assert(!JSON.stringify(result).includes('SYNTHETIC_PRIVATE_CANARY'));
  }
});
check('content and item snapshots must agree with delta text', () => {
  for (const snapshot of [itemDone('NO'), { type: 'response.content_part.done', output_index: 1, content_index: 0,
    part: { type: 'output_text', text: 'SYNTHETIC_PRIVATE_CANARY' } }]) {
    const result = streamSummary([delta('OK', 1), textDone('OK'), snapshot, textless]);
    assert.equal(result.passed, false); assert.equal(result.textDiagnostics.snapshotsMatch, false);
    assert(!JSON.stringify(result).includes('SYNTHETIC_PRIVATE_CANARY'));
  }
});
check('duplicate items and text following an item completion are rejected', () => {
  for (const events of [[delta('OK', 1), textDone('OK'), itemDone('OK'), itemDone('OK'), textless],
    [itemDone('OK'), delta('OK', 1), textDone('OK'), textless]]) {
    const result = streamSummary(events);
    assert.equal(result.passed, false); assert.equal(result.textDiagnostics.streamOrderValid, false);
  }
});
check('reasoning and text cannot share an output index', () => {
  const result = streamSummary([delta('OK'), textDone('OK', 0), textless]);
  assert.equal(result.passed, false); assert.equal(result.textDiagnostics.indicesCompatible, false);
});
check('interleaved reconstructed parts use numeric output and content order', () => {
  const result = streamSummary([delta('K', 10), textDone('K', 10), delta('O', 2), textDone('O', 2), textless]);
  assert.equal(result.passed, true); assert.equal(result.replySource, 'stream');
});
check('bad text completion shape cannot be used for reconstruction', () => {
  for (const event of [textDone(123), textDone('OK', -1), { type: 'response.output_text.done', text: 'OK' }]) {
    const result = streamSummary([delta('OK', 1), event, textless]);
    assert.equal(result.passed, false); assert.equal(result.textDiagnostics.doneShapeValid, false);
  }
});
check('JSON primitives and missing event type fail closed', () => {
  for (const raw of ['null', '[]', '42', '{}']) {
    const result = summarizeResponse(200, 'text/event-stream', Buffer.from(`data: ${raw}\n\n`));
    assert.equal(result.passed, false); assert.equal(result.category, 'INVALID_SSE_EVENT');
  }
});
check('malformed or non-assistant terminal messages are not a fallback opportunity', () => {
  for (const item of [{ type: 'message' }, { type: 'message', content: [], role: 'user' },
    { type: 'message', content: [], status: 'in_progress' },
    { type: 'message', content: [{ type: 'text', text: 'OK' }] }]) {
    const result = streamSummary([delta('OK', 1), textDone('OK'), { ...complete, response: { ...complete.response, output: [item] } }]);
    assert.equal(result.passed, false); assert.equal(result.category, 'INVALID_RESPONSE_SHAPE');
  }
});
check('empty item completion cannot contradict a nonempty stream', () => {
  const item = itemDone('OK'); item.item.content = [];
  const result = streamSummary([delta('OK', 1), textDone('OK'), item, textless]);
  assert.equal(result.passed, false); assert.equal(result.textDiagnostics.snapshotsMatch, false);
});
check('duplicate content completion prevents reconstruction', () => {
  const part = { type: 'response.content_part.done', output_index: 1, content_index: 0,
    part: { type: 'output_text', text: 'OK' } };
  const result = streamSummary([delta('OK', 1), textDone('OK'), part, part, textless]);
  assert.equal(result.passed, false); assert.equal(result.textDiagnostics.streamOrderValid, false);
});
// Search mode: the envelope the gateway sends, and a backend search counted rather than
// treated as an unexpected tool. Never reaches the network; every fixture is local.
const searchItem = { id: 'ws_synthetic', type: 'web_search_call', status: 'completed' };
const searchComplete = { type: 'response.completed', response: { model: liteModel, status: 'completed',
  reasoning: { effort: liteEffort }, output: [searchItem,
    { type: 'message', content: [{ type: 'output_text', text: 'SYNTHETIC_REPLY' }] }],
  usage: { input_tokens: 1, output_tokens: 1, total_tokens: 2 } } };
const liteSummary = (...events) => summarizeResponse(200, 'text/event-stream',
  Buffer.concat(events.map(sse)), null, true);
check('search envelope shape', () => {
  const body = buildLiteBody(liteSearchEnvelope());
  assert.equal('instructions' in body, false);
  assert.equal('tools' in body, false);
  // An empty namespace drew a 400 naming tools, so with no client tools the item is not sent.
  assert.equal(body.input.some(item => item.type === 'additional_tools'), false);
  assert.equal(body.reasoning.context, 'all_turns');
  assert.equal(typeof body.prompt_cache_key, 'string');
  assert.equal(body.client_metadata.session_id, body.prompt_cache_key);
  assert.equal(body.model, liteModel);
});
check('the question cannot be answered from memory', () => {
  const body = buildLiteBody(liteSearchEnvelope());
  assert.equal(body.input[0].content[0].text, LITE_PROMPT);
  assert.match(LITE_PROMPT, /NO_SEARCH/);
});
check('a named refusal is reported as a boolean', () => {
  const refusal = { ...searchComplete, response: { ...searchComplete.response,
    output: [{ type: 'message', content: [{ type: 'output_text', text: 'NO_SEARCH' }] }] } };
  const result = liteSummary(refusal);
  assert.equal(result.noSearchDeclared, true);
  assert.equal(result.webSearchCalls, 0);
  assert.equal(result.passed, false);
  assert.equal(liteSummary(searchComplete).noSearchDeclared, false);
});
check('backend search counted, not unexpected', () => {
  const result = liteSummary(searchComplete);
  assert.equal(result.webSearchCalls, 1);
  assert.equal(result.unexpectedTool, false);
  assert.equal(result.passed, true);
});
check('search events count once per item', () => {
  const result = liteSummary({ type: 'response.web_search_call.in_progress', item_id: 'ws_synthetic' },
    { type: 'response.web_search_call.completed', item_id: 'ws_synthetic' }, searchComplete);
  assert.equal(result.webSearchCalls, 1);
});
check('search envelope accepted but no search fails', () => {
  const result = liteSummary({ ...searchComplete,
    response: { ...searchComplete.response, output: [searchComplete.response.output[1]] } });
  assert.equal(result.webSearchCalls, 0);
  assert.equal(result.passed, false);
});
check('a search item is still unexpected in connectivity mode', () =>
  assert.equal(summary({ ...complete, response: { ...complete.response, output: [searchItem] } }).unexpectedTool, true));
check('error vocabulary reported, message text is not', () => {
  const result = summarizeResponse(400, 'application/json', Buffer.from(JSON.stringify({ error: {
    type: 'invalid_request_error', code: 'empty_array', param: 'input[0].tools',
    message: 'SYNTHETIC_PRIVATE_CANARY' } })), null, true);
  assert.deepEqual(result.errorLabels, { type: 'invalid_request_error', code: 'empty_array', param: 'input[0].tools' });
  assert.equal(JSON.stringify(result).includes('SYNTHETIC_PRIVATE_CANARY'), false);
});
check('an unbounded error label is dropped', () => {
  const result = summarizeResponse(400, 'application/json', Buffer.from(JSON.stringify({ error: {
    type: 'a b c', code: 'x'.repeat(200), param: { nested: 1 } } })), null, true);
  assert.equal(result.errorLabels, undefined);
});
check('rejected field named without echoing upstream text', () => {
  const result = summarizeResponse(400, 'application/json',
    Buffer.from('{"error":{"message":"Unknown parameter: client_metadata. SYNTHETIC_PRIVATE_CANARY"}}'), null, true);
  assert.deepEqual(result.rejectedFields, ['client_metadata']);
  assert.equal(JSON.stringify(result).includes('SYNTHETIC_PRIVATE_CANARY'), false);
});
// The version is read, not pinned: an unvalidated version is reported, not silently sent and
// not aborted. Only an unreadable or malformed version stops the run.
check('installed version reported as unverified', () => {
  const result = readClientVersion({ status: 0, stdout: 'codex-cli 9.9.9' + String.fromCharCode(10) });
  assert.equal(result.clientVersion, '9.9.9');
  assert.equal(result.clientVersionStatus, 'unverified');
});
check('baseline version reported as reference', () => {
  const result = readClientVersion({ status: 0, stdout: `codex-cli ${REFERENCE_CLIENT_VERSION}` });
  assert.equal(result.clientVersionStatus, 'reference');
});
check('unreadable version stops the run', () => {
  for (const result of [{ status: 1, stdout: 'codex-cli 1.2.3' }, { error: new Error('x'), status: 0, stdout: '' },
    { status: 0, stdout: 'something else' }, { status: 0, stdout: 'codex-cli 1.2.3 extra' }]) {
    assert.throws(() => readClientVersion(result), /CLI_VERSION_UNREADABLE/);
  }
});
check('malformed version stops the run', () =>
  assert.throws(() => readClientVersion({ status: 0, stdout: 'codex-cli not.a.version' }), /CLI_VERSION_INVALID/));
// A search request identifies itself as the reference client, because the backend picks the
// instructions — and with them the built-in toolset — from that identity. Everything else keeps
// the identity the ordinary path has always sent.
const credential = { accessToken: 'synthetic', account: 'synthetic-account' };
check('search request uses the reference client identity', () => {
  const headers = buildHeaders(credential, '1.2.3', '{}', true);
  assert.equal(headers.originator, 'codex_exec');
  assert.match(headers['User-Agent'], /^codex_exec\/1\.2\.3 \(.+\) xterm-256color \(codex_exec; 1\.2\.3\)$/);
  assert.equal('Version' in headers, false);
  assert.equal('Openai-Beta' in headers, false);
});
check('the ordinary request identity is unchanged', () => {
  const headers = buildHeaders(credential, '1.2.3', '{}');
  assert.equal(headers.originator, 'codex_cli_rs');
  assert.equal(headers['User-Agent'], 'codex-cli/1.2.3 (Windows; x64)');
  assert.equal(headers.Version, '1.2.3');
  assert.equal(headers['Openai-Beta'], 'responses=experimental');
});
check('both carry the same credential and framing headers', () => {
  for (const headers of [buildHeaders(credential, '1.2.3', '{}'), buildHeaders(credential, '1.2.3', '{}', true)]) {
    assert.equal(headers.Authorization, 'Bearer synthetic');
    assert.equal(headers['chatgpt-account-id'], 'synthetic-account');
    assert.equal(headers.Accept, 'text/event-stream');
    assert.equal(headers['Content-Length'], 2);
  }
});
console.log(JSON.stringify({ offlineTests: count, passed: count, credentialReads: 0, networkRequests: 0 }));
