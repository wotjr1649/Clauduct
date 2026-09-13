import { runManagedPlan } from './managed-development.mjs';
const [mode, root, powershell, count] = process.argv.slice(2);
try {
  const interrupt = mode === '--local-interrupt-after-step';
  if (!(interrupt ? process.argv.length === 6 && /^[1-9][0-9]{0,2}$/.test(count)
    : process.argv.length === 5 && ['--local-plan', '--live-plan'].includes(mode))) throw new Error('MANAGED_PLAN_ARGUMENTS');
  const result = await runManagedPlan({ root, powershell, localNative: mode.startsWith('--local-'),
    interruptAfterCompleted: interrupt ? Number(count) : 0 });
  console.log(JSON.stringify({ suite: result.suite, root: result.root, passed: result.passed,
    completedSteps: result.completedSteps, totalSteps: result.totalSteps, charged: result.account.charged,
    localNative: result.localNative, longStageEvidence: false, releaseVerdict: 'HOLD' }));
} catch (error) {
  const known = /^(?:MANAGED_PLAN_[A-Z_]+|MANAGED_DEVELOPMENT_[A-Z_]+|MANAGED_LEDGER_[A-Z_]+|EXECUTION_ACCOUNT_[A-Z_]+|INTERRUPTION_[A-Z_]+)$/;
  console.log(JSON.stringify({ suite: 'managed-development-plan', passed: false,
    failure: known.test(error.message) ? error.message : 'MANAGED_PLAN_FAILED' }));
  process.exitCode = 1;
}
