import { openSync, writeSync, closeSync } from 'node:fs';
import { join } from 'node:path';
import { createExecutionReservation, settleExecutionReservation } from '../execution-reservation.mjs';

const [root, mode] = process.argv.slice(2);
if (process.argv.length !== 4 || !['reserved', 'torn-settlement', 'settled'].includes(mode)) throw new Error('PUBLIC_RESERVATION_MODE');
createExecutionReservation(root, { basisHash: 'a'.repeat(64),
  previous: { attempts: 10, inputTokens: 100, outputTokens: 20, elapsedMs: 1000 },
  limits: { attempts: 20, inputTokens: 2000, outputTokens: 500, elapsedMs: 30000 },
  allowance: { attempts: 6, inputTokens: 1000, outputTokens: 200, elapsedMs: 20000 } });
let fd;
if (mode === 'torn-settlement') {
  // A real interrupted file write, with the reservation kept in another file.
  fd = openSync(join(root, 'execution-settlement.json'), 'wx');
  writeSync(fd, '{"version":');
} else if (mode === 'settled') {
  settleExecutionReservation(root, { evidenceHash: 'b'.repeat(64),
    observed: { attempts: 5, inputTokens: 150, outputTokens: 30, elapsedMs: 2000 } });
}
process.stdout.write('PUBLIC_RESERVATION_READY\n');
setTimeout(() => { if (fd !== undefined) closeSync(fd); process.exitCode = 3; }, 5000);
