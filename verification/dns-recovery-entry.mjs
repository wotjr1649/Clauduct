import { installDnsFailure } from './connection-fault.mjs';
let fault;
try {
  fault = installDnsFailure();
  await import('./guarded-headless-entry.mjs');
} catch {
  process.stderr.write('GUARDED_ENTRY VERIFICATION_CONNECTION_FAULT_REJECTED\n');
  process.exitCode = 1;
} finally {
  try {
    fault?.restore();
    process.stderr.write(`CLAUDUCT_CONNECTION_FAULT ${JSON.stringify(fault?.snapshot() ?? { connectionCalls: 0, injected: 0, restored: false })}\n`);
  } catch {
    process.stderr.write('GUARDED_ENTRY VERIFICATION_CONNECTION_FAULT_REJECTED\n');
    process.exitCode = 1;
  }
}
