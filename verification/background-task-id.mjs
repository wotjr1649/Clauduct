import { NativeError } from '../src/native-protocol.mjs';

const fail = () => { throw new NativeError('VERIFICATION_TOOL_INPUT_REJECTED'); };
const identifier = value => typeof value === 'string' && /^[A-Za-z0-9_-]{1,128}$/.test(value);

// Use only with a fixed, reviewed worker. Its native Bash result must belong to
// the exact tool call already authorized by the fixture. A model's proposed ID
// or a substring from arbitrary shell output is not ownership evidence.
export function readBackgroundTaskId(body, callId) {
  if (!identifier(callId) || !body || !Array.isArray(body.input) || body.input.length > 4096) fail();
  const results = body.input.filter(item => item?.type === 'function_call_output' && item.call_id === callId);
  if (results.length !== 1 || !Array.isArray(results[0].output) || results[0].output.length !== 1) fail();
  const block = results[0].output[0];
  if (block?.type !== 'input_text' || typeof block.text !== 'string' || Buffer.byteLength(block.text) > 16384) fail();
  const matches = [...block.text.matchAll(/^Command running in background with ID: ([A-Za-z0-9_-]{1,128})\./gm)];
  if (matches.length !== 1 || matches[0].index !== 0) fail();
  return matches[0][1];
}

export function createBackgroundTaskBindings() {
  const bindings = new Map();
  return Object.freeze({
    observe(body, callId) {
      const id = readBackgroundTaskId(body, callId);
      if (bindings.has(callId)) { if (bindings.get(callId) !== id) fail(); }
      else {
        if (bindings.size >= 2 || [...bindings.values()].includes(id)) fail();
        bindings.set(callId, id);
      }
      return id;
    },
    matches(callId, taskId) {
      return identifier(callId) && identifier(taskId) && bindings.has(callId) && bindings.get(callId) === taskId;
    },
    count: () => bindings.size
  });
}
