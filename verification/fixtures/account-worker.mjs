import { writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { reserveExecutionAccount } from '../execution-account.mjs';
import { settleExecutionReservation } from '../execution-reservation.mjs';

const [root, mode] = process.argv.slice(2);
if (process.argv.length !== 4 || !['claim', 'settled', 'torn'].includes(mode)) throw new Error('PUBLIC_ACCOUNT_MODE');
try {
  const claim = reserveExecutionAccount(root, { executionHash: 'b'.repeat(64),
    allowance: { attempts: 6, inputTokens: 1000, outputTokens: 200, elapsedMs: 20000 } });
  if (mode === 'settled') settleExecutionReservation(claim.entry, { evidenceHash: 'c'.repeat(64),
    observed: { attempts: 5, inputTokens: 150, outputTokens: 30, elapsedMs: 2000 } });
  if (mode === 'torn') writeFileSync(join(claim.entry, 'execution-settlement.json'), '{"version":', { flag: 'wx' });
  console.log(JSON.stringify({ state: 'PUBLIC_ACCOUNT_READY' }));
  setTimeout(() => { process.exitCode = 3; }, 5000);
} catch (error) {
  const permitted = ['EXECUTION_ACCOUNT_BUSY', 'EXECUTION_ACCOUNT_PENDING', 'EXECUTION_ACCOUNT_ENTRY_INCOMPLETE'];
  console.log(JSON.stringify({ state: permitted.includes(error.message) ? error.message : 'PUBLIC_ACCOUNT_UNEXPECTED_FAILURE' }));
}
