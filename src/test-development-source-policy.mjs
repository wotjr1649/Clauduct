import assert from 'node:assert/strict';
import { checkDevelopmentSource } from '../verification/development-source-policy.mjs';
import { createFixtureToolPolicy } from '../verification/fixture-tool-policy.mjs';
import { BASELINE_SOURCE } from '../verification/development-fixture.mjs';

let checks = 0;
const source = `export function parseRetryAfterSeconds(value) {
  if (typeof value !== 'string' || value.length > 128 || /[\\r\\n]/.test(value) || !/^[ \\t]*[0-9]+[ \\t]*$/.test(value)) return null;
  const milliseconds = Number(value) * 1000;
  return Number.isSafeInteger(milliseconds) ? milliseconds : null;
}
`;
for (const value of [BASELINE_SOURCE, source]) { assert.equal(checkDevelopmentSource(value), true); checks++; }
const body = text => `export function parseRetryAfterSeconds(value) { ${text} }`;
const invalid = [null, '', source + 'export const ms = 0;', source.repeat(100),
  body('return process.env;'), body('return globalThis;'), body('return import("node:fs");'),
  body('return eval(value);'), body('return Function(value)();'), body('return value.constructor;'),
  body('return value["constructor"];'), body('return `PUBLIC_TEMPLATE`;'), body('return "PUBLIC_UNREVIEWED_TEXT";'),
  body('return /PUBLIC_UNREVIEWED_REGEX/.test(value);'), body('return /^(a+)+$/.test(value);'),
  body('while (true) {}'), body('for (;;) {}'), body('return parseRetryAfterSeconds(value);'),
  body('Number.isSafeInteger = value;'), body('Number++;'), body('--Number;'),
  body('const Number = value;'), body('return value();'), body('return value.test(value);'),
  body('return Math.isSafeInteger(value);'), body('return Number.min(value);'),
  body('return /[\\r\\n]/g.test(value);'), body('return 9007199254740991;'),
  body('return "str\\x69ng";'), body('return \u200bnull;'), body('// hidden content\n return null;'),
  source.replace('value) {', 'value, result) {'), source + 'Number(0);'];
for (const value of invalid) {
  assert.throws(() => checkDevelopmentSource(value), error => error.message === 'DEVELOPMENT_SOURCE_REJECTED'); checks++;
}
const event = (name, args = {}, id = `call_public_${checks}`) => ({ type: 'response.completed', response: {
  output: [{ type: 'function_call', name, arguments: JSON.stringify(args), call_id: id }] } });
const policy = () => createFixtureToolPolicy({ version: 1, kind: 'development', workingRoot: process.cwd() });
const check = policy();
for (const [name, args] of [['read_task', {}], ['run_tests', {}], ['write_source', { code: source }], ['run_tests', {}]]) {
  assert.doesNotThrow(() => check(event(`mcp__fixture__${name}`, args))); checks++;
}
for (const value of [event('Bash', { command: 'public' }), event('ToolSearch', { query: 'fixture' }),
  event('mcp__fixture__write_source', { code: source }), event('mcp__fixture__run_tests'),
  event('mcp__fixture__read_task', { path: '../control' })]) {
  assert.throws(() => policy()(value), error => error.code === 'VERIFICATION_TOOL_INPUT_REJECTED'); checks++;
}
const writePolicy = policy();
writePolicy(event('mcp__fixture__read_task', {}, 'call_read'));
writePolicy(event('mcp__fixture__run_tests', {}, 'call_baseline'));
assert.throws(() => writePolicy(event('mcp__fixture__write_source', { code: body('return process.env;') })),
  error => error.code === 'VERIFICATION_TOOL_INPUT_REJECTED'); checks++;
const duplicate = policy(), first = event('mcp__fixture__read_task', {}, 'call_duplicate');
duplicate(first);
assert.throws(() => duplicate(event('mcp__fixture__run_tests', {}, 'call_duplicate')),
  error => error.code === 'VERIFICATION_TOOL_INPUT_REJECTED'); checks++;
const batch = event('mcp__fixture__read_task');
batch.response.output.push(event('mcp__fixture__run_tests').response.output[0]);
assert.throws(() => policy()(batch), error => error.code === 'VERIFICATION_TOOL_INPUT_REJECTED'); checks++;
const finish = createFixtureToolPolicy({ version: 1, kind: 'development', phase: 'finish', workingRoot: process.cwd() });
assert.doesNotThrow(() => finish(event('mcp__fixture__read_task', {}, 'call_finish_read'))); checks++;
assert.doesNotThrow(() => finish(event('mcp__fixture__run_tests', {}, 'call_finish_test'))); checks++;
for (const value of [event('mcp__fixture__write_source', { code: source }, 'call_finish_write'),
  event('mcp__fixture__read_task', {}, 'call_finish_repeat_read'), event('mcp__fixture__run_tests', {}, 'call_finish_repeat_test')]) {
  assert.throws(() => finish(value), error => error.code === 'VERIFICATION_TOOL_INPUT_REJECTED'); checks++;
}
console.log(JSON.stringify({ suite: 'development-source-policy', checks, externalRequests: 0, credentialReads: 0 }));
