export function publicBudgetTask(label, { localFixture = true } = {}) {
  if (typeof label !== 'string' || !/^[A-Za-z0-9_-]{1,64}$/.test(label) || typeof localFixture !== 'boolean') throw new Error('PUBLIC_BUDGET_TASK_ARGUMENTS');
  const row = (name, input, expected) => ({ name, inputKind: 'json', input, expected });
  const cases = [
    row('remaining', { used: 1, reserved: 2, limit: 10 }, 7),
    row('zero', { used: 0, reserved: 0, limit: 0 }, 0),
    row('all-used', { used: 10, reserved: 0, limit: 10 }, 0),
    row('all-reserved', { used: 0, reserved: 10, limit: 10 }, 0),
    row('mixed-full', { used: 7, reserved: 3, limit: 10 }, 0),
    row('over-limit', { used: 8, reserved: 3, limit: 10 }, null),
    row('safe-boundary', { used: 9007199254740990, reserved: 1, limit: 9007199254740991 }, 0),
    row('unsafe-sum', { used: 9007199254740991, reserved: 1, limit: 9007199254740991 }, null),
    row('extra-data', { used: 1, reserved: 2, limit: 10, ignored: 'PUBLIC' }, 7),
    row('inert-code-string', '"; throw new Error("PUBLIC_INERT"); //', null)
  ];
  for (const field of ['used', 'reserved', 'limit']) {
    const missing = { used: 1, reserved: 2, limit: 10 }; delete missing[field]; cases.push(row(`missing-${field}`, missing, null));
    for (const [index, invalid] of [-1, 0.5, '1', null, {}].entries()) cases.push(row(`invalid-${field}-${index}`, { used: 1, reserved: 2, limit: 10, [field]: invalid }, null));
  }
  for (const [index, invalid] of [null, false, 0, 'PUBLIC', []].entries()) cases.push(row(`invalid-container-${index}`, invalid, null));
  for (const inputKind of ['undefined', 'nan', 'positive-infinity', 'negative-infinity']) cases.push({ name: inputKind, inputKind, input: null, expected: null });
  return { version: 1, functionName: 'remainingExecutionBudget',
    requirements: `Implement remainingExecutionBudget(value) for cumulative execution admission reporting. PUBLIC_FIXTURE_${label}.
The baseline subtracts only observed use and forgets reservations. Accept a non-null, non-array object with nonnegative safe-integer used, reserved and limit fields. Ignore extra fields. Return limit - used - reserved only when used + reserved is a safe integer not exceeding limit. Return null for every other input, without coercion, throwing or mutation. Zero remaining is valid. This pure calculation neither grants more budget nor starts work.`,
    baseline: 'export function remainingExecutionBudget(value) {\n  return value.limit - value.used;\n}\n',
    fields: ['used', 'reserved', 'limit'], numbers: ['0'], strings: ['object'], cases,
    localFixtureSource: localFixture ? `export function remainingExecutionBudget(value) {
  if (typeof value !== 'object' || value === null || Array.isArray(value)) return null;
  if (!Number.isSafeInteger(value.used) || value.used < 0 || !Number.isSafeInteger(value.reserved) || value.reserved < 0 || !Number.isSafeInteger(value.limit) || value.limit < 0) return null;
  const sum = value.used + value.reserved;
  if (!Number.isSafeInteger(sum) || sum > value.limit) return null;
  return value.limit - sum;
}
` : null };
}
