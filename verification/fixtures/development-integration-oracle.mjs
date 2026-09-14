import { readFileSync } from 'node:fs';
import { pathToFileURL } from 'node:url';
import { isDeepStrictEqual } from 'node:util';

// The controller validates and hashes this data before starting the child.
// Expressions select only case values, earlier results, records and safe sums.
function argument(expression, inputs, results) {
  if (expression.kind === 'input') return inputs[expression.index];
  if (expression.kind === 'result') return results[expression.index];
  if (expression.kind === 'literal') return expression.value;
  if (expression.kind === 'object') return Object.fromEntries(Object.entries(expression.fields)
    .map(([key, value]) => [key, argument(value, inputs, results)]));
  if (expression.kind === 'sum') {
    const left = argument(expression.left, inputs, results), right = argument(expression.right, inputs, results);
    return Number.isSafeInteger(left) && Number.isSafeInteger(right) && Number.isSafeInteger(left + right) ? left + right : null;
  }
  throw new Error('INTEGRATION_EXPRESSION_INVALID');
}
let checks = 0;
const failures = [];
try {
  const specification = JSON.parse(readFileSync(process.argv[2], 'utf8'));
  const operations = [];
  for (const source of specification.modules) {
    const module = await import(pathToFileURL(source.path).href);
    if (typeof module[source.exportName] !== 'function') throw new Error('MISSING_EXPORT');
    operations.push(module[source.exportName]);
  }
  for (const test of specification.cases) {
    checks++;
    const inputs = JSON.parse(JSON.stringify(test.inputs)), before = JSON.stringify(inputs), results = [];
    try {
      for (const step of specification.steps) results.push(operations[step.moduleIndex](argument(step.argument, inputs, results)));
      if (!isDeepStrictEqual(results.at(-1), test.expected) || JSON.stringify(inputs) !== before) failures.push(test.name);
    } catch { failures.push(test.name); }
  }
} catch { failures.push('INTEGRATION_LOAD_FAILED'); }
const passed = failures.length === 0;
console.log(JSON.stringify({ passed, checks, failures }));
process.exitCode = passed ? 0 : 1;
