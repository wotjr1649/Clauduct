import assert from 'node:assert/strict';
import { Agent, request } from 'node:https';
import { installDnsFailure } from '../verification/connection-fault.mjs';
const options = { host: 'chatgpt.com', port: 443, rejectUnauthorized: true };
let passed = 0;
for (const invalid of [{ host: 'public.invalid' }, { port: 80 }, { rejectUnauthorized: false }]) {
  class LocalAgent { createConnection() { throw new Error('ORIGINAL_MUST_NOT_RUN'); } }
  const fault = installDnsFailure(LocalAgent);
  assert.throws(() => new LocalAgent().createConnection({ ...options, ...invalid }, () => {}),
    error => error.code === 'VERIFICATION_CONNECTION_FAULT_REJECTED');
  assert.deepEqual(fault.snapshot(), { connectionCalls: 0, injected: 0, restored: false });
  fault.restore(); passed++;
}
{
  let delegated = 0;
  class LocalAgent { createConnection(value, callback) { delegated++; assert.equal(value, options); assert.equal(typeof callback, 'function'); return 'ORIGINAL'; } }
  const original = LocalAgent.prototype.createConnection, instance = new LocalAgent();
  const fault = installDnsFailure(LocalAgent);
  const error = await new Promise(done => instance.createConnection(options, done));
  assert.equal(error.code, 'EAI_AGAIN'); assert.equal(delegated, 0);
  assert.equal(instance.createConnection(options, () => {}), 'ORIGINAL'); assert.equal(delegated, 1);
  fault.restore(); fault.restore(); assert.equal(LocalAgent.prototype.createConnection, original);
  assert.deepEqual(fault.snapshot(), { connectionCalls: 2, injected: 1, restored: true }); passed++;
}
{
  class LocalAgent { createConnection() {} }
  const fault = installDnsFailure(LocalAgent), next = () => {};
  LocalAgent.prototype.createConnection = next;
  assert.throws(() => fault.restore(), error => error.code === 'VERIFICATION_CONNECTION_FAULT_REJECTED');
  assert.equal(LocalAgent.prototype.createConnection, next); passed++;
}
// Confirm the installed Node HTTPS Agent callback/options without opening a socket.
const original = Agent.prototype.createConnection, agent = new Agent({ keepAlive: false });
const fault = installDnsFailure();
try {
  const error = await new Promise(done => {
    const req = request('https://chatgpt.com/backend-api/codex/responses', { method: 'POST', agent,
      rejectUnauthorized: true, signal: AbortSignal.timeout(1000) });
    req.once('error', done); req.end('{}');
  });
  assert.equal(error.code, 'EAI_AGAIN');
  assert.deepEqual(fault.snapshot(), { connectionCalls: 1, injected: 1, restored: false }); passed++;
} finally { fault.restore(); agent.destroy(); }
assert.equal(Agent.prototype.createConnection, original);
console.log(JSON.stringify({ suite: 'connection-fault', passed, actualHttpsRequestObject: true,
  originalHttpsConnections: 0, externalRequests: 0, actualCredentialReads: 0 }));
