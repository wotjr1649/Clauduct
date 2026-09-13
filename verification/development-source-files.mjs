import { lstatSync, realpathSync, openSync, fstatSync, readSync, closeSync, writeFileSync, readFileSync, existsSync } from 'node:fs';
import { join, resolve, dirname } from 'node:path';
import { createHash } from 'node:crypto';
import { fileURLToPath } from 'node:url';
import { spawnSync } from 'node:child_process';
import { developmentTask, developmentTaskFiles, developmentOracleSource } from './development-tasks.mjs';
import { developmentSourceArguments } from './development-source-policy.mjs';

const hash = text => createHash('sha256').update(text).digest('hex');
const need = value => { if (!value) throw new Error('DEVELOPMENT_PATH'); };
const requireBatch = value => { if (!value) throw new Error('DEVELOPMENT_SOURCE_BATCH_INVALID'); };
const encode = value => JSON.stringify(value) + '\n';
const shape = (value, names) => value && typeof value === 'object' && !Array.isArray(value)
  && Object.keys(value).sort().join(',') === [...names].sort().join(',');
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
  const before = readDevelopmentSource(work, taskId); // Validate every existing target before the first write.
  if (!task.parts) { writeFileSync(join(work, task.sourceFile), input.code); return; }
  const batches = readDevelopmentSourceBatches(work, taskId), previous = batches.at(-1);
  if (previous && !previous.done) throw new Error('DEVELOPMENT_SOURCE_WRITE_PENDING');
  requireBatch(before === (previous?.intent.source ?? task.baseline));
  if (previous?.intent.source === source) return { source, writes: 0, confirmed: 0 };
  requireBatch(batches.length < 2 && before !== source);
  const intermediate = JSON.parse(before).files;
  for (let index = 0; index < input.files.length; index++) {
    intermediate[index] = input.files[index];
    requireBatch(Buffer.byteLength(encode({ files: intermediate })) <= 8192);
  }
  const files = task.parts.map((part, index) => {
    const path = join(work, part.path), info = lstatSync(path, { bigint: true }), value = read(path, 8192);
    requireBatch(value === JSON.parse(before).files[index].code);
    const after = lstatSync(path, { bigint: true });
    requireBatch(info.ino === after.ino && info.dev === after.dev && info.mtimeNs === after.mtimeNs);
    return { path: part.path, beforeHash: hash(value), dev: String(info.dev), ino: String(info.ino), mtimeNs: String(info.mtimeNs) };
  });
  const index = batches.length + 1;
  const intent = { version: 1, index, taskId, workHash: hash(resolve(work).toLowerCase()), ownerPid: process.pid,
    beforeHash: hash(before), source, files };
  const text = encode(intent); requireBatch(Buffer.byteLength(text) <= 32768);
  writeFileSync(batchPath(work, index), text, { flag: 'wx' });
  const batch = { intent, intentHash: hash(text), done: false, recovery: null };
  record({ event: 'SOURCE_WRITE_STARTED', sha256: hash(source), fileCount: task.parts.length, intentHash: batch.intentHash });
  return applyBatch(work, taskId, batch, record);
}
const batchPath = (work, index, suffix = '') => join(work, `source-write-${index}${suffix}.json`);
export function readDevelopmentSourceBatches(work, taskId) {
  const task = developmentTask(taskId); requireBatch(task.parts !== undefined);
  const result = []; let before = task.baseline;
  for (let index = 1; index <= 2; index++) {
    const path = batchPath(work, index);
    if (!existsSync(path)) {
      requireBatch(!existsSync(batchPath(work, index, '-done')) && !existsSync(batchPath(work, index, '-recovery')));
      if (index === 1) requireBatch(!existsSync(batchPath(work, 2)));
      continue;
    }
    requireBatch(index === 1 || result.at(-1)?.done === true);
    const text = read(path, 32768), intent = JSON.parse(text), intentHash = hash(text);
    requireBatch(text === encode(intent) && shape(intent, ['version', 'index', 'taskId', 'workHash', 'ownerPid', 'beforeHash', 'source', 'files'])
      && intent.version === 1 && intent.index === index && intent.taskId === taskId && intent.workHash === hash(resolve(work).toLowerCase())
      && Number.isSafeInteger(intent.ownerPid) && intent.ownerPid > 0 && intent.beforeHash === hash(before)
      && Array.isArray(intent.files) && intent.files.length === task.parts.length);
    developmentSourceArguments(intent.source, taskId);
    requireBatch(intent.source !== before);
    for (let part = 0; part < task.parts.length; part++) {
      const file = intent.files[part];
      requireBatch(shape(file, ['path', 'beforeHash', 'dev', 'ino', 'mtimeNs']) && file.path === task.parts[part].path
        && file.beforeHash === hash(JSON.parse(before).files[part].code)
        && ['dev', 'ino', 'mtimeNs'].every(key => typeof file[key] === 'string' && /^[0-9]{1,30}$/.test(file[key])));
    }
    let done = false, recovery = null;
    if (existsSync(batchPath(work, index, '-recovery'))) {
      recovery = JSON.parse(read(batchPath(work, index, '-recovery'), 1024));
      requireBatch(shape(recovery, ['intentHash', 'ownerPid']) && recovery.intentHash === intentHash
        && Number.isSafeInteger(recovery.ownerPid) && recovery.ownerPid > 0 && recovery.ownerPid !== intent.ownerPid);
    }
    if (existsSync(batchPath(work, index, '-done'))) {
      const value = JSON.parse(read(batchPath(work, index, '-done'), 1024));
      requireBatch(shape(value, ['intentHash', 'sourceHash']) && value.intentHash === intentHash && value.sourceHash === hash(intent.source));
      done = true;
    }
    result.push({ intent, intentHash, done, recovery }); before = intent.source;
  }
  return result;
}
export function readDevelopmentSourceWriteEvidence(work, taskId) {
  return encode(readDevelopmentSourceBatches(work, taskId).map(batch => ({ index: batch.intent.index,
    intent: read(batchPath(work, batch.intent.index), 32768),
    done: batch.done ? read(batchPath(work, batch.intent.index, '-done'), 1024) : null,
    recovery: batch.recovery ? read(batchPath(work, batch.intent.index, '-recovery'), 1024) : null })));
}
function sourceStates(work, batch) {
  const proposed = JSON.parse(batch.intent.source).files;
  return batch.intent.files.map((file, index) => {
    const path = join(work, file.path), info = lstatSync(path, { bigint: true }), value = read(path, 8192);
    requireBatch(String(info.dev) === file.dev && String(info.ino) === file.ino);
    if (value === proposed[index].code) return 'applied';
    if (hash(value) !== file.beforeHash || String(info.mtimeNs) !== file.mtimeNs) throw new Error('DEVELOPMENT_SOURCE_EFFECT_UNKNOWN');
    return 'pending';
  });
}
export function readDevelopmentSourceState(work, taskId) {
  const batches = readDevelopmentSourceBatches(work, taskId), batch = batches.at(-1);
  requireBatch(batch !== undefined);
  return { batch, batchCount: batches.length, states: sourceStates(work, batch), source: readDevelopmentSource(work, taskId) };
}
function applyBatch(work, taskId, batch, record) {
  const files = JSON.parse(batch.intent.source).files, states = sourceStates(work, batch);
  let writes = 0, confirmed = 0;
  for (let index = 0; index < files.length; index++) {
    requireBatch(sourceStates(work, batch)[index] === states[index]);
    const file = files[index];
    if (states[index] === 'pending') {
      writeFileSync(join(work, file.path), file.code); writes++;
      record({ event: 'SOURCE_FILE_WRITTEN', path: file.path, sha256: hash(file.code) });
    } else { confirmed++; record({ event: 'SOURCE_FILE_CONFIRMED', path: file.path, sha256: hash(file.code) }); }
  }
  need(readDevelopmentSource(work, taskId) === batch.intent.source);
  record({ event: 'SOURCE_WRITTEN', sha256: hash(batch.intent.source) });
  writeFileSync(batchPath(work, batch.intent.index, '-done'), encode({ intentHash: batch.intentHash, sourceHash: hash(batch.intent.source) }), { flag: 'wx' });
  return { source: batch.intent.source, writes, confirmed };
}
function recoverDevelopmentSource(work, taskId, record, terminatedOwnerPid, expectedSource, expectedIntentHash) {
  const batch = readDevelopmentSourceBatches(work, taskId).at(-1); requireBatch(batch !== undefined);
  requireBatch(batch.intent.source === expectedSource && batch.intentHash === expectedIntentHash);
  if (batch.done) {
    requireBatch(readDevelopmentSource(work, taskId) === batch.intent.source);
    return { source: batch.intent.source, writes: 0, confirmed: 0 };
  }
  // Entrypoints establish termination and bind the exact previously inspected
  // intent. The MCP path uses its child handle; managed recovery also requires
  // the native tree's protected interruption proof before reaching this helper.
  requireBatch(batch.intent.ownerPid === terminatedOwnerPid && batch.intent.ownerPid !== process.pid);
  if (batch.recovery) throw new Error('DEVELOPMENT_SOURCE_RECOVERY_UNCERTAIN');
  sourceStates(work, batch); // Reject every unknown target before claiming or writing.
  writeFileSync(batchPath(work, batch.intent.index, '-recovery'), encode({ intentHash: batch.intentHash, ownerPid: process.pid }), { flag: 'wx' });
  record({ event: 'SOURCE_RECOVERY_STARTED', intentHash: batch.intentHash });
  return applyBatch(work, taskId, batch, record);
}
export function recoverStoppedDevelopmentSource(work, taskId, record, expectedIntentHash) {
  const { batch } = readDevelopmentSourceState(work, taskId);
  requireBatch(batch.intentHash === expectedIntentHash);
  if (!batch.done) {
    try { process.kill(batch.intent.ownerPid, 0); throw new Error('SOURCE_OWNER_ACTIVE'); }
    catch (error) { if (error.code !== 'ESRCH') throw new Error('DEVELOPMENT_SOURCE_OWNER_UNVERIFIED'); }
  }
  return recoverDevelopmentSource(work, taskId, record, batch.intent.ownerPid, batch.intent.source, expectedIntentHash);
}
export function writeDevelopmentSourceWithRecovery(work, taskId, source, record) {
  requireBatch(taskId === 'retry-project'); developmentSourceArguments(source, taskId);
  const previous = readDevelopmentSourceBatches(work, taskId).at(-1);
  if (previous && !previous.done) throw new Error('DEVELOPMENT_SOURCE_WRITE_PENDING');
  if (previous?.intent.source === source) return writeDevelopmentSource(work, taskId, source, record);
  const verification = dirname(fileURLToPath(import.meta.url)), fixtures = join(verification, 'fixtures');
  const worker = join(fixtures, 'development-write-worker.mjs');
  const reads = [worker, work, ...['development-source-files.mjs', 'development-source-policy.mjs', 'development-source-grammar.mjs',
    'development-tasks.mjs', 'registered-development-tasks.mjs'].map(name => join(verification, name)),
    ...['development-oracle.mjs', 'development-window-oracle.mjs', 'development-project-oracle.mjs'].map(name => join(fixtures, name))];
  const env = Object.fromEntries(['SystemRoot', 'WINDIR'].filter(key => process.env[key]).map(key => [key, process.env[key]]));
  const child = spawnSync(process.execPath, ['--permission', ...reads.map(path => `--allow-fs-read=${path}`), `--allow-fs-write=${work}`,
    worker, work, taskId], { cwd: work, env, input: source, windowsHide: true, encoding: 'utf8', timeout: 5000, maxBuffer: 4096 });
  if (child.error || child.signal || child.status !== 71 || child.stdout !== '' || child.stderr !== '') throw new Error('SOURCE_WORKER_UNVERIFIED');
  const interrupted = readDevelopmentSourceBatches(work, taskId).at(-1);
  requireBatch(interrupted?.intent.source === source);
  record({ event: 'SOURCE_WORKER_INTERRUPTED', exitCode: 71 });
  const recovered = recoverDevelopmentSource(work, taskId, record, child.pid, source, interrupted.intentHash);
  requireBatch(recovered.source === source && recovered.writes === 1 && recovered.confirmed === 1);
  return recovered;
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
export function verifyDevelopmentSourceWrites(source, events, taskId, work) {
  const task = developmentTask(taskId); if (!task.parts) return;
  const reject = () => { throw new Error('DEVELOPMENT_SOURCE_WRITES_INCOMPLETE'); };
  let batches;
  try { batches = readDevelopmentSourceBatches(work, taskId); } catch { reject(); }
  if (batches.length < 1 || batches.some(batch => !batch.done) || batches.at(-1).intent.source !== source) reject();
  const rows = events.filter(row => ['SOURCE_WRITE_STARTED', 'SOURCE_FILE_WRITTEN', 'SOURCE_FILE_CONFIRMED', 'SOURCE_RECOVERY_STARTED', 'SOURCE_WRITTEN'].includes(row.event));
  let cursor = 0;
  for (const batch of batches) {
    const first = rows[cursor++], values = JSON.parse(batch.intent.source).files, written = new Set();
    if (!first || first.event !== 'SOURCE_WRITE_STARTED' || first.fileCount !== task.parts.length || Object.keys(first).length !== 4
      || first.sha256 !== hash(batch.intent.source) || first.intentHash !== batch.intentHash) reject();
    let recovered = false, index = 0;
    while (rows[cursor]?.event !== 'SOURCE_WRITTEN') {
      const row = rows[cursor++]; if (!row) reject();
      if (row.event === 'SOURCE_RECOVERY_STARTED') {
        if (!batch.recovery || recovered || Object.keys(row).length !== 2 || row.intentHash !== batch.intentHash) reject();
        recovered = true; index = 0; continue;
      }
      if (!['SOURCE_FILE_WRITTEN', 'SOURCE_FILE_CONFIRMED'].includes(row.event) || Object.keys(row).length !== 3
        || index >= values.length || row.path !== values[index].path || row.sha256 !== hash(values[index].code)) reject();
      if (row.event === 'SOURCE_FILE_WRITTEN') { if (written.has(row.path)) reject(); written.add(row.path); }
      else if (!recovered && batch.intent.files[index].beforeHash !== row.sha256) reject();
      index++;
    }
    const last = rows[cursor++];
    if (index !== values.length || recovered !== Boolean(batch.recovery) || Object.keys(last).length !== 2 || last.sha256 !== first.sha256) reject();
  }
  if (cursor !== rows.length) reject();
}
