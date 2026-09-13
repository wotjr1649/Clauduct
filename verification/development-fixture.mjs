import { mkdtempSync, mkdirSync, copyFileSync, writeFileSync, readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { createHash } from 'node:crypto';

export const BASELINE_SOURCE = 'export function parseRetryAfterSeconds(value) {\n  const seconds = Number(value);\n  return Number.isFinite(seconds) && seconds >= 0 ? Math.min(seconds * 1000, 5000) : null;\n}\n';
export const DEVELOPMENT_TASK = `# Retry-After seconds parser

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
`;
export const sourceHash = source => createHash('sha256').update(source).digest('hex');
export function createDevelopmentFixture({ waitMode = 'wait' } = {}) {
  if (!['wait', 'check'].includes(waitMode)) throw new Error('INVALID_DEVELOPMENT_MODE');
  const project = dirname(dirname(fileURLToPath(import.meta.url)));
  const root = mkdtempSync(join(project, '.tmp', 'native-development-'));
  for (const name of ['work', 'control', 'config', 'temp']) mkdirSync(join(root, name));
  const work = join(root, 'work'), control = join(root, 'control');
  writeFileSync(join(work, 'retry-after-seconds.mjs'), BASELINE_SOURCE, { flag: 'wx' });
  writeFileSync(join(control, 'TASK.md'), DEVELOPMENT_TASK, { flag: 'wx' });
  copyFileSync(join(project, 'verification', 'fixtures', 'development-oracle.mjs'), join(control, 'oracle.mjs'));
  writeFileSync(join(control, 'review.json'), JSON.stringify({ sha256: sourceHash(BASELINE_SOURCE), approved: true }), { flag: 'wx' });
  const script = join(project, 'verification', 'fixtures', 'development-mcp.mjs');
  const policy = join(project, 'verification', 'development-source-policy.mjs');
  const args = ['--permission', '--allow-child-process', `--allow-fs-read=${script}`, `--allow-fs-read=${policy}`, `--allow-fs-read=${work}`, `--allow-fs-read=${control}`,
    `--allow-fs-write=${work}`, script, root, waitMode];
  writeFileSync(join(work, '.mcp.json'), JSON.stringify({ mcpServers: { fixture: { type: 'stdio', command: process.execPath, args,
    env: { ANTHROPIC_AUTH_TOKEN: '', ANTHROPIC_BASE_URL: '', ANTHROPIC_API_KEY: '', CLAUDE_CODE_OAUTH_TOKEN: '' } } } }), { flag: 'wx' });
  return { project, root, work, control, script, args,
    oracleHash: sourceHash(readFileSync(join(control, 'oracle.mjs'))), taskHash: sourceHash(DEVELOPMENT_TASK) };
}
