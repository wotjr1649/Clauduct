import assert from 'node:assert/strict';
import { createAuthOwnerProtocol } from '../verification/auth-owner-protocol.mjs';

const root = 'C:\\PublicFixture\\codex-home';
const initial = { id: 0, result: { codexHome: root, platformFamily: 'windows', platformOs: 'windows', userAgent: 'PUBLIC_DISCARDED_AGENT' } };
const account = id => ({ id, result: { account: { type: 'chatgpt', email: 'public@example.com', planType: 'pro' }, requiresOpenaiAuth: true } });
const notification = { method: 'account/updated', params: { authMode: 'chatgpt', planType: 'pro' } };
const packet = value => Buffer.from(JSON.stringify(value) + '\n');
const channel = refresh => createAuthOwnerProtocol({ expectedCodexHome: root, refresh });
let checks = 0;
for (const refresh of [false, true]) {
  const protocol = channel(refresh), outgoing = protocol.takeOutgoing().map(JSON.parse);
  assert.deepEqual(outgoing.map(value => value.method), ['initialize']);
  const first = packet(initial);
  for (const byte of first) protocol.push(Buffer.from([byte]));
  outgoing.push(...protocol.takeOutgoing().map(JSON.parse));
  assert.deepEqual(outgoing.slice(1), [{ method: 'initialized', params: {} }, { method: 'account/read', id: 1, params: { refreshToken: false } }]);
  protocol.push(packet(notification)); protocol.push(packet(account(1)));
  const more = protocol.takeOutgoing().map(JSON.parse); outgoing.push(...more);
  assert.deepEqual(more, refresh ? [{ method: 'account/read', id: 2, params: { refreshToken: true } }] : []);
  if (refresh) protocol.push(packet(account(2)));
  protocol.end(); const result = protocol.snapshot();
  assert.equal(result.exchangeComplete, true); assert.equal(result.refreshReplyReceived, refresh);
  assert.equal(result.normalRefreshVerified, false); assert.equal(result.pendingBytes, 0);
  assert.doesNotMatch(JSON.stringify({ outgoing, result }), /public@example|PUBLIC_DISCARDED_AGENT|codex-home|planType/);
  checks++;
}
const badInitial = [
  { ...initial, id: 1 }, { ...initial, id: '0' }, { ...initial, error: { message: 'PUBLIC_DISCARDED_ERROR' } },
  { ...initial, result: { ...initial.result, codexHome: 'C:\\OtherFixture\\codex-home' } },
  { ...initial, result: { ...initial.result, platformOs: 'linux' } }, { ...initial, result: { ...initial.result, extra: true } },
  { method: 'item/commandExecution/requestApproval', id: 9, params: { command: 'PUBLIC_UNREVIEWED_COMMAND' } },
  { method: 'account/chatgptAuthTokens/refresh', id: 9, params: {} },
  { method: 'account/updated', params: { authMode: 'chatgptAuthTokens' } },
  { method: 'thread/started', params: { thread: 'PUBLIC_OTHER_THREAD' } }, null, [], { jsonrpc: 'other', ...initial }
];
for (const value of badInitial) {
  const protocol = channel(true); protocol.takeOutgoing(); protocol.push(packet(value));
  assert.notEqual(protocol.snapshot().failure, null); assert.deepEqual(protocol.takeOutgoing(), []);
  protocol.end(); assert.equal(protocol.snapshot().exchangeComplete, false);
  assert.doesNotMatch(JSON.stringify(protocol.snapshot()), /PUBLIC_DISCARDED_ERROR|PUBLIC_OTHER_THREAD|PUBLIC_UNREVIEWED_COMMAND/); checks++;
}
for (const result of [null, { requiresOpenaiAuth: true, account: null }, { requiresOpenaiAuth: false, account: { type: 'chatgpt' } },
  { requiresOpenaiAuth: true, account: { type: 'apiKey' } }, { requiresOpenaiAuth: true, account: { type: 'amazonBedrock' } },
  { requiresOpenaiAuth: true, account: { type: 'chatgpt', accessToken: 'PUBLIC_UNEXPECTED_FIELD' } }]) {
  const protocol = channel(true); protocol.takeOutgoing(); protocol.push(packet(initial)); protocol.takeOutgoing();
  protocol.push(packet({ id: 1, result }));
  assert.equal(protocol.snapshot().failure, 'AUTH_OWNER_NO_CHATGPT_ACCOUNT'); assert.deepEqual(protocol.takeOutgoing(), []); checks++;
}
for (const [bytes, failure] of [[Buffer.from('x'.repeat(16385)), 'AUTH_OWNER_LINE_LIMIT'],
  [Buffer.alloc(65537, 32), 'AUTH_OWNER_OUTPUT_LIMIT'], [Buffer.from([255,10]), 'AUTH_OWNER_INVALID_JSON_OR_UTF8'],
  [Buffer.from('not-json\n'), 'AUTH_OWNER_INVALID_JSON_OR_UTF8']]) {
  const protocol = channel(true); protocol.push(bytes);
  assert.equal(protocol.snapshot().failure, failure); assert.deepEqual(protocol.takeOutgoing(), []); checks++;
}
{
  const protocol = channel(true); protocol.takeOutgoing();
  protocol.push(Buffer.concat([packet(initial), packet({ method: 'PUBLIC_UNEXPECTED_METHOD', params: {} })]));
  assert.deepEqual(protocol.takeOutgoing(), []); assert.equal(protocol.snapshot().failure, 'AUTH_OWNER_UNEXPECTED_METHOD'); checks++;
}
{
  const protocol = channel(false); protocol.takeOutgoing(); protocol.push(packet(initial)); protocol.takeOutgoing();
  protocol.push(packet(account(1))); protocol.push(packet(account(1)));
  assert.equal(protocol.snapshot().failure, 'AUTH_OWNER_RESPONSE_MISMATCH'); checks++;
}
{
  const protocol = channel(false); protocol.push(packet(initial).subarray(0, -1)); protocol.end();
  assert.equal(protocol.snapshot().failure, 'AUTH_OWNER_TRUNCATED_LINE'); checks++;
}
{
  const protocol = channel(false); protocol.end();
  assert.equal(protocol.snapshot().failure, 'AUTH_OWNER_INCOMPLETE'); checks++;
}
{
  const protocol = channel(false); protocol.takeOutgoing();
  for (let index = 0; index < 65; index++) protocol.push(packet(notification));
  assert.equal(protocol.snapshot().failure, 'AUTH_OWNER_RECORD_LIMIT'); checks++;
}
for (const options of [{ expectedCodexHome: '../home' }, { expectedCodexHome: root, refresh: 'true' }, { expectedCodexHome: root + '\n' }]) {
  assert.throws(() => createAuthOwnerProtocol(options), { message: 'AUTH_OWNER_ARGUMENTS' }); checks++;
}
{
  const protocol = channel(false); protocol.push(packet(initial));
  assert.equal(protocol.snapshot().failure, 'AUTH_OWNER_RESPONSE_MISMATCH'); assert.deepEqual(protocol.takeOutgoing(), []); checks++;
}
{
  const protocol = channel(true); protocol.takeOutgoing(); protocol.push(Buffer.concat([packet(initial), packet(account(1)), packet(account(2))]));
  assert.equal(protocol.snapshot().failure, 'AUTH_OWNER_RESPONSE_MISMATCH'); assert.deepEqual(protocol.takeOutgoing(), []); checks++;
}
{
  const protocol = channel(false); protocol.takeOutgoing(); protocol.push(packet(initial)); protocol.takeOutgoing();
  protocol.push(packet(account(1))); protocol.push(packet({ result: account(1).result }));
  assert.equal(protocol.snapshot().failure, 'AUTH_OWNER_RESPONSE_MISMATCH'); checks++;
}
for (const afterEnd of ['packet', 'end']) {
  const protocol = channel(false); protocol.takeOutgoing(); protocol.push(packet(initial)); protocol.takeOutgoing();
  protocol.push(packet(account(1))); protocol.end();
  if (afterEnd === 'packet') protocol.push(packet(notification)); else protocol.end();
  assert.equal(protocol.snapshot().exchangeComplete, false); assert.deepEqual(protocol.takeOutgoing(), []); checks++;
}
console.log(JSON.stringify({ suite: 'auth-owner-protocol', checks, normalRefresh: 'NOT_RUN',
  actualOwnerConnections: 0, actualCredentialReads: 0, externalRequests: 0 }));
