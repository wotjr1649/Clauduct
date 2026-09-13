import { Agent } from 'node:https';
import { NativeError } from '../src/native-protocol.mjs';

// Verification process only. Reject one connection before DNS/TLS/socket I/O,
// then delegate to the unchanged HTTPS implementation and certificate checks.
export function installDnsFailure(AgentType = Agent) {
  const fail = () => { throw new NativeError('VERIFICATION_CONNECTION_FAULT_REJECTED'); };
  const prototype = AgentType?.prototype, original = prototype?.createConnection;
  if (typeof original !== 'function') fail();
  let connectionCalls = 0, injected = 0, restored = false;
  function replacement(options, ...args) {
    if (restored || connectionCalls >= 256 || options?.host !== 'chatgpt.com'
      || ![443, '443'].includes(options.port) || options.rejectUnauthorized !== true
      || typeof args[0] !== 'function') fail();
    connectionCalls++;
    if (injected === 0) {
      injected++;
      const callback = args[0];
      queueMicrotask(() => callback(Object.assign(new Error('PUBLIC_DNS_FAILURE'), { code: 'EAI_AGAIN' })));
      return;
    }
    return Reflect.apply(original, this, [options, ...args]);
  }
  prototype.createConnection = replacement;
  return Object.freeze({
    snapshot: () => ({ connectionCalls, injected, restored }),
    restore() {
      if (restored) return;
      if (prototype.createConnection !== replacement) fail();
      prototype.createConnection = original; restored = true;
    }
  });
}
