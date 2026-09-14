import { spawn } from 'node:child_process';
import { writeFileSync } from 'node:fs';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { CODEX_EXE, CODEX_ROOT } from '../src/runtime-paths.mjs';
import { createAuthOwnerProtocol } from './auth-owner-protocol.mjs';
import { temporaryDir } from './temporary-dir.mjs';

// Read-only inspection through the installed CLI's existing control-socket
// proxy. No daemon start, thread, model turn, refresh, login or config command.
if (process.argv.length !== 3 || process.argv[2] !== '--existing-owner-read-only') throw new Error('OWNER_READ_ARGUMENTS');
if (process.env.CODEX_HOME && resolve(process.env.CODEX_HOME).toLowerCase() !== resolve(CODEX_ROOT).toLowerCase()
  || process.env.USERPROFILE && resolve(process.env.USERPROFILE).toLowerCase() !== dirname(CODEX_ROOT).toLowerCase()) throw new Error('OWNER_READ_HOME_MISMATCH');
const project = dirname(dirname(fileURLToPath(import.meta.url)));
const root = temporaryDir(project, 'auth-owner-read-');
const env = Object.fromEntries(['SystemRoot','WINDIR','USERPROFILE','HOMEDRIVE','HOMEPATH','APPDATA','LOCALAPPDATA']
  .filter(key => process.env[key]).map(key => [key, process.env[key]]));
const protocol = createAuthOwnerProtocol({ expectedCodexHome: CODEX_ROOT, refresh: false });
const child = spawn(CODEX_EXE, ['app-server', 'proxy'], { cwd: root, env, windowsHide: true, stdio: ['pipe','pipe','pipe'] });
let controllerFailure = null, stderrBytes = 0, stdoutEnded = false, stopAttempted = false, sentPackets = 0;
const started = Date.now();
const stop = () => {
  if (!stopAttempted && child.exitCode === null && child.signalCode === null) { stopAttempted = true; child.kill(); }
};
const send = () => { for (const packet of protocol.takeOutgoing()) { sentPackets++; child.stdin.write(packet); } };
const watchdog = setTimeout(() => { controllerFailure = 'OWNER_READ_TIMEOUT'; stop(); }, 10000);
child.stdin.on('error', () => { controllerFailure ??= 'OWNER_READ_PIPE_ERROR'; stop(); });
child.stderr.on('data', chunk => {
  stderrBytes += chunk.length;
  if (stderrBytes > 65536) { controllerFailure ??= 'OWNER_READ_STDERR_LIMIT'; stop(); }
});
child.stdout.on('data', chunk => {
  protocol.push(chunk);
  const result = protocol.snapshot();
  if (result.failure) stop();
  else if (result.phase === 'complete') child.stdin.end();
  else send();
});
child.stdout.once('end', () => { stdoutEnded = true; protocol.end(); });
child.stdout.on('error', () => { controllerFailure ??= 'OWNER_READ_PIPE_ERROR'; stop(); });
const exitCode = await new Promise(done => {
  child.once('spawn', send);
  child.once('error', () => { controllerFailure = 'OWNER_READ_START_FAILED'; });
  child.once('close', code => { clearTimeout(watchdog); done(code); });
});
let proxyStopped = false;
try { process.kill(child.pid, 0); }
catch (error) { if (error.code === 'ESRCH') proxyStopped = true; else controllerFailure ??= 'OWNER_READ_STOP_UNVERIFIED'; }
const snapshot = protocol.snapshot();
const result = { suite: 'auth-owner-existing-read', root, elapsedMs: Date.now() - started, exitCode, controllerFailure,
  stdoutEnded, stderrBytes, proxyStopped, stopAttempted, sentPackets, ...snapshot,
  existingOwnerIdentityVerified: false,
  explicitRefreshRequests: 0, modelTurnsStarted: 0, directCredentialFileReads: 0, directCredentialFileWrites: 0,
  configurationCommandsIssued: 0, daemonStartCommandsIssued: 0, releaseVerdict: 'HOLD' };
writeFileSync(join(root, 'result.json'), JSON.stringify(result) + '\n', { flag: 'wx' });
console.log(JSON.stringify(result));
process.exitCode = snapshot.exchangeComplete && proxyStopped && exitCode === 0 && !controllerFailure ? 0 : 1;
