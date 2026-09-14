import assert from 'node:assert/strict';
import { join } from 'node:path';
import { discoverRuntime } from './runtime-paths.mjs';
const home = 'C:\\Public User', cwd = 'D:\\Public Project', node = 'C:\\Program Files\\nodejs\\node.exe';
const standalone = join(home, 'AppData/Local/Programs/OpenAI/Codex/bin/codex.exe');
const claude = join(home, '.local/bin/claude.exe'), npm = join(home, 'AppData/Roaming/npm/node_modules/@openai/codex');
let checks = 0;
function resolve(files, env = {}, metadata = { name: '@openai/codex', bin: { codex: 'bin/codex.js' } }) {
  return discoverRuntime({ home, cwd, node, env, file: path => files.includes(path), packageInfo: () => metadata });
}
let value = resolve([standalone, claude]);
assert.deepEqual(value.codex, { file: standalone, args: [], found: true }); checks++;
assert.deepEqual(value.claude, { file: claude, args: [], found: true }); checks++;
value = resolve([join(npm, 'bin/codex.js'), claude]);
assert.deepEqual(value.codex, { file: node, args: [join(npm, 'bin/codex.js')], found: true }); checks++;
for (const metadata of [{ name: 'other', bin: { codex: 'bin/codex.js' } }, { name: '@openai/codex', bin: { codex: '../other.js' } }]) {
  assert.equal(resolve([join(npm, 'bin/codex.js')], {}, metadata).codex.found, false); checks++;
}
value = resolve([join('D:\\Tools', 'claude.exe'), join('D:\\Tools', 'codex.exe')], { Path: 'D:\\Tools' });
assert.equal(value.claude.found && value.codex.found, true); checks++;
value = resolve([join(cwd, 'claude.exe'), join(cwd, 'codex.exe')], { PATH: `.;${cwd};relative;` });
assert.equal(value.claude.found || value.codex.found, false); checks++;
value = resolve([], { USERPROFILE: 'D:\\Foreign User', CODEX_HOME: 'D:\\Foreign User' });
assert.equal(value.codex.file, standalone); assert.equal(value.codex.found, false); checks++;
value = resolve([standalone, join(npm, 'bin/codex.js')]);
assert.equal(value.codex.file, standalone); checks++;
console.log(JSON.stringify({ suite: 'runtime-discovery', passed: true, checks, credentialReads: 0, externalRequests: 0 }));
