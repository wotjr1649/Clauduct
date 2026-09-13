import { developmentTask } from './development-tasks.mjs';
import { checkDevelopmentSourceAgainstTask } from './development-source-grammar.mjs';

export function checkDevelopmentSource(source, taskId) {
  let task;
  try { task = developmentTask(taskId); } catch { throw new Error('DEVELOPMENT_SOURCE_REJECTED'); }
  return checkDevelopmentSourceAgainstTask(source, task);
}
