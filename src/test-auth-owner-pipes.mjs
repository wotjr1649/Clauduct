import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { fileURLToPath } from 'node:url';
import { createAuthOwnerProtocol } from '../verification/auth-owner-protocol.mjs';

const worker = fileURLToPath(new URL('../verification/fixtures/auth-owner-public-worker.mjs', import.meta.url));
const env = Object.fromEntries(['SystemRoot','WINDIR'].filter(key => process.env[key]).map(key => [key, process.env[key]]));
const rows = [];
for (const mode of ['normal','wrong-home','wrong-account','approval','unsolicited','duplicate','truncated','oversize','timeout']) {
  const protocol = createAuthOwnerProtocol({ expectedCodexHome: 'C:\\PublicFixture\\codex-home', refresh: true });
  const child = spawn(process.execPath, ['--permission', `--allow-fs-read=${worker}`, worker, mode],
    { env, windowsHide: true, stdio: ['pipe','pipe','pipe'] });
  let stderrBytes = 0, failure = null, stopAttempted = false, stdoutEnded = false, sentPackets = 0;
  const send = () => { for (const packet of protocol.takeOutgoing()) { sentPackets++; child.stdin.write(packet); } };
  const stop = () => {
    if (!stopAttempted && child.exitCode === null && child.signalCode === null) {
      stopAttempted = true; child.kill();
    }
  };
  const watchdog = setTimeout(() => { failure = 'PUBLIC_OWNER_TIMEOUT'; stop(); }, mode === 'timeout' ? 300 : 2500);
  child.stdin.on('error', () => { failure ??= 'PUBLIC_OWNER_PIPE_ERROR'; stop(); });
  child.stderr.on('data', chunk => { stderrBytes += chunk.length; if (stderrBytes > 65536) { failure ??= 'PUBLIC_OWNER_STDERR_LIMIT'; stop(); } });
  child.stdout.on('data', chunk => {
    protocol.push(chunk);
    const snapshot = protocol.snapshot();
    if (snapshot.failure) stop();
    else if (snapshot.phase === 'complete') child.stdin.end();
    else send();
  });
  child.stdout.once('end', () => { stdoutEnded = true; protocol.end(); });
  child.stdout.on('error', () => { failure ??= 'PUBLIC_OWNER_PIPE_ERROR'; stop(); });
  const result = await new Promise(done => {
    child.once('error', () => { failure = 'PUBLIC_OWNER_START_FAILED'; });
    child.once('close', code => { clearTimeout(watchdog); done({ exitCode: code }); });
    child.once('spawn', send);
  });
  const snapshot = protocol.snapshot();
  let alive = true;
  try { process.kill(child.pid, 0); }
  catch (error) { if (error.code === 'ESRCH') alive = false; else throw error; }
  assert.equal(alive, false); assert.equal(stdoutEnded, true); assert.equal(stderrBytes, 0);
  assert.equal(snapshot.normalRefreshVerified, false);
  assert.doesNotMatch(JSON.stringify(snapshot), /public@example|PUBLIC_DISCARDED_AGENT|PUBLIC_UNEXECUTED_COMMAND|codex-home/);
  if (mode === 'normal') {
    assert.equal(result.exitCode, 0); assert.equal(failure, null); assert.equal(snapshot.exchangeComplete, true);
    assert.equal(snapshot.refreshReplyReceived, true); assert.equal(sentPackets, 4);
  } else {
    assert.equal(snapshot.exchangeComplete, false);
    assert.ok(snapshot.failure !== null || failure !== null);
    if (['wrong-home','approval','unsolicited','truncated','oversize','timeout'].includes(mode)) assert.equal(snapshot.refreshRequested, false);
    if (['wrong-home','approval','unsolicited','truncated','oversize','timeout'].includes(mode)) assert.ok(sentPackets <= 1);
  }
  rows.push({ mode, ...result, controllerFailure: failure, sentPackets, alive, stdoutEnded, stderrBytes, ...snapshot });
}
console.log(JSON.stringify({ suite: 'auth-owner-pipes', checks: rows.length, realPublicChildren: rows.length, rows,
  actualOwnerConnections: 0, actualCredentialReads: 0, externalRequests: 0, normalRefresh: 'NOT_RUN' }));
