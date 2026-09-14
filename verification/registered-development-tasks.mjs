import { writeFileSync, lstatSync, realpathSync, mkdirSync, existsSync, openSync, fstatSync, readSync, closeSync } from 'node:fs';
import { join, dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { createHash } from 'node:crypto';
import { checkDevelopmentSourceAgainstTask } from './development-source-grammar.mjs';

const project = dirname(dirname(fileURLToPath(import.meta.url))), registry = join(project, '.tmp', 'development-task-registry');
const encode = value => JSON.stringify(value) + '\n';
const hash = value => createHash('sha256').update(value).digest('hex');
const need = ok => { if (!ok) throw new Error('REGISTERED_DEVELOPMENT_TASK_INVALID'); };
const shape = (value, keys) => value && typeof value === 'object' && !Array.isArray(value)
  && Object.keys(value).length === keys.length && keys.every(key => Object.hasOwn(value, key));
const fixedLocals = ['value', 'result', 'first', 'second', 'third', 'sum', 'amount', 'remaining', 'delay', 'seconds', 'milliseconds', 'ms', 'digits', 'trimmed'];
const forbidden = new Set(['constructor', 'prototype', '__proto__', 'process', 'globalThis', 'global', 'Function', 'eval', 'require',
  'Number', 'Math', 'Array', 'export', 'function', 'const', 'return', 'typeof', 'null', 'if', 'else', 'then', 'arguments',
  'async', 'await', 'break', 'case', 'catch', 'class', 'continue', 'debugger', 'default', 'delete', 'do', 'extends',
  'false', 'finally', 'for', 'import', 'in', 'instanceof', 'new', 'super', 'switch', 'this', 'throw', 'true', 'try',
  'var', 'void', 'while', 'with', 'yield', 'let', 'static', 'enum', 'implements', 'interface', 'package', 'private', 'protected', 'public']);
const memberNames = new Set(['isFinite', 'isSafeInteger', 'min', 'isArray', 'test', 'trim', 'length']);
function jsonValue(value, depth = 0, counter = { nodes: 0 }) {
  need(depth <= 8 && ++counter.nodes <= 4096);
  if (value === null || typeof value === 'boolean' || typeof value === 'number' && Number.isFinite(value)) return;
  if (typeof value === 'string') { need(value.length <= 4096); return; }
  need(value && typeof value === 'object');
  for (const [key, child] of Object.entries(value)) { need(key.length <= 128); jsonValue(child, depth + 1, counter); }
}
function canonical(value) {
  if (value === null || typeof value !== 'object') return JSON.stringify(value);
  if (Array.isArray(value)) return '[' + value.map(canonical).join(',') + ']';
  return '{' + Object.keys(value).sort().map(key => JSON.stringify(key) + ':' + canonical(value[key])).join(',') + '}';
}
function oracleSource(functionName, cases) {
  // Only JSON string literals enter this fixed program. Case content cannot
  // select code, an import, a path, a command or a destination.
  return `import { pathToFileURL } from 'node:url';
import { isDeepStrictEqual } from 'node:util';
const cases = JSON.parse(${JSON.stringify(JSON.stringify(cases))});
const failures = [];
try {
  const module = await import(pathToFileURL(process.argv[2]).href);
  const operation = module[${JSON.stringify(functionName)}];
  if (typeof operation !== 'function') failures.push('MISSING_EXPORT');
  else for (const test of cases) {
    const value = test.inputKind === 'json' ? test.input : test.inputKind === 'undefined' ? undefined
      : test.inputKind === 'nan' ? NaN : test.inputKind === 'positive-infinity' ? Infinity : -Infinity;
    const before = JSON.stringify(test.input);
    try { if (!isDeepStrictEqual(operation(value), test.expected) || JSON.stringify(test.input) !== before) failures.push(test.name); }
    catch { failures.push(test.name); }
  }
} catch { failures.push('MODULE_LOAD_FAILED'); }
const passed = failures.length === 0;
console.log(JSON.stringify({ passed, checks: cases.length, failures }));
process.exitCode = passed ? 0 : 1;
`;
}
function normalize(text) {
  need(typeof text === 'string' && Buffer.byteLength(text) <= 65536);
  let record;
  try { record = JSON.parse(text); } catch { need(false); }
  need(encode(record) === text && shape(record, ['version', 'functionName', 'requirements', 'baseline', 'fields', 'numbers', 'strings', 'cases', 'localFixtureSource'])
    && record.version === 1 && typeof record.functionName === 'string' && /^[A-Za-z][A-Za-z0-9_]{0,63}$/.test(record.functionName)
    && !forbidden.has(record.functionName) && !fixedLocals.includes(record.functionName) && !memberNames.has(record.functionName)
    && typeof record.requirements === 'string' && record.requirements.length > 0 && Buffer.byteLength(record.requirements) <= 8192
    && !/[\x00-\x08\x0b\x0c\x0e-\x1f\x7f]/.test(record.requirements)
    && typeof record.baseline === 'string' && (record.localFixtureSource === null || typeof record.localFixtureSource === 'string'));
  for (const key of ['fields', 'numbers', 'strings']) need(Array.isArray(record[key]) && record[key].length <= 32
    && new Set(record[key]).size === record[key].length && record[key].every(value => typeof value === 'string'));
  need(record.fields.every(name => /^[A-Za-z][A-Za-z0-9_]{0,47}$/.test(name) && name !== 'value' && name !== record.functionName
    && !forbidden.has(name) && !memberNames.has(name)));
  need(record.numbers.length > 0 && record.numbers.every(value => /^(?:0|[1-9][0-9]{0,15})$/.test(value) && Number.isSafeInteger(Number(value))));
  need(record.strings.every(value => value.length <= 128 && /^[\x20-\x21\x23-\x26\x28-\x5b\x5d-\x7e]*$/.test(value)));
  need(Array.isArray(record.cases) && record.cases.length > 0 && record.cases.length <= 128);
  const names = new Set();
  for (const test of record.cases) {
    need(shape(test, ['name', 'inputKind', 'input', 'expected']) && typeof test.name === 'string'
      && /^[A-Za-z0-9_-]{1,64}$/.test(test.name) && !names.has(test.name)
      && ['json', 'undefined', 'nan', 'positive-infinity', 'negative-infinity'].includes(test.inputKind)
      && (test.inputKind === 'json' || test.input === null)
      && (test.expected === null || typeof test.expected === 'boolean' || typeof test.expected === 'number' && Number.isFinite(test.expected)
        || typeof test.expected === 'string' && test.expected.length <= 4096));
    names.add(test.name); jsonValue(test.input);
  }
  const variables = fixedLocals.filter(name => !record.fields.includes(name));
  const task = { sourceFile: 'implementation.mjs', functionName: record.functionName, oracleFile: null,
    checks: record.cases.length, baseline: record.baseline, registered: true, localFixtureSource: record.localFixtureSource,
    variables, globals: ['Number', 'Math', 'Array'], numbers: record.numbers, strings: record.strings,
    regexes: ['/^[ \\t]*[0-9]+[ \\t]*$/', '/[\\r\\n]/'],
    members: { isFinite: ['Number'], isSafeInteger: ['Number'], min: ['Math'], isArray: ['Array'], test: ['REGEX'],
      length: [...variables], trim: [...variables], ...Object.fromEntries(record.fields.map(name => [name, ['value']])) },
    calls: ['Number', 'isFinite', 'isSafeInteger', 'min', 'isArray', 'test', 'trim'] };
  task.workKey = hash(encode({ functionName: record.functionName, baseline: record.baseline,
    cases: [...new Set(record.cases.map(({ inputKind, input, expected }) => canonical({ inputKind, input, expected })))].sort() }));
  checkDevelopmentSourceAgainstTask(task.baseline, task);
  if (task.localFixtureSource !== null) checkDevelopmentSourceAgainstTask(task.localFixtureSource, task);
  task.oracleSource = oracleSource(task.functionName, record.cases);
  need(Buffer.byteLength(task.oracleSource) <= 49152);
  task.task = `# Reviewed pure-function development task

${record.requirements}

Implement the named export ${task.functionName}(value) in implementation.mjs. Run the baseline tests first, then write the implementation and rerun all ${task.checks} tests. The independent oracle is outside your write scope.
Use only read_task, write_source and run_tests. run_tests waits for the outer developer's source review. Finish only after the tests pass, then reply exactly CLAUDUCT_DEVELOPMENT_DONE.
Keep the module pure: no imports, loops, recursion, file/network/process/environment access, dynamic execution, global/input mutation, logging, dependencies or asynchronous work. One exported function, if/else, const, return, typeof and null only. The allowed local identifiers are ${variables.join(', ')}. Global access is limited to Number, Number.isFinite/isSafeInteger, Math.min and Array.isArray. Only parameter properties ${record.fields.join(', ') || '(none)'}, .length, .trim(), and the fixed HTTP digit/newline regular expressions are allowed. Numeric literals: ${record.numbers.join(', ')}. String literals: ${JSON.stringify(record.strings)}. No comments or additional declarations. The language boundary does not replace source review or tests.
`;
  for (const value of Object.values(task)) if (value && typeof value === 'object') {
    for (const nested of Object.values(value)) if (Array.isArray(nested)) Object.freeze(nested);
    Object.freeze(value);
  }
  return Object.freeze(task);
}
export function registeredDevelopmentTaskPath(taskId) {
  need(typeof taskId === 'string' && /^registered-[a-f0-9]{64}$/.test(taskId));
  return join(registry, taskId.slice('registered-'.length) + '.json');
}
function readRecord(path) {
  let fd;
  try {
    const before = lstatSync(path, { bigint: true });
    need(before.isFile() && !before.isSymbolicLink() && before.nlink === 1n && before.size > 0n && before.size <= 65536n
      && realpathSync(path).toLowerCase() === resolve(path).toLowerCase());
    fd = openSync(path, 'r');
    const opened = fstatSync(fd, { bigint: true }); need(opened.dev === before.dev && opened.ino === before.ino && opened.size === before.size);
    const buffer = Buffer.alloc(65537); let length = 0;
    while (length < buffer.length) { const count = readSync(fd, buffer, length, buffer.length - length, length); if (!count) break; length += count; }
    const after = fstatSync(fd, { bigint: true });
    need(BigInt(length) === before.size && after.size === before.size && after.mtimeNs === before.mtimeNs);
    return new TextDecoder('utf-8', { fatal: true }).decode(buffer.subarray(0, length));
  } catch { throw new Error('REGISTERED_DEVELOPMENT_TASK_INVALID'); }
  finally { if (fd !== undefined) closeSync(fd); }
}
export function readRegisteredDevelopmentTask(taskId) {
  const text = readRecord(registeredDevelopmentTaskPath(taskId));
  need(hash(text) === taskId.slice('registered-'.length));
  return normalize(text);
}
export function registerDevelopmentTask(text, expectedHash) {
  // The caller reviews the intended secret-free payload before registration.
  // Its hash pins those bytes; it is not an authorization or an approval.
  need(typeof text === 'string' && typeof expectedHash === 'string' && /^[a-f0-9]{64}$/.test(expectedHash)
    && Buffer.byteLength(text) <= 65536 && hash(text) === expectedHash);
  normalize(text);
  for (const path of [join(project, '.tmp'), registry]) {
    if (!existsSync(path)) { try { mkdirSync(path); } catch (error) { if (error.code !== 'EEXIST') throw error; } }
    const info = lstatSync(path); need(info.isDirectory() && !info.isSymbolicLink() && realpathSync(path).toLowerCase() === path.toLowerCase());
  }
  const taskId = `registered-${expectedHash}`, path = registeredDevelopmentTaskPath(taskId);
  try { writeFileSync(path, text, { flag: 'wx' }); } catch (error) { if (error.code !== 'EEXIST') throw error; }
  readRegisteredDevelopmentTask(taskId);
  return taskId;
}
