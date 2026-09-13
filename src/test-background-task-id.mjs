import assert from 'node:assert/strict';
import { readBackgroundTaskId, createBackgroundTaskBindings } from '../verification/background-task-id.mjs';

let checks = 0;
const output = (callId = 'call_public_1', id = 'public_task_alpha') => ({ type: 'function_call_output', call_id: callId,
  output: [{ type: 'input_text', text: `Command running in background with ID: ${id}. Output is being written to: PUBLIC_OUTPUT_PATH` }] });
const body = value => ({ input: [value] });
const rejects = operation => { assert.throws(operation, error => error.code === 'VERIFICATION_TOOL_INPUT_REJECTED'); checks++; };
assert.equal(readBackgroundTaskId(body(output()), 'call_public_1'), 'public_task_alpha'); checks++;
for (const value of [null, {}, { input: null }, { input: [] }, { input: Array(4097).fill({}) },
  { input: [output(), output()] }, body(output('different_call')),
  body({ ...output(), output: [] }), body({ ...output(), output: [output().output[0], output().output[0]] }),
  body({ ...output(), output: 'Command running in background with ID: public_task_alpha.' }),
  body({ ...output(), output: [{ type: 'input_image', text: output().output[0].text }] })]) {
  rejects(() => readBackgroundTaskId(value, 'call_public_1'));
}
for (const text of ['', 'Task ID: public_task_alpha', 'prefix\n' + output().output[0].text,
  output().output[0].text + '\n' + output().output[0].text,
  'Command running in background with ID: ../private.',
  'Command running in background with ID: ' + 'x'.repeat(129) + '.',
  output().output[0].text + 'x'.repeat(16384)]) {
  rejects(() => readBackgroundTaskId(body({ ...output(), output: [{ type: 'input_text', text }] }), 'call_public_1'));
}
for (const callId of [undefined, null, {}, '', '../private', 'x'.repeat(129)]) rejects(() => readBackgroundTaskId(body(output()), callId));
const bindings = createBackgroundTaskBindings();
assert.equal(bindings.matches('call_public_1', 'public_task_alpha'), false); checks++;
assert.equal(bindings.observe(body(output()), 'call_public_1'), 'public_task_alpha');
assert.equal(bindings.observe(body(output()), 'call_public_1'), 'public_task_alpha');
assert.equal(bindings.count(), 1); checks++;
assert.equal(bindings.observe(body(output('call_public_2', 'public_task_beta')), 'call_public_2'), 'public_task_beta');
assert.equal(bindings.matches('call_public_1', 'public_task_alpha'), true);
assert.equal(bindings.matches('call_public_2', 'public_task_beta'), true);
assert.equal(bindings.matches('call_public_1', 'public_task_beta'), false); checks++;
rejects(() => bindings.observe(body(output('call_public_1', 'public_task_beta')), 'call_public_1'));
rejects(() => bindings.observe(body(output('call_public_3', 'public_task_alpha')), 'call_public_3'));
rejects(() => bindings.observe(body(output('call_public_3', 'public_task_gamma')), 'call_public_3'));
assert.equal(bindings.count(), 2); checks++;
console.log(JSON.stringify({ suite: 'background-task-id', checks, externalRequests: 0, actualCredentialReads: 0 }));
