import assert from 'node:assert/strict';
import { mkdtempSync } from 'node:fs';
import { join, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';
import { spawnSync } from 'node:child_process';
import { initializeRecovery, recoveryOracle } from '../verification/unattended-recovery.mjs';

const project = dirname(dirname(fileURLToPath(import.meta.url)));
const script = join(project, 'verification', 'fixtures', 'native-recovery-mcp.mjs');
const root = mkdtempSync(join(project, '.tmp', 'native-recovery-mcp-'));
const manifest = initializeRecovery(root), work = join(root, 'work');
const call = (id, name, args = {}) => ({ jsonrpc: '2.0', id, method: 'tools/call', params: { name, arguments: args } });
const requests = [
  { jsonrpc: '2.0', id: 1, method: 'initialize', params: { protocolVersion: '2025-03-26' } },
  { jsonrpc: '2.0', id: 2, method: 'tools/list' },
  call(3, 'apply_effect', { path: '../oracle.json' }), call(4, 'effect_status'), call(5, 'complete_report'),
  call(6, 'apply_effect'), call(7, 'apply_effect'), call(8, 'effect_status'), call(9, 'complete_report'),
  call(10, 'complete_report'), call(11, 'arbitrary_command')
];
const env = Object.fromEntries(['SystemRoot', 'WINDIR'].filter(name => process.env[name]).map(name => [name, process.env[name]]));
const result = spawnSync(process.execPath, ['--permission', `--allow-fs-read=${script}`, `--allow-fs-read=${work}`,
  `--allow-fs-write=${work}`, script, work, manifest.operationId, 'return'], { cwd: work, env, windowsHide: true,
  encoding: 'utf8', input: requests.map(message => JSON.stringify(message)).join('\n') + '\n', timeout: 5000, maxBuffer: 16384 });
assert.equal(result.error, undefined); assert.equal(result.status, 0); assert.equal(result.stderr, '');
const replies = result.stdout.trim().split('\n').map(line => JSON.parse(line));
assert.deepEqual(replies.map(reply => reply.id), requests.map(request => request.id));
assert.equal(replies[0].result.protocolVersion, '2025-03-26');
assert.deepEqual(replies[1].result.tools.map(tool => tool.name), ['apply_effect', 'effect_status', 'complete_report']);
assert.equal(replies[2].error.code, -32602);
assert.equal(JSON.parse(replies[3].result.content[0].text).count, 0);
assert.equal(replies[4].result.isError, true);
for (const index of [5, 6, 7]) assert.deepEqual(JSON.parse(replies[index].result.content[0].text), { operationId: manifest.operationId, count: 1 });
for (const index of [8, 9]) assert.equal(JSON.parse(replies[index].result.content[0].text).completed, true);
assert.equal(replies[10].error.code, -32602);
assert.equal(recoveryOracle(root), true);
console.log(JSON.stringify({ suite: 'native-recovery-mcp', checks: 16, duplicateEffects: 0,
  externalRequests: 0, actualCredentialReads: 0, evidenceRoot: root }));
