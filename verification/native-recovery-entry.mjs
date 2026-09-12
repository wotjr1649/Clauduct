import { appendFileSync, statSync, readFileSync } from 'node:fs';
import { join } from 'node:path';
import { spawn } from 'node:child_process';
import { main } from '../src/clauduct.mjs';
import { openUserTransport, safeEntryCategory } from '../poc/user-session.mjs';
import { createFixtureToolPolicy } from './fixture-tool-policy.mjs';

// Called only by the reviewed manager, with its fresh root and fixed CLI array.
// IPC authorization follows the manager's durable PID record.
const [runRoot, phase, ...args] = process.argv.slice(2);
if (!runRoot || !['effect', 'finish', 'development'].includes(phase) || !process.send) throw new Error('INVALID_RECOVERY_ENTRY');
const ledger = join(runRoot, `transport-${phase}.jsonl`);
const budget = JSON.parse(readFileSync(join(runRoot, 'budget.json'), 'utf8'));
const noTools = budget.allowToolCalls === false
  ? createFixtureToolPolicy({ version: 1, kind: 'none', workingRoot: process.cwd() }) : null;
let observedInput = 0, observedOutput = 0;
function record(value) {
  if (statSync(ledger).size > 65536) throw new Error('RECOVERY_LEDGER_LIMIT');
  appendFileSync(ledger, JSON.stringify({ at: Date.now(), ...value }) + '\n', { flush: true });
}
const startTimer = setTimeout(() => process.exit(1), 5000);
await new Promise(done => process.once('message', message => { if (message?.start === true) done(); else process.exit(1); }));
clearTimeout(startTimer);
try {
  await main({ args,
    startClient: (file, nativeArgs, options) => {
      const child = spawn(file, nativeArgs, options);
      record({ event: 'NATIVE_STARTED', pid: child.pid });
      return child;
    },
    openTransport: options => {
      const transport = openUserTransport(options);
      return Object.freeze({ ...transport,
        send: async (body, signal, settings = {}) => {
          if (observedInput >= budget.maxObservedInputTokens || observedOutput >= budget.maxObservedOutputTokens) {
            record({ event: 'USAGE_BUDGET_STOP' });
            throw Object.assign(new Error('REQUEST_BUDGET'), { code: 'REQUEST_BUDGET' });
          }
          const model = ['gpt-5.6-luna', 'gpt-5.6-sol'].includes(body.model) ? body.model : 'other';
          const effort = ['max', 'low'].includes(body.reasoning?.effort) ? body.reasoning.effort : 'other';
          record({ event: 'REQUEST_STARTED', model, effort });
          const original = settings.onEvent;
          try {
            return await transport.send(body, signal, { ...settings, onEvent: original ? async event => {
              if (event.type === 'response.completed') {
                const usage = {};
                for (const key of ['input_tokens', 'output_tokens', 'total_tokens']) {
                  const value = event.response?.usage?.[key];
                  if (Number.isSafeInteger(value) && value >= 0) usage[key] = value;
                }
                record({ event: 'USAGE', usage });
                observedInput += usage.input_tokens ?? 0; observedOutput += usage.output_tokens ?? 0;
              }
              noTools?.(event);
              await original(event);
            } : undefined });
          } finally {
            record({ event: 'REQUEST_SETTLED', attempts: transport.diagnostics().requestAttempts });
          }
        }
      });
    }
  });
} catch (error) {
  process.stderr.write(`RECOVERY_ENTRY ${safeEntryCategory(error)}\n`); process.exitCode = 1;
} finally { if (process.connected) process.disconnect(); }
