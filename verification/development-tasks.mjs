// Public, fixed development requirements. The controller selects a task before
// starting native; model arguments never select a path, policy or oracle.
const tasks = {
  'retry-after-seconds': {
    sourceFile: 'retry-after-seconds.mjs', functionName: 'parseRetryAfterSeconds',
    oracleFile: 'development-oracle.mjs', checks: 28,
    baseline: 'export function parseRetryAfterSeconds(value) {\n  const seconds = Number(value);\n  return Number.isFinite(seconds) && seconds >= 0 ? Math.min(seconds * 1000, 5000) : null;\n}\n',
    task: `# Retry-After seconds parser

Implement the pure named export parseRetryAfterSeconds(value) in retry-after-seconds.mjs for Clauduct's retry policy.
The existing function incorrectly caps long delays at five seconds and accepts non-HTTP numeric formats.

First run the fixed tests on the baseline, then write your implementation and run the tests again. Finish only after all 28 cases pass.
Accept only primitive strings of at most 128 characters, containing one or more ASCII decimal digits, with optional leading/trailing HTTP OWS (space or tab).
Return their nonnegative integer duration in milliseconds without a five-second cap. For example, 60 -> 60000 and 300 -> 300000. Leading zeros are valid.
Return null for all other inputs, including empty strings, CR/LF, other whitespace, dates, signs, fractions, exponent/hex syntax, arrays, objects, and multiplication outside Number's safe integer range.
This bounded task handles delta-seconds only. HTTP-date parsing and request scheduling belong to a later integration task.
Keep the module pure: no imports, file or network access, process/environment access, dynamic execution, global mutation, logging, dependencies, or asynchronous work.
The fixture accepts one exported function using if/else, const and return, parameter value, local names seconds/milliseconds/result/ms/digits/trimmed, typeof, Number, Number.isFinite/isSafeInteger, Math.min, .length, .trim(), and the regular expressions /^[ \\t]*[0-9]+[ \\t]*$/ or /[\\r\\n]/. Only the string literal 'string' and numeric literals 0, 1, 128, 1000, 5000 are allowed. Do not add comments or other declarations. This language restriction is checked before a source write; it does not replace the tests or source review.
Use only read_task, write_source, and run_tests. The independent oracle and execution settings are outside your write scope.
run_tests waits for the outer developer's source review. Do not alter or bypass that review. After tests pass, reply exactly CLAUDUCT_DEVELOPMENT_DONE.
`,
    variables: ['value', 'seconds', 'milliseconds', 'result', 'ms', 'digits', 'trimmed'],
    globals: ['Number', 'Math'], numbers: ['0', '1', '128', '1000', '5000'], strings: ['string'],
    regexes: ['/^[ \\t]*[0-9]+[ \\t]*$/', '/[\\r\\n]/'],
    members: { isFinite: ['Number'], isSafeInteger: ['Number'], min: ['Math'], test: ['REGEX'],
      length: ['value', 'seconds', 'milliseconds', 'result', 'ms', 'digits', 'trimmed'],
      trim: ['value', 'seconds', 'milliseconds', 'result', 'ms', 'digits', 'trimmed'] },
    calls: ['Number', 'isFinite', 'isSafeInteger', 'min', 'test', 'trim']
  },
  'retry-delay-window': {
    sourceFile: 'retry-delay-window.mjs', functionName: 'retryDelayWithinBudget',
    oracleFile: 'development-window-oracle.mjs', checks: 32,
    baseline: 'export function retryDelayWithinBudget(value) {\n  return value.retryAtMs - value.nowMs;\n}\n',
    task: `# Retry delay within an execution deadline

Implement the pure named export retryDelayWithinBudget(value) in retry-delay-window.mjs.
The baseline subtracts timestamps without validating the input or respecting the operation deadline. This can schedule an expired operation or return a negative delay.

First run the fixed baseline tests, then implement the change and run the tests again. All 32 cases must pass before the final report.
Accept a non-null, non-array object whose nowMs, retryAtMs and deadlineMs fields are nonnegative safe integers. Extra fields are ignored. Reject every other input by returning null, without coercion or throwing.
If deadlineMs <= nowMs, return null. If retryAtMs >= deadlineMs, return null: do not shorten the requested retry delay to squeeze a retry inside the budget.
Otherwise return zero if retryAtMs <= nowMs, or retryAtMs - nowMs for a future retry before the deadline.
For {nowMs:1000,retryAtMs:61000,deadlineMs:62000}, return 60000. For {nowMs:1000,retryAtMs:61000,deadlineMs:60000}, return null. A retry exactly at the deadline is rejected.
This function decides whether one retry fits; it does not wait, refresh credentials, retry a request, or change a durable deadline.
Keep the module pure and do not mutate the input. No imports, file/network/process/environment access, dynamic execution, global mutation, logging, dependencies, or asynchronous work.
The fixture accepts one exported function using if/else, const, return, typeof, null, parameter value, local names now/retryAt/deadline/delay/result, Number.isSafeInteger, Array.isArray, comparisons, arithmetic and boolean operators. Only value.nowMs/value.retryAtMs/value.deadlineMs property reads, the string literal 'object', and numeric literal 0 are permitted. No comments, loops, additional declarations or arbitrary property access. This language check precedes every write and does not replace source review or tests.
Use only read_task, write_source and run_tests. The oracle and execution settings are outside your write scope. run_tests waits for the outer developer's source review; do not bypass it. After the tests pass, reply exactly CLAUDUCT_DEVELOPMENT_DONE.
`,
    variables: ['value', 'now', 'retryAt', 'deadline', 'delay', 'result'],
    globals: ['Number', 'Array'], numbers: ['0'], strings: ['object'], regexes: [],
    members: { isSafeInteger: ['Number'], isArray: ['Array'], nowMs: ['value'], retryAtMs: ['value'], deadlineMs: ['value'] },
    calls: ['isSafeInteger', 'isArray']
  }
};
for (const task of Object.values(tasks)) {
  for (const value of Object.values(task)) if (value && typeof value === 'object') {
    for (const nested of Object.values(value)) if (Array.isArray(nested)) Object.freeze(nested);
    Object.freeze(value);
  }
  Object.freeze(task);
}
Object.freeze(tasks);
export const DEFAULT_DEVELOPMENT_TASK_ID = 'retry-after-seconds';
export function developmentTask(taskId = DEFAULT_DEVELOPMENT_TASK_ID) {
  if (typeof taskId !== 'string' || !Object.hasOwn(tasks, taskId)) throw new Error('INVALID_DEVELOPMENT_TASK');
  return tasks[taskId];
}
