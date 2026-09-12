import { readFileSync, writeFileSync, lstatSync } from 'node:fs';
import { join } from 'node:path';
import { createNativeLoopbackTransport } from '../../src/native-transport.mjs';

const [root, phase] = process.argv.slice(2);
const need = (ok, label) => { if (!ok) throw new Error(label); };
let transport;
try {
  need(root && ['initial', 'resume'].includes(phase), 'INVALID_PHASE');
  const read = path => {
    const stat = lstatSync(path); need(stat.isFile() && !stat.isSymbolicLink() && stat.size <= 4096, 'INVALID_STATE');
    return JSON.parse(readFileSync(path, 'utf8'));
  };
  const manifest = read(join(root, 'manifest.json'));
  need(/^public-retry-(60|300)$/.test(manifest.taskId) && Number.isInteger(manifest.port) && manifest.port > 0 && manifest.port < 65536, 'INVALID_STATE');
  if (phase === 'resume') {
    const state = read(join(root, 'waiting.json'));
    need(state.taskId === manifest.taskId && Number.isSafeInteger(state.retryAtMs), 'INVALID_STATE');
    if (Date.now() < state.retryAtMs) {
      console.log(JSON.stringify({ state: 'WAITING', attempts: 0, credentialReads: 0 })); process.exit(2);
    }
  }
  let credentialReads = 0;
  transport = createNativeLoopbackTransport(manifest.port, { requestBudget: 2,
    credentialSupplier: () => { credentialReads++; return { accessToken: 'synthetic', account: 'synthetic' }; } });
  try {
    const events = await transport.send({ taskId: manifest.taskId, instruction: 'Complete the original public report.' }, AbortSignal.timeout(4000));
    need(events.at(-1)?.type === 'response.completed', 'INCOMPLETE');
    writeFileSync(join(root, 'report.json'), JSON.stringify({ taskId: manifest.taskId, completed: true }), { flag: 'wx' });
    console.log(JSON.stringify({ state: 'VERIFIED', attempts: transport.diagnostics().requestAttempts, credentialReads }));
  } catch (error) {
    if (phase !== 'initial' || error.code !== 'UPSTREAM_RETRY_DEFERRED') throw error;
    need(Number.isSafeInteger(error.retryAtMs) && error.retryAtMs > Date.now(), 'INVALID_STATE');
    writeFileSync(join(root, 'waiting.json'), JSON.stringify({ taskId: manifest.taskId, retryAtMs: error.retryAtMs }), { flag: 'wx' });
    console.log(JSON.stringify({ state: 'WAITING', attempts: transport.diagnostics().requestAttempts, credentialReads })); process.exitCode = 2;
  }
} catch { console.log(JSON.stringify({ state: 'WORKER_FAILED' })); process.exitCode = 1; }
finally { if (transport) await transport.close(); }
