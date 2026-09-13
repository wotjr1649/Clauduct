import { resolve } from 'node:path';
import { NativeError } from '../src/native-protocol.mjs';
import { searchRequestBody } from '../src/native-search.mjs';
import { verifyCompletionRelayTarget } from './completion-relay-target.mjs';
import { checkDevelopmentSource } from './development-source-policy.mjs';
import { developmentTask } from './development-tasks.mjs';

const need = ok => { if (!ok) throw new NativeError('VERIFICATION_TOOL_INPUT_REJECTED'); };
const fields = (value, allowed) => value && typeof value === 'object' && !Array.isArray(value)
  && Object.keys(value).every(key => allowed.includes(key));
const text = (value, maximum) => typeof value === 'string' && value.length > 0 && value.length <= maximum;

// Verification-only execution boundary. Model output is data; only these exact
// reviewed effects may be delivered to the native tools during a live fixture.
export function createFixtureToolPolicy(policy) {
  need(fields(policy, ['version', 'kind', 'workingRoot', 'readPath', 'workflowScript', 'parentPrompt', 'childPrompts', 'completionMode', 'phase', 'taskId']) && policy.version === 1
    && ['agent', 'completion', 'workflow', 'image', 'webfetch', 'websearch', 'none', 'recovery', 'development'].includes(policy.kind) && text(policy.workingRoot, 1024));
  if (['agent', 'completion', 'image'].includes(policy.kind)) need(text(policy.readPath, 1024));
  else if (policy.kind === 'workflow') need(text(policy.workflowScript, 8192));
  if (policy.kind === 'completion') need(text(policy.parentPrompt, 8192) && Array.isArray(policy.childPrompts)
    && policy.childPrompts.length === 2 && policy.childPrompts.every(value => text(value, 1024))
    && policy.childPrompts[0] !== policy.childPrompts[1]);
  need(policy.completionMode === undefined || policy.kind === 'completion' && ['foreground', 'fork', 'relay'].includes(policy.completionMode));
  need(policy.kind === 'recovery' ? ['effect', 'finish'].includes(policy.phase)
    : policy.kind === 'development' ? policy.phase === undefined || policy.phase === 'finish' : policy.phase === undefined);
  need(policy.taskId === undefined || policy.kind === 'development');
  if (policy.kind === 'development') { try { developmentTask(policy.taskId); } catch { need(false); } }
  const recoveryOrder = policy.phase === 'effect' ? ['mcp__fixture__apply_effect']
    : ['mcp__fixture__effect_status', 'mcp__fixture__complete_report'];
  const recoveryIds = new Set();
  let recoveryStep = 0;
  let developmentRead = false, developmentTests = 0, developmentWrites = 0, developmentLast = '';
  const readPath = ['agent', 'completion', 'image'].includes(policy.kind) ? resolve(policy.readPath).toLowerCase() : null;
  const completionCalls = new Set();
  const completionCallIds = new Map();
  let relaySent = false, relayCollected = false, relayTarget;
  return event => {
    if (event?.type !== 'response.completed' || !Array.isArray(event.response?.output)) return;
    need(event.response.output.length <= 64);
    if (['recovery', 'development'].includes(policy.kind)) need(event.response.output.filter(item => item?.type === 'function_call').length <= 1);
    for (const item of event.response.output) {
      if (item?.type !== 'function_call') continue;
      need(policy.kind !== 'none');
      need(text(item.arguments, 65536));
      let input;
      try { input = JSON.parse(item.arguments); } catch { need(false); }
      need(input && typeof input === 'object' && !Array.isArray(input));
      if (policy.kind === 'recovery') {
        need(recoveryStep < recoveryOrder.length && item.name === recoveryOrder[recoveryStep] && Object.keys(input).length === 0
          && typeof item.call_id === 'string' && /^[A-Za-z0-9_-]{1,200}$/.test(item.call_id) && !recoveryIds.has(item.call_id));
        recoveryIds.add(item.call_id); recoveryStep++;
      } else if (policy.kind === 'development') {
        need(typeof item.call_id === 'string' && /^[A-Za-z0-9_-]{1,200}$/.test(item.call_id) && !recoveryIds.has(item.call_id));
        if (item.name === 'mcp__fixture__read_task') {
          need(!developmentRead && Object.keys(input).length === 0); developmentRead = true;
        } else if (item.name === 'mcp__fixture__run_tests') {
          need(developmentRead && ['mcp__fixture__read_task', 'mcp__fixture__write_source'].includes(developmentLast)
            && developmentTests < (policy.phase === 'finish' ? 1 : 3) && Object.keys(input).length === 0); developmentTests++;
        } else if (item.name === 'mcp__fixture__write_source') {
          need(policy.phase !== 'finish' && developmentRead && developmentTests > 0 && developmentLast === 'mcp__fixture__run_tests'
            && developmentWrites < 2 && fields(input, ['code']) && typeof input.code === 'string');
          try { checkDevelopmentSource(input.code, policy.taskId); } catch { need(false); }
          developmentWrites++;
        } else need(false);
        recoveryIds.add(item.call_id); developmentLast = item.name;
      } else if (policy.kind === 'agent' && item.name === 'Agent') {
        need(fields(input, ['description', 'prompt', 'subagent_type', 'run_in_background', 'max_turns'])
          && input.subagent_type === 'clauduct-probe-inherit' && (input.run_in_background === undefined || input.run_in_background === false)
          && text(input.prompt, 8192) && text(input.description, 256)
          && (input.max_turns === undefined || Number.isInteger(input.max_turns) && input.max_turns >= 1 && input.max_turns <= 3));
      } else if (policy.kind === 'completion' && item.name === 'Agent') {
        const parent = input.subagent_type === 'clauduct-inherit' && input.prompt === policy.parentPrompt;
        const child = input.subagent_type === 'clauduct-probe-inherit' && policy.childPrompts.includes(input.prompt);
        need(fields(input, ['description', 'prompt', 'subagent_type', 'run_in_background', 'max_turns'])
          && (parent || child) && (policy.completionMode === 'fork' ? !Object.hasOwn(input, 'run_in_background') : input.run_in_background === (!parent || policy.completionMode === 'relay'))
          && text(input.description, 256)
          && (input.max_turns === undefined || Number.isInteger(input.max_turns) && input.max_turns >= 1 && input.max_turns <= (parent ? 6 : 3))
          && !completionCalls.has(input.prompt) && completionCalls.size < 3);
        if (policy.completionMode === 'relay') {
          need(typeof item.call_id === 'string' && /^[A-Za-z0-9_-]{1,200}$/.test(item.call_id)
            && ![...completionCallIds.values()].includes(item.call_id));
          completionCallIds.set(input.prompt, item.call_id);
        }
        completionCalls.add(input.prompt);
      } else if (policy.kind === 'completion' && policy.completionMode === 'relay' && item.name === 'SendMessage') {
        need(!relaySent && fields(input, ['to', 'message']) && input.message === 'PUBLIC_CHILDREN_COMPLETED');
        relayTarget = verifyCompletionRelayTarget({ workingRoot: policy.workingRoot, target: input.to,
          parentCall: completionCallIds.get(policy.parentPrompt), childCalls: policy.childPrompts.map(prompt => completionCallIds.get(prompt)) });
        relaySent = true;
      } else if (policy.kind === 'completion' && policy.completionMode === 'relay' && item.name === 'TaskOutput') {
        need(relaySent && !relayCollected && fields(input, ['task_id', 'block', 'timeout'])
          && input.task_id === relayTarget && input.block === true && input.timeout === 60000);
        relayCollected = true;
      } else if (['agent', 'completion', 'image'].includes(policy.kind) && item.name === 'Read') {
        need(fields(input, ['file_path', 'offset', 'limit']) && text(input.file_path, 1024)
          && resolve(policy.workingRoot, input.file_path).toLowerCase() === readPath
          && (input.offset === undefined || Number.isInteger(input.offset) && input.offset >= 0 && input.offset <= 2000)
          && (input.limit === undefined || Number.isInteger(input.limit) && input.limit > 0 && input.limit <= 2000));
      } else if (policy.kind === 'workflow' && item.name === 'Workflow') {
        need(fields(input, ['script', 'description']) && input.script === policy.workflowScript
          && (input.description === undefined || text(input.description, 256)));
      } else if (policy.kind === 'workflow' && item.name === 'StructuredOutput') {
        need(fields(input, ['sum']) && input.sum === 5);
      } else if (policy.kind === 'webfetch' && item.name === 'WebFetch') {
        need(fields(input, ['url', 'prompt']) && ['https://example.com', 'https://example.com/'].includes(input.url)
          && text(input.prompt, 2048));
      } else if (policy.kind === 'websearch' && item.name === 'WebSearch') {
        need(fields(input, ['query', 'allowed_domains', 'blocked_domains']) && input.query === 'Node.js documentation'
          && Array.isArray(input.allowed_domains) && input.allowed_domains.length === 1 && input.allowed_domains[0] === 'nodejs.org'
          && (input.blocked_domains === undefined || Array.isArray(input.blocked_domains) && input.blocked_domains.length === 0));
      } else if (['agent', 'completion', 'workflow'].includes(policy.kind) && item.name === 'TaskOutput') {
        need(fields(input, ['task_id', 'block', 'timeout']) && typeof input.task_id === 'string'
          && /^[A-Za-z0-9_-]{1,200}$/.test(input.task_id) && (input.block === undefined || input.block === true)
          && (input.timeout === undefined || Number.isInteger(input.timeout) && input.timeout >= 0 && input.timeout <= 120000));
      } else need(false);
    }
  };
}

export function guardFixtureTransport(transport, policy, { onUsage } = {}) {
  const check = createFixtureToolPolicy(policy);
  need(onUsage === undefined || typeof onUsage === 'function');
  const active = new Set();
  let requestsStarted = 0;
  const fixtureProgress = () => ({ requestsStarted, activeCount: active.size, truncated: active.size > 16,
    active: [...active].slice(0, 16).map(job => {
      const timing = job.timings.at(-1);
      return { request: job.request, elapsedMs: Math.max(0, Math.round(performance.now() - job.started)),
        attempt: job.timings.length, status: Number.isInteger(timing?.status) && timing.status >= 100 && timing.status <= 599 ? timing.status : null,
        phase: !timing ? 'credentials' : timing.endedMs !== null ? 'cleanup' : timing.requestFlushedMs === null ? 'request'
          : timing.headersMs === null ? 'headers' : timing.firstBodyMs === null ? 'body' : 'stream',
        sawCompletion: ['completed', 'done'].includes(timing?.terminalState) };
    }) });
  const usage = { inputTokens: 0, outputTokens: 0, completions: 0, maxInputTokens: 131072, maxOutputTokens: 32768, imageFormatMask: 0 };
  const snapshot = () => ({ ...usage, requestAttempts: transport.diagnostics?.().requestAttempts ?? null });
  const inspect = event => {
    if (event?.type === 'response.completed') {
      const value = event.response?.usage;
      if (Number.isSafeInteger(value?.input_tokens) && value.input_tokens >= 0
        && Number.isSafeInteger(value?.output_tokens) && value.output_tokens >= 0) {
        usage.inputTokens += value.input_tokens; usage.outputTokens += value.output_tokens; usage.completions++;
        onUsage?.(snapshot());
      }
    }
    check(event);
  };
  const checkBudget = () => {
    if (usage.inputTokens >= usage.maxInputTokens || usage.outputTokens >= usage.maxOutputTokens) throw new NativeError('REQUEST_BUDGET');
  };
  return Object.freeze({ ...transport, fixtureUsage: snapshot, fixtureProgress, search: async (body, signal) => {
    checkBudget();
    need(policy.kind === 'websearch' && ['gpt-5.6-luna', 'gpt-5.6-sol'].includes(body?.model));
    const expected = searchRequestBody(null, body.model, { query: 'Node.js documentation', allowed: ['nodejs.org'], blocked: null });
    need(JSON.stringify(body) === JSON.stringify(expected));
    return transport.search(expected, signal);
  }, send: async (body, signal, options = {}) => {
    checkBudget();
    for (const item of body.input ?? []) for (const block of [
      ...(Array.isArray(item.content) ? item.content : []),
      ...(item.type === 'function_call_output' && Array.isArray(item.output) ? item.output : [])
    ]) {
      if (block.type !== 'input_image') continue;
      const type = /^data:image\/(png|jpeg|gif|webp);base64,/.exec(block.image_url)?.[1];
      need(type !== undefined);
      usage.imageFormatMask |= { png: 1, jpeg: 2, gif: 4, webp: 8 }[type];
    }
    const job = { request: ++requestsStarted, started: performance.now(), timings: options.attemptTimings ?? [] };
    active.add(job);
    try {
      if (typeof options.onEvent !== 'function') {
        const events = await transport.send(body, signal, { ...options, attemptTimings: job.timings });
        for (const event of events) inspect(event);
        return events;
      }
      return await transport.send(body, signal, { ...options, attemptTimings: job.timings,
        onEvent: async event => { inspect(event); await options.onEvent(event); } });
    } finally { active.delete(job); }
  } });
}
