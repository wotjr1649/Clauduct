import assert from 'node:assert/strict';
import { lstatSync, readFileSync, readdirSync, realpathSync, existsSync } from 'node:fs';
import { join, resolve, relative, isAbsolute, basename } from 'node:path';
import { fileURLToPath } from 'node:url';

export function verifyResultRelayEvidence(root) {
  root = resolve(root);
  function read(path, limit = 1048576) {
    const rel = relative(root, path), stat = lstatSync(path);
    assert.ok(rel && !isAbsolute(rel) && !rel.startsWith('..') && stat.isFile() && !stat.isSymbolicLink()
      && realpathSync(path).toLowerCase() === path.toLowerCase() && stat.size <= limit);
    return new TextDecoder('utf-8', { fatal: true }).decode(readFileSync(path));
  }
  const entry = JSON.parse(read(join(root, 'entry-result.json'), 16384));
  const prior = JSON.parse(read(join(root, 'result.json')));
  const { ids, mode, model, effort } = entry;
  assert.ok(['success', 'failure', 'cancel'].includes(mode) && ['sol', 'luna'].includes(model));
  assert.ok(Object.keys(ids).length === 3 && new Set(Object.values(ids)).size === 3
    && Object.values(ids).every(id => /^[A-Za-z0-9_-]{1,128}$/.test(id)));
  const projects = join(root, 'config', 'projects');
  const projectNames = readdirSync(projects); assert.ok(projectNames.length <= 16);
  const matches = projectNames.flatMap(name => {
    const project = join(projects, name), names = readdirSync(project); assert.ok(names.length <= 32);
    return names.filter(file => /^[a-f0-9-]{36}\.jsonl$/.test(file)
      && existsSync(join(project, basename(file, '.jsonl'), 'subagents', `agent-${ids.parent}.meta.json`)))
      .map(file => ({ project, file }));
  });
  assert.equal(matches.length, 1);
  const { project, file } = matches[0], sessionId = basename(file, '.jsonl');
  const rows = path => read(path).trimEnd().split('\n').map(JSON.parse);
  const main = rows(join(project, file));
  const agents = join(project, sessionId, 'subagents');
  assert.equal(readdirSync(agents).filter(name => name.endsWith('.meta.json')).length, 3);
  const agentRows = {};
  for (const [name, id] of Object.entries(ids)) {
    const metadata = JSON.parse(read(join(agents, `agent-${id}.meta.json`), 16384));
    assert.equal(metadata.agentType, 'clauduct-inherit');
    assert.equal(metadata.parentAgentId ?? undefined, name === 'parent' ? undefined : ids.parent);
    agentRows[name] = rows(join(agents, `agent-${id}.jsonl`));
    assert.ok(agentRows[name].filter(row => ['user', 'assistant'].includes(row.type)).every(row => row.sessionId === sessionId && row.agentId === id));
  }
  const tools = source => source.flatMap(row => row.type === 'assistant' && Array.isArray(row.message?.content)
    ? row.message.content.filter(block => block.type === 'tool_use') : []);
  const mainTools = tools(main), parentTools = tools(agentRows.parent);
  assert.equal(mainTools.filter(tool => tool.name === 'Agent').length, 1);
  assert.equal(parentTools.filter(tool => tool.name === 'Agent').length, 2);
  assert.equal(mainTools.filter(tool => tool.name === 'SendMessage').length, mode === 'cancel' ? 0 : 1);
  for (const tool of mainTools.filter(tool => tool.name === 'SendMessage')) assert.equal(tool.input.to, ids.parent);
  const collected = main.filter(row => row.type === 'user' && row.toolUseResult?.retrieval_status === 'success').map(row => row.toolUseResult.task);
  assert.ok(collected.some(task => task.task_id === ids.alpha && task.status === 'completed'));
  if (mode !== 'cancel') assert.ok(collected.some(task => task.task_id === ids.beta && task.status === (mode === 'failure' ? 'failed' : 'completed')));
  const writes = [...mainTools, ...parentTools].filter(tool => tool.name === 'Write');
  const report = JSON.stringify({ alpha: 'completed', beta: mode === 'failure' ? 'failed' : mode === 'cancel' ? 'cancelled' : 'completed', handled: true }) + '\n';
  assert.equal(writes.length, 1); assert.equal(writes[0].input.file_path, join(root, 'work', 'relay-report.json'));
  assert.equal(writes[0].input.content, report); assert.equal(read(join(root, 'work', 'relay-report.json')), report);
  const status = prior.status;
  assert.equal(Object.keys(status.cleanup).length, 9); assert.ok(Object.values(status.cleanup).every(value => value === true));
  assert.ok(prior.remaining.length === 0 && prior.exitCode === 0 && prior.timedOut === false);
  const relayed = status.recentRequests.filter(row => row.selectionSource === 'verified-result-relay');
  assert.equal(relayed.length, mode === 'cancel' ? 0 : 2);
  assert.ok(relayed.every(row => row.success && row.model === `gpt-5.6-${model}` && row.effort === effort));
  assert.equal(entry.childRequests.alpha, 1); assert.equal(entry.childRequests.beta, 1);
  if (mode === 'failure') {
    assert.ok(entry.failureInjected);
    assert.ok(status.recentRequests.some(row => !row.success && row.failureCategory === 'UPSTREAM_RESPONSE_FAILED'));
    assert.ok(agentRows.beta.some(row => row.isApiErrorMessage === true));
  }
  if (mode === 'cancel') assert.ok(entry.cancelObserved && !entry.relaySent && entry.parentRequests === 3);
  return { root, model, effort, mode, passed: true, source: 'actual-native-fixed-public-responses',
    actualBackendRequests: 0, credentialReads: 0, publicResponses: entry.requests, relayedRequests: relayed.length,
    nativeChildStarts: 2, fileWrites: writes.length, sameParent: true, sameSession: true, failurePreserved: mode === 'failure',
    parentNotResumedAfterCancel: mode === 'cancel', cleanupChecks: 9, remaining: 0, elapsedMs: prior.elapsedMs };
}
if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  try { process.stdout.write(JSON.stringify(verifyResultRelayEvidence(process.argv[2])) + '\n'); }
  catch { process.stderr.write('RESULT_RELAY_EVIDENCE_INVALID\n'); process.exitCode = 1; }
}
