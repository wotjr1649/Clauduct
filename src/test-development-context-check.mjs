import assert from 'node:assert/strict';
import { createDevelopmentContextCheck } from '../verification/native-development-entry.mjs';
const input = [
  { role: 'user', content: [{ type: 'input_text', text: 'Implement parseRetryAfterSeconds(value).' }] },
  { role: 'assistant', content: [{ type: 'output_text', text: 'CLAUDUCT_DEVELOPMENT_DONE' }] }
];
let checks = 0;
const equal = (value, expected) => { assert.deepEqual(value, expected); checks++; };
const good = createDevelopmentContextCheck('retry-after-seconds');
const accepted = good(input);
equal(accepted.previousFunctionObserved, true);
equal(accepted.previousCompletionObserved, true);
equal(accepted.observedInputBytes, Buffer.byteLength(JSON.stringify(input)));
equal(Object.isFrozen(accepted), true);
equal(good([]), accepted);
for (const value of [undefined, null, [], {}, input.slice(0, 1), input.slice(1),
  input.map(item => ({ ...item, role: 'user' }))]) {
  const check = createDevelopmentContextCheck('retry-after-seconds');
  const denied = check(value);
  equal(denied.previousFunctionObserved && denied.previousCompletionObserved, false);
  equal(check(input), denied);
}
const cyclic = []; cyclic.push(cyclic);
for (const value of [cyclic, [1n]]) {
  const check = createDevelopmentContextCheck('retry-after-seconds');
  assert.throws(() => check(value)); checks++;
  equal(check(input).previousFunctionObserved, false);
  equal(check(input).previousCompletionObserved, false);
}
const other = createDevelopmentContextCheck('retry-delay-window')(input);
equal(other.previousFunctionObserved, false);
for (const value of ['constructor', '../control', null]) {
  assert.throws(() => createDevelopmentContextCheck(value), { message: 'INVALID_DEVELOPMENT_TASK' }); checks++;
}
console.log(JSON.stringify({ suite: 'development-context-check', checks, deniedInitialBindingsStayDenied: true,
  rawInputPersisted: false, actualNativeExecutions: 0, actualCredentialReads: 0, externalRequests: 0 }));
