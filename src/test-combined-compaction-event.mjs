// F18 combined event: a child failure, a compaction record and a restart in one session.
// Each leg passes on its own elsewhere; nothing had ever run the three together, which is
// why the shipping table carried F18 as NOT_RUN.
//
// The invariant under test is that combining the legs changes no leg's verdict: for every
// trailing-record shape the outcome is identical whether the child ended cleanly or failed,
// and a restart never resumes from on-disk records alone. The record shapes come from 15
// real compactions read structurally on 2026-09-15; only the summary text is synthetic.
import assert from 'node:assert/strict';
import { mkdtemp, mkdir, writeFile, rm } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import { join } from 'node:path';
import { createAgentSelection } from './agent-selection.mjs';

await mkdir(new URL('../.tmp/', import.meta.url), { recursive: true });
const root = await mkdtemp(fileURLToPath(new URL('../.tmp/combined-compaction-', import.meta.url)));
const dir = join(root, 'session', 'subagents');
const binding = id => ({ id, role: id === 'child' ? 'Explore' : id === 'parent' ? 'claude' : 'general-purpose',
  sessionId: 'session', transcriptPath: join(root, 'session.jsonl'), nativeRegistered: true, stop: false });
const meta = id => ({ agentType: binding(id).role, toolUseId: `call_${id}`,
  ...(id === 'root' ? {} : { parentAgentId: id === 'child' ? 'parent' : 'root' }),
  ...(id === 'child' ? { model: 'haiku' } : {}) });
const file = (id, suffix) => join(dir, `agent-${id}.${suffix}`);
const save = (id, suffix, data) => writeFile(file(id, suffix), suffix === 'jsonl'
  ? data.map(row => JSON.stringify(row)).join('\n') + '\n' : JSON.stringify(data));
const final = (id, failed = false) => ({ type: 'assistant', sessionId: 'session', agentId: id,
  uuid: `uuid_${id}`, timestamp: new Date().toISOString(), ...(failed && { isApiErrorMessage: true }),
  message: { id: `response_${id}`, role: 'assistant', stop_reason: failed ? 'stop_sequence' : 'end_turn', content: [] } });
const notification = (status = 'completed') => ({ type: 'user', sessionId: 'session', agentId: 'parent',
  uuid: 'notification_1', timestamp: new Date().toISOString(), isMeta: true, origin: { kind: 'task-notification' },
  message: { role: 'user', content: 'SYNTHETIC_HARNESS_NOTICE\n<task-notification>\n<task-id>child</task-id>\n'
    + '<tool-use-id>call_child</tool-use-id>\n<output-file>SYNTHETIC_UNUSED_PATH</output-file>\n'
    + `<status>${status}</status>\n<summary>SYNTHETIC</summary>\n<result>SYNTHETIC_PRIVATE</result>\n</task-notification>` } });
const toolCall = id => ({ content: [{ type: 'tool_use', id: `call_${id}`, name: 'Agent',
  input: { subagent_type: binding(id).role, ...(id === 'child' ? { model: 'haiku' } : {}) } }] });

// A compaction can only reach this layer as records appended to the parent transcript.
// `measured-pair` is the sequence observed in 15 real compactions on this machine: a
// system boundary the resume scan's user/assistant filter drops, immediately followed by
// a plain user record it keeps. The other entries isolate one shape at a time.
const boundary = () => ({ type: 'system', subtype: 'compact_boundary', sessionId: 'session',
  uuid: 'compact_3', timestamp: new Date().toISOString(), compactMetadata: { trigger: 'auto', preTokens: 98097 } });
const summary = () => ({ type: 'user', sessionId: 'session', agentId: 'parent', uuid: 'compact_1',
  timestamp: new Date().toISOString(),
  message: { role: 'user', content: '<analysis>SYNTHETIC</analysis>\n<summary>SYNTHETIC</summary>' } });
const trailingRows = {
  none: () => [],
  'user-summary': () => [summary()],
  'user-meta-summary': () => [{ ...summary(), uuid: 'compact_1b', isMeta: true }],
  'assistant-summary': () => [{ type: 'assistant', sessionId: 'session', agentId: 'parent', uuid: 'compact_2',
    timestamp: new Date().toISOString(),
    message: { id: 'msg_compact', role: 'assistant', stop_reason: 'end_turn', content: [] } }],
  'system-boundary': () => [boundary()],
  'measured-pair': () => [boundary(), summary()],
};
// Only a boundary with nothing after it leaves the notification last. The measured pair
// does not, so a compaction in a parent agent's transcript blocks this resume.
const resumes = ['none', 'system-boundary'];

let checks = 0;
const watchdog = setTimeout(() => { console.error('COMBINED_COMPACTION_TIMEOUT'); process.exit(1); }, 20000);

// childFailed drives the failure leg: the child terminates with an API error, not a clean turn.
async function setup({ childFailed, trailing }) {
  const selection = createAgentSelection({ projectsRoot: root, timeoutMs: 35 });
  for (const id of ['root', 'parent', 'child']) {
    if (id === 'root') {
      selection.remember({ content: [{ type: 'tool_use', id: 'root_skill', name: 'Skill', input: { skill: 'code-review' } }] }, 'session');
      selection.linkSkill({ sessionId: 'session', toolUseId: 'root_skill', id, skill: 'code-review' });
      await save(id, 'meta.json', { agentType: 'general-purpose', name: 'code-review', spawnDepth: 1 });
    } else {
      selection.remember(toolCall(id), 'session', meta(id).parentAgentId);
      await save(id, 'meta.json', meta(id));
    }
    await selection.resolve(binding(id));
    const request = selection.begin('session', id);
    if (id === 'child' && childFailed) selection.failed('session', id, request);
    else selection.delivered('session', id, request, final(id).message);
  }
  await save('child', 'jsonl', [final('child', childFailed)]);
  await save('parent', 'jsonl', [final('parent'), notification(childFailed ? 'failed' : 'completed'), ...trailing]);
  return selection;
}
const outcome = async selection => {
  try { return (await selection.resolve(binding('parent')))?.source ?? 'no-source'; }
  catch (error) { return `reject:${error?.selectionReason ?? error?.completionFailure ?? error?.message}`; }
};

const grid = {};
try {
  await mkdir(dir, { recursive: true });
  for (const childFailed of [false, true]) {
    for (const [name, row] of Object.entries(trailingRows)) {
      const selection = await setup({ childFailed, trailing: row() });
      const live = await outcome(selection);
      // Restart: a new process holds no receipts and must not rebuild one from the records.
      const restarted = await outcome(createAgentSelection({ projectsRoot: root, timeoutMs: 35 }));

      assert.equal(live, resumes.includes(name) ? 'verified-completion-resume' : 'reject:CALL');
      // A trailing record must never manufacture a completion or a relay.
      assert.notEqual(live, 'verified-result-relay');
      assert.equal(restarted, 'reject:CALL');
      grid[`${childFailed ? 'failed' : 'ok'}/${name}`] = live; checks += 3;
    }
  }
  // The combination invariant: overlapping the legs changes no leg's verdict. A clean child
  // and a failed child reach the same outcome under every trailing shape, so the compaction
  // record and the failure do not interact.
  for (const name of Object.keys(trailingRows)) {
    assert.equal(grid[`failed/${name}`], grid[`ok/${name}`], name); checks++;
  }
} finally {
  clearTimeout(watchdog);
  await rm(root, { recursive: true, force: true });
}
process.stdout.write(JSON.stringify({ suite: 'combined-compaction-event', checks,
  resumePreservedBy: resumes, resumeBlockedBy: Object.keys(trailingRows).filter(name => !resumes.includes(name)),
  restartResumedFromRecordsAlone: 0, failureChangedAnyVerdict: false,
  externalRequests: 0, actualCredentialReads: 0, actualClaude: 0,
  measuredPairBlocksResume: true, subagentCompactionsObservedOnThisMachine: 0,
  notRun: ['compaction inside a parent agent transcript', 'default 400K/320K compaction firing'] }) + '\n');
