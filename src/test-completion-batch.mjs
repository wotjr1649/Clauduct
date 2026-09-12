import assert from 'node:assert/strict';
import { mkdtemp, mkdir, readFile, writeFile, rm } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import { join, resolve, sep } from 'node:path';
import { createAgentSelection } from './agent-selection.mjs';

const temporaryRoot = fileURLToPath(new URL('../.tmp/', import.meta.url));
const root = await mkdtemp(join(temporaryRoot, 'completion-batch-'));
const selected = { model: 'gpt-5.6-luna', effort: 'max' };
const directory = join(root, 'session', 'subagents');
const binding = id => ({ id, role: 'claude', sessionId: 'session', transcriptPath: join(root, 'session.jsonl'), nativeRegistered: true });
const metadata = id => ({ agentType: 'claude', toolUseId: `call_${id}`, model: 'inherit', ...(id !== 'parent' && { parentAgentId: 'parent' }) });
const final = id => ({ type: 'assistant', sessionId: 'session', agentId: id, timestamp: new Date().toISOString(),
  message: { id: `response_${id}`, role: 'assistant', stop_reason: 'end_turn', content: [] } });
const notice = (id, uuid = `notice_${id}`) => ({ type: 'user', sessionId: 'session', agentId: 'parent',
  uuid, timestamp: new Date().toISOString(), isMeta: true, origin: { kind: 'task-notification' },
  message: { role: 'user', content: `PUBLIC_NOTICE\n<task-notification><task-id>${id}</task-id><tool-use-id>call_${id}</tool-use-id><status>completed</status><summary>Public task done</summary><result>PUBLIC_RESULT</result></task-notification>` } });
const saveRows = (id, rows) => writeFile(join(directory, `agent-${id}.jsonl`), rows.map(row => JSON.stringify(row)).join('\n') + '\n');
async function setup(options = {}) {
  const selection = createAgentSelection({ projectsRoot: root, timeoutMs: 20, ...options });
  for (const id of ['parent', 'first', 'second']) {
    selection.remember({ content: [{ type: 'tool_use', id: `call_${id}`, name: 'Agent', input: { subagent_type: 'claude', model: 'inherit' } }] },
      'session', id === 'parent' ? undefined : 'parent', selected);
    await writeFile(join(directory, `agent-${id}.meta.json`), JSON.stringify(metadata(id)));
    await selection.resolve(binding(id));
    const record = final(id);
    selection.delivered('session', id, selection.begin('session', id), record.message);
    await saveRows(id, [record]);
  }
  await saveRows('parent', [final('parent'), notice('first'), notice('second')]);
  return selection;
}
let checks = 0;
try {
  await mkdir(directory, { recursive: true });
  {
    const selection = await setup();
    const route = await selection.resolve(binding('parent'));
    assert.equal(route.source, 'verified-completion-resume');
    assert.deepEqual(route.route, selected);
    // A batch must consume the first child's receipt as well as the last one.
    selection.delivered('session', 'parent', selection.begin('session', 'parent'), final('parent').message);
    await saveRows('parent', [notice('first', 'replayed_first')]);
    await assert.rejects(selection.resolve(binding('parent')), error => error.completionFailure === 'CHILD_COMPLETION'); checks++;
  }
  for (const fault of ['duplicate-child', 'duplicate-uuid', 'unknown-child', 'failed-first', 'wrong-session', 'too-many']) {
    const selection = await setup();
    let rows = [notice('first'), notice('second')];
    if (fault === 'duplicate-child') rows = [notice('first'), notice('first', 'different_uuid')];
    if (fault === 'duplicate-uuid') rows[1].uuid = rows[0].uuid;
    if (fault === 'unknown-child') rows[0] = notice('unknown');
    if (fault === 'failed-first') rows[0].message.content = rows[0].message.content.replace('<status>completed', '<status>failed');
    if (fault === 'wrong-session') rows[0].sessionId = 'different_session';
    if (fault === 'too-many') rows = Array.from({ length: 65 }, (_, index) => notice('first', `notice_${index}`));
    await saveRows('parent', rows);
    await assert.rejects(selection.resolve(binding('parent')), undefined, fault);
    // Rejection consumes no valid receipt, so the intact batch can still finish.
    await saveRows('parent', [notice('first'), notice('second')]);
    assert.equal((await selection.resolve(binding('parent'))).source, 'verified-completion-resume'); checks++;
  }
  {
    const selection = await setup();
    const results = await Promise.allSettled([selection.resolve(binding('parent')), selection.resolve(binding('parent'))]);
    assert.equal(results.filter(result => result.status === 'fulfilled').length, 1);
    assert.equal(results.filter(result => result.status === 'rejected').length, 1); checks++;
  }
  {
    const selection = await setup(), row = notice('first');
    row.message.content = row.message.content.replace('PUBLIC_RESULT', notice('second').message.content);
    await saveRows('parent', [row]);
    await assert.rejects(selection.resolve(binding('parent')), error => error.completionFailure === 'NOTIFICATION_HEADER'); checks++;
  }
  for (const interruption of ['new-child-request', 'cancelled-parent']) {
    let armed = false, selection;
    const controller = new AbortController();
    selection = await setup({ readMetadata: async current => {
      const value = JSON.parse(await readFile(join(directory, `agent-${current.id}.meta.json`), 'utf8'));
      if (armed && current.id === 'second') {
        armed = false;
        if (interruption === 'new-child-request') selection.begin('session', 'first');
        else controller.abort();
      }
      return value;
    } });
    armed = true;
    await assert.rejects(selection.resolve(binding('parent'), controller.signal), error =>
      interruption === 'cancelled-parent' ? error.name === 'AbortError' : error.message === 'AGENT_SELECTION_UNVERIFIED');
    if (interruption === 'new-child-request') {
      const record = final('first');
      selection.delivered('session', 'first', selection.begin('session', 'first'), record.message);
      await saveRows('first', [record]);
    }
    await saveRows('parent', [notice('first'), notice('second')]);
    assert.equal((await selection.resolve(binding('parent'))).source, 'verified-completion-resume'); checks++;
  }
  console.log(JSON.stringify({ suite: 'completion-batch', checks, externalRequests: 0, actualCredentialReads: 0,
    actualClaude: 0, liveMultipleNotifications: 'NOT_RUN' }));
} finally {
  assert.ok(resolve(root).startsWith(resolve(temporaryRoot) + sep));
  assert.ok(root.startsWith(join(temporaryRoot, 'completion-batch-')));
  await rm(root, { recursive: true });
}
