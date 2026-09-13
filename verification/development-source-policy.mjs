import { developmentTask } from './development-tasks.mjs';
import { checkDevelopmentSourceAgainstTask } from './development-source-grammar.mjs';

export function checkDevelopmentSource(source, taskId) {
  let task;
  try { task = developmentTask(taskId); } catch { throw new Error('DEVELOPMENT_SOURCE_REJECTED'); }
  if (!task.parts) return checkDevelopmentSourceAgainstTask(source, task);
  try {
    if (typeof source !== 'string' || Buffer.byteLength(source) > 8192) throw new Error();
    const value = JSON.parse(source);
    if (JSON.stringify(value) + '\n' !== source) throw new Error();
    const normalized = developmentSourceFromArguments(value, taskId);
    if (normalized !== source) throw new Error();
    return normalized;
  } catch { throw new Error('DEVELOPMENT_SOURCE_REJECTED'); }
}

export function developmentSourceFromArguments(value, taskId) {
  try {
    const task = developmentTask(taskId);
    if (!value || typeof value !== 'object' || Array.isArray(value)) throw new Error();
    if (!task.parts) {
      if (Object.keys(value).length !== 1 || !Object.hasOwn(value, 'code')) throw new Error();
      checkDevelopmentSourceAgainstTask(value.code, task); return value.code;
    }
    if (Object.keys(value).length !== 1 || !Array.isArray(value.files) || value.files.length !== task.parts.length) throw new Error();
    for (let index = 0; index < task.parts.length; index++) {
      const file = value.files[index], part = task.parts[index];
      if (!file || Object.keys(file).length !== 2 || file.path !== part.path || !Object.hasOwn(file, 'code')) throw new Error();
      checkDevelopmentSourceAgainstTask(file.code, developmentTask(part.taskId));
    }
    const source = JSON.stringify({ files: task.parts.map((part, index) => ({ path: part.path, code: value.files[index].code })) }) + '\n';
    if (Buffer.byteLength(source) > 8192) throw new Error();
    return source;
  } catch { throw new Error('DEVELOPMENT_SOURCE_REJECTED'); }
}
export function developmentSourceArguments(source, taskId) {
  checkDevelopmentSource(source, taskId);
  return developmentTask(taskId).parts ? JSON.parse(source) : { code: source };
}
