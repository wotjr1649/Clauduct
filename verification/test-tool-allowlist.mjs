import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';

const hook = fileURLToPath(new URL('tool-allowlist.mjs', import.meta.url));
const cwd = fileURLToPath(new URL('../', import.meta.url));
const valid = { hook_event_name: 'PreToolUse', cwd, session_id: 'synthetic-session', turn_id: 'synthetic-turn',
  tool_use_id: 'synthetic-call', tool_name: 'probe_echo', tool_input: { value: 'probe' } };
const cases = [
  ['exact call', valid, true],
  ['shell', { ...valid, tool_name: 'Bash', tool_input: { command: 'echo FIXTURE' } }, false],
  ['code wrapper', { ...valid, tool_name: 'functions.exec' }, false],
  ['nested shell name', { ...valid, tool_name: 'exec_command' }, false],
  ['collaboration', { ...valid, tool_name: 'spawn_agent' }, false],
  ['MCP read', { ...valid, tool_name: 'read_mcp_resource' }, false],
  ['skill read', { ...valid, tool_name: 'skills__read' }, false],
  ['prefix spoof', { ...valid, tool_name: 'probe_echo_extra' }, false],
  ['namespace spoof', { ...valid, tool_name: 'mcp__probe_echo' }, false],
  ['case spoof', { ...valid, tool_name: 'PROBE_ECHO' }, false],
  ['wrong value', { ...valid, tool_input: { value: 'other' } }, false],
  ['extra argument', { ...valid, tool_input: { value: 'probe', command: 'echo FIXTURE' } }, false],
  ['string arguments', { ...valid, tool_input: '{"value":"probe"}' }, false],
  ['missing event', { ...valid, hook_event_name: undefined }, false],
  ['wrong event', { ...valid, hook_event_name: 'PostToolUse' }, false],
  ['missing turn', { ...valid, turn_id: undefined }, false],
  ['outside root', { ...valid, cwd: 'D:\\AIDEV' }, false],
  ['null', null, false],
  ['array', [], false],
  ['broken JSON', '{', false, true],
  ['oversized input', ' '.repeat(65537), false, true],
  ['invalid UTF-8', Buffer.from([0xff]), false, true]
];
for (const [name, input, accepted, raw] of cases) {
  const result = spawnSync(process.execPath, [hook], { cwd, input: raw ? input : JSON.stringify(input),
    encoding: 'utf8', timeout: 5000, maxBuffer: 16384, windowsHide: true });
  assert.equal(result.error, undefined, `${name}: process failure`);
  assert.equal(result.status, 0, `${name}: process exit`);
  assert.equal(result.stderr, '', `${name}: unexpected stderr`);
  const output = JSON.parse(result.stdout);
  if (accepted) assert.deepEqual(output, {}, `${name}: must not override existing policy`);
  else assert.equal(output.hookSpecificOutput?.permissionDecision, 'deny', `${name}: must deny`);
}
console.log(JSON.stringify({ tests: cases.length, passed: cases.length, hookAppliedToCodex: false }));
