import assert from 'node:assert/strict';
import { mkdtemp, mkdir, writeFile, rm } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import { join } from 'node:path';
import { createAgentSelection } from './agent-selection.mjs';
import { startNativeGateway } from './native-gateway.mjs';
import { registerBinding } from './agent-route.mjs';
import { NativeError } from './native-protocol.mjs';
import { request } from 'node:http';

const root = await mkdtemp(fileURLToPath(new URL('../.tmp/failed-completion-', import.meta.url)));
const directory = join(root, 'session', 'subagents');
const route = { model: 'gpt-5.6-luna', effort: 'max' };
const binding = id => ({ id, role: 'claude', sessionId: 'session', transcriptPath: join(root, 'session.jsonl'), nativeRegistered: true });
const meta = id => ({ agentType: 'claude', toolUseId: `call_${id}`, model: 'inherit', ...(id !== 'parent' && { parentAgentId: 'parent' }) });
const record = (id, failed = false) => ({ type: 'assistant', sessionId: 'session', agentId: id,
  timestamp: new Date().toISOString(), ...(failed && { isApiErrorMessage: true }),
  message: { id: `response_${id}`, role: 'assistant', stop_reason: failed ? 'stop_sequence' : 'end_turn', content: [] } });
const notice = (id, status = 'failed', uuid = `notice_${id}`) => ({ type: 'user', sessionId: 'session', agentId: 'parent',
  uuid, timestamp: new Date().toISOString(), isMeta: true, origin: { kind: 'task-notification' },
  message: { role: 'user', content: `<task-notification><task-id>${id}</task-id><tool-use-id>call_${id}</tool-use-id><status>${status}</status><summary>PUBLIC_TASK_RESULT</summary></task-notification>` } });
const save = (id, rows) => writeFile(join(directory, `agent-${id}.jsonl`), rows.map(row => JSON.stringify(row)).join('\n') + '\n');
async function setup({ deferChild = false } = {}) {
  const selection = createAgentSelection({ projectsRoot: root, timeoutMs: 5 });
  for (const id of ['parent', 'child', 'sibling']) {
    selection.remember({ content: [{ type: 'tool_use', id: `call_${id}`, name: 'Agent', input: { subagent_type: 'claude', model: 'inherit' } }] },
      'session', id === 'parent' ? undefined : 'parent', route);
    await writeFile(join(directory, `agent-${id}.meta.json`), JSON.stringify(meta(id)));
    if (id === 'child' && deferChild) continue;
    await selection.resolve(binding(id));
    const request = selection.begin('session', id);
    if (id === 'child') selection.failed('session', id, request);
    else selection.delivered('session', id, request, record(id).message);
    await save(id, [record(id, id === 'child')]);
  }
  await save('parent', [record('parent'), notice('child'), notice('sibling', 'completed')]);
  return selection;
}
let checks = 0;
try {
  await mkdir(directory, { recursive: true });
  {
    const selection = await setup();
    const result = await selection.resolve(binding('parent'));
    assert.equal(result.source, 'verified-completion-resume'); assert.deepEqual(result.route, route);
    selection.delivered('session', 'parent', selection.begin('session', 'parent'), record('parent').message);
    await save('parent', [notice('child', 'failed', 'replay')]);
    await assert.rejects(selection.resolve(binding('parent'))); checks++;
  }
  for (const fault of ['no-receipt', 'stale-receipt', 'successful-child', 'not-api-error', 'wrong-session', 'wrong-child',
    'wrong-origin', 'stopped-child', 'changed-model', 'future-error', 'old-error', 'killed', 'forged-completed', 'later-request',
    'missing-message', 'wrong-message-role', 'invalid-message-id', 'nonterminal-error']) {
    const selection = await setup();
    if (fault === 'no-receipt' || fault === 'later-request') selection.begin('session', 'child');
    if (fault === 'stale-receipt') {
      const old = selection.begin('session', 'child'); selection.begin('session', 'child');
      selection.failed('session', 'child', old);
    }
    if (fault === 'successful-child') selection.delivered('session', 'child', selection.begin('session', 'child'), record('child').message);
    if (['not-api-error', 'wrong-session', 'wrong-child', 'future-error', 'old-error',
      'missing-message', 'wrong-message-role', 'invalid-message-id', 'nonterminal-error'].includes(fault)) {
      const final = record('child', fault !== 'not-api-error');
      if (fault === 'wrong-session') final.sessionId = 'other';
      if (fault === 'wrong-child') final.agentId = 'sibling';
      if (fault === 'future-error') final.timestamp = new Date(Date.now() + 60000).toISOString();
      if (fault === 'old-error') final.timestamp = '2000-01-01T00:00:00.000Z';
      if (fault === 'missing-message') delete final.message;
      if (fault === 'wrong-message-role') final.message.role = 'user';
      if (fault === 'invalid-message-id') final.message.id = 'invalid id';
      if (fault === 'nonterminal-error') final.message.stop_reason = 'tool_use';
      await save('child', [final]);
    }
    if (fault === 'stopped-child' || fault === 'changed-model') await writeFile(join(directory, 'agent-child.meta.json'),
      JSON.stringify({ ...meta('child'), ...(fault === 'stopped-child' ? { stoppedByUser: true } : { model: 'sol' }) }));
    if (['wrong-origin', 'killed', 'forged-completed'].includes(fault)) {
      const row = notice('child', fault === 'killed' ? 'killed' : fault === 'forged-completed' ? 'completed' : 'failed');
      if (fault === 'wrong-origin') row.origin.kind = 'user';
      await save('parent', [row]);
    }
    await assert.rejects(selection.resolve(binding('parent')), undefined, fault); checks++;
  }
  {
    const selection = await setup();
    const results = await Promise.allSettled([selection.resolve(binding('parent')), selection.resolve(binding('parent'))]);
    assert.equal(results.filter(result => result.status === 'fulfilled').length, 1); checks++;
  }
  {
    const selection = await setup(), current = selection.begin('session', 'child');
    selection.delivered('session', 'child', current, record('child').message);
    selection.failed('session', 'child', current);
    await save('child', [record('child')]); await save('parent', [notice('child', 'completed')]);
    assert.equal((await selection.resolve(binding('parent'))).source, 'verified-completion-resume'); checks++;
  }
  {
    const selection = await setup(), current = selection.begin('session', 'child');
    selection.failed('session', 'child', current);
    selection.delivered('session', 'child', current, record('child').message);
    await save('child', [record('child', true)]); await save('parent', [notice('child')]);
    assert.equal((await selection.resolve(binding('parent'))).source, 'verified-completion-resume'); checks++;
  }
  // The request must fail through the real HTTP handler to create its receipt.
  // A failed notification without that call still has no authority to resume.
  {
    const selection = await setup({ deferChild: true });
    let sends = 0;
    const gateway = await startNativeGateway({ agentSelection: selection,
      admissionOptions: { freeBytes: () => 16 * 1024 ** 3 }, transport: {
        send: async () => { sends++; throw new NativeError('UPSTREAM_RESPONSE_FAILED'); },
        close: async () => {}, diagnostics: () => ({}) } });
    try {
      await registerBinding({ id: 'child', role: 'claude', stop: false, sessionId: 'session', transcriptPath: binding('child').transcriptPath },
        { ANTHROPIC_BASE_URL: `http://127.0.0.1:${gateway.port}`, ANTHROPIC_AUTH_TOKEN: gateway.clientHeaders().Authorization.slice(7) });
      const body = JSON.stringify({ model: 'luna', stream: true, max_tokens: 100,
        messages: [{ role: 'user', content: 'PUBLIC_CHILD_FAILURE' }] });
      const response = await new Promise((done, reject) => {
        const req = request({ host: '127.0.0.1', port: gateway.port, path: '/v1/messages', method: 'POST', agent: false,
          signal: AbortSignal.timeout(3000), headers: { ...gateway.clientHeaders(), 'anthropic-version': '2023-06-01',
            'content-type': 'application/json', 'content-length': Buffer.byteLength(body),
            'x-claude-code-session-id': 'session', 'x-claude-code-agent-id': 'child', 'x-claude-code-parent-agent-id': 'parent' } }, res => {
          let text = ''; res.on('data', chunk => { text += chunk; }); res.once('error', reject);
          res.once('end', () => done({ status: res.statusCode, text, complete: res.complete }));
        });
        req.once('error', reject); req.end(body);
      });
      assert.equal(response.status, 502); assert.equal(response.complete, true);
      assert.match(response.text, /UPSTREAM_RESPONSE_FAILED/); assert.equal(sends, 1);
      await save('child', [record('child', true)]);
      await save('parent', [record('parent'), notice('child'), notice('sibling', 'completed')]);
      assert.equal((await selection.resolve(binding('parent'))).source, 'verified-completion-resume');
      assert.equal(gateway.diagnostics().lifetime.failed, 1); assert.equal(gateway.diagnostics().lifetime.succeeded, 0);
      checks++;
    } finally {
      const cleanup = await gateway.close();
      assert.equal(cleanup.cleanupFailed, false); assert.equal(cleanup.activeSockets, 0); assert.equal(cleanup.activeJobs, 0);
    }
  }
} finally { await rm(root, { recursive: true, force: true }); }
console.log(JSON.stringify({ suite: 'failed-completion-resume', checks, actualModelRequests: 0, actualCredentialReads: 0 }));
