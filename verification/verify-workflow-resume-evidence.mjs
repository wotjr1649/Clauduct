import assert from 'node:assert/strict';
import { readFileSync, readdirSync, lstatSync, realpathSync, existsSync } from 'node:fs';
import { createHash } from 'node:crypto';
import { join, resolve, relative, isAbsolute, basename, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';

export function verifyWorkflowResumeEvidence(root, { firstOnly = false } = {}) {
  root = resolve(root);
  assert.ok(/^native-workflow-stored-[a-f0-9]{32}$/.test(basename(root)));
  const digest = bytes => createHash('sha256').update(bytes).digest('hex');
  function bytes(path, limit = 2097152) {
    const rel = relative(root, path), stat = lstatSync(path);
    assert.ok(rel && !isAbsolute(rel) && !rel.startsWith('..') && stat.isFile() && !stat.isSymbolicLink() && stat.size <= limit);
    assert.equal(realpathSync(path).toLowerCase(), path.toLowerCase()); return readFileSync(path);
  }
  const json = path => JSON.parse(bytes(path).toString('utf8').replace(/^\uFEFF/, ''));
  const rows = path => bytes(path).toString('utf8').trimEnd().split('\n').map(JSON.parse);
  const hashRecord = path => ({ path, bytes: bytes(path).length, sha256: digest(bytes(path)) });
  const manifest = json(join(root, 'manifest.json')), first = json(join(root, 'first-result.json')), state = json(join(root, 'first-state.json'));
  const { model, sessionId } = manifest, effort = model === 'sol' ? 'low' : 'max';
  assert.ok(['sol', 'luna'].includes(model) && /^[a-f0-9-]{36}$/.test(sessionId));
  assert.equal(state.sessionId, sessionId); assert.equal(first.entry.sessionId, sessionId);
  function phase(result, name) {
    assert.ok(result.passed && result.exitCode === 0 && !result.timedOut && result.tree.stopped && result.tree.remaining.length === 0);
    assert.equal(result.entry.actualBackendRequests, 0); assert.equal(result.entry.credentialReads, 0);
    assert.equal(result.entry.model, model); assert.equal(result.entry.effort, effort); assert.equal(result.entry.sessionId, sessionId);
    assert.equal(Object.keys(result.status.cleanup).length, 9); assert.ok(Object.values(result.status.cleanup).every(value => value === true));
    const output = rows(join(root, `${name}-stdout.txt`)), init = output.filter(row => row.type === 'system' && row.subtype === 'init');
    assert.ok(init.length >= 1 && init.length <= 4 && init.every(row => row.session_id === sessionId && row.permissionMode === 'bypassPermissions'));
    const finals = output.filter(row => row.type === 'result');
    assert.ok(finals.length >= 1 && finals.at(-1).is_error === false && finals.at(-1).session_id === sessionId);
  }
  phase(first, 'first');
  assert.equal(first.entry.cachedAgentRequests, 1); assert.equal(first.entry.retryAgentRequests, 1); assert.equal(first.entry.failureInjected, true);
  assert.ok(first.status.recentRequests.some(row => row.failureCategory === 'UPSTREAM_RESPONSE_FAILED'));
  const projects = join(root, 'config/projects');
  const folders = readdirSync(projects); assert.ok(folders.length <= 16);
  const paths = folders.map(name => join(projects, name, `${sessionId}.jsonl`)).filter(existsSync); assert.equal(paths.length, 1);
  const main = rows(paths[0]), sessionRoot = join(dirname(paths[0]), sessionId);
  const tools = main.flatMap(row => row.type === 'assistant' && Array.isArray(row.message?.content) ? row.message.content.filter(item => item.type === 'tool_use') : []);
  const workflows = tools.filter(item => item.name === 'Workflow');
  assert.equal(workflows.length, firstOnly ? 1 : 2);
  assert.equal(bytes(state.scriptPath).toString('utf8'), workflows[0].input.script);
  assert.equal(digest(bytes(state.scriptPath)), state.scriptDigest);
  assert.equal(state.scriptPath, join(sessionRoot, 'workflows/scripts', `public-stored-restart-${state.runId}.js`));
  assert.equal(state.transcriptDir, join(sessionRoot, 'subagents/workflows', state.runId));
  const taskIds = [];
  for (const call of workflows) {
    const resultRows = main.filter(row => row.type === 'user' && Array.isArray(row.message?.content) && row.message.content.some(item => item.type === 'tool_result' && item.tool_use_id === call.id));
    assert.equal(resultRows.length, 1); const result = resultRows[0].toolUseResult;
    assert.equal(result.runId, state.runId); assert.equal(result.scriptPath, state.scriptPath); assert.equal(result.transcriptDir, state.transcriptDir);
    assert.equal(result.status, 'async_launched'); assert.equal(result.taskType, 'local_workflow'); taskIds.push(result.taskId);
  }
  assert.equal(new Set(taskIds).size, taskIds.length);
  const journalPath = join(state.transcriptDir, 'journal.jsonl'), journal = rows(journalPath);
  assert.deepEqual(journal.map(row => row.type), firstOnly ? ['launched', 'started', 'result', 'started', 'failed']
    : ['launched', 'started', 'result', 'started', 'failed', 'started', 'result']);
  assert.equal(journal[1].label, 'cached'); assert.equal(journal[3].label, 'retry');
  assert.equal(journal[2].agentId, journal[1].agentId); assert.equal(journal[2].key, journal[1].key); assert.deepEqual(journal[2].result, { sum: 5 });
  assert.equal(journal[4].agentId, journal[3].agentId); assert.equal(journal[4].key, journal[3].key);
  const checkpointPath = join(root, 'work/checkpoint.json'), reportPath = join(root, 'work/report.json');
  const writes = tools.filter(item => item.name === 'Write'); assert.equal(writes.length, firstOnly ? 1 : 2);
  assert.equal(writes[0].input.file_path, checkpointPath); assert.equal(writes[0].input.content, '{"seed":5,"stage":"created"}\n');
  assert.equal(bytes(checkpointPath).toString('utf8'), writes[0].input.content);
  const evidence = [join(root, 'manifest.json'), join(root, 'first-result.json'), paths[0], state.scriptPath, journalPath, checkpointPath];
  let last;
  if (firstOnly) { assert.equal(existsSync(reportPath), false); }
  else {
    const prior = json(join(root, 'first-verified.json')); assert.ok(prior.passed && prior.firstOnly);
    for (const file of prior.evidence) {
      const data = bytes(file.path);
      if ([paths[0], journalPath].includes(file.path)) assert.equal(digest(data.subarray(0, file.bytes)), file.sha256);
      else assert.equal(digest(data), file.sha256);
    }
    last = json(join(root, 'resume-result.json')); phase(last, 'resume');
    assert.ok(last.startedMs >= first.startedMs + first.elapsedMs);
    assert.notEqual(`${last.pid}:${last.startedMs}`, `${first.pid}:${first.startedMs}`);
    assert.equal(last.entry.cachedAgentRequests, 0); assert.equal(last.entry.retryAgentRequests, 1);
    assert.equal(last.entry.sameRun, true); assert.equal(last.entry.effectConfirmed, true);
    assert.deepEqual(workflows[1].input, { scriptPath: state.scriptPath, resumeFromRunId: state.runId });
    assert.equal(journal[5].label, 'retry'); assert.equal(journal[5].key, journal[3].key); assert.notEqual(journal[5].agentId, journal[3].agentId);
    assert.equal(journal[6].agentId, journal[5].agentId); assert.equal(journal[6].key, journal[5].key); assert.deepEqual(journal[6].result, { sum: 7 });
    const selected = last.status.recentRequests.filter(row => row.role === 'workflow-subagent' && row.success);
    assert.equal(selected.length, 1); assert.equal(selected[0].selectionSource, 'workflow-result');
    assert.equal(selected[0].model, `gpt-5.6-${model}`); assert.equal(selected[0].effort, effort);
    assert.equal(writes[1].input.file_path, reportPath); assert.equal(writes[1].input.content, '{"cached":5,"retried":7,"total":12}\n');
    assert.equal(bytes(reportPath).toString('utf8'), writes[1].input.content);
    assert.deepEqual(readdirSync(join(root, 'work')).sort(), ['checkpoint.json', 'report.json']);
    evidence.push(join(root, 'resume-result.json'), join(root, 'first-verified.json'), reportPath);
  }
  for (const item of journal.filter(row => row.type === 'started')) {
    const path = join(state.transcriptDir, `agent-${item.agentId}.jsonl`), metadataPath = join(state.transcriptDir, `agent-${item.agentId}.meta.json`);
    const metadata = json(metadataPath), transcript = rows(path);
    assert.equal(metadata.agentType, 'workflow-subagent'); assert.equal(metadata.description, item.label); assert.equal(metadata.spawnDepth, 1);
    assert.ok(metadata.parentAgentId == null && metadata.stoppedByUser !== true);
    assert.ok(transcript.filter(row => ['user', 'assistant'].includes(row.type)).every(row => row.sessionId === sessionId && row.agentId === item.agentId));
    const terminal = transcript.filter(row => row.type === 'assistant').at(-1);
    if (item.agentId === journal[3].agentId) assert.equal(terminal.isApiErrorMessage, true);
    else {
      const output = terminal.message.content.filter(block => block.type === 'tool_use' && block.name === 'StructuredOutput');
      assert.equal(output.length, 1); assert.deepEqual(output[0].input, { sum: item.label === 'cached' ? 5 : 7 });
      assert.ok(transcript.some(row => row.type === 'user' && Array.isArray(row.message?.content)
        && row.message.content.some(block => block.type === 'tool_result' && block.tool_use_id === output[0].id && block.is_error !== true)));
    }
    evidence.push(path, metadataPath);
  }
  return { passed: true, firstOnly, root, model, effort, sessionId, runId: state.runId, permissionMode: 'bypassPermissions',
    cachedAgentExecutions: 1, retryAgentExecutions: firstOnly ? 1 : 2, cachedResult: 5, finalResult: firstOnly ? null : 12,
    checkpointWrites: 1, reportWrites: firstOnly ? 0 : 1, previousWorkerStopped: true, newProcess: !firstOnly,
    publicTransportCalls: first.entry.requests + (last?.entry.requests ?? 0), actualBackendRequests: 0, credentialReads: 0,
    cleanupChecksEachPhase: 9, remainingOwned: 0, evidence: evidence.map(hashRecord) };
}
if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  try { process.stdout.write(JSON.stringify(verifyWorkflowResumeEvidence(process.argv[2], { firstOnly: process.argv[3] === '--first' })) + '\n'); }
  catch { process.stderr.write('WORKFLOW_RESUME_EVIDENCE_INVALID\n'); process.exitCode = 1; }
}
