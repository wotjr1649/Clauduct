import { appendFileSync, lstatSync, existsSync } from 'node:fs';
import { join } from 'node:path';
import { writeDevelopmentSource } from '../development-source-files.mjs';
const [work, taskId] = process.argv.slice(2);
if (process.argv.length !== 4 || taskId !== 'retry-project') throw new Error('PUBLIC_SOURCE_WORKER_ARGUMENTS');
const chunks = []; let bytes = 0;
for await (const chunk of process.stdin) {
  if ((bytes += chunk.length) > 8192) throw new Error('PUBLIC_SOURCE_WORKER_INPUT');
  chunks.push(chunk);
}
const source = new TextDecoder('utf-8', { fatal: true }).decode(Buffer.concat(chunks));
writeDevelopmentSource(work, taskId, source, row => {
  // Deliberate local fault after the actual first file write, before its receipt.
  if (row.event === 'SOURCE_FILE_WRITTEN') process.exit(71);
  const events = join(work, 'events.jsonl');
  if (existsSync(events) && lstatSync(events).size > 32768) throw new Error('PUBLIC_SOURCE_WORKER_LIMIT');
  appendFileSync(events, JSON.stringify(row) + '\n');
});
throw new Error('PUBLIC_SOURCE_WORKER_DID_NOT_INTERRUPT');
