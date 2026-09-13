import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { assessLongStageSequence, LONG_STAGE_REQUIREMENTS } from '../verification/long-stage-policy.mjs';

const hash = value => createHash('sha256').update(`public:${value}`).digest('hex');
const identity = { candidateCommit: 'a'.repeat(40), archiveHash: hash('archive'), oracleHash: hash('oracle'),
  runtimeHash: hash('runtime'), model: 'luna', effort: 'max' };
const fixture = () => LONG_STAGE_REQUIREMENTS.map((requirement, index) => ({ ...identity, stage: requirement.stage,
  runId: hash(`run-${index}`), state: 'completed', elapsedMs: requirement.minimumMs,
  developmentEvidenceIds: Array.from({ length: requirement.minimumDevelopment }, (_, task) => hash(`task-${index}-${task}`)),
  backendAttempts: requirement.minimumDevelopment + 20, backendCompletions: requirement.minimumDevelopment + 10,
  contextWindow: 400000, compactAt: 320000,
  resumes: 1, faultRecoveries: 2, mainCompactions: index === 4 ? 10 : 1, childCompactions: 2, authRefreshes: 2,
  processRecoveries: 1, parallelCycles: 2, isolatedCancellations: 2,
  interventions: 0, lostEffects: 0, duplicateEffects: 0, falseCompletions: 0, mixedTasks: 0,
  routeMismatches: 0, budgetOverruns: 0, ledgerGaps: 0, orphanProcesses: 0, paddingRequests: 0, idleToFillMs: 0
}));
let checks = 0;
const empty = assessLongStageSequence(identity, []);
assert.equal(empty.criteriaMet, false); assert.equal(empty.nextRequiredStage, '4h'); checks++;
const full = assessLongStageSequence(identity, fixture());
assert.equal(full.criteriaMet, true); assert.equal(full.requiredTotalMs, 532800000);
assert.equal(full.observedTotalMs, 532800000); assert.equal(full.releaseVerdict, 'HOLD');
assert.equal(full.observationsIndependentlyVerified, false); checks++;
for (let length = 1; length < 5; length++) {
  const result = assessLongStageSequence(identity, fixture().slice(0, length));
  assert.equal(result.criteriaMet, false); assert.equal(result.completedStageCount, length);
  assert.equal(result.nextRequiredStage, LONG_STAGE_REQUIREMENTS[length].stage); checks++;
}
for (const field of ['candidateCommit','archiveHash','oracleHash','runtimeHash','model','effort']) {
  const runs = fixture(); runs[1][field] = field === 'model' ? 'sol' : field === 'effort' ? 'low' : hash('different');
  const result = assessLongStageSequence(identity, runs);
  assert.equal(result.criteriaMet, false); assert.ok(result.rows[1].missing.includes('IDENTITY_MISMATCH'));
  assert.ok(result.rows[2].missing.includes('PRIOR_STAGE_INCOMPLETE')); checks++;
}
for (let index = 0; index < 5; index++) {
  for (const mutate of [row => { row.elapsedMs--; }, row => { row.developmentEvidenceIds.pop(); },
    row => { row.state = 'failed'; }, row => { row.state = 'running'; }]) {
    const runs = fixture(); mutate(runs[index]);
    assert.equal(assessLongStageSequence(identity, runs).criteriaMet, false); checks++;
  }
}
for (const field of ['interventions','lostEffects','duplicateEffects','falseCompletions','mixedTasks',
  'routeMismatches','budgetOverruns','ledgerGaps','orphanProcesses','paddingRequests','idleToFillMs']) {
  const runs = fixture(); runs[0][field] = 1;
  assert.ok(assessLongStageSequence(identity, runs).rows[0].missing.includes(`NONZERO_${field}`)); checks++;
}
for (const field of ['resumes','faultRecoveries']) {
  const runs = fixture(); runs[0][field] = 0;
  assert.equal(assessLongStageSequence(identity, runs).criteriaMet, false); checks++;
}
for (const field of ['mainCompactions','childCompactions','authRefreshes','processRecoveries','isolatedCancellations']) {
  const runs = fixture(); for (const row of runs.slice(1,4)) row[field] = 0;
  const result = assessLongStageSequence(identity, runs);
  assert.ok(result.rows[3].missing.includes(`REPEATED_24H_${field}_NOT_MET`));
  assert.ok(result.rows[4].missing.includes('PRIOR_STAGE_INCOMPLETE')); checks++;
}
for (const [field, below] of [['mainCompactions',9],['childCompactions',1],['authRefreshes',1],['processRecoveries',0],
  ['parallelCycles',1],['isolatedCancellations',1],['faultRecoveries',1]]) {
  const runs = fixture(); runs[4][field] = below;
  assert.ok(assessLongStageSequence(identity, runs).rows[4].missing.includes(`LONG_72H_${field}_NOT_MET`)); checks++;
}
for (const mutate of [runs => { runs[0].developmentEvidenceIds[1] = runs[0].developmentEvidenceIds[0]; },
  runs => { runs[1].developmentEvidenceIds[0] = runs[0].developmentEvidenceIds[0]; },
  runs => { runs[0].backendCompletions = 0; }, runs => { runs[0].contextWindow = 16000; },
  runs => { runs[0].compactAt = 12000; }]) {
  const runs = fixture(); mutate(runs); assert.equal(assessLongStageSequence(identity, runs).criteriaMet, false); checks++;
}
for (const mutate of [runs => { runs[1].runId = runs[0].runId; }, runs => { runs.reverse(); },
  runs => { runs.push({ ...runs[3], runId: hash('fourth-24h') }); }, runs => { runs[0].elapsedMs = NaN; },
  runs => { runs[0].elapsedMs = -1; }, runs => { runs[4].elapsedMs++; }, runs => { runs[0].authRefreshes = null; },
  runs => { delete runs[0].authRefreshes; }, runs => { runs[0].backendCompletions = runs[0].backendAttempts + 1; },
  runs => { runs[0].developmentEvidenceIds[0] = 'unreviewed'; }, runs => { runs[0].extra = true; }]) {
  const runs = fixture(); mutate(runs);
  assert.throws(() => assessLongStageSequence(identity, runs), { message: 'LONG_STAGE_EVIDENCE_INVALID' }); checks++;
}
for (const override of [{ model: 'sol', effort: 'max' }, { model: 'luna', effort: 'low' }, { archiveHash: null },
  { candidateCommit: Object('a'.repeat(40)) }, { extra: true }]) {
  assert.throws(() => assessLongStageSequence({ ...identity, ...override }, []), { message: 'LONG_STAGE_EVIDENCE_INVALID' }); checks++;
}
const sol = { ...identity, model: 'sol', effort: 'low' };
assert.equal(assessLongStageSequence(sol, fixture().map(run => ({ ...run, model: 'sol', effort: 'low' }))).criteriaMet, true); checks++;
console.log(JSON.stringify({ suite: 'long-stage-policy', checks, syntheticCounterFixtures: true,
  actualLongRuns: 0, externalRequests: 0, actualCredentialReads: 0 }));
