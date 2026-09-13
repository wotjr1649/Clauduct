import { createManagedPlan } from '../managed-development.mjs';
const [mode, root] = process.argv.slice(2);
if (process.argv.length !== 4 || !['create', 'exit-after-ready'].includes(mode)) throw new Error('PUBLIC_PLAN_WORKER_ARGUMENTS');
try {
  const state = createManagedPlan(root, { model: 'sol', localNative: true,
    taskIds: ['retry-after-seconds', 'retry-delay-window'], maxDurationMs: 180000 });
  if (mode === 'exit-after-ready') process.exit(73);
  console.log(JSON.stringify({ created: true, planHash: state.planHash }));
} catch (error) {
  if (error.code !== 'EEXIST') throw new Error('PUBLIC_PLAN_WORKER_FAILED');
  console.log(JSON.stringify({ created: false, reason: 'PLAN_EXISTS' }));
}
