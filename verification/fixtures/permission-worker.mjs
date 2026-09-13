import { appendFileSync, writeFileSync } from 'node:fs';

const label = process.argv[2];
if (process.argv.length !== 3 || !['allowed', 'denied'].includes(label)) throw new Error('PUBLIC_PERMISSION_ARGUMENTS');
appendFileSync('worker-started.jsonl', JSON.stringify({ label, event: 'started' }) + '\n');
writeFileSync(`${label}-effect.json`, JSON.stringify({ label, event: 'performed' }) + '\n', { flag: 'wx' });
process.stdout.write(`PUBLIC_EFFECT_${label.toUpperCase()}\n`);
