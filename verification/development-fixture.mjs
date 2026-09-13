import { mkdtempSync, mkdirSync, writeFileSync, readFileSync, existsSync, lstatSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { createHash } from 'node:crypto';
import { developmentTask, developmentTaskFiles, developmentOracleSource, DEFAULT_DEVELOPMENT_TASK_ID } from './development-tasks.mjs';
import { registeredDevelopmentTaskPath } from './registered-development-tasks.mjs';
import { readDevelopmentOracle } from './development-source-files.mjs';

export const BASELINE_SOURCE = developmentTask().baseline;
export const DEVELOPMENT_TASK = developmentTask().task;
export const sourceHash = source => createHash('sha256').update(source).digest('hex');
export function createDevelopmentFixture({ waitMode = 'wait', holdAfterPass = false, holdAfterSourceWrite = false, holdAfterTaskRead = false,
  holdAfterFirstSourceWrite = false, recoverAfterFirstSourceWrite = false, taskId = DEFAULT_DEVELOPMENT_TASK_ID } = {}) {
  if (!['wait', 'check'].includes(waitMode) || typeof holdAfterPass !== 'boolean' || typeof holdAfterSourceWrite !== 'boolean'
    || typeof holdAfterTaskRead !== 'boolean' || typeof recoverAfterFirstSourceWrite !== 'boolean' || typeof holdAfterFirstSourceWrite !== 'boolean'
    || [holdAfterPass, holdAfterSourceWrite, holdAfterTaskRead, recoverAfterFirstSourceWrite, holdAfterFirstSourceWrite].filter(Boolean).length > 1
    || (recoverAfterFirstSourceWrite || holdAfterFirstSourceWrite) && taskId !== 'retry-project') throw new Error('INVALID_DEVELOPMENT_MODE');
  const task = developmentTask(taskId);
  const project = dirname(dirname(fileURLToPath(import.meta.url)));
  const temporaryRoot = join(project, '.tmp');
  for (const path of [project, temporaryRoot]) {
    if (existsSync(path)) {
      const info = lstatSync(path);
      if (!info.isDirectory() || info.isSymbolicLink()) throw new Error('DEVELOPMENT_PATH');
    } else if (path === temporaryRoot) mkdirSync(path);
    else throw new Error('DEVELOPMENT_PATH');
  }
  const root = mkdtempSync(join(temporaryRoot, 'native-development-'));
  for (const name of ['work', 'control', 'config', 'temp']) mkdirSync(join(root, name));
  const work = join(root, 'work'), control = join(root, 'control');
  for (const part of developmentTaskFiles(taskId)) writeFileSync(join(work, part.path), developmentTask(part.taskId).baseline, { flag: 'wx' });
  writeFileSync(join(control, 'TASK.md'), task.task, { flag: 'wx' });
  writeFileSync(join(control, 'oracle.mjs'), developmentOracleSource(taskId), { flag: 'wx' });
  for (const name of task.oracleDependencies ?? []) writeFileSync(join(control, name), readFileSync(join(project, 'verification', 'fixtures', name)), { flag: 'wx' });
  writeFileSync(join(control, 'review.json'), JSON.stringify({ sha256: sourceHash(task.baseline), approved: true }), { flag: 'wx' });
  const script = join(project, 'verification', 'fixtures', 'development-mcp.mjs');
  const policy = join(project, 'verification', 'development-source-policy.mjs');
  const args = ['--permission', '--allow-child-process', `--allow-fs-read=${script}`, `--allow-fs-read=${policy}`,
    `--allow-fs-read=${join(project, 'verification', 'development-source-files.mjs')}`,
    `--allow-fs-read=${join(project, 'verification', 'development-source-grammar.mjs')}`,
    `--allow-fs-read=${join(project, 'verification', 'registered-development-tasks.mjs')}`,
    ...(task.registered ? [`--allow-fs-read=${registeredDevelopmentTaskPath(taskId)}`] : []),
    `--allow-fs-read=${join(project, 'verification', 'development-tasks.mjs')}`, `--allow-fs-read=${work}`, `--allow-fs-read=${control}`,
    `--allow-fs-write=${work}`, script, root, waitMode, taskId,
    ...(holdAfterPass ? ['hold-after-pass'] : holdAfterSourceWrite ? ['hold-after-source'] : holdAfterTaskRead ? ['hold-after-read']
      : recoverAfterFirstSourceWrite ? ['recover-after-first-file'] : holdAfterFirstSourceWrite ? ['hold-after-first-file'] : [])];
  writeFileSync(join(work, '.mcp.json'), JSON.stringify({ mcpServers: { fixture: { type: 'stdio', command: process.execPath, args,
    env: { ANTHROPIC_AUTH_TOKEN: '', ANTHROPIC_BASE_URL: '', ANTHROPIC_API_KEY: '', CLAUDE_CODE_OAUTH_TOKEN: '' } } } }), { flag: 'wx' });
  return { project, root, work, control, script, args, taskId, task,
    oracleHash: sourceHash(readDevelopmentOracle(control, taskId)), taskHash: sourceHash(task.task) };
}
