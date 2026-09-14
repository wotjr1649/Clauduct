const hour = 3600000;
const stages = ['4h', '24h-1', '24h-2', '24h-3', '72h'];
const durations = [4, 24, 24, 24, 72].map(value => value * hour);
const minimumDevelopment = [10, 30, 30, 30, 100];
const counts = ['resumes', 'faultRecoveries', 'mainCompactions', 'childCompactions', 'authRefreshes',
  'processRecoveries', 'parallelCycles', 'isolatedCancellations'];
const zeroCounts = ['interventions', 'lostEffects', 'duplicateEffects', 'falseCompletions', 'mixedTasks',
  'routeMismatches', 'budgetOverruns', 'ledgerGaps', 'orphanProcesses', 'paddingRequests', 'idleToFillMs'];
const identities = ['candidateCommit', 'archiveHash', 'oracleHash', 'runtimeHash', 'model', 'effort'];
const shape = (value, fields) => value && typeof value === 'object' && !Array.isArray(value)
  && Object.keys(value).length === fields.length && fields.every(key => Object.hasOwn(value, key));
const integer = value => Number.isSafeInteger(value) && value >= 0;
const digest = value => typeof value === 'string' && /^[a-f0-9]{64}$/.test(value);
const fail = () => { throw new Error('LONG_STAGE_EVIDENCE_INVALID'); };

export const LONG_STAGE_REQUIREMENTS = Object.freeze(stages.map((stage, index) => Object.freeze({
  stage, minimumMs: durations[index], minimumDevelopment: minimumDevelopment[index]
})));

// This is a count/sequence assessment of already-collected observations. It
// never authenticates a result file, launches work or grants a release PASS.
// The run owner must independently bind observations to actual artifacts,
// timers, backend routes and unchanged binaries before using this assessment.
export function assessLongStageSequence(identity, runs) {
  if (!shape(identity, identities) || typeof identity.candidateCommit !== 'string' || !/^[a-f0-9]{40}$/.test(identity.candidateCommit)
    || !['archiveHash', 'oracleHash', 'runtimeHash'].every(key => digest(identity[key]))
    || !Object.hasOwn({ luna: 'max', sol: 'low' }, identity.model)
    || identity.effort !== { luna: 'max', sol: 'low' }[identity.model]
    || !Array.isArray(runs) || runs.length > stages.length) fail();
  const issues = [], rows = [], runIds = new Set(), developmentIds = new Set();
  let halted = false;
  for (let index = 0; index < runs.length; index++) {
    const run = runs[index], problems = [];
    if (!shape(run, [...identities, 'stage', 'runId', 'state', 'elapsedMs', 'developmentEvidenceIds',
      'backendAttempts', 'backendCompletions', 'contextWindow', 'compactAt', ...counts, ...zeroCounts])
      || run.stage !== stages[index] || !digest(run.runId) || runIds.has(run.runId)
      || !['running', 'completed', 'failed'].includes(run.state)
      || !integer(run.elapsedMs) || run.elapsedMs > 72 * hour
      || !integer(run.backendAttempts) || !integer(run.backendCompletions) || run.backendCompletions > run.backendAttempts
      || !Array.isArray(run.developmentEvidenceIds) || run.developmentEvidenceIds.length > 4096
      || !run.developmentEvidenceIds.every(digest)
      || ![...counts, ...zeroCounts].every(key => integer(run[key]))) fail();
    runIds.add(run.runId);
    if (identities.some(key => run[key] !== identity[key])) problems.push('IDENTITY_MISMATCH');
    if (run.contextWindow !== 400000 || run.compactAt !== 320000) problems.push('DEFAULT_CONTEXT_NOT_USED');
    if (halted) problems.push('PRIOR_STAGE_INCOMPLETE');
    if (run.state !== 'completed') problems.push(run.state === 'failed' ? 'RUN_FAILED' : 'RUN_NOT_FINISHED');
    if (run.elapsedMs < durations[index]) problems.push('DURATION_NOT_MET');
    for (const key of zeroCounts) if (run[key] !== 0) problems.push(`NONZERO_${key}`);
    const seen = new Set();
    for (const id of run.developmentEvidenceIds) {
      if (seen.has(id) || developmentIds.has(id)) problems.push('DEVELOPMENT_EVIDENCE_REUSED');
      seen.add(id);
    }
    for (const id of seen) developmentIds.add(id);
    if (seen.size < minimumDevelopment[index]) problems.push('DEVELOPMENT_COUNT_NOT_MET');
    if (seen.size > run.backendCompletions) problems.push('DEVELOPMENT_EXCEEDS_BACKEND_COMPLETIONS');
    if (index === 0) {
      if (run.resumes < 1) problems.push('RESUME_NOT_OBSERVED');
      if (run.faultRecoveries < 1) problems.push('FAULT_RECOVERY_NOT_OBSERVED');
    }
    if (index === 3) {
      const batch = runs.slice(1, 4);
      for (const [key, minimum] of [['mainCompactions', 3], ['childCompactions', 1], ['authRefreshes', 1],
        ['processRecoveries', 1], ['isolatedCancellations', 1]]) {
        if (batch.reduce((sum, row) => sum + row[key], 0) < minimum) problems.push(`REPEATED_24H_${key}_NOT_MET`);
      }
    }
    if (index === 4) {
      for (const [key, minimum] of [['mainCompactions', 10], ['childCompactions', 2], ['authRefreshes', 2],
        ['processRecoveries', 1], ['parallelCycles', 2], ['isolatedCancellations', 2], ['faultRecoveries', 2]]) {
        if (run[key] < minimum) problems.push(`LONG_72H_${key}_NOT_MET`);
      }
    }
    const missing = [...new Set(problems)];
    halted ||= missing.length > 0;
    rows.push({ stage: run.stage, criteriaMet: missing.length === 0, developmentCount: seen.size, elapsedMs: run.elapsedMs, missing });
    issues.push(...missing.map(reason => ({ stage: run.stage, reason })));
  }
  const firstIncomplete = rows.find(row => !row.criteriaMet)?.stage;
  return {
    model: identity.model, effort: identity.effort,
    criteriaMet: runs.length === stages.length && !halted,
    nextRequiredStage: firstIncomplete ?? stages[runs.length] ?? null,
    requiredTotalMs: 148 * hour,
    observedTotalMs: runs.reduce((sum, row) => sum + row.elapsedMs, 0),
    completedStageCount: rows.filter(row => row.criteriaMet).length,
    rows, issues, observationsIndependentlyVerified: false, releaseVerdict: 'HOLD'
  };
}
