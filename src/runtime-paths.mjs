import { userInfo } from 'node:os';
import { join, dirname, isAbsolute, resolve } from 'node:path';
import { statSync, readFileSync } from 'node:fs';

// Bind the credential home and installed executables to the OS user, not a caller's
// USERPROFILE override or one developer's username. No configuration or credential is read.
const userRoot = userInfo().homedir;
export const CODEX_ROOT = join(userRoot, '.codex');
const isFile = path => { try { return statSync(path).isFile(); } catch { return false; } };
const readPackage = path => {
  try { if (statSync(path).size > 65536) return null; return JSON.parse(readFileSync(path, 'utf8')); } catch { return null; }
};

// Resolve applications, never shell aliases/functions or a relative PATH entry.
// npm's known JavaScript entry runs through this Node, not through cmd/PowerShell.
export function discoverRuntime({ home = userRoot, env = process.env, cwd = process.cwd(), node = process.execPath,
  file = isFile, packageInfo = readPackage } = {}) {
  const pathValue = Object.entries(env).find(([name]) => name.toLowerCase() === 'path')?.[1] ?? '';
  const dirs = [...new Set(pathValue.split(';').slice(0, 64).map(value => value.trim().replace(/^"(.*)"$/, '$1'))
    .filter(value => isAbsolute(value) && resolve(value).toLowerCase() !== resolve(cwd).toLowerCase()))];
  const standalone = join(home, 'AppData', 'Local', 'Programs', 'OpenAI', 'Codex', 'bin', 'codex.exe');
  const native = join(home, '.local', 'bin', 'claude.exe');
  const claude = [native, ...dirs.map(dir => join(dir, 'claude.exe'))].find(file);
  let codex = [standalone, ...dirs.map(dir => join(dir, 'codex.exe'))].find(file);
  let codexArgs = [];
  if (!codex) {
    const npmDirs = [...new Set([join(home, 'AppData', 'Roaming', 'npm'), dirname(node), ...dirs])];
    for (const dir of npmDirs) {
      if (resolve(dir).toLowerCase() === resolve(cwd).toLowerCase()) continue;
      const root = join(dir, 'node_modules', '@openai', 'codex'), entry = join(root, 'bin', 'codex.js');
      if (!file(entry)) continue;
      const metadata = packageInfo(join(root, 'package.json'));
      if (metadata?.name === '@openai/codex' && metadata.bin?.codex === 'bin/codex.js') {
        codex = node; codexArgs = [entry]; break;
      }
    }
  }
  return { node, claude: { file: claude ?? native, args: [], found: !!claude },
    codex: { file: codex ?? standalone, args: codexArgs, found: !!codex } };
}
const runtime = discoverRuntime();
export const CODEX_EXE = runtime.codex.file;
export const CODEX_ARGS = Object.freeze(runtime.codex.args);
export const CLAUDE_EXE = runtime.claude.file;
