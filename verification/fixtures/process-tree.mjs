import { spawn } from 'node:child_process';
import { writeFileSync } from 'node:fs';
import { join } from 'node:path';
const [root, levelText] = process.argv.slice(2), level = Number(levelText);
if (!root || ![0, 1, 2].includes(level)) process.exit(1);
writeFileSync(join(root, `pid-${level}.json`), JSON.stringify({ pid: process.pid }), { flag: 'wx' });
if (level < 2) spawn(process.execPath, [process.argv[1], root, String(level + 1)],
  { cwd: root, env: process.env, windowsHide: true, stdio: 'ignore', shell: false });
setTimeout(() => process.exit(0), 10000);
