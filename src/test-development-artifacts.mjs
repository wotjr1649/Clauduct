import assert from 'node:assert/strict';
import { readFileSync, writeFileSync, renameSync, mkdirSync } from 'node:fs';
import { join } from 'node:path';
import { randomUUID } from 'node:crypto';
import { createDevelopmentFixture, sourceHash } from '../verification/development-fixture.mjs';
import { readDevelopmentArtifactHashes, verifyDevelopmentArtifacts } from '../verification/development-artifacts.mjs';
import { registerDevelopmentTask } from '../verification/registered-development-tasks.mjs';
import { publicBudgetTask } from '../verification/fixtures/registered-budget-task.mjs';

let checks = 0;
const roots = [];
const equal = (actual, expected) => { assert.deepEqual(actual, expected); checks++; };
const denied = action => { assert.throws(action, { message: 'DEVELOPMENT_ARTIFACT_CHANGED' }); checks++; };
function fixture(taskId = 'retry-delay-window') {
  const value = createDevelopmentFixture({ taskId, waitMode: 'check' }); roots.push(value.root);
  // Artifact hashing only: this synthetic record makes no execution/PASS claim.
  writeFileSync(join(value.work, 'events.jsonl'), '{"event":"TASK_READ"}\n', { flag: 'wx' });
  const artifactHashes = readDevelopmentArtifactHashes(value.root, taskId);
  const result = { taskId, sourceSha256: sourceHash(value.task.baseline), artifactHashes };
  const budget = { taskHash: value.taskHash, oracleHash: value.oracleHash, mcpHash: sourceHash(readFileSync(join(value.work, '.mcp.json'))) };
  return { ...value, result, budget };
}
const base = fixture();
equal(Object.keys(base.result.artifactHashes).length, 6);
equal(verifyDevelopmentArtifacts(base.root, base.result, base.budget), base.result.artifactHashes);
const record = publicBudgetTask(randomUUID()), text = JSON.stringify(record) + '\n';
const registered = fixture(registerDevelopmentTask(text, sourceHash(text)));
equal(verifyDevelopmentArtifacts(registered.root, registered.result, registered.budget), registered.result.artifactHashes);
for (const [directory, name] of [['work', 'retry-window.mjs'], ['control', 'TASK.md'], ['control', 'oracle.mjs'],
  ['control', 'review.json'], ['work', '.mcp.json'], ['work', 'events.jsonl']]) {
  const current = fixture(), path = join(current.root, directory, name === 'retry-window.mjs' ? current.task.sourceFile : name);
  writeFileSync(path, readFileSync(path, 'utf8') + '\n');
  denied(() => verifyDevelopmentArtifacts(current.root, current.result, current.budget));
}
for (const change of [row => { row.artifactHashes = null; }, row => { delete row.artifactHashes.source; },
  row => { row.artifactHashes.extra = '0'.repeat(64); }, row => { row.artifactHashes.source = 1; },
  row => { row.artifactHashes.source = '0'.repeat(64); }, row => { row.sourceSha256 = '0'.repeat(64); }]) {
  const result = structuredClone(base.result); change(result);
  denied(() => verifyDevelopmentArtifacts(base.root, result, base.budget));
}
for (const key of ['taskHash', 'oracleHash', 'mcpHash']) {
  denied(() => verifyDevelopmentArtifacts(base.root, base.result, { ...base.budget, [key]: '0'.repeat(64) }));
}
const unreviewed = fixture();
writeFileSync(join(unreviewed.control, 'review.json'), JSON.stringify({ sha256: unreviewed.result.sourceSha256, approved: false }));
denied(() => readDevelopmentArtifactHashes(unreviewed.root, unreviewed.taskId));
const invalid = fixture(), source = 'export function retryDelayWithinBudget(value) { return process.env; }';
writeFileSync(join(invalid.work, invalid.task.sourceFile), source);
writeFileSync(join(invalid.control, 'review.json'), JSON.stringify({ sha256: sourceHash(source), approved: true }));
denied(() => readDevelopmentArtifactHashes(invalid.root, invalid.taskId));
const large = fixture(); writeFileSync(join(large.work, 'events.jsonl'), ' '.repeat(32769));
denied(() => readDevelopmentArtifactHashes(large.root, large.taskId));
const occupied = fixture(), path = join(occupied.work, occupied.task.sourceFile);
renameSync(path, path + '.public-backup'); mkdirSync(path);
denied(() => readDevelopmentArtifactHashes(occupied.root, occupied.taskId));
denied(() => readDevelopmentArtifactHashes('C:\\PublicFixture\\outside', 'retry-delay-window'));
equal(readFileSync(join(base.work, base.task.sourceFile), 'utf8'), base.task.baseline);
console.log(JSON.stringify({ suite: 'development-artifacts', checks, roots, syntheticArtifactRecords: true,
  evidenceDoesNotExecuteOrApproveSource: true, changedArtifactsRejected: true, dynamicLinkChecks: 'BLOCKED_NOT_RUN',
  actualNativeExecutions: 0, actualModelRequests: 0, actualCredentialReads: 0 }));
