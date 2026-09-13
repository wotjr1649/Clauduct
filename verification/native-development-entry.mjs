import { appendFileSync, statSync, readFileSync, lstatSync } from 'node:fs';
import { join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { spawn } from 'node:child_process';
import { main } from '../src/clauduct.mjs';
import { NativeError } from '../src/native-protocol.mjs';
import { openUserTransport, safeEntryCategory } from '../poc/user-session.mjs';
import { guardFixtureTransport } from './fixture-tool-policy.mjs';
import { createVerificationLedger } from './verification-ledger.mjs';
import { developmentTask } from './development-tasks.mjs';

export function createDevelopmentContextCheck(previousTaskId) {
  const previousFunction = developmentTask(previousTaskId).functionName;
  let observation = null;
  return input => {
    if (observation) return observation;
    // Cache a denial before inspecting input. A serialization failure or a
    // later changed request cannot turn a failed initial binding into success.
    observation = Object.freeze({ previousTaskId, previousFunctionObserved: false, previousCompletionObserved: false, observedInputBytes: 0 });
    const text = JSON.stringify(input ?? null);
    observation = Object.freeze({ previousTaskId, previousFunctionObserved: Array.isArray(input) && text.includes(previousFunction),
      previousCompletionObserved: Array.isArray(input) && input.some(item => item?.role === 'assistant'
        && JSON.stringify(item).includes('CLAUDUCT_DEVELOPMENT_DONE')), observedInputBytes: Buffer.byteLength(text) });
    return observation;
  };
}

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
  developmentTask(budget.taskId);
  if (typeof budget.taskId !== 'string' || !Object.hasOwn({ luna: 'max', sol: 'low' }, budget.model) || budget.effort !== { luna: 'max', sol: 'low' }[budget.model]
    || budget.maxObservedInputTokens !== 131072 || budget.maxObservedOutputTokens !== 32768
    || !Number.isSafeInteger(budget.requestLimit) || budget.requestLimit < 1 || budget.requestLimit > 16) throw new Error('INVALID_DEVELOPMENT_ENTRY');
  const ledger = createVerificationLedger(join(runRoot, `usage-${phase}`));
  const logPath = join(runRoot, `transport-${phase}.jsonl`);
  let guarded, previousInput = 0, previousOutput = 0, contextRecorded = false, contextAllowed = !budget.continuedFrom;
  const contextCheck = budget.continuedFrom ? createDevelopmentContextCheck(budget.previousTaskId) : null;
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
      if (options.requestBudget !== budget.requestLimit) throw new Error('INVALID_DEVELOPMENT_ENTRY');
      const factory = transportFactory ?? options.transportFactory;
      const transport = openTransport({ ...options, transportFactory: configuration => factory({ ...configuration,
        onAttempt: value => ledger.record('attempt', { ...guarded?.fixtureUsage(), requestAttempts: value.requestAttempts }),
        onResponseLimits: value => { record({ event: 'RESPONSE_LIMITS', ...value }); } }) });
      guarded = guardFixtureTransport(transport, { version: 1, kind: 'development', taskId: budget.taskId, workingRoot: process.cwd(),
        ...(phase === 'development-finish' ? { phase: 'finish' } : {}) }, { onUsage: value => {
        ledger.record('usage', value);
        record({ event: 'USAGE', usage: { input_tokens: value.inputTokens - previousInput, output_tokens: value.outputTokens - previousOutput } });
        previousInput = value.inputTokens; previousOutput = value.outputTokens;
      }, onUsageUnobserved: value => {
        ledger.record('usage-unobserved', value);
        record({ event: 'USAGE_UNOBSERVED', completions: value.unobservedCompletions });
      } });
      return Object.freeze({ ...guarded, send: async (body, signal, settings = {}) => {
        if (body.model !== `gpt-5.6-${budget.model}` || body.reasoning?.effort !== budget.effort) throw new NativeError('VERIFICATION_TOOL_INPUT_REJECTED');
        if (contextCheck && !contextRecorded) {
          // Inspect only the intended public task history and persist booleans
          // and a byte count. No raw input, profile or transcript is recorded.
          const observation = { event: 'CONTEXT_OBSERVED', ...contextCheck(body.input) };
          record(observation);
          contextAllowed = observation.previousFunctionObserved && observation.previousCompletionObserved;
          contextRecorded = true;
        }
        if (!contextAllowed) throw new NativeError('VERIFICATION_TOOL_INPUT_REJECTED');
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
