import { readFileSync, writeFileSync } from 'node:fs';
import { join, resolve } from 'node:path';
import { main } from '../src/clauduct.mjs';
import { NativeError } from '../src/native-protocol.mjs';

const [root, model, mode] = process.argv.slice(2);
if (process.argv.length !== 5 || resolve(root, 'work') !== process.cwd()
  || !['sol', 'luna'].includes(model) || !['success', 'failure', 'cancel'].includes(mode)) throw new Error('FIXTURE_ARGUMENTS');
const effort = model === 'sol' ? 'low' : 'max';
const evidence = { model, effort, mode, requests: 0, parentRequests: 0, mainRequests: 0,
  childRequests: { alpha: 0, beta: 0 }, actualBackendRequests: 0, credentialReads: 0,
  completed: false, failureInjected: false, cancelObserved: false, relaySent: false, reportMatched: false };
const calls = {}, ids = {};
let serial = 0, activeRequests = 0;
function response(body, name, input, text) {
  const id = `public_relay_${++serial}`;
  const item = name ? { type: 'function_call', id: `item_${id}`, call_id: `call_${id}`, name, arguments: JSON.stringify(input), status: 'completed' }
    : { type: 'message', id: `item_${id}`, role: 'assistant', status: 'completed', content: [{ type: 'output_text', text, annotations: [] }] };
  const events = [{ type: 'response.created', response: { id, status: 'in_progress' } },
    { type: 'response.output_item.added', output_index: 0, item: { ...item, status: 'in_progress', ...(name ? { arguments: '' } : { content: [] }) } },
    ...(name ? [{ type: 'response.function_call_arguments.delta', output_index: 0, item_id: item.id, delta: item.arguments },
      { type: 'response.function_call_arguments.done', output_index: 0, item_id: item.id, arguments: item.arguments }]
      : [{ type: 'response.output_text.delta', output_index: 0, item_id: item.id, content_index: 0, delta: text },
        { type: 'response.output_text.done', output_index: 0, item_id: item.id, content_index: 0, text }]),
    { type: 'response.output_item.done', output_index: 0, item },
    { type: 'response.completed', response: { id, status: 'completed', model: body.model, reasoning: body.reasoning,
      output: [item], usage: { input_tokens: 1, output_tokens: 1, total_tokens: 2 } } }];
  return { events, callId: item.call_id };
}
function use(body, key, name, input) { const result = response(body, name, input); calls[key] = result.callId; return result.events; }
function output(body, key) {
  const rows = body.input.filter(item => item.type === 'function_call_output' && item.call_id === calls[key]);
  if (rows.length !== 1 || !Array.isArray(rows[0].output) || rows[0].output.length > 4) throw new Error('FIXTURE_RESULT_BINDING');
  const text = rows[0].output.map(block => { if (block.type !== 'input_text' || typeof block.text !== 'string') throw new Error('FIXTURE_RESULT_BINDING'); return block.text; }).join('\n');
  if (text.length > 32768 || text.startsWith('Tool execution failed:')) throw new Error('FIXTURE_TOOL_FAILED');
  return text;
}
function observe(body, name) {
  const text = output(body, name), matches = [...text.matchAll(/^agentId: ([A-Za-z0-9_-]{1,128}) \(/gm)];
  if (!text.startsWith('Async agent launched successfully.') || matches.length !== 1 || ids[name] && ids[name] !== matches[0][1]) throw new Error('FIXTURE_AGENT_ID');
  ids[name] = matches[0][1];
  if (new Set(Object.values(ids)).size !== Object.keys(ids).length) throw new Error('FIXTURE_AGENT_COLLISION');
}
const report = JSON.stringify({ alpha: 'completed', beta: mode === 'failure' ? 'failed' : mode === 'cancel' ? 'cancelled' : 'completed', handled: true }) + '\n';
try {
  await main({ args: ['--model', model, '--effort', effort, '--verify-model-route', '--verify-request-limit', '24', '--gpt-agents',
    '-p', '--output-format', 'stream-json', '--verbose', '--tools', 'Agent,TaskOutput,TaskStop,SendMessage,Write',
    '--allowedTools', 'Agent,TaskOutput,TaskStop,SendMessage,Write', '--max-turns', '12', '--',
    'PUBLIC_RELAY_MAIN: collect the public nested task outcomes and complete the report. Do not restart a cancelled parent.'],
    openTransport: () => ({ close: async () => {}, diagnostics: () => ({ requestAttempts: evidence.requests, activeRequests, activeSockets: 0 }),
      send: async (body, signal) => {
        activeRequests++;
        try {
          signal.throwIfAborted();
          if (++evidence.requests > 24 || body.model !== `gpt-5.6-${model}` || body.reasoning?.effort !== effort) throw new Error('FIXTURE_ROUTE_BOUND');
          const texts = body.input.filter(item => item.role === 'user').flatMap(item => Array.isArray(item.content)
            ? item.content.filter(b => b.type === 'input_text').map(b => b.text) : []);
          const child = ['alpha', 'beta'].find(name => texts.includes(`PUBLIC_RELAY_CHILD_${name}`));
          if (child) {
            if (++evidence.childRequests[child] !== 1) throw new Error('FIXTURE_CHILD_RESTARTED');
            if (child === 'beta' && mode === 'failure') { evidence.failureInjected = true; throw new NativeError('UPSTREAM_RESPONSE_FAILED'); }
            if (child === 'beta' && mode === 'cancel') {
              let timer;
              try {
                await new Promise((done, reject) => {
                  signal.addEventListener('abort', done, { once: true });
                  timer = setTimeout(() => reject(new Error('FIXTURE_CANCEL_TIMEOUT')), 10000);
                });
                evidence.cancelObserved = signal.aborted; throw new NativeError('CANCELLED');
              } finally { clearTimeout(timer); }
            }
            return response(body, undefined, undefined, `PUBLIC_RELAY_RESULT_${child}`).events;
          }
          if (texts.includes('PUBLIC_RELAY_PARENT')) {
            const step = ++evidence.parentRequests;
            if (step > 1) observe(body, 'alpha');
            if (step > 2) observe(body, 'beta');
            if (step <= 2) {
              const name = step === 1 ? 'alpha' : 'beta';
              return use(body, name, 'Agent', { subagent_type: 'clauduct-inherit', description: `Public ${name} child`,
                prompt: `PUBLIC_RELAY_CHILD_${name}`, run_in_background: true, max_turns: 1 });
            }
            if (step === 3) return response(body, undefined, undefined, 'PUBLIC_PARENT_WAITING').events;
            if (mode === 'cancel' || !evidence.relaySent) throw new Error('FIXTURE_PARENT_UNEXPECTED_RESUME');
            if (step === 4) return use(body, 'report', 'Write', { file_path: join(root, 'work', 'relay-report.json'), content: report });
            if (step !== 5 || !output(body, 'report')) throw new Error('FIXTURE_PARENT_TURNS');
            return response(body, undefined, undefined, 'PUBLIC_PARENT_COMPLETED').events;
          }
          if (!texts.some(text => text.startsWith('PUBLIC_RELAY_MAIN:'))) throw new Error('FIXTURE_ROLE_UNKNOWN');
          const step = ++evidence.mainRequests;
          if (step === 1) return use(body, 'parent', 'Agent', { subagent_type: 'clauduct-inherit', description: 'Public result parent',
            prompt: 'PUBLIC_RELAY_PARENT', run_in_background: true, max_turns: 8 });
          observe(body, 'parent');
          if (step === 2) return use(body, 'parent-wait', 'TaskOutput', { task_id: ids.parent, block: true, timeout: 5000 });
          if (!output(body, 'parent-wait').includes('PUBLIC_PARENT_WAITING') || !ids.alpha || !ids.beta) throw new Error('FIXTURE_PARENT_INCOMPLETE');
          if (step === 3) return use(body, 'collect-alpha', 'TaskOutput', { task_id: ids.alpha, block: true, timeout: 5000 });
          if (step === 4) {
            if (!output(body, 'collect-alpha').includes('PUBLIC_RELAY_RESULT_alpha')) throw new Error('FIXTURE_ALPHA_INCOMPLETE');
            return use(body, 'collect-beta', mode === 'cancel' ? 'TaskStop' : 'TaskOutput',
              { task_id: ids.beta, ...(mode !== 'cancel' && { block: true, timeout: 5000 }) });
          }
          if (step === 5) {
            output(body, 'collect-beta');
            if (mode === 'cancel') return use(body, 'stop-parent', 'TaskStop', { task_id: ids.parent });
            evidence.relaySent = true;
            return use(body, 'resume-parent', 'SendMessage', { to: ids.parent,
              message: 'PUBLIC_COLLECTED_RESULTS: alpha completed; beta ' + (mode === 'failure' ? 'failed' : 'completed') + '. Write the outcome report and finish the original task.' });
          }
          if (step === 6) {
            if (mode === 'cancel') {
              // A completed parent may no longer be stoppable; no resume is sent.
              if (!evidence.cancelObserved) throw new Error('FIXTURE_CANCEL_NOT_OBSERVED');
              return use(body, 'report-cancel', 'Write', { file_path: join(root, 'work', 'relay-report.json'), content: report });
            }
            output(body, 'resume-parent');
            return use(body, 'parent-final', 'TaskOutput', { task_id: ids.parent, block: true, timeout: 5000 });
          }
          if (step !== 7 || mode !== 'cancel' && !output(body, 'parent-final').includes('PUBLIC_PARENT_COMPLETED')) throw new Error('FIXTURE_MAIN_INCOMPLETE');
          evidence.reportMatched = readFileSync(join(root, 'work', 'relay-report.json'), 'utf8') === report;
          if (!evidence.reportMatched) throw new Error('FIXTURE_REPORT_MISMATCH');
          evidence.completed = true;
          return response(body, undefined, undefined, 'PUBLIC_RELAY_COMPLETE').events;
        } finally { activeRequests--; }
      } }) });
} catch { evidence.failure = 'NATIVE_RESULT_RELAY_FAILED'; process.exitCode = 1; }
finally {
  if (!evidence.completed) { evidence.failure ??= 'NATIVE_RESULT_RELAY_INCOMPLETE'; process.exitCode = 1; }
  writeFileSync(join(root, 'entry-result.json'), JSON.stringify({ ...evidence, ids }) + '\n', { flag: 'wx' });
}
