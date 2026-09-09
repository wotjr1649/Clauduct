import { request } from 'node:http';
import { resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { createHash } from 'node:crypto';

export function contextFromEnvironment(source) {
  const fields = { window: 'CLAUDE_CODE_MAX_CONTEXT_TOKENS', autoCompactWindow: 'CLAUDE_CODE_AUTO_COMPACT_WINDOW',
    compactPercent: 'CLAUDE_AUTOCOMPACT_PCT_OVERRIDE' };
  const result = Object.fromEntries(Object.entries(fields).map(([key, env]) => [key,
    typeof source?.[env] === 'string' && /^\d+(\.\d+)?$/.test(source[env]) ? Number(source[env]) : NaN]));
  return Object.values(result).every(value => Number.isFinite(value) && value > 0)
    && Number.isSafeInteger(result.window) && Number.isSafeInteger(result.autoCompactWindow)
    && result.compactPercent <= 100 ? result : null;
}

export function bindingFrom(input, source) {
  if (input?.hook_event_name === 'PostToolUse' && input.tool_name === 'Workflow') {
    const result = input.tool_response, script = input.tool_input?.script;
    if (result?.status !== 'async_launched' || result.taskType !== 'local_workflow'
      || typeof script !== 'string' || input.tool_input.scriptPath !== undefined
      || input.tool_input.name !== undefined || input.tool_input.resumeFromRunId !== undefined) return null;
    const valid = value => typeof value === 'string' && /^[A-Za-z0-9_-]{1,200}$/.test(value);
    if (!valid(input.session_id) || !valid(input.tool_use_id) || !valid(result.taskId)
      || typeof result.runId !== 'string' || !/^wf_[a-z0-9-]{6,}$/.test(result.runId)
      || typeof result.workflowName !== 'string' || !/^[A-Za-z0-9_-]{1,100}$/.test(result.workflowName)
      || ![input.transcript_path, result.transcriptDir, result.scriptPath].every(value => typeof value === 'string' && value.length <= 4096)
      || (input.agent_id !== undefined && !valid(input.agent_id)) || Buffer.byteLength(script) > 524288) throw new Error('INVALID_AGENT_BINDING');
    return { kind: 'workflow-result', sessionId: input.session_id, toolUseId: input.tool_use_id,
      taskId: result.taskId, runId: result.runId, workflowName: result.workflowName,
      transcriptPath: input.transcript_path, transcriptDir: result.transcriptDir, scriptPath: result.scriptPath,
      scriptDigest: createHash('sha256').update(script).digest('hex'), ...(input.agent_id && { parent: input.agent_id }) };
  }
  if (input?.hook_event_name === 'PostToolUse' && input.tool_name === 'SendMessage') {
    const valid = value => typeof value === 'string' && /^[A-Za-z0-9_-]{1,200}$/.test(value);
    if (input.tool_response?.success !== true || typeof input.tool_input?.message !== 'string' || !input.tool_input.message.trim()) return null;
    if (!valid(input.session_id) || !valid(input.tool_use_id) || !valid(input.tool_input.to)
      || (input.agent_id !== undefined && !valid(input.agent_id))) return null;
    return { kind: 'resume-result', sessionId: input.session_id, toolUseId: input.tool_use_id,
      id: input.tool_input.to, ...(input.agent_id && { parent: input.agent_id }) };
  }
  if (input?.hook_event_name === 'PostToolUse' && input.tool_name === 'Skill') {
    const result = input.tool_response;
    if (!result || result.status !== 'forked' || result.background !== true || result.success !== true) return null;
    const valid = value => typeof value === 'string' && /^[A-Za-z0-9_-]{1,200}$/.test(value);
    if (!valid(input.session_id) || !valid(input.tool_use_id) || !valid(result.agentId)
      || (input.agent_id !== undefined && !valid(input.agent_id))
      || typeof result.commandName !== 'string' || !result.commandName || result.commandName.length > 200) throw new Error('INVALID_AGENT_BINDING');
    return { kind: 'skill-result', sessionId: input.session_id, toolUseId: input.tool_use_id,
      id: result.agentId, skill: result.commandName, ...(input.agent_id && { parent: input.agent_id }) };
  }
  if (!input || !['SubagentStart', 'SubagentStop'].includes(input.hook_event_name)
    || typeof input.agent_id !== 'string' || !/^[A-Za-z0-9_-]{1,200}$/.test(input.agent_id)
    || typeof input.agent_type !== 'string' || input.agent_type.length > 200) throw new Error('INVALID_AGENT_BINDING');
  // Only the native transcript location is forwarded, never its content or prompts.
  const stop = input.hook_event_name === 'SubagentStop';
  const contextPolicy = !stop && source ? contextFromEnvironment(source) : null;
  const location = !stop && typeof input.session_id === 'string' && typeof input.transcript_path === 'string'
    ? { sessionId: input.session_id, transcriptPath: input.transcript_path } : {};
  return { id: input.agent_id, role: input.agent_type, stop, ...location, ...(contextPolicy && { contextPolicy }) };
}
export async function registerBinding(binding, source) {
  const base = source.ANTHROPIC_BASE_URL;
  if (typeof base !== 'string' || !/^http:\/\/127\.0\.0\.1:[0-9]{1,5}$/.test(base)) throw new Error('INVALID_GATEWAY');
  const port = Number(new URL(base).port);
  if (port < 1 || port > 65535 || typeof source.ANTHROPIC_AUTH_TOKEN !== 'string') throw new Error('INVALID_GATEWAY');
  const raw = JSON.stringify(binding);
  await new Promise((done, reject) => {
    const req = request({ hostname: '127.0.0.1', port, method: 'POST', path: '/clauduct/agents', agent: false,
      signal: AbortSignal.timeout(3000), headers: { Authorization: `Bearer ${source.ANTHROPIC_AUTH_TOKEN}`,
        'Content-Type': 'application/json', 'Content-Length': Buffer.byteLength(raw) } }, res => {
      res.resume(); res.on('error', reject); res.on('end', () => res.statusCode === 200 ? done() : reject(new Error('REGISTRATION_FAILED')));
    });
    req.on('error', reject); req.end(raw);
  });
}
if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const timer = setTimeout(() => { process.stderr.write('CLAUDUCT_AGENT_ROUTE_TIMEOUT\n'); process.exit(1); }, 5000);
  try {
    let raw = '';
    for await (const chunk of process.stdin) { raw += chunk; if (Buffer.byteLength(raw) > 1048576) throw new Error('INPUT_TOO_LARGE'); }
    const binding = bindingFrom(JSON.parse(raw), process.env);
    if (binding) await registerBinding(binding, process.env);
  } catch { process.stderr.write('CLAUDUCT_AGENT_ROUTE_FAILED\n'); process.exitCode = 1; }
  finally { clearTimeout(timer); }
}
