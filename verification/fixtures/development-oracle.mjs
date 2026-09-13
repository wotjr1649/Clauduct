import { pathToFileURL, fileURLToPath } from 'node:url';
import { resolve } from 'node:path';
const cases = [
  ['zero', '0', 0], ['one', '1', 1000], ['sixty-not-capped', '60', 60000], ['three-hundred-not-capped', '300', 300000],
  ['large-not-capped', '86400', 86400000], ['leading-zeros', '00060', 60000], ['http-ows', ' \t300\t ', 300000],
  ['empty', '', null], ['spaces-only', ' \t ', null], ['number-input', 60, null], ['null-input', null, null],
  ['undefined-input', undefined, null], ['array-input', ['60'], null], ['object-input', {}, null],
  ['negative', '-1', null], ['signed', '+1', null], ['fraction', '1.5', null], ['exponent', '1e3', null],
  ['hex', '0x10', null], ['infinity', 'Infinity', null], ['nan', 'NaN', null], ['date-not-seconds', 'Sun, 13 Sep 2026 00:00:00 GMT', null],
  ['comma-values', '60,300', null], ['crlf', '60\r\n', null], ['unicode-space', '\u00a060', null],
  ['overflow', '9007199254741', null], ['safe-large', '9007199254740', 9007199254740000], ['size-limit', '0'.repeat(129), null]
];
export function checkRetryAfterSeconds(operation) {
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
  try { result = checkRetryAfterSeconds((await import(pathToFileURL(process.argv[2]).href)).parseRetryAfterSeconds); }
  catch { result = { passed: false, checks: cases.length, failures: ['MODULE_LOAD_FAILED'] }; }
  console.log(JSON.stringify(result)); process.exitCode = result.passed ? 0 : 1;
}
