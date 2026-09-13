import { createInterface } from 'node:readline';
import { openSync, closeSync, writeFileSync, readFileSync, lstatSync, fstatSync, writeSync } from 'node:fs';
import { join, resolve } from 'node:path';

const [directory, operationId, hold, audit] = process.argv.slice(2);
if (!directory || !/^[0-9a-f-]{36}$/.test(operationId) || !['hold', 'return'].includes(hold)
  || audit !== undefined && audit !== 'audit' || process.argv.length > 6) throw new Error('INVALID_FIXTURE_ARGUMENTS');
const root = resolve(directory), rootStat = lstatSync(root);
if (!rootStat.isDirectory() || rootStat.isSymbolicLink()) throw new Error('INVALID_FIXTURE_ROOT');
const receiptPath = join(root, 'operation.json'), reportPath = join(root, 'report.json');
const receipt = JSON.stringify({ operationId, count: 1 });
function readReceipt() {
  try {
    const stat = lstatSync(receiptPath);
    if (!stat.isFile() || stat.isSymbolicLink() || stat.size > 1024) throw new Error('INVALID_RECEIPT');
    if (readFileSync(receiptPath, 'utf8') !== receipt) throw new Error('INVALID_RECEIPT');
    return 1;
  } catch (error) { if (error.code === 'ENOENT') return 0; throw error; }
}
function writeExclusive(path, text) {
  const fd = openSync(path, 'wx');
  try { writeFileSync(fd, text); } finally { closeSync(fd); }
}
const toolNames = ['apply_effect', 'effect_status', 'complete_report'];
const descriptions = {
  apply_effect: 'Apply the one public test operation once. Its receipt is idempotent. No parameters.',
  effect_status: 'Read the public test operation receipt, returning its count. No parameters.',
  complete_report: 'Complete the original report from the verified receipt. No parameters.'
};
const content = value => ({ content: [{ type: 'text', text: JSON.stringify(value) }] });
function recordCall(name) {
  if (audit !== 'audit') return;
  const path = join(root, 'mcp-events.jsonl');
  let before;
  try { before = lstatSync(path); }
  catch (error) { if (error.code !== 'ENOENT') throw error; }
  if (before && (!before.isFile() || before.isSymbolicLink() || before.size > 16300)) throw new Error('INVALID_CALL_AUDIT');
  const fd = openSync(path, before ? 'r+' : 'wx');
  try {
    const current = fstatSync(fd);
    if (!current.isFile() || current.size > 16300 || before &&
      (before.dev !== current.dev || before.ino !== current.ino || before.size !== current.size)) throw new Error('INVALID_CALL_AUDIT');
    // The append is before the effect, so an audit failure cannot silently allow it.
    const row = Buffer.from(JSON.stringify({ name }) + '\n');
    // A positional append keeps a replaced pathname from redirecting this descriptor.
    if (writeSync(fd, row, 0, row.length, current.size) !== row.length) throw new Error('INVALID_CALL_AUDIT');
  } finally { closeSync(fd); }
}
async function respond(message) {
  if (!Object.hasOwn(message, 'id')) return;
  if (!['number', 'string'].includes(typeof message.id)) return;
  const reply = { jsonrpc: '2.0', id: message.id };
  if (message.method === 'initialize' && /^20\d\d-\d\d-\d\d$/.test(message.params?.protocolVersion)) {
    return { ...reply, result: { protocolVersion: message.params.protocolVersion, capabilities: { tools: {} },
      serverInfo: { name: 'clauduct-recovery-fixture', version: '1.0.0' } } };
  }
  if (message.method === 'ping') return { ...reply, result: {} };
  if (message.method === 'tools/list') return { ...reply, result: { tools: toolNames.map(name => ({ name,
    description: descriptions[name], inputSchema: { type: 'object', properties: {}, additionalProperties: false } })) } };
  const args = message.params?.arguments ?? {};
  if (message.method !== 'tools/call' || !toolNames.includes(message.params?.name)
    || !args || typeof args !== 'object' || Array.isArray(args) || Object.keys(args).length !== 0) {
    return { ...reply, error: { code: -32602, message: 'INVALID_FIXTURE_REQUEST' } };
  }
  recordCall(message.params.name);
  if (message.params.name === 'apply_effect') {
    if (readReceipt() === 0) writeExclusive(receiptPath, receipt);
    // The manager kills this owned process tree after the receipt exists and
    // before an MCP result is written. A fixed timer is a second stop bound.
    if (hold === 'hold') await new Promise(done => setTimeout(done, 15000));
    return { ...reply, result: content({ operationId, count: readReceipt() }) };
  }
  if (message.params.name === 'effect_status') return { ...reply, result: content({ operationId, count: readReceipt() }) };
  if (readReceipt() !== 1) return { ...reply, result: { ...content({ state: 'MISSING_EFFECT' }), isError: true } };
  const report = JSON.stringify({ operationId, operationCount: 1, result: 'effect-reconciled' });
  try { writeExclusive(reportPath, report); }
  catch (error) {
    if (error.code !== 'EEXIST') throw error;
    const stat = lstatSync(reportPath);
    if (!stat.isFile() || stat.isSymbolicLink() || stat.size > 1024 || readFileSync(reportPath, 'utf8') !== report) throw new Error('INVALID_REPORT');
  }
  return { ...reply, result: content({ completed: true, operationCount: 1 }) };
}
const lines = createInterface({ input: process.stdin });
try {
  for await (const line of lines) {
    if (Buffer.byteLength(line) > 65536) throw new Error('FIXTURE_INPUT_LIMIT');
    const message = JSON.parse(line);
    if (!message || typeof message !== 'object' || Array.isArray(message)) throw new Error('INVALID_FIXTURE_REQUEST');
    const reply = await respond(message);
    if (reply) process.stdout.write(JSON.stringify(reply) + '\n');
  }
} catch {
  process.stderr.write('RECOVERY_FIXTURE_FAILED\n'); process.exitCode = 1;
} finally { lines.close(); }
