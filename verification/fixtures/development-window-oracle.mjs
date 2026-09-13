import { pathToFileURL, fileURLToPath } from 'node:url';
import { resolve } from 'node:path';
const record = (nowMs, retryAtMs, deadlineMs) => Object.freeze({ nowMs, retryAtMs, deadlineMs });
const max = Number.MAX_SAFE_INTEGER;
const cases = [
  ['future', record(1000, 61000, 62000), 60000],
  ['long-delay', record(0, 300000, 300001), 300000],
  ['zero-origin', record(0, 0, 1), 0], ['retry-now', record(1000, 1000, 1001), 0],
  ['past-retry', record(1000, 0, 1001), 0], ['one-ms', record(1000, 1001, 1002), 1],
  ['at-deadline', record(1000, 2000, 2000), null], ['after-deadline', record(1000, 61000, 60000), null],
  ['deadline-now', record(1000, 0, 1000), null], ['deadline-past', record(1000, 0, 999), null],
  ['large-safe', record(0, max - 1, max), max - 1], ['large-nearby', record(max - 2, max - 1, max), 1],
  ['unsafe-now', record(max + 1, 0, max), null], ['unsafe-retry', record(0, max + 1, max), null],
  ['unsafe-deadline', record(0, 1, max + 1), null], ['negative-now', record(-1, 1, 2), null],
  ['negative-retry', record(0, -1, 2), null], ['negative-deadline', record(0, 0, -1), null],
  ['fraction-now', record(0.5, 1, 2), null], ['fraction-retry', record(0, 1.5, 2), null],
  ['fraction-deadline', record(0, 1, 2.5), null], ['nan', record(NaN, 1, 2), null],
  ['infinity', record(0, Infinity, 2), null], ['string-field', record(0, '1', 2), null],
  ['null', null, null], ['undefined', undefined, null], ['primitive-string', 'public', null],
  ['primitive-number', 1, null], ['empty-array', Object.freeze([]), null],
  ['array-with-fields', Object.freeze(Object.assign([], { nowMs: 0, retryAtMs: 1, deadlineMs: 2 })), null],
  ['missing-field', Object.freeze({ nowMs: 0, retryAtMs: 1 }), null],
  ['extra-field', Object.freeze({ nowMs: 0, retryAtMs: 1, deadlineMs: 2, ignored: 'public' }), 1]
];
export function checkRetryDelayWithinBudget(operation) {
  const failures = [];
  if (typeof operation !== 'function') failures.push('MISSING_EXPORT');
  else for (const [name, value, expected] of cases) {
    try { if (operation(value) !== expected) failures.push(name); }
    catch { failures.push(name); }
  }
  return { passed: failures.length === 0, checks: cases.length, failures };
}
if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  let result;
  try { result = checkRetryDelayWithinBudget((await import(pathToFileURL(process.argv[2]).href)).retryDelayWithinBudget); }
  catch { result = { passed: false, checks: cases.length, failures: ['MODULE_LOAD_FAILED'] }; }
  console.log(JSON.stringify(result)); process.exitCode = result.passed ? 0 : 1;
}
