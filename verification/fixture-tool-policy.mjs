import { resolve } from 'node:path';
import { NativeError } from '../src/native-protocol.mjs';
import { searchRequestBody } from '../src/native-search.mjs';

const need = ok => { if (!ok) throw new NativeError('VERIFICATION_TOOL_INPUT_REJECTED'); };
const fields = (value, allowed) => value && typeof value === 'object' && !Array.isArray(value)
  && Object.keys(value).every(key => allowed.includes(key));
const text = (value, maximum) => typeof value === 'string' && value.length > 0 && value.length <= maximum;

// Verification-only execution boundary. Model output is data; only these exact
// reviewed effects may be delivered to the native tools during a live fixture.
export function createFixtureToolPolicy(policy) {
  need(fields(policy, ['version', 'kind', 'workingRoot', 'readPath', 'workflowScript']) && policy.version === 1
    && ['agent', 'workflow', 'image', 'webfetch', 'websearch', 'none'].includes(policy.kind) && text(policy.workingRoot, 1024));
  if (['agent', 'image'].includes(policy.kind)) need(text(policy.readPath, 1024));
  else if (policy.kind === 'workflow') need(text(policy.workflowScript, 8192));
  const readPath = ['agent', 'image'].includes(policy.kind) ? resolve(policy.readPath).toLowerCase() : null;
  return event => {
    if (event?.type !== 'response.completed' || !Array.isArray(event.response?.output)) return;
    need(event.response.output.length <= 64);
    for (const item of event.response.output) {
      if (item?.type !== 'function_call') continue;
      need(policy.kind !== 'none');
      need(text(item.arguments, 65536));
      let input;
      try { input = JSON.parse(item.arguments); } catch { need(false); }
      if (policy.kind === 'agent' && item.name === 'Agent') {
        need(fields(input, ['description', 'prompt', 'subagent_type', 'run_in_background', 'max_turns'])
          && input.subagent_type === 'clauduct-probe-inherit' && (input.run_in_background === undefined || input.run_in_background === false)
          && text(input.prompt, 8192) && text(input.description, 256)
          && (input.max_turns === undefined || Number.isInteger(input.max_turns) && input.max_turns >= 1 && input.max_turns <= 3));
      } else if (['agent', 'image'].includes(policy.kind) && item.name === 'Read') {
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
      } else if (['agent', 'workflow'].includes(policy.kind) && item.name === 'TaskOutput') {
        need(fields(input, ['task_id', 'block', 'timeout']) && typeof input.task_id === 'string'
          && /^[A-Za-z0-9_-]{1,200}$/.test(input.task_id) && (input.block === undefined || input.block === true)
          && (input.timeout === undefined || Number.isInteger(input.timeout) && input.timeout >= 0 && input.timeout <= 120000));
      } else need(false);
    }
  };
}

export function guardFixtureTransport(transport, policy) {
  const check = createFixtureToolPolicy(policy);
  const usage = { inputTokens: 0, outputTokens: 0, completions: 0, maxInputTokens: 131072, maxOutputTokens: 32768, imageFormatMask: 0 };
  const inspect = event => {
    if (event?.type === 'response.completed') {
      const value = event.response?.usage;
      if (Number.isSafeInteger(value?.input_tokens) && value.input_tokens >= 0
        && Number.isSafeInteger(value?.output_tokens) && value.output_tokens >= 0) {
        usage.inputTokens += value.input_tokens; usage.outputTokens += value.output_tokens; usage.completions++;
      }
    }
    check(event);
  };
  const checkBudget = () => {
    if (usage.inputTokens >= usage.maxInputTokens || usage.outputTokens >= usage.maxOutputTokens) throw new NativeError('REQUEST_BUDGET');
  };
  return Object.freeze({ ...transport, fixtureUsage: () => ({ ...usage,
    requestAttempts: transport.diagnostics?.().requestAttempts ?? null }), search: async (body, signal) => {
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
    if (typeof options.onEvent !== 'function') {
      const events = await transport.send(body, signal, options);
      for (const event of events) inspect(event);
      return events;
    }
    return transport.send(body, signal, { ...options, onEvent: async event => { inspect(event); await options.onEvent(event); } });
  } });
}
