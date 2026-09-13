import { lstatSync, realpathSync, openSync, fstatSync, readSync, closeSync, writeFileSync, readFileSync } from 'node:fs';
import { join, resolve } from 'node:path';
import { createHash } from 'node:crypto';
import { developmentTask, developmentTaskFiles, developmentOracleSource } from './development-tasks.mjs';
import { developmentSourceArguments } from './development-source-policy.mjs';

const hash = text => createHash('sha256').update(text).digest('hex');
const need = value => { if (!value) throw new Error('DEVELOPMENT_PATH'); };
function read(path, limit) {
  const before = lstatSync(path, { bigint: true });
  need(before.isFile() && !before.isSymbolicLink() && before.nlink === 1n && before.size <= BigInt(limit)
    && realpathSync(path).toLowerCase() === resolve(path).toLowerCase());
  const fd = openSync(path, 'r');
  try {
    const opened = fstatSync(fd, { bigint: true }); need(opened.dev === before.dev && opened.ino === before.ino && opened.size === before.size);
    const value = Buffer.alloc(limit + 1); let length = 0;
    while (length < value.length) { const count = readSync(fd, value, length, value.length - length, length); if (!count) break; length += count; }
    const after = fstatSync(fd, { bigint: true });
    need(BigInt(length) === before.size && after.size === before.size && after.mtimeNs === before.mtimeNs && after.nlink === 1n);
    return new TextDecoder('utf-8', { fatal: true }).decode(value.subarray(0, length));
  } finally { closeSync(fd); }
}
export function developmentSourcePaths(work, taskId) {
  return developmentTaskFiles(taskId).map(part => join(work, part.path));
}
export function readDevelopmentSource(work, taskId) {
  const task = developmentTask(taskId);
  if (!task.parts) return read(join(work, task.sourceFile), 8192);
  const source = JSON.stringify({ files: task.parts.map(part => ({ path: part.path, code: read(join(work, part.path), 8192) })) }) + '\n';
  need(Buffer.byteLength(source) <= 8192); return source;
}
export function writeDevelopmentSource(work, taskId, source, record) {
  const task = developmentTask(taskId), input = developmentSourceArguments(source, taskId);
  readDevelopmentSource(work, taskId); // Validate every existing target before the first write.
  if (!task.parts) { writeFileSync(join(work, task.sourceFile), input.code); return; }
  record({ event: 'SOURCE_WRITE_STARTED', sha256: hash(source), fileCount: task.parts.length });
  for (const file of input.files) {
    writeFileSync(join(work, file.path), file.code);
    record({ event: 'SOURCE_FILE_WRITTEN', path: file.path, sha256: hash(file.code) });
  }
  need(readDevelopmentSource(work, taskId) === source);
}
export function developmentOracleInvocation(control, work, taskId) {
  const task = developmentTask(taskId), oracle = join(control, 'oracle.mjs'), sources = developmentSourcePaths(work, taskId);
  return { oracle, argument: task.parts ? work : sources[0], readPaths: [oracle,
    ...(task.oracleDependencies ?? []).map(name => join(control, name)), ...sources] };
}
export function readDevelopmentOracle(control, taskId) {
  const task = developmentTask(taskId), source = read(join(control, 'oracle.mjs'), 65536);
  return task.oracleDependencies ? JSON.stringify({ files: [{ path: 'oracle.mjs', code: source },
    ...task.oracleDependencies.map(path => ({ path, code: read(join(control, path), 65536) }))] }) + '\n' : source;
}
export function expectedDevelopmentOracle(project, taskId) {
  const task = developmentTask(taskId), source = developmentOracleSource(taskId);
  return task.oracleDependencies ? JSON.stringify({ files: [{ path: 'oracle.mjs', code: source },
    ...task.oracleDependencies.map(path => ({ path, code: readFileSync(join(project, 'verification', 'fixtures', path), 'utf8') }))] }) + '\n' : source;
}
export function verifyDevelopmentSourceWrites(source, events, taskId) {
  const task = developmentTask(taskId); if (!task.parts) return;
  const reject = () => { throw new Error('DEVELOPMENT_SOURCE_WRITES_INCOMPLETE'); };
  const rows = events.filter(row => ['SOURCE_WRITE_STARTED', 'SOURCE_FILE_WRITTEN', 'SOURCE_WRITTEN'].includes(row.event));
  const size = task.parts.length + 2, values = JSON.parse(source).files;
  if (rows.length < size || rows.length > size * 2 || rows.length % size) reject();
  for (let start = 0; start < rows.length; start += size) {
    const first = rows[start], last = rows[start + size - 1];
    if (first.event !== 'SOURCE_WRITE_STARTED' || first.fileCount !== task.parts.length || Object.keys(first).length !== 3
      || last.event !== 'SOURCE_WRITTEN' || Object.keys(last).length !== 2 || first.sha256 !== last.sha256
      || typeof first.sha256 !== 'string' || !/^[a-f0-9]{64}$/.test(first.sha256)) reject();
    for (let index = 0; index < task.parts.length; index++) {
      const row = rows[start + index + 1];
      if (row.event !== 'SOURCE_FILE_WRITTEN' || Object.keys(row).length !== 3 || row.path !== task.parts[index].path
        || typeof row.sha256 !== 'string' || !/^[a-f0-9]{64}$/.test(row.sha256)
        || start + size === rows.length && row.sha256 !== hash(values[index].code)) reject();
    }
  }
  if (rows.at(-1).sha256 !== hash(source)) reject();
}
