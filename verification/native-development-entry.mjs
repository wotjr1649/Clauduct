import { appendFileSync, statSync, readFileSync, lstatSync } from 'node:fs';
import { join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { spawn } from 'node:child_process';
import { main } from '../src/clauduct.mjs';
import { NativeError } from '../src/native-protocol.mjs';
import { openUserTransport, safeEntryCategory } from '../poc/user-session.mjs';
import { guardFixtureTransport } from './fixture-tool-policy.mjs';
import { createVerificationLedger } from './verification-ledger.mjs';

// Separate from recovery's historical entry: only the reviewed development
// tools can reach this native child, with a durable reservation before HTTPS.
export async function runNativeDevelopmentEntry({ entryArgs = process.argv.slice(2), openTransport = openUserTransport, transportFactory } = {}) {
  const [runRoot, phase, ...args] = entryArgs;
  if (typeof runRoot !== 'string' || resolve(runRoot, 'work') !== process.cwd() || !['development', 'development-finish'].includes(phase)
    || !process.send || typeof openTransport !== 'function'
    || transportFactory !== undefined && typeof transportFactory !== 'function') throw new Error('INVALID_DEVELOPMENT_ENTRY');
  const path = join(runRoot, phase === 'development' ? 'budget.json' : 'budget-finish.json'), info = lstatSync(path);
  if (!info.isFile() || info.isSymbolicLink() || info.size > 16384) throw new Error('INVALID_DEVELOPMENT_ENTRY');
  const budget = JSON.parse(readFileSync(path, 'utf8'));
  if (!Object.hasOwn({ luna: 'max', sol: 'low' }, budget.model) || budget.effort !== { luna: 'max', sol: 'low' }[budget.model]
    || budget.maxObservedInputTokens !== 131072 || budget.maxObservedOutputTokens !== 32768) throw new Error('INVALID_DEVELOPMENT_ENTRY');
  const ledger = createVerificationLedger(join(runRoot, `usage-${phase}`));
  const logPath = join(runRoot, `transport-${phase}.jsonl`);
  let guarded, previousInput = 0, previousOutput = 0;
  function record(value) {
    const line = JSON.stringify({ at: Date.now(), ...value }) + '\n';
    if (statSync(logPath).size + Buffer.byteLength(line) > 65536) throw new Error('DEVELOPMENT_LEDGER_LIMIT');
    appendFileSync(logPath, line, { flush: true });
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
      guarded = guardFixtureTransport(transport, { version: 1, kind: 'development', workingRoot: process.cwd(),
        ...(phase === 'development-finish' ? { phase: 'finish' } : {}) }, { onUsage: value => {
        ledger.record('usage', value);
        record({ event: 'USAGE', usage: { input_tokens: value.inputTokens - previousInput, output_tokens: value.outputTokens - previousOutput } });
        previousInput = value.inputTokens; previousOutput = value.outputTokens;
      } });
      return Object.freeze({ ...guarded, send: async (body, signal, settings = {}) => {
        if (body.model !== `gpt-5.6-${budget.model}` || body.reasoning?.effort !== budget.effort) throw new NativeError('VERIFICATION_TOOL_INPUT_REJECTED');
        record({ event: 'REQUEST_STARTED', model: body.model, effort: body.reasoning.effort });
        try { return await guarded.send(body, signal, settings); }
        finally { record({ event: 'REQUEST_SETTLED', attempts: transport.diagnostics().requestAttempts }); }
      } });
    } });
  } catch (error) { process.stderr.write(`DEVELOPMENT_ENTRY ${safeEntryCategory(error)}\n`); process.exitCode = 1; }
  finally {
    try { ledger.record('final', guarded?.fixtureUsage() ?? {}); }
    catch { process.stderr.write('DEVELOPMENT_ENTRY VERIFICATION_LEDGER_INVALID\n'); process.exitCode = 1; }
    finally { ledger.close(); }
    if (process.connected) process.disconnect();
  }
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  try { await runNativeDevelopmentEntry(); }
  catch { process.stderr.write('DEVELOPMENT_ENTRY INVALID_DEVELOPMENT_ENTRY\n'); process.exitCode = 1; }
}
