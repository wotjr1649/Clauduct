import { createManagedLedgerAccount } from '../managed-ledger-account.mjs';
const [mode, sourceDirectory, ledgerHash, manifestHash] = process.argv.slice(2);
if (process.argv.length !== 6 || !['create', 'exit-after-ready'].includes(mode)) throw new Error('PUBLIC_LEDGER_WORKER_ARGUMENTS');
try {
  const account = createManagedLedgerAccount({ sourceDirectory, ledgerHash, manifestHash, localNative: true });
  if (mode === 'exit-after-ready') process.exit(73);
  console.log(JSON.stringify({ created: true, root: account.root }));
} catch (error) {
  if (error.message !== 'MANAGED_LEDGER_ALREADY_BOUND') throw new Error('PUBLIC_LEDGER_WORKER_FAILED');
  console.log(JSON.stringify({ created: false, reason: 'MANAGED_LEDGER_ALREADY_BOUND' }));
}
