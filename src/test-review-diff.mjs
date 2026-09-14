import assert from 'node:assert/strict';
import { mkdtemp, writeFile, readFile, readdir, unlink, rmdir } from 'node:fs/promises';
import { spawnSync } from 'node:child_process';
import { join, resolve, relative } from 'node:path';
import { fileURLToPath } from 'node:url';
import { reviewDiff } from './review-diff.mjs';

const parent = fileURLToPath(new URL('./', import.meta.url));
const env = Object.fromEntries(Object.entries(process.env).filter(([key]) => /^(PATH|SystemRoot|WINDIR|TEMP|TMP)$/i.test(key)));
Object.assign(env, { GIT_CONFIG_NOSYSTEM: '1', GIT_CONFIG_GLOBAL: process.platform === 'win32' ? 'NUL' : '/dev/null', GIT_TERMINAL_PROMPT: '0' });
// The project runner clears the child environment down to SystemRoot/WINDIR/TEMP/TMP, so git
// cannot be resolved there at all and this file could never pass under it. Report that as unrun
// rather than red: a failure everyone expects is a failure nobody reads, which is how this came
// to be written down as a shell that cannot spawn instead of a PATH that is not there. Only a
// git that will not start skips — a git that runs and then fails is still a failure.
const gitProbe = spawnSync('git', ['--version'], { cwd: parent, env, encoding: 'utf8', timeout: 10000, windowsHide: true });
if (gitProbe.error) {
  console.log(JSON.stringify({ suite: 'review-diff', notRun: ['git: no PATH in this environment'],
    probe: gitProbe.error.code ?? 'SPAWN_FAILED', actualClaude: 0, externalRequests: 0 }));
  process.exit(0);
}
const root = await mkdtemp(join(parent, 'review-diff-fixture-'));
const git = args => {
  const r = spawnSync('git', args, { cwd: root, env, encoding: 'utf8', timeout: 10000, maxBuffer: 1024 * 1024, windowsHide: true });
  assert.equal(r.status, 0, 'synthetic git command failed'); return r.stdout;
};
try {
  git(['init', '-q', '--template=']);
  const target = join(root, 'sample.mjs');
  await writeFile(target, 'const before = 1;\n');
  const initial = await reviewDiff(target, root);
  assert.equal(initial.kind, 'untracked-added');
  assert.match(initial.diff, /@@ -0,0 \+1,1 @@\n\+const before = 1;/);
  assert.equal(git(['status', '--porcelain']), '?? sample.mjs\n');
  git(['add', '--', 'sample.mjs']);
  git(['-c', 'user.name=Synthetic', '-c', 'user.email=synthetic@example.invalid', 'commit', '-qm', 'fixture']);
  const head = git(['rev-parse', 'HEAD']), index = await readFile(join(root, '.git', 'index'));
  await writeFile(target, 'const after = 2;\n');
  const changed = await reviewDiff(target, root);
  assert.equal(changed.kind, 'tracked-working-tree');
  assert.match(changed.diff, /-const before = 1;/); assert.match(changed.diff, /\+const after = 2;/);
  assert.equal(git(['rev-parse', 'HEAD']), head);
  assert.deepEqual(await readFile(join(root, '.git', 'index')), index);
  await assert.rejects(reviewDiff(join(parent, 'review-diff.mjs'), root));
  await writeFile(join(root, 'binary.mjs'), Buffer.from([0, 1, 2]));
  await assert.rejects(reviewDiff(join(root, 'binary.mjs'), root));
  await writeFile(join(root, "space '$().mjs"), 'x');
  const quoted = await reviewDiff(join(root, "space '$().mjs"), root);
  assert.match(quoted.diff, /No newline at end of file/);
} finally {
  // Remove only this test's newly allocated subtree, without recursive deletion.
  const remove = async dir => {
    const rel = relative(root, resolve(dir));
    assert.ok(rel === '' || (!rel.startsWith('..') && !rel.includes(':')));
    for (const entry of await readdir(dir, { withFileTypes: true })) {
      const path = join(dir, entry.name);
      if (entry.isDirectory()) await remove(path); else await unlink(path);
    }
    await rmdir(dir);
  };
  await remove(root);
}
console.log(JSON.stringify({ suite: 'review-diff', passed: true, actualClaude: 0, externalRequests: 0 }));
