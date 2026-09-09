// Verification only: leave the exact synthetic call to existing policy; deny all other calls.
import { fileURLToPath } from 'node:url';
import { resolve } from 'node:path';

const expectedCwd = resolve(fileURLToPath(new URL('../', import.meta.url))).toLowerCase();
const limit = 64 * 1024;
let chunks = [], bytes = 0, finished = false;
function finish(accepted) {
  if (finished) return;
  finished = true;
  clearTimeout(deadline);
  chunks = [];
  const output = accepted ? {} : { hookSpecificOutput: {
    hookEventName: 'PreToolUse', permissionDecision: 'deny',
    permissionDecisionReason: 'CLAUDUCT_TEST_DENY: Only the fixed probe_echo test call is permitted.'
  } };
  process.stdout.write(JSON.stringify(output) + '\n', () => process.exit(0));
}
const deadline = setTimeout(() => finish(false), 3000);
process.stdin.on('error', () => finish(false));
process.stdin.on('data', chunk => {
  if (finished) return;
  bytes += chunk.length;
  if (bytes > limit) finish(false);
  else chunks.push(chunk);
});
process.stdin.on('end', () => {
  if (finished) return;
  let accepted = false;
  try {
    const input = JSON.parse(new TextDecoder('utf-8', { fatal: true }).decode(Buffer.concat(chunks)));
    accepted = input !== null && typeof input === 'object' && !Array.isArray(input)
      && input.hook_event_name === 'PreToolUse'
      && typeof input.cwd === 'string' && resolve(input.cwd).toLowerCase() === expectedCwd
      && ['session_id', 'turn_id', 'tool_use_id'].every(key => typeof input[key] === 'string' && input[key].length > 0)
      && input.tool_name === 'probe_echo'
      && input.tool_input !== null && typeof input.tool_input === 'object' && !Array.isArray(input.tool_input)
      && Object.keys(input.tool_input).length === 1 && input.tool_input.value === 'probe';
  } catch { /* Invalid input has the same explicit deny response as a disallowed call. */ }
  finish(accepted);
});
