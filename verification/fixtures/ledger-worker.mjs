import { createVerificationLedger } from '../verification-ledger.mjs';

const ledger = createVerificationLedger(process.argv[2]);
ledger.record('attempt', { requestAttempts: 1 });
ledger.record('usage', { inputTokens: 40, outputTokens: 2, completions: 1 });
ledger.record('attempt', { requestAttempts: 2 });
process.stdout.write('LEDGER_READY\n');
// The parent kills this bounded worker before it can write a final record.
setTimeout(() => { ledger.close(); process.exitCode = 3; }, 5000);
