import { appendFileSync } from 'node:fs';
const [name, cancelled] = process.argv.slice(2);
if (!['alpha', 'beta'].includes(name) || !['alpha', 'beta'].includes(cancelled) || process.argv.length !== 4) {
  throw new Error('PUBLIC_WORKER_ARGUMENTS');
}
const file = `${name}-events.jsonl`, durationMs = name === cancelled ? 15000 : 5000;
appendFileSync(file, JSON.stringify({ event: 'started', pid: process.pid, at: Date.now(), durationMs }) + '\n', { flag: 'wx' });
setTimeout(() => {
  appendFileSync(file, JSON.stringify({ event: 'finished', at: Date.now() }) + '\n');
  process.stdout.write(`PUBLIC_${name.toUpperCase()}_COMPLETE`);
}, durationMs);
