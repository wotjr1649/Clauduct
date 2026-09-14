// Installation diagnostics only: version queries and paths, no authentication.
import { spawnSync } from 'node:child_process';
import { discoverRuntime } from './runtime-paths.mjs';

export function checkInstallation({ runtime = discoverRuntime(), run = spawnSync } = {}) {
  if (Number(process.versions.node.split('.')[0]) < 24) throw new Error('NODE_24_REQUIRED');
  const versions = {};
  const env = Object.fromEntries(Object.entries(process.env).filter(([key]) =>
    /^(SystemRoot|WINDIR|SystemDrive|ComSpec|PATH|PATHEXT|USERPROFILE|HOMEDRIVE|HOMEPATH|APPDATA|LOCALAPPDATA|TEMP|TMP)$/i.test(key)));
  for (const name of ['claude', 'codex']) {
    const command = runtime[name];
    if (!command.found) throw new Error(`${name.toUpperCase()}_NOT_INSTALLED`);
    const result = run(command.file, [...command.args, '--version'], {
      encoding: 'utf8', timeout: 5000, maxBuffer: 4096, windowsHide: true, shell: false, env });
    const pattern = name === 'codex' ? /^codex-cli ([0-9]+\.[0-9]+\.[0-9]+(?:[-+][0-9A-Za-z.-]+)?)$/
      : /^([0-9]+\.[0-9]+\.[0-9]+(?:[-+][0-9A-Za-z.-]+)?)(?: \(Claude Code\))?$/;
    const match = !result.error && !result.signal && result.status === 0 && pattern.exec(result.stdout?.trim() ?? '');
    if (!match) throw new Error(`${name.toUpperCase()}_VERSION_UNAVAILABLE`);
    versions[name] = match[1];
  }
  return { passed: true, node: process.versions.node, runtime, versions, loginChecked: false, modelRequests: 0 };
}
if (process.argv[1]?.replaceAll('\\', '/').endsWith('/install-check.mjs')) {
  try { console.log(JSON.stringify(checkInstallation())); }
  catch (error) { console.error(/^[A-Z_0-9]+$/.test(error.message) ? error.message : 'INSTALL_CHECK_FAILED'); process.exitCode = 1; }
}
