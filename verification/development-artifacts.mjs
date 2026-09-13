import { readFileSync, lstatSync, realpathSync } from 'node:fs';
import { join, resolve, dirname, basename } from 'node:path';
import { fileURLToPath } from 'node:url';
import { createHash } from 'node:crypto';
import { developmentTask } from './development-tasks.mjs';
import { checkDevelopmentSource } from './development-source-policy.mjs';
import { readDevelopmentSource, readDevelopmentOracle, verifyDevelopmentSourceWrites, readDevelopmentSourceWriteEvidence } from './development-source-files.mjs';

const project = dirname(dirname(fileURLToPath(import.meta.url)));
const fail = () => { throw new Error('DEVELOPMENT_ARTIFACT_CHANGED'); };
const need = value => { if (!value) fail(); };
const hash = bytes => createHash('sha256').update(bytes).digest('hex');
const keys = ['source', 'task', 'oracle', 'review', 'mcp', 'events'];
export function developmentEventArtifactHash(work, taskId, events) {
  return hash(developmentTask(taskId).parts ? JSON.stringify({ events,
    writes: readDevelopmentSourceWriteEvidence(work, taskId) }) + '\n' : events);
}

// These are the task's bounded local artifacts. Capturing their bytes does not
// execute code, approve a new source or establish that tests have passed.
export function readDevelopmentArtifactHashes(root, taskId) {
  try {
    need(typeof root === 'string'); root = resolve(root);
    need(dirname(root).toLowerCase() === join(project, '.tmp').toLowerCase()
      && /^native-development-[A-Za-z0-9]{6}$/.test(basename(root)));
    developmentTask(taskId);
    const bytes = { source: Buffer.from(readDevelopmentSource(join(root, 'work'), taskId)),
      oracle: Buffer.from(readDevelopmentOracle(join(root, 'control'), taskId)) };
    const paths = [['task', 'control', 'TASK.md', 65536], ['review', 'control', 'review.json', 1024],
      ['mcp', 'work', '.mcp.json', 16384], ['events', 'work', 'events.jsonl', 32768]];
    for (const [key, directory, name, limit] of paths) {
      const path = join(root, directory, name), before = lstatSync(path, { bigint: true });
      need(before.isFile() && !before.isSymbolicLink() && before.nlink === 1n && before.size <= BigInt(limit)
        && realpathSync(path).toLowerCase() === path.toLowerCase());
      bytes[key] = readFileSync(path);
      const after = lstatSync(path, { bigint: true });
      need(BigInt(bytes[key].length) === before.size && before.dev === after.dev && before.ino === after.ino
        && before.size === after.size && before.mtimeNs === after.mtimeNs);
    }
    checkDevelopmentSource(new TextDecoder('utf-8', { fatal: true }).decode(bytes.source), taskId);
    if (developmentTask(taskId).parts) {
      verifyDevelopmentSourceWrites(bytes.source.toString('utf8'),
        new TextDecoder('utf-8', { fatal: true }).decode(bytes.events).trim().split('\n').map(JSON.parse), taskId, join(root, 'work'));
    }
    const review = JSON.parse(new TextDecoder('utf-8', { fatal: true }).decode(bytes.review));
    need(review && Object.keys(review).length === 2 && review.approved === true && review.sha256 === hash(bytes.source));
    return Object.fromEntries(keys.map(key => [key, key === 'events'
      ? developmentEventArtifactHash(join(root, 'work'), taskId, bytes.events.toString('utf8')) : hash(bytes[key])]));
  } catch { fail(); }
}

export function verifyDevelopmentArtifacts(root, result, budget) {
  const expected = result?.artifactHashes;
  need(expected && typeof expected === 'object' && !Array.isArray(expected) && Object.keys(expected).length === keys.length
    && keys.every(key => Object.hasOwn(expected, key) && typeof expected[key] === 'string' && /^[a-f0-9]{64}$/.test(expected[key])));
  const current = readDevelopmentArtifactHashes(root, result.taskId);
  need(keys.every(key => current[key] === expected[key]) && current.source === result.sourceSha256
    && current.task === budget.taskHash && current.oracle === budget.oracleHash && current.mcp === budget.mcpHash);
  return current;
}
