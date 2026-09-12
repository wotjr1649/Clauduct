import { openSync, writeFileSync, closeSync, readFileSync, appendFileSync } from 'node:fs';
import { join } from 'node:path';

const [work, operationId, mode] = process.argv.slice(2);
// Parent death before authorization cannot orphan an effect-producing worker.
const startupTimer = setTimeout(() => process.exit(72), 5000);
const parentGone = () => process.exit(73);
process.once('disconnect', parentGone);
process.once('message', message => {
  clearTimeout(startupTimer);
  if (!['effect', 'finish'].includes(message?.phase)) process.exit(74);
  const path = join(work, 'operation.json');
  if (message.phase === 'effect') {
    if (message.fault === 'before-effect') process.exit(70);
    if (mode === 'opaque') appendFileSync(join(work, 'opaque-effects.jsonl'), '{"effect":1}\n');
    let fd;
    try { fd = openSync(path, 'wx'); }
    catch (error) { if (error.code !== 'EEXIST' || mode === 'opaque') throw error; }
    if (fd !== undefined) {
      // Closed-file process-crash evidence only. Node's permission model denies
      // fsync; do not disable it or claim power-loss durability for this fixture.
      try { writeFileSync(fd, JSON.stringify({ operationId, count: 1 })); }
      finally { closeSync(fd); }
    }
    if (message.fault === 'after-effect') process.exit(70);
  } else if (message.fault !== 'fake-success') {
    const receipt = JSON.parse(readFileSync(path, 'utf8'));
    if (receipt.operationId !== operationId || receipt.count !== 1) process.exit(75);
    const fd = openSync(join(work, 'report.json'), 'w');
    try { writeFileSync(fd, JSON.stringify({ operationId, operationCount: 1, result: 'effect-reconciled' })); }
    finally { closeSync(fd); }
  }
  process.send({ type: 'result', completed: true }, () => {
    process.removeListener('disconnect', parentGone);
    process.disconnect();
  });
});
