import { readFileSync, writeFileSync, readdirSync, lstatSync, realpathSync } from 'node:fs';
import { join, resolve, relative, isAbsolute, dirname } from 'node:path';
import { createHash } from 'node:crypto';
import { main } from '../src/clauduct.mjs';
import { NativeError } from '../src/native-protocol.mjs';
const [root, phase] = process.argv.slice(2);
if (process.argv.length !== 4 || !['first', 'resume'].includes(phase) || resolve(root, 'work') !== process.cwd()) throw new Error('WORKFLOW_FIXTURE_ARGUMENTS');
function read(path, limit = 2097152) {
  const rel = relative(root, path), stat = lstatSync(path);
  if (!rel || isAbsolute(rel) || rel.startsWith('..') || !stat.isFile() || stat.isSymbolicLink() || stat.size > limit
    || realpathSync(path).toLowerCase() !== path.toLowerCase()) throw new Error('WORKFLOW_FIXTURE_PATH');
  return readFileSync(path, 'utf8');
}
const manifest = JSON.parse(read(join(root, 'manifest.json'), 8192));
const { model, sessionId } = manifest, effort = model === 'sol' ? 'low' : 'max';
if (!['sol', 'luna'].includes(model) || !/^[a-f0-9-]{36}$/.test(sessionId)) throw new Error('WORKFLOW_FIXTURE_MANIFEST');
const schema = { type: 'object', properties: { sum: { type: 'number' } }, required: ['sum'], additionalProperties: false };
const script = `export const meta = { name: 'public-stored-restart', description: 'Complete public arithmetic from saved results' }; const cached = await agent('PUBLIC_CACHED_STEP: compute 2 + 3', ${JSON.stringify({ label: 'cached', model, schema })}); const retried = await agent('PUBLIC_RETRY_STEP: compute 3 + 4', ${JSON.stringify({ label: 'retry', model, schema })}); return { cached: cached.sum, retried: retried.sum, total: cached.sum + retried.sum };`;
const checkpoint = '{"seed":5,"stage":"created"}\n';
const report = '{"cached":5,"retried":7,"total":12}\n';
const evidence = { phase, model, effort, sessionId, requests: 0, mainRequests: 0, cachedAgentRequests: 0, retryAgentRequests: 0,
  failureInjected: false, launched: false, completed: false, effectConfirmed: false, actualBackendRequests: 0, credentialReads: 0 };
let activeRequests = 0, serial = 0;
const calls = {};
const digest = text => createHash('sha256').update(text).digest('hex');
const save = (name, value) => writeFileSync(join(root, name), JSON.stringify(value) + '\n', { flag: 'wx' });
function response(body, name, input, text) {
  const id = `public_workflow_${phase}_${++serial}`;
  const item = name ? { type: 'function_call', id: `item_${id}`, call_id: `call_${id}`, name, arguments: JSON.stringify(input), status: 'completed' }
    : { type: 'message', id: `item_${id}`, role: 'assistant', status: 'completed', content: [{ type: 'output_text', text, annotations: [] }] };
  return { callId: item.call_id, events: [{ type: 'response.created', response: { id, status: 'in_progress' } },
    { type: 'response.output_item.added', output_index: 0, item: { ...item, status: 'in_progress', ...(name ? { arguments: '' } : { content: [] }) } },
    ...(name ? [{ type: 'response.function_call_arguments.delta', output_index: 0, item_id: item.id, delta: item.arguments },
      { type: 'response.function_call_arguments.done', output_index: 0, item_id: item.id, arguments: item.arguments }]
      : [{ type: 'response.output_text.delta', output_index: 0, item_id: item.id, content_index: 0, delta: text },
        { type: 'response.output_text.done', output_index: 0, item_id: item.id, content_index: 0, text }]),
    { type: 'response.output_item.done', output_index: 0, item },
    { type: 'response.completed', response: { id, status: 'completed', model: body.model, reasoning: body.reasoning,
      output: [item], usage: { input_tokens: 1, output_tokens: 1, total_tokens: 2 } } }] };
}
const use = (body, key, name, input) => { const r = response(body, name, input); calls[key] = r.callId; return r.events; };
function output(body, key) {
  const matches = body.input.filter(item => item.type === 'function_call_output' && item.call_id === calls[key]);
  if (matches.length !== 1 || !Array.isArray(matches[0].output)) throw new Error('WORKFLOW_RESULT_MISSING');
  const text = matches[0].output.map(item => {
    if (item.type !== 'input_text' || typeof item.text !== 'string') throw new Error('WORKFLOW_RESULT_SHAPE');
    return item.text;
  }).join('\n');
  if (text.length > 32768) throw new Error('WORKFLOW_RESULT_BOUND');
  if (text.startsWith('Tool execution failed:')) throw new Error('WORKFLOW_TOOL_REFUSED');
  return text;
}
function launched(body, key) {
  // Native may flush its tool-result transcript after the next model request.
  // Parse the already returned bounded result, then independently compare it
  // with the structured transcript once the owned worker has stopped.
  const text = output(body, key);
  const tasks = [...text.matchAll(/Task ID: ([A-Za-z0-9_-]{1,200})(?=\s|$)/g)];
  const runs = [...text.matchAll(/Run ID: (wf_[a-z0-9-]{6,})(?=\s|$)/g)];
  const paths = [...new Set([...text.matchAll(/scriptPath:\s*"([^"\r\n]{1,4096})"/g)].map(match => match[1]))];
  if (tasks.length !== 1 || runs.length !== 1 || paths.length !== 1 || !isAbsolute(paths[0])
    || read(paths[0], 8192) !== script) throw new Error('WORKFLOW_LAUNCH_SHAPE');
  return { runId: runs[0][1], taskId: tasks[0][1], scriptPath: paths[0],
    transcriptDir: join(dirname(dirname(dirname(paths[0]))), 'subagents', 'workflows', runs[0][1]), sessionId, scriptDigest: digest(script) };
}
const old = phase !== 'first' ? JSON.parse(read(join(root, 'first-state.json'), 8192)) : null;
if (old && (old.sessionId !== sessionId || old.scriptDigest !== digest(script) || read(old.scriptPath, 8192) !== script)) throw new Error('WORKFLOW_SAVED_STATE_CHANGED');
let current;
try {
  await main({ args: ['-p', '--model', model, '--effort', effort, '--verify-model-route', '--verify-request-limit', '12',
    phase === 'first' ? '--session-id' : '--resume', sessionId, '--output-format', 'stream-json', '--verbose',
    '--tools', 'Read,Write,Workflow,TaskOutput', '--allowedTools', 'Read,Write,Workflow,TaskOutput', '--max-turns', '8', '--',
    phase === 'first' ? 'PUBLIC_WORKFLOW_MAIN: preserve a public checkpoint, run both arithmetic steps, and pause if the second step fails.'
      : 'PUBLIC_WORKFLOW_MAIN: inspect the existing checkpoint, resume the same saved workflow, reuse its completed step, and finish the original arithmetic report.'],
    openTransport: () => ({ close: async () => {}, diagnostics: () => ({ requestAttempts: evidence.requests, activeRequests, activeSockets: 0 }),
      send: async (body, signal) => {
        activeRequests++;
        try {
          signal.throwIfAborted();
          if (++evidence.requests > 12 || body.model !== `gpt-5.6-${model}` || body.reasoning?.effort !== effort) throw new Error('WORKFLOW_ROUTE_BOUND');
          const texts = body.input.filter(item => item.role === 'user').flatMap(item => Array.isArray(item.content) ? item.content.filter(part => part.type === 'input_text').map(part => part.text) : []);
          if (body.tools?.some(tool => tool.name === 'StructuredOutput') && !body.tools.some(tool => tool.name === 'Workflow')) {
            const cached = texts.some(text => text.includes('PUBLIC_CACHED_STEP'));
            const retry = texts.some(text => text.includes('PUBLIC_RETRY_STEP'));
            if (cached === retry) throw new Error('WORKFLOW_CHILD_IDENTITY');
            if (cached) evidence.cachedAgentRequests++; else evidence.retryAgentRequests++;
            if (cached && (phase !== 'first' || evidence.cachedAgentRequests !== 1) || retry && evidence.retryAgentRequests !== 1) throw new Error('WORKFLOW_UNEXPECTED_REEXECUTION');
            if (retry && phase === 'first') { evidence.failureInjected = true; throw new NativeError('UPSTREAM_RESPONSE_FAILED'); }
            return response(body, 'StructuredOutput', { sum: cached ? 5 : 7 }).events;
          }
          const step = ++evidence.mainRequests;
          if (phase === 'first') {
            if (step === 1) return use(body, 'checkpoint', 'Write', { file_path: join(root, 'work', 'checkpoint.json'), content: checkpoint });
            if (step === 2) { output(body, 'checkpoint'); return use(body, 'workflow', 'Workflow', { script }); }
            if (step === 3) {
              current = launched(body, 'workflow'); evidence.launched = true; save('first-state.json', current);
              return use(body, 'collected', 'TaskOutput', { task_id: current.taskId, block: true, timeout: 10000 });
            }
            if (step !== 4 || !output(body, 'collected') || !evidence.failureInjected || evidence.cachedAgentRequests !== 1 || evidence.retryAgentRequests !== 1) throw new Error('WORKFLOW_FIRST_PHASE_INCOMPLETE');
            evidence.effectConfirmed = read(join(root, 'work', 'checkpoint.json'), 1024) === checkpoint;
            evidence.completed = evidence.effectConfirmed;
            return response(body, undefined, undefined, 'PUBLIC_WORKFLOW_PAUSED_AFTER_SAVED_RESULT').events;
          }
          if (step === 1) return use(body, 'checkpoint-read', 'Read', { file_path: join(root, 'work', 'checkpoint.json') });
          if (step === 2) {
            evidence.effectConfirmed = output(body, 'checkpoint-read').includes(checkpoint.trim()) && read(join(root, 'work', 'checkpoint.json'), 1024) === checkpoint;
            if (!evidence.effectConfirmed) throw new Error('WORKFLOW_EFFECT_UNKNOWN');
            return use(body, 'workflow', 'Workflow', { scriptPath: old.scriptPath, resumeFromRunId: old.runId });
          }
          if (step === 3) {
            current = launched(body, 'workflow'); evidence.launched = true; evidence.sameRun = current.runId === old.runId;
            save(`${phase}-state.json`, current);
            return use(body, 'collected', 'TaskOutput', { task_id: current.taskId, block: true, timeout: 10000 });
          }
          if (step === 4) {
            if (!output(body, 'collected').includes('12') || evidence.cachedAgentRequests !== 0 || evidence.retryAgentRequests !== 1) throw new Error('WORKFLOW_RESUME_INCOMPLETE');
            return use(body, 'report', 'Write', { file_path: join(root, 'work', 'report.json'), content: report });
          }
          if (step !== 5 || !output(body, 'report') || read(join(root, 'work', 'report.json'), 1024) !== report) throw new Error('WORKFLOW_REPORT_INCOMPLETE');
          evidence.completed = true;
          return response(body, undefined, undefined, 'PUBLIC_WORKFLOW_ORIGINAL_TASK_COMPLETED').events;
        } catch (error) {
          if (!(error instanceof NativeError)) evidence.failure ??= /^WORKFLOW_[A-Z_]+$/.test(error.message) ? error.message : 'WORKFLOW_HANDLER_FAILED';
          throw error;
        } finally { activeRequests--; }
      } }) });
} catch (error) { evidence.failure ??= /^WORKFLOW_[A-Z_]+$/.test(error.message) ? error.message : 'WORKFLOW_NATIVE_FAILED'; process.exitCode = 1; }
finally {
  if (!evidence.completed) { evidence.failure ??= 'WORKFLOW_INCOMPLETE'; process.exitCode = 1; }
  save(`${phase}-entry.json`, evidence);
}
