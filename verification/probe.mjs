// Bounded protocol probe. Capture mode uses a local rejecting endpoint, never OpenAI inference.
import { spawn, spawnSync } from 'node:child_process';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { createServer } from 'node:http';
import { gunzipSync, zstdDecompressSync } from 'node:zlib';
import assert from 'node:assert/strict';

const root = fileURLToPath(new URL('../', import.meta.url));
const executable = process.argv[2];
assert(executable && /^[A-Za-z]:[\\/].*codex\.exe$/i.test(executable), 'Absolute codex.exe path required');
const profiles = { 'gpt-5.6-sol': 'xhigh', 'gpt-5.6-terra': 'xhigh', 'gpt-5.6-luna': 'max', 'gpt-6-astra': 'xhigh' };
const mode = process.argv[3] ?? '--catalog';
assert(['--catalog', '--inventory', '--thread-check', '--capture-tools', '--capture-default', '--capture-restricted', '--capture-classic', '--fixture-roundtrip', '--fixture-large', '--fixture-cancel'].includes(mode), 'Unknown probe mode');
const fixtureMode = mode.startsWith('--fixture-');
const captureMode = mode.startsWith('--capture-') || fixtureMode;
const cancelMode = mode === '--fixture-cancel';
const syntheticPrompt = 'Synthetic protocol capture. Do not execute tools. Reply OK.' +
  (mode === '--fixture-large' ? '\n' + '한글🙂\r\n"인용" \\경로\n'.repeat(20000) : '');
let fixtureToolCalls = 0, fixtureOutputReturned = false, fixtureFinalText = false, fixtureInterrupted = false;
let activeThreadId;
function fixtureResponse(response, index) {
  // Deliberately synthetic provider output. Tests Codex RPC routing, not real model behavior.
  const item = index === 1
    ? { type: 'function_call', id: 'fc_probe', call_id: 'call_probe', name: 'probe_echo', arguments: '{"value":"probe"}', status: 'completed' }
    : { type: 'message', id: 'msg_probe', role: 'assistant', phase: 'final_answer', status: 'completed', content: [{ type: 'output_text', text: 'PROBE_DONE', annotations: [] }] };
  const result = { id: `resp_probe_${index}`, object: 'response', status: 'completed', output: [item],
    usage: { input_tokens: 1, output_tokens: 1, total_tokens: 2, input_tokens_details: { cached_tokens: 0 } } };
  const events = [
    { type: 'response.created', response: { ...result, status: 'in_progress', output: [] } },
    { type: 'response.output_item.added', output_index: 0, item },
    ...(index === 1 ? [] : [{ type: 'response.output_text.delta', item_id: 'msg_probe', output_index: 0, content_index: 0, delta: 'PROBE_DONE' }]),
    { type: 'response.output_item.done', output_index: 0, item },
    { type: 'response.completed', response: result }
  ];
  response.writeHead(200, { 'content-type': 'text/event-stream' });
  response.end(events.map((event, sequence_number) => `event: ${event.type}\ndata: ${JSON.stringify({ ...event, sequence_number })}\n\n`).join(''));
}
let captureServer, captureResolve, captureReject, captureCount = 0;
const captured = new Promise((resolve, reject) => { captureResolve = resolve; captureReject = reject; });
captured.catch(() => {});
const args = ['app-server'];
if (captureMode) {
  captureServer = createServer((request, response) => {
    if (request.method !== 'POST' || request.url !== '/v1/responses' || ++captureCount > (fixtureMode ? 2 : 1)) {
      response.writeHead(400).end(); return;
    }
    // No authentication is configured for this synthetic provider. Never display headers or bodies.
    if (request.headers.authorization) {
      request.resume(); response.writeHead(400).end(); captureReject(new Error('Unexpected authorization header')); return;
    }
    let size = 0;
    const chunks = [];
    request.on('error', () => captureReject(new Error('Capture input error')));
    request.on('data', chunk => {
      size += chunk.length;
      if (size > 4 * 1024 * 1024) { request.destroy(); captureReject(new Error('Capture input too large')); }
      else chunks.push(chunk);
    });
    request.on('end', () => {
      let capturePhase = 'decompress';
      try {
        const raw = Buffer.concat(chunks);
        const encoding = request.headers['content-encoding'];
        let decoded = raw;
        if (encoding === 'gzip') decoded = gunzipSync(raw, { maxOutputLength: 4 * 1024 * 1024 });
        else if (encoding === 'zstd') decoded = zstdDecompressSync(raw, { maxOutputLength: 4 * 1024 * 1024 });
        else assert(!encoding || encoding === 'identity', 'Unsupported content encoding');
        capturePhase = 'parse-json';
        const body = JSON.parse(decoded.toString('utf8'));
        if (fixtureMode && captureCount === 2) {
          fixtureOutputReturned = (body.input ?? []).some(item => item.type === 'function_call_output' && item.call_id === 'call_probe' && JSON.stringify(item.output).includes('PROBE_VALUE'));
          assert(fixtureOutputReturned, 'Fixture tool output missing');
          fixtureResponse(response, 2); return;
        }
        capturePhase = 'tools-array';
        assert(body.tools === undefined || Array.isArray(body.tools), 'Unexpected tool catalog type');
        capturePhase = 'tool-identifiers';
        const names = [];
        function collect(items, prefix = '') {
          for (const item of items) {
            const name = item.name ?? item.function?.name ?? item.type;
            assert(typeof name === 'string' && /^[a-zA-Z0-9_.-]{1,120}$/.test(name), 'Unrecognized tool identifier');
            if (Array.isArray(item.tools)) collect(item.tools, `${prefix}${name}.`);
            else names.push(`${prefix}${name}`);
          }
        }
        collect(body.tools ?? []);
        for (const item of body.input ?? []) {
          if (item.type === 'additional_tools') {
            assert(Array.isArray(item.tools), 'Unexpected additional_tools schema');
            collect(item.tools);
          }
        }
        captureResolve({ requestReceived: true, astraRequested: body.model === 'gpt-6-astra',
          xhighRequested: body.reasoning?.effort === 'xhigh', tools: names,
          toolsOmitted: body.tools === undefined, inputArrayPresent: Array.isArray(body.input),
          syntheticPromptPresent: (body.input ?? []).some(item => Array.isArray(item.content) && item.content.some(part => part.type === 'input_text' && part.text === syntheticPrompt)),
          syntheticPromptBytes: Buffer.byteLength(syntheticPrompt),
          toolNamesInInput: Object.fromEntries(['probe_echo', 'shell_command', 'exec_command', 'apply_patch', 'spawn_agent', 'mcp__', 'namespace functions', '# Tools'].map(name => [name, JSON.stringify(body.input).includes(name)])),
          declaredNestedTools: [...new Set((body.input ?? []).filter(item => item.type === 'additional_tools')
            .flatMap(item => [...JSON.stringify(item.tools).matchAll(/### `?([A-Za-z0-9_:.-]{1,120})/g)].map(match => match[1])))],
          toolChoice: ['auto', 'none', 'required'].includes(body.tool_choice) ? body.tool_choice : 'structured-or-other',
          rootFields: Object.keys(body).filter(key => /^[a-z_]{1,60}$/.test(key)),
          inputCount: Array.isArray(body.input) ? body.input.length : null,
          inputStructure: (body.input ?? []).map(item => ({
            type: typeof item.type === 'string' && /^[a-z_]{1,50}$/.test(item.type) ? item.type : null,
            keys: Object.keys(item).filter(key => /^[a-z_]{1,50}$/.test(key)),
            containsProbeTool: JSON.stringify(item).includes('probe_echo'),
            contentTypes: Array.isArray(item.content) ? item.content.map(part => part.type).filter(type => /^[a-z_]{1,50}$/.test(type)) : []
          })),
          inputRoles: [...new Set((body.input ?? []).map(item => item.role))].filter(role => ['user', 'assistant', 'system', 'developer'].includes(role)),
          wrappedRequest: Boolean(body.request || body.response),
          onlyProbeTool: names.length === 1 && names[0] === 'probe_echo', upstreamInferenceSent: false });
      } catch {
        captureReject(new Error(`Capture failed at ${capturePhase} (body withheld)`));
        response.writeHead(400).end(); return;
      }
      if (fixtureMode) { fixtureResponse(response, 1); return; }
      response.writeHead(400, { 'content-type': 'application/json' });
      response.end(JSON.stringify({ error: { message: 'Synthetic capture complete; no inference service.', type: 'invalid_request_error' } }));
    });
  });
  captureServer.requestTimeout = 15000;
  captureServer.headersTimeout = 10000;
  await new Promise((resolve, reject) => { captureServer.once('error', reject); captureServer.listen(0, '127.0.0.1', resolve); });
  const port = captureServer.address().port;
  args.push('-c', 'model_provider="preflight_capture"', '-c',
    `model_providers.preflight_capture={name="Local capture",base_url="http://127.0.0.1:${port}/v1",wire_api="responses",requires_openai_auth=false,supports_websockets=false,request_max_retries=0,stream_max_retries=0,stream_idle_timeout_ms=10000}`);
}
const child = spawn(executable, args, { cwd: root, windowsHide: true, stdio: ['pipe', 'pipe', 'pipe'] });
const pending = new Map();
let sequence = 0, received = 0, stderrBytes = 0, buffer = '', fatal = null;
let threadStarted = false, turnRequested = false, hookEvents = 0, hookCompletions = 0, hookFailed = false;
let resolveClose;
const closed = new Promise(resolve => { resolveClose = resolve; });
function fail(message) {
  fatal ??= new Error(message);
  for (const waiter of pending.values()) waiter.reject(fatal);
  pending.clear();
  captureReject(fatal);
  child.stdin.end();
}
child.on('error', () => fail('App-server process error (details withheld)'));
child.on('close', () => { fail('App-server closed'); resolveClose(); });
child.stdin.on('error', () => fail('App-server input error'));
child.stderr.on('data', chunk => {
  stderrBytes += chunk.length;
  if (stderrBytes > 1024 * 1024) fail('stderr exceeded bound');
});
child.stdout.setEncoding('utf8');
child.stdout.on('data', chunk => {
  received += Buffer.byteLength(chunk);
  if (received > 4 * 1024 * 1024) return fail('stdout exceeded bound');
  buffer += chunk;
  let end;
  while ((end = buffer.indexOf('\n')) >= 0) {
    const line = buffer.slice(0, end);
    buffer = buffer.slice(end + 1);
    if (!line.trim()) continue;
    let message;
    try { message = JSON.parse(line); } catch { return fail('Invalid JSON-RPC output'); }
    if (message.method?.toLowerCase().includes('hook')) hookEvents++;
    if (message.method === 'hook/completed') {
      const run = message.params?.run;
      if (run?.status !== 'completed' || run.entries?.some(entry => ['stop', 'error'].includes(entry.kind))) {
        hookFailed = true;
        return fail('Hook did not complete successfully; do not retry this action');
      }
      hookCompletions++;
    }
    if (fixtureMode && message.method === 'item/tool/call' && message.id !== undefined) {
      const params = message.params;
      if (params?.tool !== 'probe_echo' || params.threadId !== activeThreadId || params.callId !== 'call_probe' || JSON.stringify(params.arguments) !== '{"value":"probe"}' || fixtureToolCalls !== 0) {
        return fail('Unexpected fixture tool call');
      }
      fixtureToolCalls++;
      if (cancelMode) {
        rpc('turn/interrupt', { threadId: activeThreadId, turnId: params.turnId }).catch(() => fail('Fixture interrupt request failed'));
        continue;
      }
      child.stdin.write(JSON.stringify({ id: message.id, result: { contentItems: [{ type: 'inputText', text: 'PROBE_VALUE' }], success: true } }) + '\n');
      continue;
    }
    if (fixtureMode && message.method === 'item/completed' && message.params?.item?.type === 'agentMessage') {
      fixtureFinalText ||= message.params.item.text === 'PROBE_DONE';
    }
    if (fixtureMode && message.method === 'turn/completed') {
      const done = pending.get('fixture-turn');
      pending.delete('fixture-turn');
      fixtureInterrupted = message.params?.turn?.status === 'interrupted';
      if (message.params?.turn?.status === (cancelMode ? 'interrupted' : 'completed')) done?.resolve(true);
      else done?.reject(new Error('Fixture turn failed (error details withheld)'));
    }
    if (message.method && message.id !== undefined) return fail('Unexpected server request; no approvals granted');
    const waiter = pending.get(message.id);
    if (!waiter) continue;
    pending.delete(message.id);
    if (message.error) waiter.reject(new Error(`RPC error ${Number(message.error.code)}`));
    else waiter.resolve(message.result);
  }
});
function rpc(method, params) {
  if (fatal) return Promise.reject(fatal);
  const id = ++sequence;
  return new Promise((resolve, reject) => {
    pending.set(id, { resolve, reject });
    child.stdin.write(JSON.stringify({ id, method, params }) + '\n');
  });
}
const deadline = setTimeout(() => fail('Probe exceeded 45 seconds'), 45000);
let summary;
try {
  await rpc('initialize', { clientInfo: { name: 'local_preflight', version: '0.0.0' }, capabilities: { experimentalApi: true } });
  child.stdin.write(JSON.stringify({ method: 'initialized' }) + '\n');
  // Two in-flight metadata requests test response ID matching, not model concurrency.
  const [first, second] = await Promise.all([
    rpc('model/list', { includeHidden: true, limit: 100 }),
    rpc('model/list', { includeHidden: true, limit: 100 })
  ]);
  assert(Array.isArray(first.data) && Array.isArray(second.data), 'Model list schema mismatch');
  assert(!first.nextCursor && !second.nextCursor, 'Catalog pagination requires an explicit follow-up');
  const models = Object.entries(profiles).map(([model, effort]) => {
    const found = first.data.find(item => item.model === model);
    const supported = found?.supportedReasoningEfforts.map(item => item.reasoningEffort) ?? [];
    return { model, requestedEffort: effort, present: Boolean(found), effortSupported: supported.includes(effort) };
  });
  const thread = JSON.parse(readFileSync(new URL('schema/v2/ThreadStartParams.json', import.meta.url), 'utf8'));
  let inventory;
  if (['--inventory', '--thread-check'].includes(mode)) {
    const features = await rpc('experimentalFeature/list', { limit: 200 });
    assert(!features.nextCursor, 'Feature pagination requires follow-up');
    const selected = features.data.filter(item => /tool|environment|exec|hook|plugin|agent|apps|browser|computer|image|code_mode/.test(item.name));
    const { config } = await rpc('config/read', { cwd: root, includeLayers: false });
    inventory = {
      features: selected.map(item => ({ name: item.name, enabled: item.enabled, stage: item.stage })),
      mcpServerCount: Object.keys(config.mcp_servers ?? {}).length,
      toolRegistryConfigPresent: Object.hasOwn(config, 'tool_registry'),
      environmentConfigPresent: Object.hasOwn(config, 'environments')
    };
  }
  let threadCheck;
  let capture;
  if (mode === '--thread-check' || captureMode) {
    const started = await rpc('thread/start', {
      model: 'gpt-6-astra', cwd: root, ephemeral: true, ...(mode === '--capture-default' ? {} : { environments: [] }),
      approvalPolicy: 'on-request', sandbox: 'read-only', allowProviderModelFallback: false,
      ...(['--capture-restricted', '--capture-classic'].includes(mode) || fixtureMode ? { config: {
        features: { shell_tool: false, unified_exec: false, multi_agent: false, view_image: false,
          image_generation: false, browser_use: false, computer_use: false, in_app_browser: false, apps: false,
          ...(mode === '--capture-classic' ? { code_mode_host: false } : {}) },
        web_search: 'disabled'
      } } : {}),
      dynamicTools: [{ type: 'function', name: 'probe_echo', description: 'Return fixed synthetic test data.',
        inputSchema: { type: 'object', properties: { value: { type: 'string', enum: ['probe'] } }, required: ['value'], additionalProperties: false } }]
    });
    threadStarted = true;
    activeThreadId = started.thread.id;
    threadCheck = { astraSelected: started.model === 'gpt-6-astra', ephemeral: started.thread.ephemeral,
      instructionSourceCount: started.instructionSources?.length ?? null,
      sandboxType: started.sandbox?.type ?? null, approvalPolicy: started.approvalPolicy };
    if (['--capture-restricted', '--capture-classic'].includes(mode)) {
      const effective = await rpc('experimentalFeature/list', { threadId: started.thread.id, limit: 200 });
      threadCheck.restrictedFlags = effective.data.filter(item => ['shell_tool', 'unified_exec', 'multi_agent', 'apps', 'hooks', 'code_mode_host'].includes(item.name))
        .map(item => ({ name: item.name, enabled: item.enabled }));
    }
    if (captureMode) {
      assert(started.thread.modelProvider === 'preflight_capture', 'Local capture provider not selected');
      turnRequested = true;
      const fixtureDone = fixtureMode ? new Promise((resolve, reject) => pending.set('fixture-turn', { resolve, reject })) : null;
      fixtureDone?.catch(() => {});
      await rpc('turn/start', { threadId: started.thread.id, model: 'gpt-6-astra', effort: 'xhigh',
        ...(mode === '--capture-default' ? {} : { environments: [] }),
        input: [{ type: 'text', text: syntheticPrompt }] });
      capture = await captured;
      if (fixtureMode) {
        await fixtureDone;
        assert(capture.syntheticPromptPresent, 'Synthetic prompt changed in transport');
        if (cancelMode) assert(fixtureToolCalls === 1 && fixtureInterrupted && !fixtureOutputReturned, 'Fixture cancel incomplete');
        else assert(fixtureToolCalls === 1 && fixtureOutputReturned && fixtureFinalText, 'Fixture round trip incomplete');
      }
    }
  }
  summary = { initialized: true, concurrentMetadataRequests: true, catalogComplete: true, models,
    ...(inventory ? { inventory } : {}),
    ...(threadCheck ? { threadCheck } : {}),
    ...(capture ? { capture } : {}),
    ...(fixtureMode ? { fixture: { dynamicToolCalls: fixtureToolCalls, outputReturned: fixtureOutputReturned, finalTextObserved: fixtureFinalText, interrupted: fixtureInterrupted, actualModelUsed: false } } : {}),
    schema: { dynamicTools: Boolean(thread.properties.dynamicTools), environments: Boolean(thread.properties.environments),
      noProviderFallback: Boolean(thread.properties.allowProviderModelFallback) },
    threadStarted, turnRequested, modelRequestSent: false, nativeToolIsolationVerified: false };
} catch (error) {
  summary = { failed: true, reason: error.message, threadStarted, turnRequested, modelRequestSent: false };
  process.exitCode = 1;
} finally {
  clearTimeout(deadline);
  child.stdin.end();
  let grace;
  const exited = await Promise.race([closed.then(() => true), new Promise(resolve => { grace = setTimeout(() => resolve(false), 2000); })]);
  clearTimeout(grace);
  if (!exited && child.pid) {
    // Exact task-owned PID only. This is cleanup, not a security sandbox.
    const killed = spawnSync('C:\\Windows\\System32\\taskkill.exe', ['/PID', String(child.pid), '/T', '/F'],
      { windowsHide: true, stdio: 'ignore', timeout: 5000 });
    if (killed.status !== 0) { summary.cleanupFailed = true; process.exitCode = 1; }
  }
  summary.normalExitObserved = exited;
  summary.hookEventCount = hookEvents;
  summary.successfulHookCompletions = hookCompletions;
  if (hookFailed) { summary.failed = true; summary.hookFailed = true; process.exitCode = 1; }
  if (captureServer) {
    captureServer.closeAllConnections();
    await new Promise(resolve => captureServer.close(resolve));
  }
  console.log(JSON.stringify(summary, null, 2));
}
