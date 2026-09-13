import { pathToFileURL } from 'node:url';
import { join } from 'node:path';
import { checkRetryAfterSeconds } from './development-oracle.mjs';
import { checkRetryDelayWithinBudget } from './development-window-oracle.mjs';
const max = Number.MAX_SAFE_INTEGER;
const cases = [
  ['long', ['60', 1000, 62000], 60000], ['longer', ['300', 0, 300001], 300000],
  ['deadline-equal', ['60', 1000, 61000], null], ['deadline-exceeded', ['60', 1000, 60000], null],
  ['zero', ['0', 1000, 1001], 0], ['expired', ['0', 1000, 1000], null],
  ['ows', [' 005\t', 10, 5011], 5000], ['large-valid', ['1', max - 2000, max], 1000],
  ['timestamp-overflow', ['1', max - 999, max], null], ['negative-now', ['1', -1, 2000], null],
  ['fraction-now', ['1', 0.5, 2000], null], ['string-deadline', ['1', 0, '2000'], null],
  ['short', ['1', 0, 2000], 1000], ['exponent', ['1e2', 0, 200000], null],
  ['newline', ['1\n', 0, 2000], null], ['null', [null, 0, 2000], null],
  ['sign', ['-1', 0, 2000], null], ['large-delay', ['9007199254740', 0, max], 9007199254740000],
  ['first-unsafe-second', ['9007199254741', 0, max], null], ['duration-overflow', ['9007199254742', 0, max], null],
  ['object-header', [{}, 0, 2000], null]
];
const failures = [];
try {
  const parse = (await import(pathToFileURL(join(process.argv[2], 'retry-after-seconds.mjs')).href)).parseRetryAfterSeconds;
  const window = (await import(pathToFileURL(join(process.argv[2], 'retry-delay-window.mjs')).href)).retryDelayWithinBudget;
  failures.push(...checkRetryAfterSeconds(parse).failures.map(name => 'seconds_' + name));
  failures.push(...checkRetryDelayWithinBudget(window).failures.map(name => 'window_' + name));
  for (const [name, [header, nowMs, deadlineMs], expected] of cases) {
    try {
      const delay = parse(header), retryAtMs = Number.isSafeInteger(nowMs) && Number.isSafeInteger(delay)
        && Number.isSafeInteger(nowMs + delay) ? nowMs + delay : null;
      if (window(Object.freeze({ nowMs, retryAtMs, deadlineMs })) !== expected) failures.push('project_' + name);
    } catch { failures.push('project_' + name); }
  }
} catch { failures.push('MODULE_LOAD_FAILED'); }
const passed = failures.length === 0;
console.log(JSON.stringify({ passed, checks: 81, failures }));
process.exitCode = passed ? 0 : 1;
