import { createServer } from 'node:http';
import { readFileSync, writeFileSync, lstatSync, existsSync } from 'node:fs';
import { join, resolve } from 'node:path';
import { main } from '../src/clauduct.mjs';
import { createNativeLoopbackTransport } from '../src/native-transport.mjs';

const [root] = process.argv.slice(2);
if (!root || process.argv.length !== 3 || resolve(root, 'work') !== process.cwd()) throw new Error('PERMISSION_ENTRY_ARGUMENTS');
const manifestPath = join(root, 'manifest.json'), info = lstatSync(manifestPath);
if (!info.isFile() || info.isSymbolicLink() || info.size > 16384) throw new Error('PERMISSION_MANIFEST');
const { model, effort, sessionId } = JSON.parse(readFileSync(manifestPath, 'utf8'));
if (!['luna', 'sol'].includes(model) || effort !== ({ luna: 'max', sol: 'low' })[model]
  || !/^[0-9a-f-]{36}$/.test(sessionId)) throw new Error('PERMISSION_MANIFEST');
const commands = Object.fromEntries(['allowed', 'denied'].map(label => [label,
  `node --permission --allow-fs-read=permission-worker.mjs --allow-fs-write=worker-started.jsonl --allow-fs-write=${label}-effect.json permission-worker.mjs ${label}`]));
const marker = 'PUBLIC_PERMISSION_DENIAL_VERIFIED';
const evidence = { requests: 0, routeMatched: true, allowedResultObserved: false, deniedResultObserved: false,
  allowedEffectObserved: false, deniedEffectAbsent: false, failure: null, actualModelRequests: 0, credentialReads: 0 };
let server, transport;
const need = (ok, code) => { if (!ok) throw new Error(code); };
function toolResult(body, callId) {
  const outputs = body.input.filter(item => item.type === 'function_call_output' && item.call_id === callId);
  need(outputs.length === 1 && Array.isArray(outputs[0].output), 'PERMISSION_RESULT_MISSING');
  return outputs[0].output.filter(item => item.type === 'input_text' && typeof item.text === 'string').map(item => item.text).join('\n');
}
function response(step, command) {
  const id = `public_permission_${step}`, itemId = `item_${step}`, callId = `call_permission_${step}`;
  const item = command ? { id: itemId, type: 'function_call', name: 'Bash', call_id: callId,
    arguments: JSON.stringify({ command, description: 'Public permission boundary fixture' }), status: 'completed' }
    : { id: itemId, type: 'message', role: 'assistant', status: 'completed',
      content: [{ type: 'output_text', text: marker, annotations: [] }] };
  const events = [{ type: 'response.created', response: { id, status: 'in_progress' } },
    { type: 'response.output_item.added', output_index: 0, item: command ? { ...item, arguments: '', status: 'in_progress' }
      : { ...item, content: [], status: 'in_progress' } },
    ...(command ? [
      { type: 'response.function_call_arguments.delta', output_index: 0, item_id: itemId, delta: item.arguments },
      { type: 'response.function_call_arguments.done', output_index: 0, item_id: itemId, arguments: item.arguments }
    ] : [
      { type: 'response.output_text.delta', output_index: 0, item_id: itemId, content_index: 0, delta: marker },
      { type: 'response.output_text.done', output_index: 0, item_id: itemId, content_index: 0, text: marker }
    ]), { type: 'response.output_item.done', output_index: 0, item },
    { type: 'response.completed', response: { id, status: 'completed', model: `gpt-5.6-${model}`, reasoning: { effort },
      output: [item], usage: { input_tokens: 1, output_tokens: 1, total_tokens: 2 } } }];
  return events.map(event => `data: ${JSON.stringify(event)}\n\n`).join('') + 'data: [DONE]\n\n';
}
const known = ['PERMISSION_REQUEST', 'PERMISSION_ROUTE', 'PERMISSION_TOOLS', 'PERMISSION_RESULT_MISSING',
  'PERMISSION_ALLOWED_FAILED', 'PERMISSION_DENIAL_MISSING', 'PERMISSION_EFFECT_MISMATCH'];
try {
  server = createServer((req, res) => {
    void (async () => {
      need(++evidence.requests <= 3 && req.method === 'POST' && req.url === '/backend-api/codex/responses', 'PERMISSION_REQUEST');
      let bytes = 0, text = '';
      for await (const chunk of req) { bytes += chunk.length; need(bytes <= 1048576, 'PERMISSION_REQUEST'); text += chunk.toString(); }
      const body = JSON.parse(text);
      evidence.routeMatched &&= body.model === `gpt-5.6-${model}` && body.reasoning?.effort === effort;
      need(evidence.routeMatched, 'PERMISSION_ROUTE');
      need(Array.isArray(body.input) && Array.isArray(body.tools) && body.tools.some(tool => tool.name === 'Bash'), 'PERMISSION_TOOLS');
      const step = evidence.requests;
      if (step >= 2) {
        const output = toolResult(body, 'call_permission_1');
        evidence.allowedResultObserved = output.includes('PUBLIC_EFFECT_ALLOWED') && !output.includes('Tool execution failed:');
        need(evidence.allowedResultObserved, 'PERMISSION_ALLOWED_FAILED');
      }
      if (step === 3) {
        const output = toolResult(body, 'call_permission_2');
        evidence.deniedResultObserved = output.startsWith('Tool execution failed:') && /permission|denied|not allowed/i.test(output);
        need(evidence.deniedResultObserved, 'PERMISSION_DENIAL_MISSING');
        const allowed = JSON.parse(readFileSync(join(root, 'work', 'allowed-effect.json'), 'utf8'));
        evidence.allowedEffectObserved = allowed.label === 'allowed' && allowed.event === 'performed';
        evidence.deniedEffectAbsent = !existsSync(join(root, 'work', 'denied-effect.json'));
        need(evidence.allowedEffectObserved && evidence.deniedEffectAbsent, 'PERMISSION_EFFECT_MISMATCH');
      }
      res.writeHead(200, { 'Content-Type': 'text/event-stream' });
      res.end(response(step, step === 1 ? commands.allowed : step === 2 ? commands.denied : null));
    })().catch(error => { evidence.failure ??= known.includes(error.message) ? error.message : 'PERMISSION_HANDLER_FAILED'; res.writeHead(400).end(); });
  });
  await new Promise((done, reject) => { server.once('error', reject); server.listen(0, '127.0.0.1', done); });
  await main({ args: ['-p', '--model', model, '--effort', effort, '--verify-model-route', '--verify-request-limit', '3',
    '--session-id', sessionId, '--output-format', 'stream-json', '--verbose', '--tools', 'Bash',
    '--allowedTools', `Bash(${commands.allowed})`, '--disallowedTools', `Bash(${commands.denied})`, '--max-turns', '4', '--',
    `Run the fixed allowed public worker once. Verify that the denied public worker is refused and creates no effect. Do not retry or bypass its denial. Report ${marker} only after the refusal is established.`],
    openTransport: () => { transport = createNativeLoopbackTransport(server.address().port, { requestBudget: 3 }); return transport; } });
} catch { evidence.failure ??= 'PERMISSION_ENTRY_FAILED'; process.exitCode = 1; }
finally {
  await transport?.close();
  if (server) { server.closeAllConnections(); await new Promise(done => server.close(done)); }
  writeFileSync(join(root, 'permission-evidence.json'), JSON.stringify(evidence) + '\n', { flag: 'wx' });
}
