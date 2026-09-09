// Runs through native Bash permissions, never inside the gateway.
import { spawnSync } from 'node:child_process';
import { readFile, realpath, stat } from 'node:fs/promises';
import { resolve, relative, isAbsolute } from 'node:path';
import { fileURLToPath } from 'node:url';

const limit = 2 * 1024 * 1024;
const fail = () => { throw new Error('REVIEW_DIFF_UNAVAILABLE'); };
export async function reviewDiff(target, cwd = process.cwd()) {
  const root = await realpath(cwd), path = await realpath(resolve(cwd, target));
  const rel = relative(root, path);
  if (!rel || isAbsolute(rel) || rel === '..' || rel.startsWith('../') || rel.startsWith('..\\')) fail();
  const info = await stat(path);
  if (!info.isFile() || info.size > limit) fail();
  const env = Object.fromEntries(Object.entries(process.env).filter(([key]) => /^(PATH|SystemRoot|WINDIR|TEMP|TMP)$/i.test(key)));
  Object.assign(env, { GIT_CONFIG_NOSYSTEM: '1', GIT_CONFIG_GLOBAL: process.platform === 'win32' ? 'NUL' : '/dev/null',
    GIT_OPTIONAL_LOCKS: '0', GIT_TERMINAL_PROMPT: '0', GIT_LITERAL_PATHSPECS: '1' });
  const git = args => {
    const result = spawnSync('git', ['--no-pager', '-c', 'core.fsmonitor=false', ...args],
      { cwd: root, env, encoding: 'utf8', timeout: 10000, maxBuffer: limit, windowsHide: true, shell: false });
    if (result.error || result.signal || result.status === null) fail();
    return result;
  };
  const repo = git(['rev-parse', '--show-toplevel']);
  if (repo.status !== 0) fail();
  // Configured clean/process filters could execute code while computing a diff.
  const filters = git(['config', '--name-only', '--get-regexp', '^filter\\.']);
  if (filters.status !== 1) fail();
  const tracked = git(['ls-files', '--error-unmatch', '--', rel]);
  let kind, diff;
  if (tracked.status === 0) {
    const head = git(['rev-parse', '--verify', 'HEAD']);
    if (head.status !== 0) fail();
    const result = git(['diff', '--no-ext-diff', '--no-textconv', '--no-color', 'HEAD', '--', rel]);
    if (result.status !== 0) fail();
    kind = 'tracked-working-tree'; diff = result.stdout;
  } else if (tracked.status === 1) {
    const raw = await readFile(path);
    if (raw.length > limit || raw.includes(0)) fail();
    const text = new TextDecoder('utf-8', { fatal: true }).decode(raw);
    const lines = text.split('\n'); if (lines.at(-1) === '') lines.pop();
    const name = rel.replaceAll('\\', '/');
    diff = `--- /dev/null\n+++ ${JSON.stringify('b/' + name)}\n`;
    if (lines.length) diff += `@@ -0,0 +1,${lines.length} @@\n` + lines.map(line => '+' + line).join('\n') + '\n';
    if (text && !text.endsWith('\n')) diff += '\\ No newline at end of file\n';
    kind = 'untracked-added';
  } else fail();
  if (Buffer.byteLength(diff) > limit) fail();
  return { diagnosticVersion: 1, kind, target: path, diff };
}
if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  try {
    if (process.argv.length !== 3) fail();
    process.stdout.write(JSON.stringify(await reviewDiff(process.argv[2])) + '\n');
  } catch { process.stderr.write('REVIEW_DIFF_UNAVAILABLE: no complete diff; review is incomplete.\n'); process.exitCode = 1; }
}
