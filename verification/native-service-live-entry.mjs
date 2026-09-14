import { appendFileSync, statSync, readFileSync, lstatSync, existsSync } from 'node:fs';
import { join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { spawn } from 'node:child_process';
import { main } from '../src/clauduct.mjs';
import { NativeError } from '../src/native-protocol.mjs';
import { openUserTransport, safeEntryCategory } from '../poc/user-session.mjs';
import { guardFixtureTransport } from './fixture-tool-policy.mjs';
import { createVerificationLedger } from './verification-ledger.mjs';

// Only this entry has real backend access. The injected service signal happens
// before transport.send and is never described as an observed backend HTTP reply.
export async function runNativeServiceLiveEntry({ entryArgs = process.argv.slice(2), openTransport = openUserTransport, transportFactory } = {}) {
const [runRoot, phase, ...args] = entryArgs;
if (typeof runRoot !== 'string' || resolve(runRoot, 'work') !== process.cwd()
  || !['effect', 'finish'].includes(phase) || !process.send || typeof openTransport !== 'function'
  || transportFactory !== undefined && typeof transportFactory !== 'function') throw new Error('INVALID_RECOVERY_ENTRY');
const read = (path, limit) => {
  const info = lstatSync(path);
  if (!info.isFile() || info.isSymbolicLink() || info.size > limit) throw new NativeError('VERIFICATION_TOOL_INPUT_REJECTED');
  return readFileSync(path, 'utf8');
};
const budget = JSON.parse(read(join(runRoot, 'budget.json'), 16384));
const manifest = JSON.parse(read(join(runRoot, 'manifest.json'), 4096));
if (budget.failureMode !== 'service-signal' || !/^[0-9a-f-]{36}$/.test(manifest.operationId)
  || budget.maxObservedInputTokens !== 131072 || budget.maxObservedOutputTokens !== 32768) throw new Error('INVALID_RECOVERY_ENTRY');
const ledger = createVerificationLedger(join(runRoot, `usage-${phase}`));
const logPath = join(runRoot, `transport-${phase}.jsonl`);
const policy = { version: 1, kind: 'recovery', phase, workingRoot: process.cwd() };
const receiptPath = join(runRoot, 'work', 'operation.json');
let guarded, injected = false, previousInput = 0, previousOutput = 0;
function record(value) {
  const text = JSON.stringify({ at: Date.now(), ...value }) + '\n';
  if (statSync(logPath).size + Buffer.byteLength(text) > 65536) throw new Error('RECOVERY_LEDGER_LIMIT');
  appendFileSync(logPath, text, { flush: true });
}
const startTimer = setTimeout(() => process.exit(1), 5000);
await new Promise(done => process.once('message', message => { if (message?.start === true) done(); else process.exit(1); }));
clearTimeout(startTimer);
try {
  await main({ args, startClient: (file, nativeArgs, options) => {
    const child = spawn(file, nativeArgs, options); record({ event: 'NATIVE_STARTED', pid: child.pid }); return child;
  }, openTransport: options => {
    const factory = transportFactory ?? options.transportFactory;
    const transport = openTransport({ ...options, transportFactory: configuration => factory({ ...configuration,
      onAttempt: value => ledger.record('attempt', { ...guarded?.fixtureUsage(), requestAttempts: value.requestAttempts }) }) });
    guarded = guardFixtureTransport(transport, policy, { onUsage: value => {
      ledger.record('usage', value);
      record({ event: 'USAGE', usage: { input_tokens: value.inputTokens - previousInput, output_tokens: value.outputTokens - previousOutput } });
      previousInput = value.inputTokens; previousOutput = value.outputTokens;
    }, onUsageUnobserved: value => {
      ledger.record('usage-unobserved', value);
      record({ event: 'USAGE_UNOBSERVED', completions: value.unobservedCompletions });
    } });
    return Object.freeze({ ...guarded, send: async (body, signal, settings = {}) => {
      record({ event: 'REQUEST_STARTED', model: ['gpt-5.6-luna', 'gpt-5.6-sol'].includes(body.model) ? body.model : 'other',
        effort: ['max', 'low'].includes(body.reasoning?.effort) ? body.reasoning.effort : 'other' });
      try {
        if (phase === 'effect' && existsSync(receiptPath)) {
          if (injected || read(receiptPath, 1024) !== JSON.stringify({ operationId: manifest.operationId, count: 1 })
            || existsSync(join(runRoot, 'work', 'report.json'))) throw new NativeError('VERIFICATION_TOOL_INPUT_REJECTED');
          injected = true;
          record({ event: 'SERVICE_SIGNAL_INJECTED', category: 'UPSTREAM_HTTP_ERROR', status: 503, backendAttempted: false });
          throw Object.assign(new NativeError('UPSTREAM_HTTP_ERROR'), { statusCode: 503 });
        }
        return await guarded.send(body, signal, settings);
      } finally { record({ event: 'REQUEST_SETTLED', attempts: transport.diagnostics().requestAttempts }); }
    } });
  } });
} catch (error) { process.stderr.write(`RECOVERY_ENTRY ${safeEntryCategory(error)}\n`); process.exitCode = 1; }
finally {
  try { ledger.record('final', guarded?.fixtureUsage() ?? {}); }
  catch { process.stderr.write('RECOVERY_ENTRY VERIFICATION_LEDGER_INVALID\n'); process.exitCode = 1; }
  finally { ledger.close(); }
  if (process.connected) process.disconnect();
}
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  try { await runNativeServiceLiveEntry(); }
  catch { process.stderr.write('RECOVERY_ENTRY INVALID_RECOVERY_ENTRY\n'); process.exitCode = 1; }
}
