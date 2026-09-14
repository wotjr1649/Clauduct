import assert from 'node:assert/strict';
import { createServer } from 'node:http';
import { writeFileSync, renameSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { createNativeLoopbackTransport } from './native-transport.mjs';
import { createNativeCredentialSupplier } from '../poc/user-session.mjs';
import { readSmall } from '../verification/manual-http-probe.mjs';
import { temporaryDir } from '../verification/temporary-dir.mjs';

const project = dirname(dirname(fileURLToPath(import.meta.url)));
const root = temporaryDir(project, 'credential-recovery-');
const now = 1800000000000;
// Deliberately invalid synthetic signatures. Only the loopback fixture accepts
// these public values; no user auth file, real token or OAuth endpoint is used.
const token = (account, version, expired = false) => `public.${Buffer.from(JSON.stringify({
  exp: now / 1000 + (expired ? -60 : 3600), account, version })).toString('base64url')}.invalid`;
const cache = (account, version, expired = false) => JSON.stringify({ auth_mode: 'chatgpt', tokens: {
  access_token: token(account, version, expired), account_id: account } });
const good = 'data: {"type":"response.created"}\n\ndata: {"type":"response.completed"}\n\ndata: [DONE]\n\n';
const cases = ['normal-replace', '401-rotate', '401-stable', '401-account-change', '403', 'expired',
  'malformed', 'limited-401', 'cancel-supplier', 'cancel-initial-supplier', 'changed-between-requests'];
const rows = [], failures = [];
for (const method of ['send', 'search']) for (const kind of [...cases, ...(method === 'send' ? ['cancel-can-retry'] : [])]) {
  const path = join(root, `${method}-${kind}-public-cache.json`), next = join(root, `${method}-${kind}-public-next.json`);
  let version = 1, account = 'public-account', reads = 0, forced = 0, requests = 0, authMatched = true, bodyMatched = true;
  writeFileSync(path, kind === 'malformed' ? '{' : cache(account, version, kind === 'expired'), { flag: 'wx' });
  const replace = changed => {
    account = changed ? 'other-public-account' : account; version++;
    writeFileSync(next, cache(account, version), { flag: 'wx' }); renameSync(next, path);
  };
  const controller = new AbortController();
  const readSupplier = createNativeCredentialSupplier({ readConfig: () => 'cli_auth_credentials_store = "file"',
    readCredential: () => { reads++; return readSmall(path); }, runtimeCheck: () => {}, homeCheck: () => {}, now: () => now });
  const supplier = async options => {
    if (options.force) forced++;
    const value = await readSupplier(options);
    if (options.force && kind === 'cancel-supplier') controller.abort();
    if (!options.force && kind === 'cancel-initial-supplier') controller.abort();
    return value;
  };
  const server = createServer(async (req, res) => {
    requests++;
    authMatched &&= req.headers.authorization === `Bearer ${token(account, version)}`
      && req.headers['chatgpt-account-id'] === account;
    let length = 0, text = '';
    for await (const chunk of req) { length += chunk.length; if (length > 4096) { res.destroy(); return; } text += chunk.toString(); }
    let body; try { body = JSON.parse(text); } catch { bodyMatched = false; res.destroy(); return; }
    bodyMatched &&= body.publicTask === 'credential-continuity';
    if (requests > 2) { res.writeHead(400).end(); return; }
    if (kind === '403') { res.writeHead(403).end(); return; }
    if (kind.includes('401') || kind.startsWith('cancel-')) {
      if (requests === 1) {
        if (kind === '401-rotate' || kind === '401-account-change') replace(kind === '401-account-change');
        res.writeHead(401).end(); return;
      }
      if (kind !== '401-rotate') { res.writeHead(401).end(); return; }
    }
    res.writeHead(200, { 'Content-Type': method === 'send' ? 'text/event-stream' : 'application/json' });
    res.end(method === 'send' ? good : JSON.stringify({ output: 'PUBLIC_SEARCH', results: [] }));
  });
  await new Promise((done, reject) => { server.once('error', reject); server.listen(0, '127.0.0.1', done); });
  const transport = createNativeLoopbackTransport(server.address().port, { credentialSupplier: supplier,
    retryDelayMs: 0, timeoutMs: 1000, requestBudget: kind === 'limited-401' ? 1 : 4 });
  let category = 'SUCCESS', timer;
  const execute = () => method === 'search' ? transport.search({ publicTask: 'credential-continuity' }, controller.signal)
    : transport.send({ publicTask: 'credential-continuity' }, controller.signal,
      kind === 'cancel-can-retry' ? { canRetry: () => { controller.abort(); return true; } } : {});
  try {
    timer = setTimeout(() => controller.abort(), 3000);
    try {
      await execute();
      if (kind === 'normal-replace' || kind === 'changed-between-requests') { replace(kind === 'changed-between-requests'); await execute(); }
    } catch (error) {
      category = ['UNAUTHENTICATED', 'UPSTREAM_HTTP_ERROR', 'SEARCH_HTTP_ERROR', 'CREDENTIAL_ACCOUNT_CHANGED',
        'CREDENTIAL_UNAVAILABLE_OR_EXPIRED', 'REQUEST_BUDGET', 'CANCELLED'].includes(error.code) ? error.code : 'UNEXPECTED_ERROR';
    }
  } finally {
    clearTimeout(timer); await transport.close(); server.closeAllConnections(); await new Promise(done => server.close(done));
  }
  const expected = kind === '401-stable' ? ['UNAUTHENTICATED', 2, 1]
    : ['401-account-change', 'changed-between-requests'].includes(kind) ? ['CREDENTIAL_ACCOUNT_CHANGED', 1, kind === '401-account-change' ? 1 : 0]
    : kind === '403' ? [method === 'send' ? 'UPSTREAM_HTTP_ERROR' : 'SEARCH_HTTP_ERROR', 1, 0]
    : ['expired', 'malformed'].includes(kind) ? ['CREDENTIAL_UNAVAILABLE_OR_EXPIRED', 0, 0]
    : kind === 'limited-401' ? ['REQUEST_BUDGET', 1, 0]
    : kind === 'cancel-supplier' ? ['CANCELLED', 1, 1]
    : kind === 'cancel-initial-supplier' ? ['CANCELLED', 0, 0]
    : kind === 'cancel-can-retry' ? ['CANCELLED', 1, 0]
    : ['SUCCESS', 2, kind === '401-rotate' ? 1 : 0];
  const diagnostics = transport.diagnostics();
  const row = { method, kind, category, requests, attempts: diagnostics.requestAttempts, forced, reads, authMatched, bodyMatched,
    closed: diagnostics.closed && diagnostics.activeRequests === 0 && diagnostics.activeSockets === 0,
    secretFieldsAbsent: !JSON.stringify(diagnostics).includes('public-account') && !JSON.stringify(diagnostics).includes('public.') };
  for (const [field, actual, desired] of [['category', category, expected[0]], ['requests', requests, expected[1]],
    ['attempts', diagnostics.requestAttempts, expected[1]], ['forced', forced, expected[2]], ['closed', row.closed, true],
    ['authMatched', authMatched, true], ['bodyMatched', bodyMatched, true], ['secretFieldsAbsent', row.secretFieldsAbsent, true]]) {
    if (actual !== desired) failures.push({ method, kind, field, actual, expected: desired });
  }
  rows.push(row);
}
const result = { suite: 'credential-recovery', checks: rows.length, root, passed: failures.length === 0, failures,
  actualCredentialReads: 0, externalRequests: 0, normalOAuthRefresh: 'NOT_RUN', rows };
writeFileSync(join(root, 'result.json'), JSON.stringify(result) + '\n', { flag: 'wx' });
console.log(JSON.stringify(result));
assert.equal(failures.length, 0, 'CREDENTIAL_RECOVERY_BOUNDARY');
