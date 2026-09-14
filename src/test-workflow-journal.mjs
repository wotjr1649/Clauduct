import assert from 'node:assert/strict';
import { mkdtemp, mkdir, writeFile, rm } from 'node:fs/promises';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { createWorkflowSelection, workflowDigest } from './workflow-selection.mjs';

await mkdir(new URL('../.tmp/', import.meta.url), { recursive: true });
const root = await mkdtemp(fileURLToPath(new URL('../.tmp/workflow-journal-', import.meta.url)));
const session = 'synthetic-journal-session', agentId = 'synthetic-child', runId = 'wf_journal-01';
const transcriptPath = join(root, `${session}.jsonl`);
const directory = join(root, session, 'subagents', 'workflows', runId);
const scriptPath = join(root, session, 'workflows', 'scripts', `journal-${runId}.js`);
const script = 'export const meta = { name: "journal" }; return null;';
const route = { model: 'gpt-5.6-luna', effort: 'max' };
const link = { sessionId: session, runId, workflowName: 'journal', taskId: 'journal-task',
  transcriptPath, transcriptDir: directory, scriptPath, scriptDigest: workflowDigest(script) };
const started = { type: 'started', key: `v2:${'a'.repeat(64)}`, agentId, label: 'public child' };
const launched = JSON.stringify({ type: 'launched' }) + '\n';
const startLine = JSON.stringify(started) + '\n';
const historyLine = JSON.stringify({ type: 'result', agentId: 'older-child', result: 'PUBLIC_HISTORY'.repeat(16) }) + '\n';
const history = historyLine.repeat(2048);
const metaPath = join(directory, `agent-${agentId}.meta.json`);
const childPath = join(directory, `agent-${agentId}.jsonl`);
const journalPath = join(directory, 'journal.jsonl');
const binding = { id: agentId, sessionId: session, transcriptPath, role: 'workflow-subagent',
  nativeRegistered: true, requestedModel: 'luna' };
function selector() {
  const value = createWorkflowSelection(root);
  value.link(link, { session, created: 0, workflow: { digest: workflowDigest(script), route } });
  return value;
}
async function writeJournal(text) { await writeFile(journalPath, text); }
let passed = 0;
try {
  await mkdir(directory, { recursive: true });
  await mkdir(join(root, session, 'workflows', 'scripts'), { recursive: true });
  await writeFile(transcriptPath, ''); await writeFile(scriptPath, script);
  await writeFile(metaPath, JSON.stringify({ agentType: 'workflow-subagent', description: started.label, spawnDepth: 1 }));
  const firstLine = JSON.stringify({ type: 'user', sessionId: session, agentId,
    timestamp: new Date().toISOString(), message: { role: 'user', content: 'PUBLIC_WORKFLOW_TASK' } }) + '\n';
  await writeFile(childPath, firstLine);
  await writeJournal(launched + startLine);
  assert.deepEqual((await selector().resolve(binding)).route, route); passed++;

  // Editing a completed run must not invalidate an independent run's child.
  // Its journal still participates in the ownership/duplicate-origin check.
  {
    const nextId = 'independent-child', nextRunId = 'wf_journal-02';
    const nextDirectory = join(root, session, 'subagents', 'workflows', nextRunId);
    const nextScriptPath = join(root, session, 'workflows', 'scripts', `journal-${nextRunId}.js`);
    const nextScript = 'export const meta = { name: "journal" }; return 2;';
    const nextRoute = { model: 'gpt-5.6-sol', effort: 'low' };
    const nextLink = { ...link, runId: nextRunId, taskId: 'independent-task', transcriptDir: nextDirectory,
      scriptPath: nextScriptPath, scriptDigest: workflowDigest(nextScript) };
    const nextBinding = { ...binding, id: nextId, requestedModel: 'sol' };
    const nextStart = { ...started, agentId: nextId, label: 'independent child' };
    const nextStartLine = JSON.stringify(nextStart) + '\n';
    const completedOld = launched + startLine + JSON.stringify({ type: 'result', agentId, result: 'PUBLIC_DONE' }) + '\n';
    const add = (current, value, selected) => current.link(value,
      { session, created: 0, workflow: { digest: value.scriptDigest, route: selected } });
    await mkdir(nextDirectory, { recursive: true }); await writeFile(nextScriptPath, nextScript);
    await writeFile(join(nextDirectory, 'journal.jsonl'), launched + nextStartLine);
    const nextMetadata = JSON.stringify({ agentType: 'workflow-subagent', description: nextStart.label, spawnDepth: 1 });
    const nextTranscript = JSON.stringify({ type: 'user', sessionId: session,
      agentId: nextId, timestamp: new Date().toISOString(), message: { role: 'user', content: 'PUBLIC_INDEPENDENT_TASK' } }) + '\n';
    // Both origins have otherwise valid sidecars when the duplicate is injected.
    for (const target of [directory, nextDirectory]) {
      await writeFile(join(target, `agent-${nextId}.meta.json`), nextMetadata);
      await writeFile(join(target, `agent-${nextId}.jsonl`), nextTranscript);
    }
    await writeJournal(completedOld); await writeFile(scriptPath, script + '\n// PUBLIC_EDIT_AFTER_COMPLETION');
    for (const reverse of [false, true]) {
      const current = createWorkflowSelection(root);
      for (const [value, selected] of (reverse ? [[nextLink, nextRoute], [link, route]] : [[link, route], [nextLink, nextRoute]])) add(current, value, selected);
      assert.deepEqual((await current.resolve(nextBinding)).route, nextRoute); passed++;
      await writeFile(nextScriptPath, nextScript + '\n// PUBLIC_CHANGED_ACTIVE_RUN');
      await assert.rejects(current.resolve(nextBinding), error => error.selectionReason === 'IDENTITY'); passed++;
      await writeFile(nextScriptPath, nextScript);
      await writeJournal(completedOld + nextStartLine);
      await assert.rejects(current.resolve(nextBinding), error => error.selectionReason === 'IDENTITY'); passed++;
      await writeFile(scriptPath, script);
      await assert.rejects(current.resolve(nextBinding), error => error.selectionReason === 'IDENTITY'); passed++;
      await writeJournal(launched + '{"type":');
      await assert.rejects(current.resolve(nextBinding)); passed++;
      await writeJournal(completedOld); await writeFile(scriptPath, script + '\n// PUBLIC_EDIT_AFTER_COMPLETION');
    }
    await writeFile(scriptPath, script); await writeJournal(launched + startLine);
  }

  // More than 128 KiB of completed siblings must not hide a valid new child.
  assert.ok(Buffer.byteLength(history) > 131072);
  await writeJournal(launched + history + startLine);
  assert.deepEqual((await selector().resolve(binding)).route, route); passed++;
  await writeJournal(launched + startLine + history);
  assert.deepEqual((await selector().resolve(binding)).route, route); passed++;

  // A growing child transcript needs only its original identity record here.
  await writeFile(childPath, firstLine + (JSON.stringify({ type: 'assistant', message: 'PUBLIC_HISTORY'.repeat(32) }) + '\n').repeat(4096));
  assert.deepEqual((await selector().resolve(binding)).route, route); passed++;
  await writeFile(childPath, firstLine);

  for (const tail of [startLine, JSON.stringify({ type: 'failed', agentId }) + '\n',
    JSON.stringify({ type: 'result', agentId }) + '\n']) {
    await writeJournal(launched + startLine + history + tail);
    await assert.rejects(selector().resolve(binding), error => error.selectionReason === 'IDENTITY'); passed++;
  }
  for (const text of [launched + history + '{"type":', launched + history + 'null\n',
    launched + history + '[]\n', launched + history + '\n' + startLine]) {
    await writeJournal(text);
    await assert.rejects(selector().resolve(binding)); passed++;
  }
  await writeJournal(launched + startLine + JSON.stringify({ type: 'event', data: 'x'.repeat(131073) }) + '\n');
  await assert.rejects(selector().resolve(binding), error => error.selectionReason === 'SIZE'); passed++;
  await writeJournal(launched + history + JSON.stringify({ ...started, key: [started.key] }) + '\n');
  await assert.rejects(selector().resolve(binding), error => error.selectionReason === 'IDENTITY'); passed++;

  // A multibyte label spanning file-read boundaries remains valid UTF-8.
  const unicodeLine = JSON.stringify({ type: 'event', data: '가'.repeat(22000) }) + '\n';
  await writeJournal(launched + unicodeLine + startLine);
  assert.deepEqual((await selector().resolve(binding)).route, route); passed++;
  await writeFile(journalPath, Buffer.concat([Buffer.from(launched + history), Buffer.from([0xff, 10]), Buffer.from(startLine)]));
  await assert.rejects(selector().resolve(binding)); passed++;

  await writeJournal(launched + '{"type":"event"}\n'.repeat(65536) + startLine);
  await assert.rejects(selector().resolve(binding), error => error.selectionReason === 'SIZE'); passed++;
  const largeLine = JSON.stringify({ type: 'event', data: 'x'.repeat(65500) }) + '\n';
  await writeJournal(launched + largeLine.repeat(257) + startLine);
  await assert.rejects(selector().resolve(binding), error => error.selectionReason === 'SIZE'); passed++;

  await writeJournal(launched + history + startLine);
  const cancel = new AbortController(), reason = new Error('PUBLIC_CANCEL');
  const cancelled = selector().resolve(binding, cancel.signal);
  const settled = Promise.allSettled([cancelled]);
  setImmediate(() => cancel.abort(reason));
  const [result] = await settled;
  assert.equal(result.status, 'rejected'); assert.equal(result.reason, reason); passed++;
  assert.deepEqual((await selector().resolve(binding)).route, route); passed++;

  await writeFile(childPath, 'x'.repeat(1048577) + '\n');
  await assert.rejects(selector().resolve(binding), error => error.selectionReason === 'SIZE'); passed++;
  console.log(JSON.stringify({ suite: 'workflow-journal', passed, externalRequests: 0, actualClaude: 0,
    notRun: ['native cache-miss resume', 'dynamic symlink and junction checks'] }));
} finally { await rm(root, { recursive: true, force: true }); }
