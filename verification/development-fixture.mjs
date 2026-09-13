import { mkdtempSync, mkdirSync, copyFileSync, writeFileSync, readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { createHash } from 'node:crypto';
import { developmentTask, DEFAULT_DEVELOPMENT_TASK_ID } from './development-tasks.mjs';

export const BASELINE_SOURCE = developmentTask().baseline;
export const DEVELOPMENT_TASK = developmentTask().task;
export const sourceHash = source => createHash('sha256').update(source).digest('hex');
export function createDevelopmentFixture({ waitMode = 'wait', holdAfterPass = false, taskId = DEFAULT_DEVELOPMENT_TASK_ID } = {}) {
  if (!['wait', 'check'].includes(waitMode) || typeof holdAfterPass !== 'boolean') throw new Error('INVALID_DEVELOPMENT_MODE');
  const task = developmentTask(taskId);
  const project = dirname(dirname(fileURLToPath(import.meta.url)));
  const root = mkdtempSync(join(project, '.tmp', 'native-development-'));
  for (const name of ['work', 'control', 'config', 'temp']) mkdirSync(join(root, name));
  const work = join(root, 'work'), control = join(root, 'control');
  writeFileSync(join(work, task.sourceFile), task.baseline, { flag: 'wx' });
  writeFileSync(join(control, 'TASK.md'), task.task, { flag: 'wx' });
  copyFileSync(join(project, 'verification', 'fixtures', task.oracleFile), join(control, 'oracle.mjs'));
  writeFileSync(join(control, 'review.json'), JSON.stringify({ sha256: sourceHash(task.baseline), approved: true }), { flag: 'wx' });
  const script = join(project, 'verification', 'fixtures', 'development-mcp.mjs');
  const policy = join(project, 'verification', 'development-source-policy.mjs');
  const args = ['--permission', '--allow-child-process', `--allow-fs-read=${script}`, `--allow-fs-read=${policy}`,
    `--allow-fs-read=${join(project, 'verification', 'development-tasks.mjs')}`, `--allow-fs-read=${work}`, `--allow-fs-read=${control}`,
    `--allow-fs-write=${work}`, script, root, waitMode, taskId, ...(holdAfterPass ? ['hold-after-pass'] : [])];
  writeFileSync(join(work, '.mcp.json'), JSON.stringify({ mcpServers: { fixture: { type: 'stdio', command: process.execPath, args,
    env: { ANTHROPIC_AUTH_TOKEN: '', ANTHROPIC_BASE_URL: '', ANTHROPIC_API_KEY: '', CLAUDE_CODE_OAUTH_TOKEN: '' } } } }), { flag: 'wx' });
  return { project, root, work, control, script, args, taskId, task,
    oracleHash: sourceHash(readFileSync(join(control, 'oracle.mjs'))), taskHash: sourceHash(task.task) };
}
