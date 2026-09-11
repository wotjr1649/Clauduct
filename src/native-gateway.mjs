import { createServer } from 'node:http';
import { randomBytes, timingSafeEqual, createHmac } from 'node:crypto';
import { REQUEST_STAGES, FAILURE_DIAGNOSTIC_CATEGORIES, REQUEST_FAILURES, UPSTREAM_ERROR_CODES, UPSTREAM_ERROR_TYPES, UPSTREAM_INCOMPLETE_REASONS } from './native-protocol.mjs';
import { prepareNative, createNativeResponse, prepareFileReview, prepareReviewContext, verifyFileReviewStep, NativeError, need, NATIVE_LIMITS, EVENT_DIAGNOSTIC_TYPES, EVENT_TYPE_FORMATS, UPSTREAM_FAILURES, capturableEventName } from './native-protocol.mjs';
import { MODELS, ROLE_MODELS, CONTEXT_POLICY } from './models.mjs';
import { writeFrames } from './native-delivery.mjs';
import { createAdmission } from './request-admission.mjs';
import { betaFailure, unknownBetas, judgedBetas } from './native-beta.mjs';
import { SELECTION_FAILURES, SELECTION_IO_CODES, COMPLETION_FAILURES, COMPLETION_STATES } from './agent-selection.mjs';

// Fixed-label diagnostics. A composed local code carries a variable suffix built from
// this project's own allowlists, never upstream text; keep only its fixed prefix.
function diagnosticCategory(code) {
  const fixed = typeof code !== 'string' ? 'OTHER'
    : code.startsWith('AGENT_SELECTION_UNVERIFIED') ? 'AGENT_SELECTION_UNVERIFIED'
    : code.startsWith('UNSUPPORTED_BETA ') ? 'UNSUPPORTED_BETA' : code;
  return FAILURE_DIAGNOSTIC_CATEGORIES.includes(fixed) ? fixed : 'OTHER';
}

// Matches the bounded pending/verified tables in agent selection.
const MAX_AGENTS = 1024;

export async function startNativeGateway({ transport, onUnregisteredAgent, onUnmappedAgentModel, onUnsupportedEventCapture, onWebSearchUnused,
  admissionOptions, agentSelection, cleanupMs = 2000, heartbeatMs = 15000, maxAgents = MAX_AGENTS, agentIdleMs = 1800000 } = {}) {
  need(typeof transport?.send === 'function' && typeof transport?.close === 'function'
    && typeof transport?.diagnostics === 'function' && Number.isInteger(cleanupMs) && cleanupMs > 0 && cleanupMs <= 10000
    && Number.isInteger(heartbeatMs) && heartbeatMs >= 5 && heartbeatMs <= 60000
    && Number.isInteger(maxAgents) && maxAgents > 0 && maxAgents <= MAX_AGENTS
    && Number.isInteger(agentIdleMs) && agentIdleMs > 0, 'INVALID_GATEWAY_OPTIONS');
  const admission = createAdmission(admissionOptions);
  const secret = Buffer.from(`Bearer ${randomBytes(32).toString('base64url')}`), jobs = new Set(), sockets = new Set();
  const agents = new Map();
  const correlationKey = randomBytes(32), correlationScope = randomBytes(16).toString('hex');
  const reference = (kind, session, id) => session && (kind === 'session' || id)
    ? createHmac('sha256', correlationKey).update(JSON.stringify([kind, session, id ?? null])).digest('hex').slice(0, 32) : null;
  let closing = false, reason = 'NONE', requests = 0, rejected = 0, unregisteredAgentRequests = 0, port, finish, closingWork;
  let activeBodies = 0, activeDeliveries = 0, activeHeartbeats = 0, cleanupFailed = false;
  const maxObservedInputTokens = { main: 0, subagent: 0 };
  // Fixed metadata only; bounded memory, no transcript, headers or credential material.
  const recentRequests = [], failureRequests = [];
  // At most four distinct unmapped upstream event names and eight unrecognised client beta
  // names, both kept for the exit diagnostic only.
  const unsupportedEventNames = [], unknownBetaNames = [], judgedBetaLabels = [];
  const copyRecord = record => ({ ...record, retryScheduledMs: [...record.retryScheduledMs],
    attempts: record.attempts.map(attempt => ({ ...attempt })),
    ...(record.agentContextPolicy && { agentContextPolicy: { ...record.agentContextPolicy } }),
    ...(record.compactShape && { compactShape: { ...record.compactShape } }) });
  // rejectedBeforeStart counts HTTP-boundary rejections that never became a request record:
  // another route, a local boundary or authorization rejection, a malformed identifier header
  // or an agent binding failure. Inference requests are counted by started/succeeded/failed.
  const lifetime = { started: 0, succeeded: 0, failed: 0, auxiliaryMetadataEvents: 0, unsupportedEvents: 0,
    rejectedBeforeStart: 0, firstRejectedCategory: null, unmappedAgentModels: 0,
    transportRejections: 0, agentRegistrationsEvicted: 0, agentRegistrationsExpired: 0,
    webSearchRequests: 0, webSearchCalls: 0 };
  let notifiedUnmappedModel = false, notifiedEventCapture = false, notifiedWebSearchUnused = false;
  const failuresByStage = Object.fromEntries(REQUEST_STAGES.map(stage => [stage, 0]));
  const done = new Promise(resolve => { finish = resolve; });
  const diagnostics = () => ({ closing, reason, activeSockets: sockets.size, activeJobs: jobs.size,
    activeTimers: activeBodies + activeHeartbeats + Number(admission.diagnostics().timerActive), activeBodies,
    activeDeliveries, cleanupFailed, admission: admission.diagnostics(), maxObservedInputTokens: { ...maxObservedInputTokens },
    contextPolicy: CONTEXT_POLICY, contextPolicyRuntimeVerified: false,
    correlationScope, lifetime: { ...lifetime, failuresByStage: { ...failuresByStage } },
    recentRequests: recentRequests.map(copyRecord),
    failureRequests: failureRequests.map(copyRecord),
    unsupportedEventNames: [...unsupportedEventNames], unknownBetaNames: [...unknownBetaNames],
    judgedBetaLabels: [...judgedBetaLabels],
    busy: jobs.size > 0, transport: transport.diagnostics(), requests, rejected,
    persistedBodies: 0, registeredAgents: agents.size, maxAgents, unregisteredAgentRequests, sessionLifetime: null, requestBudget: null });
  function authorized(value) {
    return typeof value === 'string' && Buffer.byteLength(value) === secret.length && timingSafeEqual(Buffer.from(value), secret);
  }
  function reply(res, status, value) {
    res.writeHead(status, { 'Content-Type': 'application/json', 'Cache-Control': 'no-store', Connection: 'close' });
    res.end(JSON.stringify(value));
  }
  async function readBody(req, limit, controller) {
    need(!controller.signal.aborted, 'CANCELLED');
    activeBodies++;
    const abort = () => req.destroy(controller.signal.reason instanceof NativeError
      ? controller.signal.reason : new NativeError('CANCELLED'));
    controller.signal.addEventListener('abort', abort, { once: true });
    const timer = setTimeout(() => controller.abort(new NativeError('REQUEST_TIMEOUT')), 300000);
    try {
      let bytes = 0, raw = ''; const decoder = new TextDecoder('utf-8', { fatal: true });
      for await (const chunk of req) {
        bytes += chunk.length; need(bytes <= limit, 'INPUT_TOO_LARGE'); raw += decoder.decode(chunk, { stream: true });
      }
      raw += decoder.decode();
      try { return JSON.parse(raw); } catch { throw new NativeError('INVALID_JSON'); }
    } finally { clearTimeout(timer); controller.signal.removeEventListener('abort', abort); activeBodies--; }
  }
  async function handle(req, res, controller) {
    requests++;
    let release, activeAgent, upstream = false, timing, heartbeat, heartbeatPending = false, responseStarted = false;
    let writeTail = Promise.resolve(), deliveryError, stage = 'request';
    const stopHeartbeat = () => { if (heartbeat) { clearInterval(heartbeat); heartbeat = undefined; activeHeartbeats--; } };
    const started = performance.now();
    const elapsed = () => Math.round((performance.now() - started) * 100) / 100;
    const abort = () => { if (!res.writableFinished) {
      if (timing && !closing && !controller.signal.aborted) {
        timing.clientDisconnected = true; timing.clientDisconnectedMs = elapsed();
      }
      controller.abort();
    } };
    res.once('close', abort); req.on('error', abort); req.once('aborted', abort); res.on('error', abort);
    try {
      const names = req.rawHeaders.filter((_, i) => i % 2 === 0).map(name => name.toLowerCase());
      need(!closing && new Set(names).size === names.length, 'INVALID_HEADER');
      need(req.socket.remoteAddress === '127.0.0.1' && req.headers.host === `127.0.0.1:${port}`
        && req.headers.origin === undefined && req.headers['sec-fetch-site'] === undefined
        && req.headers.forwarded === undefined && !names.some(name => name.startsWith('x-forwarded-')), 'LOCAL_BOUNDARY_REJECTED');
      need(!['cookie', 'x-api-key', 'proxy-authorization'].some(name => names.includes(name)), 'UNEXPECTED_CREDENTIAL_SOURCE');
      const path = req.url.split('?')[0];
      if (req.method === 'HEAD' && path === '/api/hello') {
        need(req.headers.authorization === undefined || authorized(req.headers.authorization), 'LOCAL_SESSION_REQUIRED');
        res.writeHead(204, { Connection: 'close' }); res.end(); return;
      }
      need(authorized(req.headers.authorization), 'LOCAL_SESSION_REQUIRED');
      if (req.method === 'GET' && path === '/v1/models') {
        reply(res, 200, { object: 'list', data: Object.values(MODELS).map(item => ({ id: item.model, object: 'model', owned_by: 'openai' })) }); return;
      }
      if (req.method === 'GET' && path === '/clauduct/status') {
        // The captured upstream name is withheld here so it never enters model context.
        const { unsupportedEventNames: withheldEvents, unknownBetaNames: withheldBetas, ...state } = diagnostics();
        void withheldEvents; void withheldBetas; reply(res, 200, state); return;
      }
      if (req.method === 'POST' && path === '/clauduct/agents') {
        const binding = await readBody(req, 4096, controller);
        if (binding?.kind === 'workflow-result') {
          need(agentSelection && Object.keys(binding).every(key => ['kind', 'sessionId', 'toolUseId', 'taskId', 'runId',
            'workflowName', 'transcriptPath', 'transcriptDir', 'scriptPath', 'scriptDigest', 'parent'].includes(key))
            && ['sessionId', 'toolUseId', 'taskId', 'runId', 'workflowName'].every(key => typeof binding[key] === 'string' && /^[A-Za-z0-9_-]{1,200}$/.test(binding[key]))
            && ['transcriptPath', 'transcriptDir', 'scriptPath'].every(key => typeof binding[key] === 'string' && binding[key].length <= 4096)
            && typeof binding.scriptDigest === 'string' && /^[a-f0-9]{64}$/.test(binding.scriptDigest)
            && (binding.parent === undefined || (typeof binding.parent === 'string' && /^[A-Za-z0-9_-]{1,200}$/.test(binding.parent))), 'INVALID_AGENT_BINDING');
          try { agentSelection.linkWorkflow(binding); } catch { throw new NativeError('AGENT_SELECTION_UNVERIFIED_CALL'); }
          reply(res, 200, { linked: true }); return;
        }
        if (binding?.kind === 'resume-result') {
          need(agentSelection && Object.keys(binding).every(key => ['kind', 'sessionId', 'toolUseId', 'id', 'parent'].includes(key))
            && ['sessionId', 'toolUseId', 'id'].every(key => typeof binding[key] === 'string' && /^[A-Za-z0-9_-]{1,200}$/.test(binding[key]))
            && (binding.parent === undefined || (typeof binding.parent === 'string' && /^[A-Za-z0-9_-]{1,200}$/.test(binding.parent))), 'INVALID_AGENT_BINDING');
          let linked;
          try { linked = agentSelection.linkResume(binding); } catch { throw new NativeError('AGENT_SELECTION_UNVERIFIED_CALL'); }
          reply(res, 200, { linked }); return;
        }
        if (binding?.kind === 'skill-result') {
          need(agentSelection && Object.keys(binding).every(key => ['kind', 'sessionId', 'toolUseId', 'id', 'skill', 'parent'].includes(key))
            && ['sessionId', 'toolUseId', 'id'].every(key => typeof binding[key] === 'string' && /^[A-Za-z0-9_-]{1,200}$/.test(binding[key]))
            && (binding.parent === undefined || (typeof binding.parent === 'string' && /^[A-Za-z0-9_-]{1,200}$/.test(binding.parent)))
            && typeof binding.skill === 'string' && binding.skill.length > 0 && binding.skill.length <= 200, 'INVALID_AGENT_BINDING');
          try { agentSelection.linkSkill(binding); } catch { throw new NativeError('AGENT_SELECTION_UNVERIFIED_CALL'); }
          reply(res, 200, { linked: true }); return;
        }
        need(binding && Object.keys(binding).every(key => ['id', 'role', 'stop', 'contextPolicy', 'sessionId', 'transcriptPath'].includes(key))
          && typeof binding.id === 'string' && /^[A-Za-z0-9_-]{1,200}$/.test(binding.id)
          && typeof binding.role === 'string' && binding.role.length > 0 && binding.role.length <= 200
          && typeof binding.stop === 'boolean'
          && (binding.sessionId === undefined || (typeof binding.sessionId === 'string' && /^[A-Za-z0-9_-]{1,200}$/.test(binding.sessionId)))
          && (binding.transcriptPath === undefined || (typeof binding.transcriptPath === 'string' && binding.transcriptPath.length <= 4096)), 'INVALID_AGENT_BINDING');
        const context = binding.contextPolicy;
        need(context === undefined || (context && typeof context === 'object' && !Array.isArray(context)
          && Object.keys(context).length === 3 && Object.keys(context).every(key => ['window', 'autoCompactWindow', 'compactPercent'].includes(key))
          && Number.isSafeInteger(context.window) && context.window > 0
          && Number.isSafeInteger(context.autoCompactWindow) && context.autoCompactWindow > 0
          && Number.isFinite(context.compactPercent) && context.compactPercent > 0 && context.compactPercent <= 100), 'INVALID_AGENT_BINDING');
        need(!agents.has(binding.id) || agents.get(binding.id).role === binding.role, 'AGENT_BINDING_CONFLICT');
        // Registrations are removed by SubagentStop. For a session where that never arrives,
        // drop idle entries that have gone unused past the window. No timer: the sweep runs
        // here, and a registration with live requests is never a candidate.
        if (!binding.stop) {
          const stale = Date.now() - agentIdleMs;
          for (const [id, state] of agents) {
            if (id === binding.id || state.requests.size > 0 || state.lastUsedMs > stale) continue;
            state.selectionController?.abort();
            agents.delete(id);
            lifetime.agentRegistrationsExpired++;
          }
        }
        // The cap is the backstop when even the window has not released enough entries.
        if (!binding.stop && !agents.has(binding.id) && agents.size >= maxAgents) {
          const idle = [...agents].find(([, state]) => state.requests.size === 0);
          need(idle !== undefined, 'AGENT_BINDING_LIMIT');
          idle[1].selectionController?.abort();
          agents.delete(idle[0]);
          lifetime.agentRegistrationsEvicted++;
        }
        agents.get(binding.id)?.selectionController?.abort();
        // Stop or replacement invalidates active requests of this registration only.
        for (const active of agents.get(binding.id)?.requests ?? []) active.abort(new NativeError('CANCELLED'));
        if (binding.stop) {
          agents.delete(binding.id);
        }
        else {
          const state = { role: binding.role, requests: new Set(), lastUsedMs: Date.now(),
            contextPolicy: context ?? null, selectionPending: Boolean(agentSelection),
            selectionBinding: agentSelection ? { ...binding, nativeRegistered: true } : undefined, selectionController: agentSelection ? new AbortController() : undefined };
          agents.set(binding.id, state);
          // Native can persist the sidecar only after this hook returns. Resolve
          // on the first child request, never while blocking SubagentStart.
        }
        reply(res, 200, { registered: !binding.stop }); return;
      }
      need(req.method === 'POST' && path === '/v1/messages', 'UNSUPPORTED_ROUTE');
      for (const key of ['x-claude-code-session-id', 'x-claude-code-agent-id', 'x-claude-code-parent-agent-id']) {
        need(req.headers[key] === undefined || /^[A-Za-z0-9_-]{1,200}$/.test(req.headers[key]), 'INVALID_SESSION_ID');
      }
      // Do not decode or retain a large request body while waiting for memory.
      timing = { request: requests, startedAt: new Date().toISOString(), model: null, effort: null,
        sessionRef: reference('session', req.headers['x-claude-code-session-id']),
        agentRef: reference('agent', req.headers['x-claude-code-session-id'], req.headers['x-claude-code-agent-id']),
        parentRef: reference('agent', req.headers['x-claude-code-session-id'], req.headers['x-claude-code-parent-agent-id']),
        subagent: req.headers['x-claude-code-agent-id'] !== undefined,
        admissionStartedMs: elapsed(), admittedMs: null, preparedMs: null, transportStartedMs: null,
        firstEventMs: null, firstTextDeltaMs: null, firstDownstreamWriteMs: null,
        transportFinishedMs: null, finishedMs: null, retryScheduledMs: [], attempts: [], success: false };
      recentRequests.push(timing);
      lifetime.started++;
      if (recentRequests.length > 16) recentRequests.shift();
      // Record the inference request before header compatibility checks so a rejected
      // version, beta or encoding is a diagnosed failure instead of an unrecorded 400.
      need(req.headers['anthropic-version'] === '2023-06-01', 'UNSUPPORTED_VERSION');
      const betaError = betaFailure(req.headers['anthropic-beta']);
      if (betaError) throw new NativeError(betaError);
      for (const name of unknownBetas(req.headers['anthropic-beta'])) {
        if (unknownBetaNames.length < 8 && !unknownBetaNames.includes(name)) unknownBetaNames.push(name);
      }
      // Fixed labels, so unlike the raw names these stay available inside the session too.
      timing.judgedBetaLabels = judgedBetas(req.headers['anthropic-beta']);
      for (const label of timing.judgedBetaLabels) {
        if (!judgedBetaLabels.includes(label)) judgedBetaLabels.push(label);
      }
      need((req.headers['content-type'] ?? '').split(';')[0].trim().toLowerCase() === 'application/json'
        && (!req.headers['content-encoding'] || req.headers['content-encoding'] === 'identity'), 'UNSUPPORTED_ENCODING');
      // Pin the registration before either admission or body reads can yield.
      const agent = req.headers['x-claude-code-agent-id'];
      const agentBinding = agents.get(agent), role = agentBinding?.role;
      if (agentBinding) { agentBinding.lastUsedMs = Date.now(); agents.delete(agent); agents.set(agent, agentBinding); }
      activeAgent = agentBinding;
      activeAgent?.requests.add(controller);
      release = await admission.acquire(controller.signal);
      timing.admittedMs = elapsed();
      let doc = await readBody(req, NATIVE_LIMITS.requestBytes, controller);
      need(!controller.signal.aborted, 'CANCELLED');
      timing.requestedModel = Object.values(MODELS).find(item => item.model === doc?.model)?.model
        ?? (typeof doc?.model === 'string' && Object.hasOwn(MODELS, doc.model) ? MODELS[doc.model].model : null);
      stage = 'selection';
      if (agentSelection && agent !== undefined) {
        if (agentBinding?.selectionPending) {
          agentBinding.selectionWork ??= agentSelection.resolve({ ...agentBinding.selectionBinding,
            requestedModel: doc.model, requestedEffort: doc.output_config?.effort }, agentBinding.selectionController.signal).then(selection => {
            agentBinding.selection = selection;
            agentBinding.selectionPending = false;
            agentBinding.selectionBinding = undefined;
          }).catch(error => {
            agentBinding.selectionFailure = SELECTION_FAILURES.includes(error.selectionReason) ? error.selectionReason : 'UNKNOWN';
            throw Object.assign(new NativeError(`AGENT_SELECTION_UNVERIFIED_${agentBinding.selectionFailure}`), {
              selectionReason: agentBinding.selectionFailure,
              selectionIoCode: SELECTION_IO_CODES.includes(error.selectionIoCode) ? error.selectionIoCode : null,
              completionFailure: COMPLETION_FAILURES.includes(error.completionFailure) ? error.completionFailure : null,
              completionParentState: COMPLETION_STATES.includes(error.completionParentState) ? error.completionParentState : null,
              completionChildState: COMPLETION_STATES.includes(error.completionChildState) ? error.completionChildState : null
            });
          });
          await agentBinding.selectionWork;
          need(!controller.signal.aborted, 'CANCELLED');
        }
        need(agentBinding && !agentBinding.selectionPending,
          `AGENT_SELECTION_UNVERIFIED_${agentBinding?.selectionFailure ?? (agentBinding ? 'PENDING' : 'UNREGISTERED')}`);
        need(agentBinding.selection.sessionId === req.headers['x-claude-code-session-id']
          && agentBinding.selection.parent === req.headers['x-claude-code-parent-agent-id'], 'AGENT_SELECTION_UNVERIFIED');
      }
      const selectionRequest = agentSelection?.begin(req.headers['x-claude-code-session-id'], agent);
      const route = agentSelection ? agentBinding?.selection?.route : Object.hasOwn(ROLE_MODELS, role) ? ROLE_MODELS[role] : undefined;
      stage = 'prepare';
      const prepared = prepareNative(doc, { subagent: agent !== undefined, route,
        turnToolChanges: req.headers['anthropic-beta']?.split(',').some(value => value.trim() === 'mid-conversation-tool-changes-2026-07-01') });
      stage = 'review';
      prepareReviewContext(prepared, agentBinding?.selection);
      if (agentBinding?.selection?.review) prepareFileReview(prepared, doc);
      doc = undefined;
      timing.selectionSource = agentBinding?.selection?.source ?? null;
      timing.model = prepared.selected.model; timing.effort = prepared.selected.effort; timing.preparedMs = elapsed();
      timing.purpose = prepared.purpose; timing.requestedEffort = prepared.requestedEffort;
      timing.compactShape = prepared.compactShape;
      timing.role = Object.hasOwn(ROLE_MODELS, role) || ['claude', 'workflow-subagent'].includes(role) ? role : null;
      timing.roleRegistered = agent !== undefined && agents.has(agent);
      timing.agentContextPolicy = agentBinding?.contextPolicy ? { ...agentBinding.contextPolicy } : null;
      // Fixed booleans/counts only: whether the client asked for backend search and how many
      // searches the backend actually ran. Neither carries a query or a result.
      timing.webSearchRequested = prepared.webSearch === true;
      // Counted where the request is shaped, not where it succeeds: a search request that
      // fails upstream is exactly the one the lifetime totals have to show.
      if (timing.webSearchRequested) lifetime.webSearchRequests++;
      // Registration is routing metadata, not authorization. Never bypass native tool/permission hooks.
      if (agent !== undefined && !agents.has(agent)) {
        unregisteredAgentRequests++;
        if (unregisteredAgentRequests === 1) onUnregisteredAgent?.();
      }
      upstream = true;
      const responseOptions = { deferText: agentBinding?.selection?.source === 'workflow-result' };
      let response = createNativeResponse(prepared, responseOptions);
      const validateResponse = (phase, action) => {
        need(!controller.signal.aborted, 'CANCELLED');
        try { return action(); }
        catch (error) {
          if (error instanceof NativeError && error.code === 'SNAPSHOT_MISMATCH' && timing.snapshotMismatchMs === undefined) {
            timing.snapshotMismatchMs = elapsed(); timing.snapshotMismatchPhase = phase;
          }
          throw error;
        }
      };
      const emit = (frames, ping = false) => {
        if (!frames.length) return Promise.resolve();
        const work = writeTail.then(async () => {
        if (controller.signal.aborted) throw new NativeError('CANCELLED');
        if (!ping) { responseStarted = true; timing.firstDownstreamWriteMs ??= elapsed(); }
        if (!res.headersSent) res.writeHead(200, { 'Content-Type': 'text/event-stream; charset=utf-8', 'Cache-Control': 'no-store', Connection: 'close' });
        activeDeliveries++;
        try {
          await writeFrames(res, frames, controller.signal);
          if (ping) { timing.pingCount = (timing.pingCount ?? 0) + 1; timing.lastPingMs = elapsed(); }
        }
        finally { activeDeliveries--; }
        });
        writeTail = work.catch(error => { deliveryError = error; controller.abort(error); });
        return work;
      };
      timing.transportStartedMs = elapsed();
      stage = 'upstream';
      const pushEvent = async event => {
        timing.firstEventMs ??= elapsed();
        timing.lastUpstreamEventMs = elapsed();
        if (event.type === 'response.output_text.delta') timing.firstTextDeltaMs ??= elapsed();
        await emit(validateResponse('stream', () => response.push(event)));
        if (event.type === 'codex.response.metadata') {
          timing.auxiliaryMetadataEvents = (timing.auxiliaryMetadataEvents ?? 0) + 1;
          lifetime.auxiliaryMetadataEvents++;
        }
      };
      const legacyEvents = await transport.send(prepared.body, controller.signal, {
        attemptTimings: timing.attempts,
        ...(prepared.upstreamHeaders && { headers: prepared.upstreamHeaders }),
        onEvent: async event => {
          await pushEvent(event);
          if (!heartbeat) {
            activeHeartbeats++;
            heartbeat = setInterval(() => {
              if (heartbeatPending || controller.signal.aborted || res.writableEnded) return;
              heartbeatPending = true;
              void emit([{ type: 'ping' }], true).then(() => { heartbeatPending = false; }, error => {
                heartbeatPending = false; deliveryError = error; controller.abort(error);
              });
            }, heartbeatMs);
          }
        },
        canRetry: () => !responseStarted && !controller.signal.aborted,
        onRetry: () => {
          need(!responseStarted, 'RETRY_AFTER_OUTPUT');
          if (timing.retryScheduledMs.length < 5) timing.retryScheduledMs.push(elapsed());
          response = createNativeResponse(prepared, responseOptions);
        }
      });
      timing.transportFinishedMs = elapsed();
      stopHeartbeat(); await writeTail;
      need(!controller.signal.aborted, 'CANCELLED');
      // Existing injected offline transports can still return the old event-array contract.
      if (Array.isArray(legacyEvents)) for (const event of legacyEvents) await pushEvent(event);
      const output = validateResponse('final', () => response.finish());
      stage = 'output-validation';
      // Fixed block kinds and counts, never their content. The client reads only the first
      // block in some paths, so record which kind that was.
      const blocks = output.message.content;
      timing.contentBlocks = { text: blocks.filter(block => block.type === 'text').length,
        toolUse: blocks.filter(block => block.type === 'tool_use').length,
        thinking: blocks.filter(block => block.type === 'redacted_thinking').length };
      timing.firstContentBlock = blocks[0]?.type ?? null;
      verifyFileReviewStep(output.message, prepared);
      // Recording routing evidence is atomic; report why it was refused instead of a
      // generic protocol rejection, so the failed turn names the unverified selection.
      let unmappedModels = 0;
      try { unmappedModels = agentSelection?.remember(output.message, req.headers['x-claude-code-session-id'], agent, prepared.selected) ?? 0; }
      catch (error) {
        const reason = SELECTION_FAILURES.includes(error?.selectionReason) ? error.selectionReason : 'UNKNOWN';
        throw Object.assign(new NativeError(`AGENT_SELECTION_UNVERIFIED_${reason}`), { selectionReason: reason });
      }
      if (unmappedModels > 0) {
        lifetime.unmappedAgentModels += unmappedModels;
        if (!notifiedUnmappedModel) { notifiedUnmappedModel = true; onUnmappedAgentModel?.(); }
      }
      const kind = agent === undefined ? 'main' : 'subagent';
      maxObservedInputTokens[kind] = Math.max(maxObservedInputTokens[kind],
        output.message.usage.input_tokens + (output.message.usage.cache_read_input_tokens ?? 0));
      stage = 'delivery';
      await emit(output.frames);
      res.end();
      timing.webSearchCalls = response.webSearchCalls?.() ?? 0;
      timing.success = true;
      if (timing.webSearchRequested) {
        lifetime.webSearchCalls += timing.webSearchCalls;
        // A completed request that asked the backend to search and got no search back is a
        // silent no-op: nothing failed, yet the feature did nothing. Say so once.
        if (timing.webSearchCalls === 0 && !notifiedWebSearchUnused) {
          notifiedWebSearchUnused = true; onWebSearchUnused?.();
        }
      }
      agentSelection?.delivered(req.headers['x-claude-code-session-id'], agent, selectionRequest, output.message);
    } catch (error) {
      stopHeartbeat(); await writeTail;
      error = deliveryError ?? error;
      rejected++;
      // Only locally constructed fixed categories cross this diagnostic boundary.
      const category = error instanceof NativeError ? error.code : upstream ? 'PROTOCOL_REJECTED' : 'INVALID_REQUEST';
      if (!timing) {
        lifetime.rejectedBeforeStart++;
        lifetime.firstRejectedCategory ??= diagnosticCategory(category);
      }
      const requestFailure = stage === 'prepare' && category === 'UNSUPPORTED_REQUEST'
        && REQUEST_FAILURES.includes(error.requestFailure) ? error.requestFailure : null;
      if (timing && category === 'UNSUPPORTED_EVENT') lifetime.unsupportedEvents++;
      const eventName = category === 'UNSUPPORTED_EVENT' ? capturableEventName(error.eventTypeName) : null;
      if (eventName && unsupportedEventNames.length < 4 && !unsupportedEventNames.includes(eventName)) {
        unsupportedEventNames.push(eventName);
        if (!notifiedEventCapture) { notifiedEventCapture = true; onUnsupportedEventCapture?.(); }
      }
      const eventKind = category === 'UNSUPPORTED_EVENT' && EVENT_DIAGNOSTIC_TYPES.includes(error.eventKind) ? error.eventKind : null;
      const eventTypeFormat = category === 'UNSUPPORTED_EVENT' && EVENT_TYPE_FORMATS.includes(error.eventTypeFormat) ? error.eventTypeFormat : null;
      const upstreamFailureEvent = Object.keys(UPSTREAM_FAILURES).find(type => UPSTREAM_FAILURES[type] === category) ?? null;
      const upstreamErrorCode = upstreamFailureEvent && UPSTREAM_ERROR_CODES.includes(error.upstreamErrorCode) ? error.upstreamErrorCode : null;
      const upstreamErrorType = upstreamFailureEvent && UPSTREAM_ERROR_TYPES.includes(error.upstreamErrorType) ? error.upstreamErrorType : null;
      const upstreamIncompleteReason = upstreamFailureEvent === 'response.incomplete'
        && UPSTREAM_INCOMPLETE_REASONS.includes(error.upstreamIncompleteReason) ? error.upstreamIncompleteReason : null;
      const completionFailure = COMPLETION_FAILURES.includes(error.completionFailure) ? error.completionFailure : null;
      const parentState = COMPLETION_STATES.includes(error.completionParentState) ? error.completionParentState : null;
      const childState = COMPLETION_STATES.includes(error.completionChildState) ? error.completionChildState : null;
      if (timing) timing.unsupportedEvent = eventKind;
      if (timing) timing.unsupportedEventTypeFormat = eventTypeFormat;
      if (timing) {
        timing.upstreamFailureEvent = upstreamFailureEvent;
        timing.upstreamErrorCode = upstreamErrorCode;
        timing.upstreamErrorType = upstreamErrorType;
        timing.upstreamIncompleteReason = upstreamIncompleteReason;
        timing.failureStage = stage;
        timing.requestFailure = requestFailure;
        timing.selectionFailure = SELECTION_FAILURES.includes(error.selectionReason) ? error.selectionReason : null;
        timing.selectionIoCode = SELECTION_IO_CODES.includes(error.selectionIoCode) ? error.selectionIoCode : null;
        timing.completionFailure = completionFailure;
        timing.completionParentState = parentState;
        timing.completionChildState = childState;
      }
      if (timing) timing.reviewDiffMismatch = category === 'REVIEW_DIFF_REQUIRED'
        && ['call-count', 'tool-name', 'command', 'background'].includes(error.reviewDiffMismatch) ? error.reviewDiffMismatch : null;
      if (timing) timing.failureCategory = diagnosticCategory(category);
      const relogin = ['UNAUTHENTICATED', 'CREDENTIAL_UNAVAILABLE_OR_EXPIRED', 'CREDENTIAL_ACCOUNT_CHANGED', 'CREDENTIAL_ACCOUNT_MISMATCH', 'CODEX_RELOGIN_REQUIRED'].includes(category);
      const status = category === 'LOCAL_SESSION_REQUIRED' ? 401 : category === 'RATE_LIMITED' ? 429
        : category === 'MEMORY_QUEUE_FULL' || relogin ? 503 : upstream ? 502 : 400;
      const failure = { type: 'error', error: { type: status === 429 ? 'rate_limit_error' : status >= 500 ? 'api_error' : 'invalid_request_error',
        message: category + (eventKind ? ` event=${eventKind}` : '')
          + (eventTypeFormat ? ` event_type_format=${eventTypeFormat}` : '')
          + (upstreamFailureEvent ? ` event=${upstreamFailureEvent}` : '')
          + (upstreamErrorCode ? ` upstream_code=${upstreamErrorCode}` : '')
          + (upstreamErrorType ? ` upstream_type=${upstreamErrorType}` : '')
          + (upstreamIncompleteReason ? ` incomplete_reason=${upstreamIncompleteReason}` : '')
          + (requestFailure ? ` request=${requestFailure}` : '')
          + (completionFailure ? ` completion=${completionFailure} parent=${parentState ?? 'NONE'} child=${childState ?? 'NONE'}` : '')
          + (relogin ? ': Codex login required; resume after logging in.' : res.headersSent ? ': Partial response; explicit resume required.' : '') } };
      if (!res.destroyed && !res.headersSent) reply(res, status, failure);
      else if (!res.destroyed && !controller.signal.aborted) {
        try { await writeFrames(res, [failure], controller.signal); res.end(); }
        catch { res.destroy(); }
      } else res.destroy();
    } finally {
      stopHeartbeat();
      activeAgent?.requests.delete(controller);
      if (timing) {
        timing.finishedMs = elapsed(); lifetime[timing.success ? 'succeeded' : 'failed']++;
        if (!timing.success) {
          failuresByStage[stage]++;
          failureRequests.push(copyRecord(timing));
          // Pin the first eight completed failures; retain the latest eight thereafter.
          if (failureRequests.length > 16) failureRequests.splice(8, 1);
        }
      }
      release?.(); res.removeListener('close', abort);
    }
  }
  // Body watchdog begins on admission, so queued requests do not expire while waiting for memory.
  const server = createServer({ maxHeaderSize: 16384, requestTimeout: 0, headersTimeout: 60000 }, (req, res) => {
    const controller = new AbortController(); const job = { controller };
    job.finished = handle(req, res, controller).finally(() => jobs.delete(job)); jobs.add(job);
  });
  server.on('connection', socket => { sockets.add(socket); socket.once('close', () => sockets.delete(socket)); socket.on('error', () => {}); });
  // Rejected by the HTTP server before any request handler ran; counted apart from requests.
  const transportReject = () => { rejected++; lifetime.transportRejections++; };
  server.on('clientError', (_error, socket) => { transportReject(); socket.destroy(); });
  for (const event of ['connect', 'upgrade']) server.on(event, (_req, socket) => { transportReject(); socket.destroy(); });
  for (const event of ['checkContinue', 'checkExpectation']) server.on(event, (_req, res) => {
    transportReject(); reply(res, 417, { type: 'error', error: { type: 'invalid_request_error', message: 'EXPECT_REJECTED' } });
  });
  await new Promise((resolve, reject) => { server.once('error', reject); server.listen(0, '127.0.0.1', resolve); });
  port = server.address().port;
  function close(why = 'CLIENT_CLOSED') {
    if (closingWork) return closingWork;
    closing = true; reason = why; secret.fill(0); correlationKey.fill(0);
    for (const state of agents.values()) state.selectionController?.abort();
    agents.clear(); admission.close();
    closingWork = (async () => {
      const serverDone = new Promise(resolve => server.close(resolve));
      for (const job of jobs) job.controller.abort();
      const socketsDone = [...sockets].map(socket => new Promise(resolve => { socket.once('close', resolve); socket.destroy(); }));
      let timer;
      const work = Promise.allSettled([Promise.resolve().then(() => transport.close()),
        ...[...jobs].map(job => job.finished), ...socketsDone, serverDone]);
      const results = await Promise.race([work, new Promise(resolve => { timer = setTimeout(() => resolve(null), cleanupMs); })]);
      clearTimeout(timer);
      if (results === null || results.some(result => result.status === 'rejected')) { cleanupFailed = true; reason = 'CLEANUP_FAILED'; }
      const state = diagnostics(); finish(state); return state;
    })();
    return closingWork;
  }
  server.on('error', () => { void close('SERVER_ERROR'); });
  return { port, close, done, diagnostics, clientHeaders() { need(!closing, 'GATEWAY_CLOSED'); return { Authorization: secret.toString() }; } };
}
